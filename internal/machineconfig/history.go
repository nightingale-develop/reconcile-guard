package machineconfig

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"time"

	machineconfigv1 "github.com/openshift/api/machineconfiguration/v1"
)

type Observation struct {
	ObservedAt time.Time                         `json:"observedAt"`
	Pool       machineconfigv1.MachineConfigPool `json:"machineConfigPool"`
}

func ReadHistory(path string) ([]Observation, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open MachineConfigPool history %q: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	var observations []Observation
	line := 0

	for scanner.Scan() {
		line++

		data := bytes.TrimSpace(scanner.Bytes())
		if len(data) == 0 {
			continue
		}

		var observation Observation
		if err := json.Unmarshal(data, &observation); err != nil {
			return nil, fmt.Errorf(
				"line %d: decode MachineConfigPool observation: %w",
				line,
				err,
			)
		}

		observations = append(observations, observation)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf(
			"line %d: read MachineConfigPool history %q: %w",
			line+1,
			path,
			err,
		)
	}

	if len(observations) == 0 {
		return nil, fmt.Errorf("MachineConfigPool history has no observations")
	}

	if _, err := AnalyzeHistory(observations); err != nil {
		return nil, err
	}

	return observations, nil
}

type HistoryReport struct {
	Pool         string
	Observations int
}

func AnalyzeHistory(observations []Observation) (HistoryReport, error) {
	if len(observations) == 0 {
		return HistoryReport{}, fmt.Errorf(
			"MachineConfigPool history has no observations",
		)
	}

	var report HistoryReport

	for i, observation := range observations {
		if observation.ObservedAt.IsZero() {
			return HistoryReport{}, fmt.Errorf(
				"observation %d: observedAt is missing",
				i+1,
			)
		}

		if observation.Pool.Name == "" {
			return HistoryReport{}, fmt.Errorf(
				"observation %d: MachineConfigPool name is missing",
				i+1,
			)
		}

		if i == 0 {
			report.Pool = observation.Pool.Name
		} else {
			if observation.Pool.Name != report.Pool {
				return HistoryReport{}, fmt.Errorf(
					"observation %d: MachineConfigPool changed from %q to %q",
					i+1,
					report.Pool,
					observation.Pool.Name,
				)
			}

			if !observation.ObservedAt.After(
				observations[i-1].ObservedAt,
			) {
				return HistoryReport{}, fmt.Errorf(
					"observation %d: observedAt must be later than the previous observation",
					i+1,
				)
			}
		}
	}

	report.Observations = len(observations)

	return report, nil
}
