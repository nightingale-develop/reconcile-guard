package contracts

import (
	"time"

	configv1 "github.com/openshift/api/config/v1"

	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
)

type ContractVerdict string

const (
	ContractPass         ContractVerdict = "PASS"
	ContractFail         ContractVerdict = "FAIL"
	ContractInconclusive ContractVerdict = "INCONCLUSIVE"
)

const normalUpgradeConditionsContract = "normal-upgrade-operator-conditions"

type ContractFinding struct {
	ObservedAt  time.Time
	Correlation CorrelationKind
	FromTime    time.Time
	ToTime      time.Time
	Condition   configv1.ClusterStatusConditionType
	Status      configv1.ConditionStatus
	Reason      string
	Message     string
}

type UpgradeContractReport struct {
	Contract            string
	Operator            string
	Verdict             ContractVerdict
	UpgradeSamples      int
	EvaluatedSamples    int
	AmbiguousSamples    int
	OutsideSamples      int
	MissingConditions   int
	UnknownPhaseSamples int
	Findings            []ContractFinding
}

func VerifyNormalUpgradeOperatorConditions(
	versionObservations []upgrade.ClusterVersionObservation,
	operatorObservations []operator.Observation,
) (UpgradeContractReport, error) {
	states, err := upgrade.AnalyzePhases(versionObservations)
	if err != nil {
		return UpgradeContractReport{}, err
	}

	operatorReport, err := operator.AnalyzeHistory(operatorObservations)
	if err != nil {
		return UpgradeContractReport{}, err
	}

	report := UpgradeContractReport{
		Contract: normalUpgradeConditionsContract,
		Operator: operatorReport.Operator,
		Verdict:  ContractInconclusive,
	}

	correlated, err := Correlate(
		states,
		operatorObservations,
	)

	if err != nil {
		return UpgradeContractReport{}, err
	}

	for _, sample := range correlated {
		switch sample.Kind {
		case CorrelationAmbiguous:
			report.AmbiguousSamples++
			continue

		case CorrelationOutside:
			report.OutsideSamples++
			continue
		}

		if sample.Phase == upgrade.UpgradePhaseUnknown {
			report.UnknownPhaseSamples++
			continue
		}

		if sample.Phase != upgrade.UpgradePhaseUpdating {
			continue
		}

		report.UpgradeSamples++
		report.EvaluatedSamples++

		available, hasAvailable := findOperatorCondition(
			sample.Observation.Operator.Status.Conditions,
			configv1.OperatorAvailable,
		)

		if !hasAvailable ||
			available.Status == configv1.ConditionUnknown {
			report.MissingConditions++
		} else if available.Status == configv1.ConditionFalse {
			report.Findings = append(
				report.Findings,
				newContractFinding(
					sample,
					available,
				),
			)
		}

		degraded, hasDegraded := findOperatorCondition(
			sample.Observation.Operator.Status.Conditions,
			configv1.OperatorDegraded,
		)

		if !hasDegraded ||
			degraded.Status == configv1.ConditionUnknown {
			report.MissingConditions++
		} else if degraded.Status == configv1.ConditionTrue {
			report.Findings = append(
				report.Findings,
				newContractFinding(
					sample,
					degraded,
				),
			)
		}
	}

	switch {
	case len(report.Findings) > 0:
		report.Verdict = ContractFail

	case report.UpgradeSamples == 0,
		report.MissingConditions > 0,
		report.AmbiguousSamples > 0,
		report.OutsideSamples > 0,
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
	sample CorrelatedObservation,
	condition configv1.ClusterOperatorStatusCondition,
) ContractFinding {
	return ContractFinding{
		ObservedAt:  sample.Observation.ObservedAt,
		Correlation: sample.Kind,
		FromTime:    sample.FromTime,
		ToTime:      sample.ToTime,
		Condition:   condition.Type,
		Status:      condition.Status,
		Reason:      condition.Reason,
		Message:     condition.Message,
	}
}
