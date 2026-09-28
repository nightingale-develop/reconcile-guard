package recording

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/nightingale-develop/reconcile-guard/internal/collector"
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

	operatorDirectory :=
		filepath.Join(directory, "operators")

	if err := os.MkdirAll(
		operatorDirectory,
		0755,
	); err != nil {
		return nil, fmt.Errorf(
			"create output directory: %w",
			err,
		)
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
	return errors.Join(err, file.Close())
}
