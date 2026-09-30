package collector

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
	configv1 "github.com/openshift/api/config/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
)

func liveObject(kind, name, rv string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "config.openshift.io/v1", "kind": kind,
		"metadata": map[string]any{"name": name, "resourceVersion": rv},
	}}
}

func liveClient(t *testing.T, objects ...runtime.Object) *fake.FakeDynamicClient {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := configv1.Install(scheme); err != nil {
		t.Fatal(err)
	}
	return fake.NewSimpleDynamicClient(scheme, objects...)
}

func startRecording(t *testing.T, client *fake.FakeDynamicClient, sink ObservationSink) (context.CancelFunc, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	r := newLiveRecorder(client, sink, func() time.Time { return time.Date(2026, 9, 28, 12, 0, 0, 0, time.FixedZone("local", 3600)) })
	go func() { done <- r.Run(ctx) }()
	t.Cleanup(cancel)
	return cancel, done
}

func recordingResult(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("recorder did not stop")
		return nil
	}
}

func nextWatch(t *testing.T, ch <-chan *watch.RaceFreeFakeWatcher) *watch.RaceFreeFakeWatcher {
	t.Helper()
	select {
	case w := <-ch:
		return w
	case <-time.After(5 * time.Second):
		t.Fatal("watch did not start")
		return nil
	}
}

func watchStream(client *fake.FakeDynamicClient, resource string) <-chan *watch.RaceFreeFakeWatcher {
	ch := make(chan *watch.RaceFreeFakeWatcher, 8)
	client.PrependWatchReactor(resource, func(clienttesting.Action) (bool, watch.Interface, error) {
		w := watch.NewRaceFreeFake()
		ch <- w
		return true, w, nil
	})
	return ch
}

func TestLiveRecorderLifecycle(t *testing.T) {
	version := liveObject("ClusterVersion", "version", "1")
	ingress := liveObject("ClusterOperator", "ingress", "1")
	network := liveObject("ClusterOperator", "network", "1")
	client := liveClient(t, version, ingress, network, liveObject("ClusterVersion", "ignored", "1"))
	versions, operators := watchStream(client, "clusterversions"), watchStream(client, "clusteroperators")
	sink := &memorySink{}
	cancel, done := startRecording(t, client, sink)
	vw, ow := nextWatch(t, versions), nextWatch(t, operators)
	waitForCounts(t, sink, 1, 2)
	// A later update is a barrier: same-stream callbacks before it have completed.
	ow.Modify(ingress.DeepCopy())
	updated := ingress.DeepCopy()
	updated.SetResourceVersion("2")
	ow.Modify(updated)
	vw.Modify(liveObject("ClusterVersion", "ignored", "2"))
	vw.Modify(version.DeepCopy())
	vw.Modify(liveObject("ClusterVersion", "version", "2"))
	waitForCounts(t, sink, 2, 3)
	ow.Delete(updated)
	ow.Add(liveObject("ClusterOperator", "ingress", "3"))
	vw.Delete(liveObject("ClusterVersion", "ignored", "2"))
	vw.Modify(liveObject("ClusterVersion", "version", "3"))
	waitForCounts(t, sink, 3, 4)
	cancel()
	if err := recordingResult(t, done); err != nil {
		t.Fatal(err)
	}
	if v, o := sink.counts(); v != 3 || o != 4 {
		t.Fatalf("duplicates/deletes recorded: %d versions, %d operators", v, o)
	}
	last := map[string]time.Time{}
	for _, obs := range sink.operators {
		if !obs.ObservedAt.After(last[obs.Operator.Name]) || obs.ObservedAt.Location() != time.UTC {
			t.Fatal("invalid stream timestamp")
		}
		last[obs.Operator.Name] = obs.ObservedAt
	}
	if !sink.versions[0].ObservedAt.Equal(sink.operators[0].ObservedAt) {
		t.Fatal("unrelated streams must not share an artificial timestamp sequence")
	}
	if _, err := operator.AnalyzeHistory([]operator.Observation{sink.operators[0]}); err != nil {
		t.Fatal(err)
	}
	for _, action := range client.Actions() {
		if action.GetVerb() != "list" && action.GetVerb() != "watch" {
			t.Fatalf("unexpected API action: %s", action.GetVerb())
		}
	}
}

func TestLiveRecorderMissingOrDeletedVersion(t *testing.T) {
	t.Run("missing initially", func(t *testing.T) {
		_, done := startRecording(t, liveClient(t, liveObject("ClusterVersion", "other", "1")), &memorySink{})
		if err := recordingResult(t, done); err == nil || !strings.Contains(err.Error(), "missing") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("deleted", func(t *testing.T) {
		version := liveObject("ClusterVersion", "version", "1")
		client := liveClient(t, version)
		watches := watchStream(client, "clusterversions")
		sink := &memorySink{}
		_, done := startRecording(t, client, sink)
		w := nextWatch(t, watches)
		waitForCounts(t, sink, 1, 0)
		w.Delete(version)
		if err := recordingResult(t, done); err == nil || !strings.Contains(err.Error(), "deleted") {
			t.Fatalf("error = %v", err)
		}
	})
}

type failingSink struct {
	version bool
	err     error
}

func (s failingSink) AppendClusterVersion(upgrade.ClusterVersionObservation) error {
	if s.version {
		return s.err
	}
	return nil
}
func (s failingSink) AppendOperator(operator.Observation) error {
	if !s.version {
		return s.err
	}
	return nil
}

func TestLiveRecorderSinkErrors(t *testing.T) {
	for _, version := range []bool{true, false} {
		t.Run(map[bool]string{true: "version", false: "operator"}[version], func(t *testing.T) {
			client := liveClient(t, liveObject("ClusterVersion", "version", "1"), liveObject("ClusterOperator", "ingress", "1"))
			want := errors.New("disk write failed")
			_, done := startRecording(t, client, failingSink{version, want})
			if err := recordingResult(t, done); !errors.Is(err, want) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestLiveRecorderCancelledBeforeStart(t *testing.T) {
	client := liveClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := newLiveRecorder(client, &memorySink{}, time.Now).Run(ctx); err != nil {
		t.Fatal(err)
	}
	if len(client.Actions()) != 0 {
		t.Fatal("cancelled recorder accessed API")
	}
}

type delayedErrorSink struct {
	entered chan struct{}
	release chan struct{}
	err     error
}

func (s *delayedErrorSink) AppendClusterVersion(upgrade.ClusterVersionObservation) error {
	close(s.entered)
	<-s.release
	return s.err
}

func (*delayedErrorSink) AppendOperator(operator.Observation) error { return nil }

func TestLiveRecorderWaitsForInFlightWriteError(t *testing.T) {
	want := errors.New("late write failure")
	sink := &delayedErrorSink{make(chan struct{}), make(chan struct{}), want}
	client := liveClient(t, liveObject("ClusterVersion", "version", "1"))
	cancel, done := startRecording(t, client, sink)
	defer func() {
		select {
		case <-sink.release:
		default:
			close(sink.release)
		}
	}()
	select {
	case <-sink.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("write did not start")
	}
	cancel()
	select {
	case err := <-done:
		t.Fatalf("returned before in-flight write completed: %v", err)
	default:
	}
	close(sink.release)
	if err := recordingResult(t, done); !errors.Is(err, want) {
		t.Fatalf("lost write error during cancellation: %v", err)
	}
}

func TestObservationClockPerResource(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.FixedZone("local", 3600))
	clock := observationClock{now: func() time.Time { return now }, last: make(map[string]time.Time)}
	first := clock.Next("ingress")
	if network := clock.Next("network"); !network.Equal(first) || network.Location() != time.UTC {
		t.Fatal("independent stream timestamp changed")
	}
	if second := clock.Next("ingress"); !second.Equal(first.Add(time.Nanosecond)) {
		t.Fatal("equal timestamps not corrected")
	}
	now = now.Add(-time.Second)
	if third := clock.Next("ingress"); !third.Equal(first.Add(2 * time.Nanosecond)) {
		t.Fatal("backwards clock not corrected")
	}
}

func TestLiveRecorderRejectsMalformedObjects(t *testing.T) {
	malformed := liveObject("ClusterVersion", "version", "1")
	malformed.Object["status"] = "not an object"
	badOperator := liveObject("ClusterOperator", "ingress", "1")
	badOperator.Object["status"] = "not an object"
	var typedNil *unstructured.Unstructured
	for _, obj := range []any{nil, typedNil, "wrong type", malformed, badOperator} {
		for _, version := range []bool{true, false} {
			if obj == malformed && !version || obj == badOperator && version {
				continue
			}
			sink := &memorySink{}
			r := newLiveRecorder(nil, sink, time.Now)
			var got error
			fail := func(err error) { got = err }
			if version {
				r.recordVersion(obj, fail)
			} else {
				r.recordOperator(obj, fail)
			}
			if got == nil {
				t.Fatalf("accepted malformed object %T (version=%v)", obj, version)
			}
			if v, o := sink.counts(); v != 0 || o != 0 {
				t.Fatal("malformed observation written")
			}
		}
	}
}

func TestLiveRecorderWatchRestart(t *testing.T) {
	client := liveClient(t, liveObject("ClusterVersion", "version", "1"), liveObject("ClusterOperator", "ingress", "1"))
	streams := watchStream(client, "clusteroperators")
	sink := &memorySink{}
	cancel, done := startRecording(t, client, sink)
	w := nextWatch(t, streams)
	waitForCounts(t, sink, 1, 1)
	w.Modify(liveObject("ClusterOperator", "ingress", "2"))
	waitForCounts(t, sink, 1, 2)
	w.Stop()
	w = nextWatch(t, streams)
	w.Modify(liveObject("ClusterOperator", "ingress", "3"))
	waitForCounts(t, sink, 1, 3)
	cancel()
	if err := recordingResult(t, done); err != nil {
		t.Fatal(err)
	}
	if v, o := sink.counts(); v != 1 || o != 3 {
		t.Fatalf("unexpected duplicate observations: %d, %d", v, o)
	}
	var watchVersions []string
	for _, action := range client.Actions() {
		if action.GetResource() == clusterOperatorResource && action.GetVerb() == "watch" {
			watchVersions = append(watchVersions, action.(clienttesting.WatchAction).GetWatchRestrictions().ResourceVersion)
		}
	}
	if len(watchVersions) < 2 || watchVersions[1] != "2" {
		t.Fatalf("restart did not resume at last resourceVersion: %v", watchVersions)
	}
}

func TestLiveRecorderPermissionError(t *testing.T) {
	client := liveClient(t)
	client.PrependReactor("list", "clusterversions", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(clusterVersionResource.GroupResource(), "", errors.New("denied"))
	})
	_, done := startRecording(t, client, &memorySink{})
	if err := recordingResult(t, done); !apierrors.IsForbidden(err) {
		t.Fatalf("error = %v", err)
	}
}

func TestLiveRecorderExpiredWatchRelists(t *testing.T) {
	client := liveClient(t, liveObject("ClusterVersion", "version", "1"), liveObject("ClusterOperator", "ingress", "1"))
	streams := watchStream(client, "clusteroperators")
	sink := &memorySink{}
	cancel, done := startRecording(t, client, sink)
	w := nextWatch(t, streams)
	waitForCounts(t, sink, 1, 1)
	// Change the tracker directly: the next LIST, rather than a watch event, must see it.
	if err := client.Tracker().Update(clusterOperatorResource, liveObject("ClusterOperator", "ingress", "2"), ""); err != nil {
		t.Fatal(err)
	}
	w.Error(&metav1.Status{Status: metav1.StatusFailure, Reason: metav1.StatusReasonExpired, Code: 410, Message: "expired"})
	_ = nextWatch(t, streams)
	waitForCounts(t, sink, 1, 2)
	cancel()
	if err := recordingResult(t, done); err != nil {
		t.Fatal(err)
	}
	lists := 0
	for _, action := range client.Actions() {
		if action.GetVerb() == "list" && action.GetResource() == clusterOperatorResource {
			lists++
		}
	}
	if lists < 2 {
		t.Fatalf("expected relist, got %d LIST calls", lists)
	}
	if v, o := sink.counts(); v != 1 || o != 2 || sink.operators[1].Operator.ResourceVersion != "2" {
		t.Fatal("relist did not record the new version exactly once")
	}
}

func TestLiveRecorderUnauthorizedRecovery(t *testing.T) {
	for _, resource := range []string{"clusterversions", "clusteroperators"} {
		for _, stage := range []string{"watch event", "watch request", "relist"} {
			t.Run(resource+"/"+stage, func(t *testing.T) {
				client := liveClient(t, liveObject("ClusterVersion", "version", "1"), liveObject("ClusterOperator", "ingress", "1"))
				streams := watchStream(client, resource)
				var reject atomic.Bool
				unauthorized := apierrors.NewUnauthorized("authentication temporarily unavailable")
				if stage == "relist" {
					client.PrependReactor("list", resource, func(clienttesting.Action) (bool, runtime.Object, error) {
						if reject.Swap(false) {
							return true, nil, unauthorized
						}
						return false, nil, nil
					})
				}
				if stage == "watch request" {
					client.PrependWatchReactor(resource, func(clienttesting.Action) (bool, watch.Interface, error) {
						if reject.Swap(false) {
							return true, nil, unauthorized
						}
						return false, nil, nil
					})
				}
				sink := &memorySink{}
				cancel, done := startRecording(t, client, sink)
				w := nextWatch(t, streams)
				waitForCounts(t, sink, 1, 1)
				switch stage {
				case "watch event":
					w.Error(&unauthorized.ErrStatus)
				case "watch request":
					reject.Store(true)
					w.Stop()
				case "relist":
					reject.Store(true)
					w.Error(&metav1.Status{Status: metav1.StatusFailure, Reason: metav1.StatusReasonExpired, Code: 410, Message: "expired"})
				}
				w = nextWatch(t, streams)
				if reject.Load() {
					t.Fatal("Unauthorized reactor not exercised")
				}
				if resource == "clusterversions" {
					w.Modify(liveObject("ClusterVersion", "version", "2"))
					waitForCounts(t, sink, 2, 1)
				} else {
					w.Modify(liveObject("ClusterOperator", "ingress", "2"))
					waitForCounts(t, sink, 1, 2)
				}
				cancel()
				if err := recordingResult(t, done); err != nil {
					t.Fatal(err)
				}
				if resource == "clusterversions" {
					if sink.versions[len(sink.versions)-1].ClusterVersion.ResourceVersion != "2" {
						t.Fatal("recovery event lost")
					}
				} else {
					if sink.operators[len(sink.operators)-1].Operator.ResourceVersion != "2" {
						t.Fatal("recovery event lost")
					}
				}
			})
		}
	}
}

func TestLiveRecorderUnauthorizedInitiallyFatal(t *testing.T) {
	for _, resource := range []string{"clusterversions", "clusteroperators"} {
		t.Run(resource, func(t *testing.T) {
			client := liveClient(t, liveObject("ClusterVersion", "version", "1"), liveObject("ClusterOperator", "ingress", "1"))
			client.PrependReactor("list", resource, func(clienttesting.Action) (bool, runtime.Object, error) {
				return true, nil, apierrors.NewUnauthorized("invalid credentials")
			})
			_, done := startRecording(t, client, &memorySink{})
			if err := recordingResult(t, done); !apierrors.IsUnauthorized(err) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestLiveRecorderForbiddenAfterSyncFatal(t *testing.T) {
	for _, resource := range []string{"clusterversions", "clusteroperators"} {
		t.Run(resource, func(t *testing.T) {
			client := liveClient(t, liveObject("ClusterVersion", "version", "1"), liveObject("ClusterOperator", "ingress", "1"))
			streams := watchStream(client, resource)
			sink := &memorySink{}
			_, done := startRecording(t, client, sink)
			w := nextWatch(t, streams)
			waitForCounts(t, sink, 1, 1)
			client.PrependWatchReactor(resource, func(clienttesting.Action) (bool, watch.Interface, error) {
				return true, nil, apierrors.NewForbidden(clusterOperatorResource.GroupResource(), "", errors.New("denied"))
			})
			w.Stop()
			if err := recordingResult(t, done); !apierrors.IsForbidden(err) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
