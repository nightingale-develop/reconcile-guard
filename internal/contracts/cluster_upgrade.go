package contracts

import (
	"fmt"

	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
)

type OperatorUpgradeReport struct {
	Operator   string
	Verdict    ContractVerdict
	Conditions UpgradeContractReport
	Version    VersionConsistencyReport
}

type ClusterUpgradeReport struct {
	Verdict               ContractVerdict
	Operators             []OperatorUpgradeReport
	PassedOperators       int
	FailedOperators       int
	InconclusiveOperators int
}

func VerifyClusterUpgrade(
	versions []upgrade.ClusterVersionObservation,
	histories [][]operator.Observation,
) (ClusterUpgradeReport, error) {
	if len(histories) == 0 {
		return ClusterUpgradeReport{}, fmt.Errorf(
			"at least one operator history is required",
		)
	}

	if _, err := upgrade.AnalyzeHistory(versions); err != nil {
		return ClusterUpgradeReport{}, err
	}

	report := ClusterUpgradeReport{
		Verdict: ContractInconclusive,
	}

	seen := make(map[string]struct{})

	for i, observations := range histories {
		history, err := operator.AnalyzeHistory(observations)
		if err != nil {
			return ClusterUpgradeReport{}, fmt.Errorf(
				"operator history %d: %w",
				i+1,
				err,
			)
		}

		if _, exists := seen[history.Operator]; exists {
			return ClusterUpgradeReport{}, fmt.Errorf(
				"duplicate operator history for %q",
				history.Operator,
			)
		}

		seen[history.Operator] = struct{}{}

		conditions, err :=
			VerifyNormalUpgradeOperatorConditions(
				versions,
				observations,
			)
		if err != nil {
			return ClusterUpgradeReport{}, fmt.Errorf(
				"operator %q conditions contract: %w",
				history.Operator,
				err,
			)
		}

		version, err :=
			VerifyOperatorVersionConsistency(
				versions,
				observations,
			)
		if err != nil {
			return ClusterUpgradeReport{}, fmt.Errorf(
				"operator %q version contract: %w",
				history.Operator,
				err,
			)
		}

		operatorVerdict := combineContractVerdicts(
			conditions.Verdict,
			version.Verdict,
		)

		report.Operators = append(
			report.Operators,
			OperatorUpgradeReport{
				Operator:   history.Operator,
				Verdict:    operatorVerdict,
				Conditions: conditions,
				Version:    version,
			},
		)

		switch operatorVerdict {
		case ContractPass:
			report.PassedOperators++

		case ContractFail:
			report.FailedOperators++

		case ContractInconclusive:
			report.InconclusiveOperators++
		}
	}

	switch {
	case report.FailedOperators > 0:
		report.Verdict = ContractFail

	case report.InconclusiveOperators > 0:
		report.Verdict = ContractInconclusive

	default:
		report.Verdict = ContractPass
	}

	return report, nil
}

func combineContractVerdicts(
	verdicts ...ContractVerdict,
) ContractVerdict {
	result := ContractPass

	for _, verdict := range verdicts {
		switch verdict {
		case ContractFail:
			return ContractFail

		case ContractInconclusive:
			result = ContractInconclusive

		case ContractPass:

		default:
			result = ContractInconclusive
		}
	}

	return result
}
