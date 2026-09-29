package recording

import (
	"fmt"
	"sort"
	"sync"

	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
)

type RunRecorder struct {
	recorder *JSONLRecorder

	mu        sync.Mutex
	clusterID string
	operators map[string]struct{}
}

func NewRunRecorder(
	directory string,
) (*RunRecorder, error) {
	recorder, err :=
		NewJSONLRecorder(directory)
	if err != nil {
		return nil, err
	}

	return &RunRecorder{
		recorder:  recorder,
		operators: make(map[string]struct{}),
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

	sort.Strings(operators)

	return RunSnapshot{
		ClusterID: r.clusterID,
		Operators: operators,
	}
}
