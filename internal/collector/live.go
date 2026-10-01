package collector

import (
	"context"
	"fmt"
	"sort"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	machineconfigv1 "github.com/openshift/api/machineconfiguration/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/nightingale-develop/reconcile-guard/internal/machineconfig"
	nodehistory "github.com/nightingale-develop/reconcile-guard/internal/node"
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

var machineConfigPoolResource = schema.GroupVersionResource{
	Group:    "machineconfiguration.openshift.io",
	Version:  "v1",
	Resource: "machineconfigpools",
}

var nodeResource = schema.GroupVersionResource{
	Group:    "",
	Version:  "v1",
	Resource: "nodes",
}

type Capture struct {
	ClusterVersion     upgrade.ClusterVersionObservation
	Operators          []operator.Observation
	MachineConfigPools []machineconfig.Observation
	Nodes              []nodehistory.Observation
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

	operators, err := c.captureOperators(ctx)
	if err != nil {
		return Capture{}, err
	}

	pools, err := c.captureMachineConfigPools(ctx)
	if err != nil {
		return Capture{}, err
	}

	nodes, err := c.captureNodes(ctx)
	if err != nil {
		return Capture{}, err
	}

	return Capture{
		ClusterVersion:     version,
		Operators:          operators,
		MachineConfigPools: pools,
		Nodes:              nodes,
	}, nil
}

func (c *LiveCollector) captureVersion(
	ctx context.Context,
) (upgrade.ClusterVersionObservation, error) {
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

	observedAt := c.now().UTC()

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

	return upgrade.ClusterVersionObservation{
		ObservedAt:     observedAt,
		ClusterVersion: version,
	}, nil
}

func (c *LiveCollector) captureOperators(
	ctx context.Context,
) ([]operator.Observation, error) {
	list, err := c.client.
		Resource(clusterOperatorResource).
		List(
			ctx,
			metav1.ListOptions{},
		)
	if err != nil {
		return nil, fmt.Errorf(
			"list ClusterOperators: %w",
			err,
		)
	}

	observedAt := c.now().UTC()

	observations := make(
		[]operator.Observation,
		0,
		len(list.Items),
	)

	for _, item := range list.Items {
		var clusterOperator configv1.ClusterOperator

		if err := runtime.DefaultUnstructuredConverter.
			FromUnstructured(
				item.Object,
				&clusterOperator,
			); err != nil {
			return nil, fmt.Errorf(
				"decode ClusterOperator %q: %w",
				item.GetName(),
				err,
			)
		}

		observations = append(
			observations,
			operator.Observation{
				ObservedAt: observedAt,
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

	return observations, nil
}

func (c *LiveCollector) captureMachineConfigPools(
	ctx context.Context,
) ([]machineconfig.Observation, error) {
	list, err := c.client.
		Resource(machineConfigPoolResource).
		List(
			ctx,
			metav1.ListOptions{},
		)
	if err != nil {
		return nil, fmt.Errorf(
			"list MachineConfigPools: %w",
			err,
		)
	}

	observedAt := c.now().UTC()

	observations := make(
		[]machineconfig.Observation,
		0,
		len(list.Items),
	)

	for _, item := range list.Items {
		var pool machineconfigv1.MachineConfigPool

		if err := runtime.DefaultUnstructuredConverter.
			FromUnstructured(
				item.Object,
				&pool,
			); err != nil {
			return nil, fmt.Errorf(
				"decode MachineConfigPool %q: %w",
				item.GetName(),
				err,
			)
		}

		observations = append(
			observations,
			machineconfig.Observation{
				ObservedAt: observedAt,
				Pool:       pool,
			},
		)
	}

	sort.Slice(
		observations,
		func(i, j int) bool {
			return observations[i].Pool.Name <
				observations[j].Pool.Name
		},
	)

	return observations, nil
}

func (c *LiveCollector) captureNodes(
	ctx context.Context,
) ([]nodehistory.Observation, error) {
	list, err := c.client.
		Resource(nodeResource).
		List(
			ctx,
			metav1.ListOptions{},
		)
	if err != nil {
		return nil, fmt.Errorf(
			"list Nodes: %w",
			err,
		)
	}

	observedAt := c.now().UTC()

	observations := make(
		[]nodehistory.Observation,
		0,
		len(list.Items),
	)

	for _, item := range list.Items {
		var node corev1.Node

		if err := runtime.DefaultUnstructuredConverter.
			FromUnstructured(
				item.Object,
				&node,
			); err != nil {
			return nil, fmt.Errorf(
				"decode Node %q: %w",
				item.GetName(),
				err,
			)
		}

		observations = append(
			observations,
			nodehistory.Observation{
				ObservedAt: observedAt,
				Node:       node,
			},
		)
	}

	sort.Slice(
		observations,
		func(i, j int) bool {
			return observations[i].Node.Name <
				observations[j].Node.Name
		},
	)

	return observations, nil
}
