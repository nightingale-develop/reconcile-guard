package upgrade

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"time"

	configv1 "github.com/openshift/api/config/v1"
)

type ClusterVersionObservation struct {
	ObservedAt     time.Time               `json:"observedAt"`
	ClusterVersion configv1.ClusterVersion `json:"clusterVersion"`
}

type ClusterVersionHistoryReport struct {
	Name           string
	Observations   int
	DesiredVersion string
}

func ReadHistory(
	path string,
) ([]ClusterVersionObservation, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf(
			"open ClusterVersion history %q: %w",
			path,
			err,
		)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(
		make([]byte, 64*1024),
		4*1024*1024,
	)

	var observations []ClusterVersionObservation
	line := 0

	for scanner.Scan() {
		line++

		data := bytes.TrimSpace(scanner.Bytes())

		if len(data) == 0 {
			continue
		}

		var observation ClusterVersionObservation

		if err := json.Unmarshal(
			data,
			&observation,
		); err != nil {
			return nil, fmt.Errorf(
				"line %d: decode JSON: %w",
				line,
				err,
			)
		}

		observations = append(
			observations,
			observation,
		)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf(
			"line %d: read ClusterVersion history %q: %w",
			line+1,
			path,
			err,
		)
	}

	if len(observations) == 0 {
		return nil, fmt.Errorf(
			"ClusterVersion history has no observations",
		)
	}

	return observations, nil
}

func AnalyzeHistory(
	observations []ClusterVersionObservation,
) (ClusterVersionHistoryReport, error) {
	if len(observations) == 0 {
		return ClusterVersionHistoryReport{}, fmt.Errorf(
			"ClusterVersion history has no observations",
		)
	}

	var report ClusterVersionHistoryReport

	for i, observation := range observations {
		if observation.ObservedAt.IsZero() {
			return ClusterVersionHistoryReport{}, fmt.Errorf(
				"observation %d: observedAt is missing",
				i+1,
			)
		}

		if _, err := AnalyzeClusterVersion(
			observation.ClusterVersion,
		); err != nil {
			return ClusterVersionHistoryReport{}, fmt.Errorf(
				"observation %d: %w",
				i+1,
				err,
			)
		}

		if i == 0 {
			report.Name = observation.ClusterVersion.Name
			continue
		}

		if observation.ClusterVersion.Name != report.Name {
			return ClusterVersionHistoryReport{}, fmt.Errorf(
				"observation %d: ClusterVersion name changed from %q to %q",
				i+1,
				report.Name,
				observation.ClusterVersion.Name,
			)
		}

		if !observation.ObservedAt.After(
			observations[i-1].ObservedAt,
		) {
			return ClusterVersionHistoryReport{}, fmt.Errorf(
				"observation %d: observedAt must be later than the previous observation",
				i+1,
			)
		}
	}

	last := observations[len(observations)-1]

	report.Observations = len(observations)
	report.DesiredVersion =
		last.ClusterVersion.Status.Desired.Version

	return report, nil
}
