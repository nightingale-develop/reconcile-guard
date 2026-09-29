package regression

import (
	"fmt"
	"sort"

	"github.com/nightingale-develop/reconcile-guard/internal/contracts"
	"github.com/nightingale-develop/reconcile-guard/internal/result"
)

const (
	conditionsContract = "normal-upgrade-operator-conditions"
	versionContract    = "operator-version-consistency"
)

// Compare uses contract verdicts only. A comparison PASS does not certify either run.
func Compare(baseline, candidate contracts.ClusterUpgradeReport) (Report, error) {
	before, err := indexOperators(baseline)
	if err != nil {
		return Report{}, fmt.Errorf("baseline: %w", err)
	}
	after, err := indexOperators(candidate)
	if err != nil {
		return Report{}, fmt.Errorf("candidate: %w", err)
	}
	report := Report{
		Verdict:   result.VerdictPass,
		Baseline:  RunSummary{VerificationVerdict: baseline.Verdict},
		Candidate: RunSummary{VerificationVerdict: candidate.Verdict},
		Scope:     Scope{BaselineOnlyOperators: []string{}, CandidateOnlyOperators: []string{}},
		Operators: []OperatorComparison{},
	}
	common := make([]string, 0)
	for name := range before {
		if _, ok := after[name]; ok {
			common = append(common, name)
		} else {
			report.Scope.BaselineOnlyOperators = append(report.Scope.BaselineOnlyOperators, name)
		}
	}
	for name := range after {
		if _, ok := before[name]; !ok {
			report.Scope.CandidateOnlyOperators = append(report.Scope.CandidateOnlyOperators, name)
		}
	}
	sort.Strings(common)
	sort.Strings(report.Scope.BaselineOnlyOperators)
	sort.Strings(report.Scope.CandidateOnlyOperators)
	report.Scope.CommonOperators = len(common)
	if len(common) == 0 || len(report.Scope.BaselineOnlyOperators) != 0 || len(report.Scope.CandidateOnlyOperators) != 0 {
		report.Verdict = result.VerdictInconclusive
	}
	for _, name := range common {
		b, c := before[name], after[name]
		comparison := OperatorComparison{
			Name: name, BaselineVerdict: b.Verdict, CandidateVerdict: c.Verdict,
			Verdict: result.VerdictPass,
			Contracts: []ContractComparison{
				compareContract(conditionsContract, b.Conditions.Verdict, c.Conditions.Verdict),
				compareContract(versionContract, b.Version.Verdict, c.Version.Verdict),
			},
		}
		for _, contract := range comparison.Contracts {
			switch contract.Change {
			case ChangeRegression:
				comparison.Verdict = result.VerdictFail
			case ChangeInconclusive:
				if comparison.Verdict != result.VerdictFail {
					comparison.Verdict = result.VerdictInconclusive
				}
			}
		}
		if comparison.Verdict == result.VerdictFail ||
			(comparison.Verdict == result.VerdictInconclusive && report.Verdict != result.VerdictFail) {
			report.Verdict = comparison.Verdict
		}
		report.Operators = append(report.Operators, comparison)
	}
	return report, nil
}

func compareContract(name string, baseline, candidate result.Verdict) ContractComparison {
	change := ChangeInconclusive
	switch {
	case baseline == "" || candidate == "", baseline == result.VerdictInconclusive || candidate == result.VerdictInconclusive:
	case baseline == candidate:
		change = ChangeUnchanged
	case baseline == result.VerdictPass && candidate == result.VerdictFail:
		change = ChangeRegression
	case baseline == result.VerdictFail && candidate == result.VerdictPass:
		change = ChangeImprovement
	}
	return ContractComparison{Name: name, BaselineVerdict: baseline, CandidateVerdict: candidate, Change: change}
}

func indexOperators(report contracts.ClusterUpgradeReport) (map[string]contracts.OperatorUpgradeReport, error) {
	if !validVerdict(report.Verdict) {
		return nil, fmt.Errorf("invalid verification verdict %q", report.Verdict)
	}
	operators := make(map[string]contracts.OperatorUpgradeReport, len(report.Operators))
	for _, operator := range report.Operators {
		if operator.Operator == "" {
			return nil, fmt.Errorf("operator name is missing")
		}
		if _, exists := operators[operator.Operator]; exists {
			return nil, fmt.Errorf("duplicate operator %q", operator.Operator)
		}
		if !validVerdict(operator.Verdict) {
			return nil, fmt.Errorf("operator %q: invalid verdict %q", operator.Operator, operator.Verdict)
		}
		for _, contract := range []struct {
			name, expected string
			verdict        result.Verdict
		}{
			{operator.Conditions.Contract, conditionsContract, operator.Conditions.Verdict},
			{operator.Version.Contract, versionContract, operator.Version.Verdict},
		} {
			if contract.name == "" && contract.verdict == "" {
				continue // A zero-value contract is absent, not a successful check.
			}
			if contract.name != contract.expected || !validVerdict(contract.verdict) {
				return nil, fmt.Errorf("operator %q: invalid %s contract name/verdict %q/%q", operator.Operator, contract.expected, contract.name, contract.verdict)
			}
		}
		operators[operator.Operator] = operator
	}
	return operators, nil
}

func validVerdict(verdict result.Verdict) bool {
	return verdict == result.VerdictPass || verdict == result.VerdictFail || verdict == result.VerdictInconclusive
}
