package recording

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nightingale-develop/reconcile-guard/internal/collector"
)

func AppendCapture(
	directory string,
	capture collector.Capture,
) error {
	operatorDirectory :=
		filepath.Join(directory, "operators")

	if err := os.MkdirAll(
		operatorDirectory,
		0755,
	); err != nil {
		return fmt.Errorf(
			"create output directory: %w",
			err,
		)
	}

	versionPath := filepath.Join(
		directory,
		"cluster-version.jsonl",
	)

	if err := appendJSONLine(
		versionPath,
		capture.ClusterVersion,
	); err != nil {
		return fmt.Errorf(
			"write ClusterVersion observation: %w",
			err,
		)
	}

	for _, observation := range capture.Operators {
		name := observation.Operator.Name

		if name == "" ||
			filepath.Base(name) != name {
			return fmt.Errorf(
				"invalid ClusterOperator name %q",
				name,
			)
		}

		path := filepath.Join(
			operatorDirectory,
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

	defer file.Close()

	return json.NewEncoder(file).Encode(value)
}
