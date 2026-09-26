package main

import (
	"fmt"
	"time"

	configv1 "github.com/openshift/api/config/v1"
)

type ContractVerdict string

const (
	ContractPass         ContractVerdict = "PASS"
	ContractFail         ContractVerdict = "FAIL"
	ContractInconclusive ContractVerdict = "INCONCLUSIVE"
)

const normalUpgradeConditionsContract = "normal-upgrade-operator-conditions"

type ContractFinding struct {
	ObservedAt time.Time
	Condition  configv1.ClusterStatusConditionType
	Status     configv1.ConditionStatus
	Reason     string
	Message    string
}

type UpgradeContractReport struct {
	Contract            string
	Operator            string
	Verdict             ContractVerdict
	UpgradeSamples      int
	EvaluatedSamples    int
	MissingSamples      int
	MissingConditions   int
	UnknownPhaseSamples int
	Findings            []ContractFinding
}

func verifyNormalUpgradeOperatorConditions(
	versionObservations []ClusterVersionObservation,
	operatorObservations []Observation,
) (UpgradeContractReport, error) {
	states, err := analyzeUpgradePhases(versionObservations)
	if err != nil {
		return UpgradeContractReport{}, err
	}

	operatorReport, err := analyzeHistory(operatorObservations)
	if err != nil {
		return UpgradeContractReport{}, err
	}

	report := UpgradeContractReport{
		Contract: normalUpgradeConditionsContract,
		Operator: operatorReport.Operator,
		Verdict:  ContractInconclusive,
	}

	operatorByTime := make(map[int64]Observation, len(operatorObservations))

	for _, observation := range operatorObservations {
		operatorByTime[observation.ObservedAt.UnixNano()] = observation
	}

	for _, state := range states {
		if state.Phase == UpgradePhaseUnknown {
			report.UnknownPhaseSamples++
			continue
		}

		if state.Phase != UpgradePhaseUpdating {
			continue
		}

		report.UpgradeSamples++

		observation, exists := operatorByTime[state.ObservedAt.UnixNano()]
		if !exists {
			report.MissingSamples++
			continue
		}

		report.EvaluatedSamples++

		available, hasAvailable := findOperatorCondition(
			observation.Operator.Status.Conditions,
			configv1.OperatorAvailable,
		)

		if !hasAvailable ||
			available.Status == configv1.ConditionUnknown {
			report.MissingConditions++
		} else if available.Status == configv1.ConditionFalse {
			report.Findings = append(
				report.Findings,
				newContractFinding(
					observation.ObservedAt,
					available,
				),
			)
		}

		degraded, hasDegraded := findOperatorCondition(
			observation.Operator.Status.Conditions,
			configv1.OperatorDegraded,
		)

		if !hasDegraded ||
			degraded.Status == configv1.ConditionUnknown {
			report.MissingConditions++
		} else if degraded.Status == configv1.ConditionTrue {
			report.Findings = append(
				report.Findings,
				newContractFinding(
					observation.ObservedAt,
					degraded,
				),
			)
		}
	}

	switch {
	case len(report.Findings) > 0:
		report.Verdict = ContractFail

	case report.UpgradeSamples == 0,
		report.MissingSamples > 0,
		report.MissingConditions > 0,
		report.UnknownPhaseSamples > 0:
		report.Verdict = ContractInconclusive

	default:
		report.Verdict = ContractPass
	}

	return report, nil
}

func findOperatorCondition(
	conditions []configv1.ClusterOperatorStatusCondition,
	conditionType configv1.ClusterStatusConditionType,
) (configv1.ClusterOperatorStatusCondition, bool) {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return condition, true
		}
	}

	return configv1.ClusterOperatorStatusCondition{}, false
}

func newContractFinding(
	observedAt time.Time,
	condition configv1.ClusterOperatorStatusCondition,
) ContractFinding {
	return ContractFinding{
		ObservedAt: observedAt,
		Condition:  condition.Type,
		Status:     condition.Status,
		Reason:     condition.Reason,
		Message:    condition.Message,
	}
}

func printUpgradeContractReport(report UpgradeContractReport) {
	fmt.Println("Contract:", report.Contract)
	fmt.Println("Operator:", report.Operator)
	fmt.Println("Verdict:", report.Verdict)
	fmt.Println("Upgrade samples:", report.UpgradeSamples)
	fmt.Println("Evaluated samples:", report.EvaluatedSamples)
	fmt.Println("Missing samples:", report.MissingSamples)
	fmt.Println("Missing conditions:", report.MissingConditions)
	fmt.Println("Unknown phase samples:", report.UnknownPhaseSamples)

	if len(report.Findings) == 0 {
		return
	}

	fmt.Println("Evidence:")

	for _, finding := range report.Findings {
		fmt.Printf(
			"  %s %s=%s",
			finding.ObservedAt.Format(time.RFC3339Nano),
			finding.Condition,
			finding.Status,
		)

		if finding.Reason != "" {
			fmt.Printf(" reason=%q", finding.Reason)
		}

		if finding.Message != "" {
			fmt.Printf(" message=%q", finding.Message)
		}

		fmt.Println()
	}
}

func contractExitCode(verdict ContractVerdict) int {
	switch verdict {
	case ContractPass:
		return 0
	case ContractFail:
		return 2
	case ContractInconclusive:
		return 3
	default:
		return 1
	}
}
