package operator

import (
	"encoding/json"
	"fmt"
	"os"

	configv1 "github.com/openshift/api/config/v1"
)

type Analysis struct {
	Name        string
	Degraded    configv1.ClusterOperatorStatusCondition
	HasDegraded bool
	Result      string
	ExitCode    int
}

func Analyze(operator configv1.ClusterOperator) (Analysis, error) {
	if operator.APIVersion != "config.openshift.io/v1" ||
		operator.Kind != "ClusterOperator" {
		return Analysis{}, fmt.Errorf(
			"unsupported resource: %s/%s",
			operator.APIVersion,
			operator.Kind,
		)
	}

	if operator.Name == "" {
		return Analysis{}, fmt.Errorf("operator name is missing")
	}

	result := Analysis{
		Name: operator.Name,
	}

	for _, condition := range operator.Status.Conditions {
		if condition.Type != configv1.OperatorDegraded {
			continue
		}

		if result.HasDegraded {
			return Analysis{}, fmt.Errorf("duplicate Degraded condition")
		}

		result.Degraded = condition
		result.HasDegraded = true
	}

	if !result.HasDegraded {
		result.Result = "UNKNOWN (Degraded condition missing)"
		result.ExitCode = 3

		return result, nil
	}

	switch result.Degraded.Status {
	case configv1.ConditionTrue:
		result.Result = "DEGRADED"
		result.ExitCode = 2

	case configv1.ConditionFalse:
		result.Result = "NOT DEGRADED (reported)"
		result.ExitCode = 0

	case configv1.ConditionUnknown:
		result.Result = "UNKNOWN"
		result.ExitCode = 3

	default:
		return Analysis{}, fmt.Errorf(
			"invalid Degraded status: %q",
			result.Degraded.Status,
		)
	}

	return result, nil
}

func ReadSnapshot(path string) (configv1.ClusterOperator, error) {
	var snapshot configv1.ClusterOperator
	data, err := os.ReadFile(path)
	if err != nil {
		return snapshot, fmt.Errorf("read file %q: %w", path, err)
	}
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return snapshot, fmt.Errorf("decode JSON: %w", err)
	}
	return snapshot, nil
}
