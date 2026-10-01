package node

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
)

const (
	CurrentMachineConfigAnnotation = "machineconfiguration.openshift.io/currentConfig"
	DesiredMachineConfigAnnotation = "machineconfiguration.openshift.io/desiredConfig"
)

type Observation struct {
	ObservedAt time.Time   `json:"observedAt"`
	Node       corev1.Node `json:"node"`
}

func ReadHistory(path string) ([]Observation, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open Node history %q: %w", path, err)
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
				"line %d: decode Node observation: %w",
				line,
				err,
			)
		}

		observations = append(observations, observation)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf(
			"line %d: read Node history %q: %w",
			line+1,
			path,
			err,
		)
	}

	if len(observations) == 0 {
		return nil, fmt.Errorf("Node history has no observations")
	}

	if _, err := AnalyzeHistory(observations); err != nil {
		return nil, err
	}

	return observations, nil
}

type HistoryReport struct {
	Node         string
	Observations int
}

func AnalyzeHistory(observations []Observation) (HistoryReport, error) {
	if len(observations) == 0 {
		return HistoryReport{}, fmt.Errorf(
			"Node history has no observations",
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

		if observation.Node.Name == "" {
			return HistoryReport{}, fmt.Errorf(
				"observation %d: Node name is missing",
				i+1,
			)
		}

		if i == 0 {
			report.Node = observation.Node.Name
		} else {
			if observation.Node.Name != report.Node {
				return HistoryReport{}, fmt.Errorf(
					"observation %d: Node changed from %q to %q",
					i+1,
					report.Node,
					observation.Node.Name,
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

func ReadyStatus(n corev1.Node) corev1.ConditionStatus {
	for _, condition := range n.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status
		}
	}

	return corev1.ConditionUnknown
}

func CurrentMachineConfig(n corev1.Node) string {
	return n.Annotations[CurrentMachineConfigAnnotation]
}

func DesiredMachineConfig(n corev1.Node) string {
	return n.Annotations[DesiredMachineConfigAnnotation]
}
