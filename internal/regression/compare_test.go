package regression

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/nightingale-develop/reconcile-guard/internal/contracts"
	"github.com/nightingale-develop/reconcile-guard/internal/result"
)

func passingReport(names ...string) contracts.ClusterUpgradeReport {
	report := contracts.ClusterUpgradeReport{Verdict: result.VerdictPass}
	for _, name := range names {
		report.Operators = append(report.Operators, contracts.OperatorUpgradeReport{
			Operator: name, Verdict: result.VerdictPass,
			Conditions: contracts.UpgradeContractReport{Contract: conditionsContract, Operator: name, Verdict: result.VerdictPass},
			Version:    contracts.VersionConsistencyReport{Contract: versionContract, Operator: name, Verdict: result.VerdictPass},
		})
	}
	return report
}

func TestComparisonMatrix(t *testing.T) {
	for _, tc := range []struct {
		baseline, candidate result.Verdict
		change              Change
		verdict             result.Verdict
	}{
		{result.VerdictPass, result.VerdictPass, ChangeUnchanged, result.VerdictPass},
		{result.VerdictPass, result.VerdictFail, ChangeRegression, result.VerdictFail},
		{result.VerdictFail, result.VerdictPass, ChangeImprovement, result.VerdictPass},
		{result.VerdictFail, result.VerdictFail, ChangeUnchanged, result.VerdictPass},
		{result.VerdictPass, result.VerdictInconclusive, ChangeInconclusive, result.VerdictInconclusive},
		{result.VerdictFail, result.VerdictInconclusive, ChangeInconclusive, result.VerdictInconclusive},
		{result.VerdictInconclusive, result.VerdictPass, ChangeInconclusive, result.VerdictInconclusive},
		{result.VerdictInconclusive, result.VerdictFail, ChangeInconclusive, result.VerdictInconclusive},
		{result.VerdictInconclusive, result.VerdictInconclusive, ChangeInconclusive, result.VerdictInconclusive},
	} {
		for _, name := range []string{conditionsContract, versionContract} {
			t.Run(name+"/"+string(tc.baseline)+"->"+string(tc.candidate), func(t *testing.T) {
				baseline, candidate := passingReport("ingress"), passingReport("ingress")
				baseline.Verdict, baseline.Operators[0].Verdict = tc.baseline, tc.baseline
				candidate.Verdict, candidate.Operators[0].Verdict = tc.candidate, tc.candidate
				index := 0
				if name == conditionsContract {
					baseline.Operators[0].Conditions.Verdict = tc.baseline
					candidate.Operators[0].Conditions.Verdict = tc.candidate
				} else {
					index = 1
					baseline.Operators[0].Version.Verdict = tc.baseline
					candidate.Operators[0].Version.Verdict = tc.candidate
				}
				got, err := Compare(baseline, candidate)
				if err != nil {
					t.Fatal(err)
				}
				if got.Verdict != tc.verdict || got.Operators[0].Verdict != tc.verdict {
					t.Fatalf("verdicts: %+v", got)
				}
				contract := got.Operators[0].Contracts[index]
				if contract.Name != name || contract.Change != tc.change || contract.BaselineVerdict != tc.baseline || contract.CandidateVerdict != tc.candidate {
					t.Fatalf("contract: %+v", contract)
				}
				if got.Baseline.VerificationVerdict != tc.baseline || got.Candidate.VerificationVerdict != tc.candidate || got.Operators[0].BaselineVerdict != tc.baseline || got.Operators[0].CandidateVerdict != tc.candidate {
					t.Fatal("verification verdicts lost")
				}
			})
		}
	}
}

func TestScopeAndPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name                string
		baseline, candidate []string
		regress, uncertain  bool
		want                result.Verdict
	}{
		{"baseline only", []string{"ingress", "network"}, []string{"ingress"}, false, false, result.VerdictInconclusive},
		{"candidate only", []string{"ingress"}, []string{"ingress", "network"}, false, false, result.VerdictInconclusive},
		{"regression with scope difference", []string{"ingress", "network"}, []string{"ingress"}, true, false, result.VerdictFail},
		{"regression and inconclusive contracts", []string{"ingress"}, []string{"ingress"}, true, true, result.VerdictFail},
		{"disjoint", []string{"ingress"}, []string{"network"}, false, false, result.VerdictInconclusive},
		{"empty", nil, nil, false, false, result.VerdictInconclusive},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, after := passingReport(tc.baseline...), passingReport(tc.candidate...)
			if tc.regress {
				after.Verdict = result.VerdictFail
				after.Operators[0].Verdict = result.VerdictFail
				after.Operators[0].Conditions.Verdict = result.VerdictFail
			}
			if tc.uncertain {
				after.Operators[0].Version.Verdict = result.VerdictInconclusive
			}
			got, err := Compare(before, after)
			if err != nil {
				t.Fatal(err)
			}
			if got.Verdict != tc.want {
				t.Fatalf("verdict=%s want=%s", got.Verdict, tc.want)
			}
			if got.Scope.CommonOperators != len(got.Operators) || got.Operators == nil || got.Scope.BaselineOnlyOperators == nil || got.Scope.CandidateOnlyOperators == nil {
				t.Fatalf("invalid scope/arrays: %+v", got)
			}
		})
	}
}

func TestComparisonOrderingAndInputUnchanged(t *testing.T) {
	before := passingReport("z", "b", "only-b", "a", "only-a")
	after := passingReport("only-d", "a", "z", "only-c", "b")
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	got, err := Compare(before, after)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, op := range got.Operators {
		names = append(names, op.Name)
		if len(op.Contracts) != 2 || op.Contracts[0].Name != conditionsContract || op.Contracts[1].Name != versionContract {
			t.Fatalf("contract order=%+v", op.Contracts)
		}
	}
	if !reflect.DeepEqual(names, []string{"a", "b", "z"}) || !reflect.DeepEqual(got.Scope.BaselineOnlyOperators, []string{"only-a", "only-b"}) || !reflect.DeepEqual(got.Scope.CandidateOnlyOperators, []string{"only-c", "only-d"}) {
		t.Fatalf("ordering=%+v", got)
	}
	for i := 0; i < 5; i++ {
		again, err := Compare(before, after)
		if err != nil || !reflect.DeepEqual(got, again) {
			t.Fatalf("nondeterministic comparison: %v", err)
		}
	}
	b, _ := json.Marshal(before)
	c, _ := json.Marshal(after)
	if string(b) != string(beforeJSON) || string(c) != string(afterJSON) {
		t.Fatal("inputs mutated")
	}
}

func TestMissingContractIsInconclusive(t *testing.T) {
	for _, side := range []string{"baseline", "candidate", "both"} {
		for _, contract := range []string{conditionsContract, versionContract} {
			t.Run(side+"/"+contract, func(t *testing.T) {
				b, c := passingReport("ingress"), passingReport("ingress")
				clear := func(report *contracts.ClusterUpgradeReport) {
					if contract == conditionsContract {
						report.Operators[0].Conditions = contracts.UpgradeContractReport{}
					} else {
						report.Operators[0].Version = contracts.VersionConsistencyReport{}
					}
				}
				if side != "candidate" {
					clear(&b)
				}
				if side != "baseline" {
					clear(&c)
				}
				got, err := Compare(b, c)
				if err != nil {
					t.Fatal(err)
				}
				if got.Verdict != result.VerdictInconclusive {
					t.Fatalf("missing contract: %+v", got)
				}
				for _, item := range got.Operators[0].Contracts {
					if item.Name == contract && item.Change != ChangeInconclusive {
						t.Fatalf("change=%s", item.Change)
					}
				}
			})
		}
	}
}

func TestCompareIgnoresSamplingCounts(t *testing.T) {
	b, c := passingReport("ingress"), passingReport("ingress")
	c.PassedOperators = 999
	c.Operators[0].Conditions.EvaluatedSamples = 999
	c.Operators[0].Conditions.UpgradeSamples = 900
	c.Operators[0].Version.EvaluatedSamples = 800
	got, err := Compare(b, c)
	if err != nil {
		t.Fatal(err)
	}
	if got.Verdict != result.VerdictPass {
		t.Fatal("sampling counts changed comparison")
	}
	for _, contract := range got.Operators[0].Contracts {
		if contract.Change != ChangeUnchanged {
			t.Fatal("sampling counts changed contract")
		}
	}
}

func TestInvalidReports(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*contracts.ClusterUpgradeReport)
	}{
		{"aggregate verdict", func(r *contracts.ClusterUpgradeReport) { r.Verdict = "UNKNOWN" }},
		{"operator verdict", func(r *contracts.ClusterUpgradeReport) { r.Operators[0].Verdict = "" }},
		{"operator name", func(r *contracts.ClusterUpgradeReport) { r.Operators[0].Operator = "" }},
		{"duplicate operator", func(r *contracts.ClusterUpgradeReport) { r.Operators = append(r.Operators, r.Operators[0]) }},
		{"contract name", func(r *contracts.ClusterUpgradeReport) { r.Operators[0].Conditions.Contract = "some-other-contract" }},
		{"contract verdict", func(r *contracts.ClusterUpgradeReport) { r.Operators[0].Version.Verdict = "INVALID" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad, good := passingReport("ingress"), passingReport("ingress")
			tc.change(&bad)
			if _, err := Compare(bad, good); err == nil {
				t.Fatal("accepted invalid baseline")
			}
			if _, err := Compare(good, bad); err == nil {
				t.Fatal("accepted invalid candidate")
			}
		})
	}
}

func TestComparisonDocument(t *testing.T) {
	r, err := Compare(passingReport(), passingReport())
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(NewDocument(r))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if string(document["schemaVersion"]) != `"1"` || string(document["command"]) != `"compare-runs"` || document["comparison"] == nil || document["result"] != nil {
		t.Fatalf("document=%s", data)
	}
	var comparison map[string]json.RawMessage
	if err := json.Unmarshal(document["comparison"], &comparison); err != nil {
		t.Fatal(err)
	}
	if string(comparison["operators"]) != "[]" {
		t.Fatalf("operators=%s", comparison["operators"])
	}
}

func TestAggregateRegressionDominatesUnknownInEitherOrder(t *testing.T) {
	for _, regressing := range []int{0, 1} {
		b, c := passingReport("a", "z"), passingReport("z", "a")
		c.Verdict = result.VerdictFail
		for i := range c.Operators {
			verdict := result.VerdictInconclusive
			if i == regressing {
				verdict = result.VerdictFail
			}
			c.Operators[i].Verdict = verdict
			c.Operators[i].Version.Verdict = verdict
		}
		got, err := Compare(b, c)
		if err != nil || got.Verdict != result.VerdictFail {
			t.Fatalf("regressing=%d report=%+v err=%v", regressing, got, err)
		}
	}
}
