package recording

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/nightingale-develop/reconcile-guard/internal/collector"
	"github.com/nightingale-develop/reconcile-guard/internal/machineconfig"
	nodehistory "github.com/nightingale-develop/reconcile-guard/internal/node"
	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
)

type JSONLRecorder struct {
	directory string
	mu        sync.Mutex
}

func NewJSONLRecorder(
	directory string,
) (*JSONLRecorder, error) {
	if directory == "" {
		return nil, fmt.Errorf(
			"output directory is required",
		)
	}

	directories := []string{
		filepath.Join(directory, "operators"),
		filepath.Join(directory, "machine-config-pools"),
		filepath.Join(directory, "nodes"),
	}

	for _, path := range directories {
		if err := os.MkdirAll(path, 0755); err != nil {
			return nil, fmt.Errorf(
				"create output directory %q: %w",
				path,
				err,
			)
		}
	}

	return &JSONLRecorder{
		directory: directory,
	}, nil
}

func (r *JSONLRecorder) AppendClusterVersion(
	observation upgrade.ClusterVersionObservation,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	path := filepath.Join(
		r.directory,
		"cluster-version.jsonl",
	)

	if err := appendJSONLine(
		path,
		observation,
	); err != nil {
		return fmt.Errorf(
			"write ClusterVersion observation: %w",
			err,
		)
	}

	return nil
}

func (r *JSONLRecorder) AppendOperator(
	observation operator.Observation,
) error {
	name := observation.Operator.Name

	if name == "" ||
		name == "." ||
		name == ".." ||
		filepath.Base(name) != name {
		return fmt.Errorf(
			"invalid ClusterOperator name %q",
			name,
		)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	path := filepath.Join(
		r.directory,
		"operators",
		name+".jsonl",
	)

	if err := appendJSONLine(
		path,
		observation,
	); err != nil {
		return fmt.Errorf(
			"write ClusterOperator %q observation: %w",
			name,
			err,
		)
	}

	return nil
}

func (r *JSONLRecorder) AppendMachineConfigPool(
	observation machineconfig.Observation,
) error {
	name := observation.Pool.Name

	if name == "" ||
		name == "." ||
		name == ".." ||
		filepath.Base(name) != name {
		return fmt.Errorf(
			"invalid MachineConfigPool name %q",
			name,
		)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	path := filepath.Join(
		r.directory,
		"machine-config-pools",
		name+".jsonl",
	)

	if err := appendJSONLine(
		path,
		observation,
	); err != nil {
		return fmt.Errorf(
			"write MachineConfigPool %q observation: %w",
			name,
			err,
		)
	}

	return nil
}

func (r *JSONLRecorder) AppendNode(
	observation nodehistory.Observation,
) error {
	name := observation.Node.Name

	if name == "" ||
		name == "." ||
		name == ".." ||
		filepath.Base(name) != name {
		return fmt.Errorf(
			"invalid Node name %q",
			name,
		)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	path := filepath.Join(
		r.directory,
		"nodes",
		name+".jsonl",
	)

	if err := appendJSONLine(
		path,
		observation,
	); err != nil {
		return fmt.Errorf(
			"write Node %q observation: %w",
			name,
			err,
		)
	}

	return nil
}

func AppendCapture(
	directory string,
	capture collector.Capture,
) error {
	recorder, err := NewJSONLRecorder(directory)
	if err != nil {
		return err
	}

	if err := recorder.AppendClusterVersion(
		capture.ClusterVersion,
	); err != nil {
		return err
	}

	for _, observation := range capture.Operators {
		if err := recorder.AppendOperator(
			observation,
		); err != nil {
			return err
		}
	}

	for _, observation := range capture.MachineConfigPools {
		if err := recorder.AppendMachineConfigPool(
			observation,
		); err != nil {
			return err
		}
	}

	for _, observation := range capture.Nodes {
		if err := recorder.AppendNode(
			observation,
		); err != nil {
			return err
		}
	}

	return nil
}

func appendJSONLine(
	path string,
	value any,
) error {
	file, err := os.OpenFile(
		path,
		os.O_CREATE|
			os.O_WRONLY|
			os.O_APPEND,
		0644,
	)
	if err != nil {
		return err
	}

	err = json.NewEncoder(file).Encode(value)

	return errors.Join(
		err,
		file.Close(),
	)
}
