package upgrade

import (
	"encoding/json"
	"fmt"
	"os"

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

func AnalyzeClusterVersion(version configv1.ClusterVersion) (ClusterVersionReport, error) {
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

func ReadClusterVersion(path string) (configv1.ClusterVersion, error) {
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
