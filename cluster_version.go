package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	configv1 "github.com/openshift/api/config/v1"
)

// ClusterVersionReport preserves reported data from one ClusterVersion snapshot.
// It is not an upgrade verdict.
type ClusterVersionReport struct {
	Name       string
	Desired    configv1.Release
	Conditions []configv1.ClusterOperatorStatusCondition
	History    []configv1.UpdateHistory
}

func analyzeClusterVersion(version configv1.ClusterVersion) (ClusterVersionReport, error) {
	if version.APIVersion != "config.openshift.io/v1" || version.Kind != "ClusterVersion" {
		return ClusterVersionReport{}, fmt.Errorf(
			"unsupported resource: %s/%s",
			version.APIVersion,
			version.Kind,
		)
	}

	if version.Name == "" {
		return ClusterVersionReport{}, fmt.Errorf("cluster version name is missing")
	}

	seen := make(map[configv1.ClusterStatusConditionType]bool)

	for _, condition := range version.Status.Conditions {
		if condition.Type == "" {
			return ClusterVersionReport{}, fmt.Errorf("condition type is missing")
		}

		if seen[condition.Type] {
			return ClusterVersionReport{}, fmt.Errorf(
				"duplicate %s condition",
				condition.Type,
			)
		}

		seen[condition.Type] = true

		switch condition.Status {
		case configv1.ConditionTrue,
			configv1.ConditionFalse,
			configv1.ConditionUnknown:
		default:
			return ClusterVersionReport{}, fmt.Errorf(
				"invalid %s status: %q",
				condition.Type,
				condition.Status,
			)
		}
	}

	return ClusterVersionReport{
		Name:       version.Name,
		Desired:    version.Status.Desired,
		Conditions: version.Status.Conditions,
		History:    version.Status.History,
	}, nil
}

func readClusterVersion(path string) (configv1.ClusterVersion, error) {
	var version configv1.ClusterVersion

	data, err := os.ReadFile(path)
	if err != nil {
		return version, fmt.Errorf("read file %q: %w", path, err)
	}

	if err := json.Unmarshal(data, &version); err != nil {
		return version, fmt.Errorf("decode JSON: %w", err)
	}

	return version, nil
}

func checkVersionFile(path string) error {
	version, err := readClusterVersion(path)
	if err != nil {
		return err
	}

	report, err := analyzeClusterVersion(version)
	if err != nil {
		return err
	}

	fmt.Println("ClusterVersion:", report.Name)
	fmt.Printf("Desired version (reported): %q\n", report.Desired.Version)
	fmt.Printf("Desired image (reported): %q\n", report.Desired.Image)
	fmt.Println("Conditions:", len(report.Conditions))

	for _, condition := range report.Conditions {
		fmt.Printf(
			"  %s: %s (reason=%q, message=%q)\n",
			condition.Type,
			condition.Status,
			condition.Reason,
			condition.Message,
		)
	}

	fmt.Println("Update history entries:", len(report.History))

	for _, entry := range report.History {
		started := "not reported"
		completed := "not reported"

		if !entry.StartedTime.IsZero() {
			started = entry.StartedTime.Time.Format(time.RFC3339Nano)
		}

		if entry.CompletionTime != nil && !entry.CompletionTime.IsZero() {
			completed = entry.CompletionTime.Time.Format(time.RFC3339Nano)
		}

		fmt.Printf(
			"  version=%q image=%q state=%q started=%s completed=%s\n",
			entry.Version,
			entry.Image,
			entry.State,
			started,
			completed,
		)
	}

	fmt.Println("Verdict: NOT EVALUATED (snapshot report only)")

	return nil
}
