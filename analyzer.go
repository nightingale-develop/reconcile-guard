package main

import "fmt"

type Condition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type ClusterOperator struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`

	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`

	Status struct {
		Conditions []Condition `json:"conditions"`
	} `json:"status"`
}

type Analysis struct {
	Name        string
	Degraded    Condition
	HasDegraded bool
	Result      string
	ExitCode    int
}

func analyzeOperator(operator ClusterOperator) (Analysis, error) {
	if operator.APIVersion != "config.openshift.io/v1" ||
		operator.Kind != "ClusterOperator" {
		return Analysis{}, fmt.Errorf(
			"unsupported resource: %s/%s",
			operator.APIVersion,
			operator.Kind,
		)
	}

	if operator.Metadata.Name == "" {
		return Analysis{}, fmt.Errorf("operator name is missing")
	}

	result := Analysis{
		Name: operator.Metadata.Name,
	}

	for _, condition := range operator.Status.Conditions {
		if condition.Type != "Degraded" {
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
	case "True":
		result.Result = "DEGRADED"
		result.ExitCode = 2

	case "False":
		result.Result = "NOT DEGRADED (reported)"
		result.ExitCode = 0

	case "Unknown":
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
