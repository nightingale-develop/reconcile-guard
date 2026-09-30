package collector

import (
	"context"
	"fmt"
	"sort"
	"time"

	configv1 "github.com/openshift/api/config/v1"

	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

var clusterVersionResource = schema.GroupVersionResource{
	Group:    "config.openshift.io",
	Version:  "v1",
	Resource: "clusterversions",
}

var clusterOperatorResource = schema.GroupVersionResource{
	Group:    "config.openshift.io",
	Version:  "v1",
	Resource: "clusteroperators",
}

type Capture struct {
	ClusterVersion upgrade.ClusterVersionObservation
	Operators      []operator.Observation
}

type LiveCollector struct {
	client dynamic.Interface
	now    func() time.Time
}

func NewLiveCollector(
	config *rest.Config,
) (*LiveCollector, error) {
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf(
			"create Kubernetes client: %w",
			err,
		)
	}

	return &LiveCollector{
		client: client,
		now:    time.Now,
	}, nil
}

func newLiveCollector(
	client dynamic.Interface,
	now func() time.Time,
) *LiveCollector {
	return &LiveCollector{
		client: client,
		now:    now,
	}
}

func (c *LiveCollector) Capture(
	ctx context.Context,
) (Capture, error) {
	version, err := c.captureVersion(ctx)
	if err != nil {
		return Capture{}, err
	}

	operatorList, err := c.client.
		Resource(clusterOperatorResource).
		List(
			ctx,
			metav1.ListOptions{},
		)
	if err != nil {
		return Capture{}, fmt.Errorf(
			"list ClusterOperators: %w",
			err,
		)
	}

	operatorsObservedAt := c.now().UTC()

	observations := make(
		[]operator.Observation,
		0,
		len(operatorList.Items),
	)

	for _, item := range operatorList.Items {
		var clusterOperator configv1.ClusterOperator

		if err := runtime.DefaultUnstructuredConverter.
			FromUnstructured(
				item.Object,
				&clusterOperator,
			); err != nil {
			return Capture{}, fmt.Errorf(
				"decode ClusterOperator %q: %w",
				item.GetName(),
				err,
			)
		}

		observations = append(
			observations,
			operator.Observation{
				ObservedAt: operatorsObservedAt,
				Operator:   clusterOperator,
			},
		)
	}

	sort.Slice(
		observations,
		func(i, j int) bool {
			return observations[i].Operator.Name <
				observations[j].Operator.Name
		},
	)

	return Capture{
		ClusterVersion: version,
		Operators:      observations,
	}, nil
}

func (c *LiveCollector) captureVersion(ctx context.Context) (upgrade.ClusterVersionObservation, error) {
	versionObject, err := c.client.
		Resource(clusterVersionResource).
		Get(
			ctx,
			"version",
			metav1.GetOptions{},
		)
	if err != nil {
		return upgrade.ClusterVersionObservation{}, fmt.Errorf(
			"get ClusterVersion/version: %w",
			err,
		)
	}

	versionObservedAt := c.now().UTC()

	var version configv1.ClusterVersion

	if err := runtime.DefaultUnstructuredConverter.
		FromUnstructured(
			versionObject.Object,
			&version,
		); err != nil {
		return upgrade.ClusterVersionObservation{}, fmt.Errorf(
			"decode ClusterVersion/version: %w",
			err,
		)
	}

	return upgrade.ClusterVersionObservation{ObservedAt: versionObservedAt, ClusterVersion: version}, nil
}
