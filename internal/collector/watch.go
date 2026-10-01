package collector

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
	configv1 "github.com/openshift/api/config/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

type ObservationSink interface {
	AppendClusterVersion(
		upgrade.ClusterVersionObservation,
	) error

	AppendOperator(
		operator.Observation,
	) error
}

type LiveRecorder struct {
	client dynamic.Interface
	sink   ObservationSink
	clock  *observationClock
}

type observationClock struct {
	mu   sync.Mutex
	now  func() time.Time
	last map[string]time.Time
}

func NewLiveRecorder(
	config *rest.Config,
	sink ObservationSink,
) (*LiveRecorder, error) {
	if config == nil {
		return nil, fmt.Errorf("Kubernetes config is required")
	}
	if sink == nil {
		return nil, fmt.Errorf(
			"observation sink is required",
		)
	}

	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf(
			"create Kubernetes client: %w",
			err,
		)
	}

	return newLiveRecorder(
		client,
		sink,
		time.Now,
	), nil
}

func newLiveRecorder(
	client dynamic.Interface,
	sink ObservationSink,
	now func() time.Time,
) *LiveRecorder {
	return &LiveRecorder{
		client: client,
		sink:   sink,
		clock: &observationClock{
			now:  now,
			last: make(map[string]time.Time),
		},
	}
}

func (c *observationClock) Next(stream string) time.Time {
	return c.Observe(stream, c.now())
}

func (c *observationClock) Observe(stream string, current time.Time) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	current = current.UTC()

	if last := c.last[stream]; !current.After(last) {
		current = last.Add(time.Nanosecond)
	}
	c.last[stream] = current

	return current
}

func (r *LiveRecorder) Run(parent context.Context) (runErr error) {
	ctx, cancel := context.WithCancel(parent)
	var workers sync.WaitGroup
	errCh := make(chan error, 1)
	var failOnce sync.Once
	fail := func(err error) {
		failOnce.Do(func() { errCh <- err; cancel() })
	}
	defer func() {
		cancel()
		workers.Wait()
		if err := pendingError(errCh); err != nil {
			runErr = err
		}
	}()
	if ctx.Err() != nil {
		return nil
	}

	for _, resource := range []schema.GroupVersionResource{clusterVersionResource, clusterOperatorResource} {
		informer := r.newInformer(resource, fail)
		record := r.recordOperator
		var deleted func(any)
		if resource == clusterVersionResource {
			record = r.recordVersion
			deleted = func(obj any) {
				if objectName(obj) == "version" {
					fail(fmt.Errorf("ClusterVersion/version was deleted"))
				}
			}
		}
		_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj any) {
				if ctx.Err() == nil {
					record(obj, fail)
				}
			},
			UpdateFunc: func(oldObj, newObj any) {
				if ctx.Err() == nil && !sameResourceVersion(oldObj, newObj) {
					record(newObj, fail)
				}
			},
			DeleteFunc: deleted,
		})
		if err != nil {
			return fmt.Errorf("register %s handler: %w", resource.Resource, err)
		}
		if err := informer.SetWatchErrorHandlerWithContext(func(ctx context.Context, reflector *cache.Reflector, err error) {
			if ctx.Err() != nil {
				return
			}

			if apierrors.IsForbidden(err) || (apierrors.IsUnauthorized(err) && !informer.HasSynced()) || apierrors.IsNotFound(err) {
				fail(fmt.Errorf("watch %s: %w", resource.Resource, err))
				return
			}
			cache.DefaultWatchErrorHandler(ctx, reflector, err)
		}); err != nil {
			return err
		}
		workers.Add(1)
		go func() { defer workers.Done(); informer.RunWithContext(ctx) }()
	}
	<-ctx.Done()
	return nil
}

type listThenWatch struct{ *cache.ListWatch }

func (*listThenWatch) IsWatchListSemanticsUnSupported() bool { return true }

func (r *LiveRecorder) newInformer(resource schema.GroupVersionResource, fail func(error)) cache.SharedIndexInformer {
	selectVersion := func(options metav1.ListOptions) metav1.ListOptions {
		if resource == clusterVersionResource {
			options.FieldSelector = fields.OneTermEqualSelector("metadata.name", "version").String()
		}
		return options
	}
	listWatch := &cache.ListWatch{
		ListWithContextFunc: func(ctx context.Context, options metav1.ListOptions) (runtime.Object, error) {
			list, err := r.client.Resource(resource).List(ctx, selectVersion(options))
			if err == nil && resource == clusterVersionResource {
				found := false
				for _, item := range list.Items {
					if item.GetName() == "version" {
						found = true
						break
					}
				}
				if !found && list.GetContinue() == "" {
					err = fmt.Errorf("ClusterVersion/version is missing from LIST")
					fail(err)
				}
			}
			return list, err
		},
		WatchFuncWithContext: func(ctx context.Context, options metav1.ListOptions) (watch.Interface, error) {
			return r.client.Resource(resource).Watch(ctx, selectVersion(options))
		},
	}
	return cache.NewSharedIndexInformer(&listThenWatch{listWatch}, &unstructured.Unstructured{}, 0, cache.Indexers{})
}

func (r *LiveRecorder) recordVersion(
	obj any,
	fail func(error),
) {
	value, ok := obj.(*unstructured.Unstructured)
	if !ok || value == nil {
		fail(
			fmt.Errorf(
				"unexpected ClusterVersion object %T",
				obj,
			),
		)
		return
	}

	if value.GetName() != "version" {
		return
	}
	if value.GetAPIVersion() != "config.openshift.io/v1" || value.GetKind() != "ClusterVersion" {
		fail(fmt.Errorf("unexpected ClusterVersion resource %s/%s", value.GetAPIVersion(), value.GetKind()))
		return
	}

	var version configv1.ClusterVersion

	if err := runtime.DefaultUnstructuredConverter.
		FromUnstructured(
			value.Object,
			&version,
		); err != nil {
		fail(
			fmt.Errorf(
				"decode ClusterVersion/version: %w",
				err,
			),
		)
		return
	}

	observation :=
		upgrade.ClusterVersionObservation{
			ObservedAt:     r.clock.Next("clusterversion/version"),
			ClusterVersion: version,
		}

	if err := r.sink.AppendClusterVersion(
		observation,
	); err != nil {
		fail(err)
	}
}

func (r *LiveRecorder) recordOperator(
	obj any,
	fail func(error),
) {
	value, ok := obj.(*unstructured.Unstructured)
	if !ok || value == nil {
		fail(
			fmt.Errorf(
				"unexpected ClusterOperator object %T",
				obj,
			),
		)
		return
	}

	if value.GetAPIVersion() != "config.openshift.io/v1" || value.GetKind() != "ClusterOperator" || value.GetName() == "" {
		fail(fmt.Errorf("unexpected ClusterOperator resource %s/%s name=%q", value.GetAPIVersion(), value.GetKind(), value.GetName()))
		return
	}
	var clusterOperator configv1.ClusterOperator

	if err := runtime.DefaultUnstructuredConverter.
		FromUnstructured(
			value.Object,
			&clusterOperator,
		); err != nil {
		fail(
			fmt.Errorf(
				"decode ClusterOperator %q: %w",
				value.GetName(),
				err,
			),
		)
		return
	}

	observation := operator.Observation{
		ObservedAt: r.clock.Next("clusteroperator/" + clusterOperator.Name),
		Operator:   clusterOperator,
	}

	if err := r.sink.AppendOperator(
		observation,
	); err != nil {
		fail(err)
	}
}

func sameResourceVersion(
	oldObj any,
	newObj any,
) bool {
	oldValue, oldOK :=
		oldObj.(*unstructured.Unstructured)

	newValue, newOK :=
		newObj.(*unstructured.Unstructured)

	if !oldOK || !newOK || oldValue == nil || newValue == nil {
		return false
	}

	return oldValue.GetResourceVersion() != "" && oldValue.GetResourceVersion() ==
		newValue.GetResourceVersion()
}

func objectName(obj any) string {
	switch value := obj.(type) {
	case *unstructured.Unstructured:
		if value == nil {
			return ""
		}
		return value.GetName()

	case cache.DeletedFinalStateUnknown:
		return objectName(value.Obj)

	default:
		return ""
	}
}

func pendingError(
	errCh <-chan error,
) error {
	select {
	case err := <-errCh:
		return err

	default:
		return nil
	}
}
