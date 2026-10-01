package recording

import (
	"fmt"
	"sort"
	"sync"

	"github.com/nightingale-develop/reconcile-guard/internal/machineconfig"
	nodehistory "github.com/nightingale-develop/reconcile-guard/internal/node"
	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
)

type RunRecorder struct {
	recorder *JSONLRecorder

	mu                 sync.Mutex
	clusterID          string
	operators          map[string]struct{}
	machineConfigPools map[string]struct{}
	nodes              map[string]struct{}
}

func NewRunRecorder(
	directory string,
) (*RunRecorder, error) {
	recorder, err := NewJSONLRecorder(directory)
	if err != nil {
		return nil, err
	}

	return &RunRecorder{
		recorder:           recorder,
		operators:          make(map[string]struct{}),
		machineConfigPools: make(map[string]struct{}),
		nodes:              make(map[string]struct{}),
	}, nil
}

func (r *RunRecorder) AppendClusterVersion(
	observation upgrade.ClusterVersionObservation,
) error {
	clusterID := string(
		observation.ClusterVersion.Spec.ClusterID,
	)

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.clusterID != "" &&
		clusterID != "" &&
		r.clusterID != clusterID {
		previous := r.clusterID

		return fmt.Errorf(
			"cluster ID changed from %q to %q",
			previous,
			clusterID,
		)
	}

	if err := r.recorder.AppendClusterVersion(
		observation,
	); err != nil {
		return err
	}

	if clusterID != "" {
		r.clusterID = clusterID
	}

	return nil
}

func (r *RunRecorder) AppendOperator(
	observation operator.Observation,
) error {
	if err := r.recorder.AppendOperator(
		observation,
	); err != nil {
		return err
	}

	r.mu.Lock()
	r.operators[observation.Operator.Name] =
		struct{}{}
	r.mu.Unlock()

	return nil
}

func (r *RunRecorder) AppendMachineConfigPool(
	observation machineconfig.Observation,
) error {
	if err := r.recorder.AppendMachineConfigPool(
		observation,
	); err != nil {
		return err
	}

	r.mu.Lock()
	r.machineConfigPools[observation.Pool.Name] =
		struct{}{}
	r.mu.Unlock()

	return nil
}

func (r *RunRecorder) AppendNode(
	observation nodehistory.Observation,
) error {
	if err := r.recorder.AppendNode(
		observation,
	); err != nil {
		return err
	}

	r.mu.Lock()
	r.nodes[observation.Node.Name] =
		struct{}{}
	r.mu.Unlock()

	return nil
}

func (r *RunRecorder) Snapshot() RunSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()

	operators := make(
		[]string,
		0,
		len(r.operators),
	)

	for name := range r.operators {
		operators = append(
			operators,
			name,
		)
	}

	machineConfigPools := make(
		[]string,
		0,
		len(r.machineConfigPools),
	)

	for name := range r.machineConfigPools {
		machineConfigPools = append(
			machineConfigPools,
			name,
		)
	}

	nodes := make(
		[]string,
		0,
		len(r.nodes),
	)

	for name := range r.nodes {
		nodes = append(
			nodes,
			name,
		)
	}

	sort.Strings(operators)
	sort.Strings(machineConfigPools)
	sort.Strings(nodes)

	return RunSnapshot{
		ClusterID:          r.clusterID,
		Operators:          operators,
		MachineConfigPools: machineConfigPools,
		Nodes:              nodes,
	}
}
