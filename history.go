package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type Observation struct {
	ObservedAt time.Time       `json:"observedAt"`
	Operator   ClusterOperator `json:"operator"`
}

type Transition struct {
	Condition string
	From      string
	To        string
	FromTime  time.Time
	ToTime    time.Time
}

type HistoryReport struct {
	Operator        string
	Observations    int
	Transitions     []Transition
	UncomparedPairs int
}

func readHistory(path string) ([]Observation, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open history %q: %w", path, err)
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
			return nil, fmt.Errorf("line %d: decode JSON: %w", line, err)
		}

		observations = append(observations, observation)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read history %q: %w", path, err)
	}

	if len(observations) == 0 {
		return nil, fmt.Errorf("history has no observations")
	}

	return observations, nil
}

func analyzeHistory(observations []Observation) (HistoryReport, error) {
	if len(observations) == 0 {
		return HistoryReport{}, fmt.Errorf("history has no observations")
	}

	conditionTypes := [...]string{"Available", "Progressing", "Degraded"}

	var report HistoryReport
	var previous map[string]string

	for i, observation := range observations {
		if observation.ObservedAt.IsZero() {
			return HistoryReport{}, fmt.Errorf(
				"observation %d: observedAt is missing",
				i+1,
			)
		}

		if _, err := analyzeOperator(observation.Operator); err != nil {
			return HistoryReport{}, fmt.Errorf("observation %d: %w", i+1, err)
		}

		if i == 0 {
			report.Operator = observation.Operator.Metadata.Name
		} else {
			if observation.Operator.Metadata.Name != report.Operator {
				return HistoryReport{}, fmt.Errorf(
					"observation %d: operator changed from %q to %q",
					i+1,
					report.Operator,
					observation.Operator.Metadata.Name,
				)
			}

			if !observation.ObservedAt.After(observations[i-1].ObservedAt) {
				return HistoryReport{}, fmt.Errorf(
					"observation %d: observedAt must be later than the previous observation",
					i+1,
				)
			}
		}

		current := make(map[string]string)

		for _, condition := range observation.Operator.Status.Conditions {
			switch condition.Type {
			case "Available", "Progressing", "Degraded":
			default:
				continue
			}

			if _, exists := current[condition.Type]; exists {
				return HistoryReport{}, fmt.Errorf(
					"observation %d: duplicate %s condition",
					i+1,
					condition.Type,
				)
			}

			switch condition.Status {
			case "True", "False", "Unknown":
			default:
				return HistoryReport{}, fmt.Errorf(
					"observation %d: invalid %s status: %q",
					i+1,
					condition.Type,
					condition.Status,
				)
			}

			current[condition.Type] = condition.Status
		}

		if i > 0 {
			for _, kind := range conditionTypes {
				before, hadBefore := previous[kind]
				after, hasAfter := current[kind]

				if !hadBefore || !hasAfter {
					report.UncomparedPairs++
					continue
				}

				if before != after {
					report.Transitions = append(
						report.Transitions,
						Transition{
							Condition: kind,
							From:      before,
							To:        after,
							FromTime:  observations[i-1].ObservedAt,
							ToTime:    observation.ObservedAt,
						},
					)
				}
			}
		}

		previous = current
	}

	report.Observations = len(observations)

	return report, nil
}
