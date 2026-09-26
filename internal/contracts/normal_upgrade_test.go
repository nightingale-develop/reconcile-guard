package contracts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"

	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
)

func contractObservation(
	observedAt time.Time,
	available configv1.ConditionStatus,
	progressing configv1.ConditionStatus,
	degraded configv1.ConditionStatus,
) operator.Observation {
	return operator.Observation{
		ObservedAt: observedAt,
		Operator: testOperator(
			configv1.ClusterOperatorStatusCondition{
				Type:   configv1.OperatorAvailable,
				Status: available,
			},
			configv1.ClusterOperatorStatusCondition{
				Type:   configv1.OperatorProgressing,
				Status: progressing,
			},
			configv1.ClusterOperatorStatusCondition{
				Type:   configv1.OperatorDegraded,
				Status: degraded,
			},
		),
	}
}

func loadContractVersions(t *testing.T) []upgrade.ClusterVersionObservation {
	t.Helper()

	observations, err := upgrade.ReadHistory(
		"../../examples/cluster-version-history.jsonl",
	)
	if err != nil {
		t.Fatal(err)
	}

	return observations
}

func TestUpgradeContractPass(t *testing.T) {
	versions := loadContractVersions(t)

	operators := []operator.Observation{
		contractObservation(
			versions[0].ObservedAt,
			configv1.ConditionTrue,
			configv1.ConditionFalse,
			configv1.ConditionFalse,
		),
		contractObservation(
			versions[1].ObservedAt,
			configv1.ConditionTrue,
			configv1.ConditionTrue,
			configv1.ConditionFalse,
		),
		contractObservation(
			versions[2].ObservedAt,
			configv1.ConditionTrue,
			configv1.ConditionFalse,
			configv1.ConditionFalse,
		),
	}

	report, err := VerifyNormalUpgradeOperatorConditions(
		versions,
		operators,
	)
	if err != nil {
		t.Fatal(err)
	}

	if report.Verdict != ContractPass {
		t.Fatalf(
			"verdict = %s, want %s",
			report.Verdict,
			ContractPass,
		)
	}

	if report.UpgradeSamples != 1 ||
		report.EvaluatedSamples != 1 ||
		len(report.Findings) != 0 {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestUpgradeContractFail(t *testing.T) {
	versions := loadContractVersions(t)

	tests := []struct {
		name      string
		available configv1.ConditionStatus
		degraded  configv1.ConditionStatus
		condition configv1.ClusterStatusConditionType
	}{
		{
			name:      "Available False",
			available: configv1.ConditionFalse,
			degraded:  configv1.ConditionFalse,
			condition: configv1.OperatorAvailable,
		},
		{
			name:      "Degraded True",
			available: configv1.ConditionTrue,
			degraded:  configv1.ConditionTrue,
			condition: configv1.OperatorDegraded,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			operators := []operator.Observation{
				contractObservation(
					versions[0].ObservedAt,
					configv1.ConditionTrue,
					configv1.ConditionFalse,
					configv1.ConditionFalse,
				),
				contractObservation(
					versions[1].ObservedAt,
					tc.available,
					configv1.ConditionTrue,
					tc.degraded,
				),
				contractObservation(
					versions[2].ObservedAt,
					configv1.ConditionTrue,
					configv1.ConditionFalse,
					configv1.ConditionFalse,
				),
			}

			report, err :=
				VerifyNormalUpgradeOperatorConditions(
					versions,
					operators,
				)
			if err != nil {
				t.Fatal(err)
			}

			if report.Verdict != ContractFail {
				t.Fatalf(
					"verdict = %s, want %s",
					report.Verdict,
					ContractFail,
				)
			}

			if len(report.Findings) != 1 {
				t.Fatalf(
					"findings = %d, want 1",
					len(report.Findings),
				)
			}

			if report.Findings[0].Condition != tc.condition {
				t.Fatalf(
					"condition = %s, want %s",
					report.Findings[0].Condition,
					tc.condition,
				)
			}
		})
	}
}

func TestUpgradeContractInconclusiveWithoutUpdatingSample(
	t *testing.T,
) {
	versions := loadContractVersions(t)

	operators := []operator.Observation{
		contractObservation(
			versions[0].ObservedAt,
			configv1.ConditionTrue,
			configv1.ConditionFalse,
			configv1.ConditionFalse,
		),
		contractObservation(
			versions[2].ObservedAt,
			configv1.ConditionTrue,
			configv1.ConditionFalse,
			configv1.ConditionFalse,
		),
	}

	report, err := VerifyNormalUpgradeOperatorConditions(
		versions,
		operators,
	)
	if err != nil {
		t.Fatal(err)
	}

	if report.Verdict != ContractInconclusive {
		t.Fatalf(
			"verdict = %s, want %s",
			report.Verdict,
			ContractInconclusive,
		)
	}

	if report.UpgradeSamples != 0 {
		t.Fatalf(
			"upgrade samples = %d, want 0",
			report.UpgradeSamples,
		)
	}
}

func TestUpgradeContractInconclusiveWithUnknownCondition(
	t *testing.T,
) {
	versions := loadContractVersions(t)

	operators := []operator.Observation{
		contractObservation(
			versions[0].ObservedAt,
			configv1.ConditionTrue,
			configv1.ConditionFalse,
			configv1.ConditionFalse,
		),
		contractObservation(
			versions[1].ObservedAt,
			configv1.ConditionUnknown,
			configv1.ConditionTrue,
			configv1.ConditionFalse,
		),
		contractObservation(
			versions[2].ObservedAt,
			configv1.ConditionTrue,
			configv1.ConditionFalse,
			configv1.ConditionFalse,
		),
	}

	report, err := VerifyNormalUpgradeOperatorConditions(
		versions,
		operators,
	)
	if err != nil {
		t.Fatal(err)
	}

	if report.Verdict != ContractInconclusive {
		t.Fatalf(
			"verdict = %s, want %s",
			report.Verdict,
			ContractInconclusive,
		)
	}

	if report.MissingConditions != 1 {
		t.Fatalf(
			"missing conditions = %d, want 1",
			report.MissingConditions,
		)
	}
}

func TestUpgradeContractUsesBracketedSample(
	t *testing.T,
) {
	versions := loadContractVersions(t)

	updating := versions[1]

	secondUpdating := updating
	secondUpdating.ObservedAt =
		updating.ObservedAt.Add(2 * time.Minute)

	versions = append(
		versions[:2],
		secondUpdating,
		versions[2],
	)

	operatorTime :=
		updating.ObservedAt.Add(time.Minute)

	operators := []operator.Observation{
		contractObservation(
			versions[0].ObservedAt,
			configv1.ConditionTrue,
			configv1.ConditionFalse,
			configv1.ConditionFalse,
		),
		contractObservation(
			operatorTime,
			configv1.ConditionTrue,
			configv1.ConditionTrue,
			configv1.ConditionFalse,
		),
		contractObservation(
			versions[3].ObservedAt,
			configv1.ConditionTrue,
			configv1.ConditionFalse,
			configv1.ConditionFalse,
		),
	}

	report, err :=
		VerifyNormalUpgradeOperatorConditions(
			versions,
			operators,
		)
	if err != nil {
		t.Fatal(err)
	}

	if report.Verdict != ContractPass {
		t.Fatalf(
			"verdict = %s, want %s",
			report.Verdict,
			ContractPass,
		)
	}

	if report.UpgradeSamples != 1 {
		t.Fatalf(
			"upgrade samples = %d, want 1",
			report.UpgradeSamples,
		)
	}
}

func TestUpgradeContractDoesNotGuessAcrossPhaseChange(
	t *testing.T,
) {
	versions := loadContractVersions(t)

	operatorTime :=
		versions[1].ObservedAt.Add(time.Minute)

	operators := []operator.Observation{
		contractObservation(
			versions[0].ObservedAt,
			configv1.ConditionTrue,
			configv1.ConditionFalse,
			configv1.ConditionFalse,
		),
		contractObservation(
			operatorTime,
			configv1.ConditionTrue,
			configv1.ConditionTrue,
			configv1.ConditionFalse,
		),
		contractObservation(
			versions[2].ObservedAt,
			configv1.ConditionTrue,
			configv1.ConditionFalse,
			configv1.ConditionFalse,
		),
	}

	report, err :=
		VerifyNormalUpgradeOperatorConditions(
			versions,
			operators,
		)
	if err != nil {
		t.Fatal(err)
	}

	if report.Verdict != ContractInconclusive {
		t.Fatalf(
			"verdict = %s, want %s",
			report.Verdict,
			ContractInconclusive,
		)
	}

	if report.AmbiguousSamples != 1 {
		t.Fatalf(
			"ambiguous samples = %d, want 1",
			report.AmbiguousSamples,
		)
	}
}

func testOperator(conditions ...configv1.ClusterOperatorStatusCondition) configv1.ClusterOperator {
	var result configv1.ClusterOperator
	result.APIVersion = "config.openshift.io/v1"
	result.Kind = "ClusterOperator"
	result.Name = "ingress"
	result.Status.Conditions = conditions
	return result
}

func TestContractEvidenceGapsAndFailurePrecedence(t *testing.T) {
	for _, gap := range []string{"ambiguous", "outside", "unknown phase", "missing condition"} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/fail=%v", gap, fail), func(t *testing.T) {
				versions := loadContractVersions(t)
				available := configv1.ConditionTrue
				if fail {
					available = configv1.ConditionFalse
				}
				sample := contractObservation(versions[1].ObservedAt, available, configv1.ConditionTrue, configv1.ConditionFalse)
				sample.Operator.Status.Conditions[0].Reason = "ExampleReason"
				sample.Operator.Status.Conditions[0].Message = "Example evidence"
				extra := contractObservation(versions[1].ObservedAt.Add(time.Minute), configv1.ConditionTrue, configv1.ConditionTrue, configv1.ConditionFalse)
				switch gap {
				case "outside":
					extra.ObservedAt = versions[2].ObservedAt.Add(time.Minute)
				case "unknown phase":
					extra.ObservedAt = versions[2].ObservedAt
					versions[2].ClusterVersion.Status.Conditions = nil
				case "missing condition":
					sample.Operator.Status.Conditions = sample.Operator.Status.Conditions[:2]
					extra.ObservedAt = versions[2].ObservedAt
				}
				operators := []operator.Observation{sample, extra}
				before, _ := json.Marshal([]any{versions, operators})
				report, err := VerifyNormalUpgradeOperatorConditions(versions, operators)
				if err != nil {
					t.Fatal(err)
				}
				want := ContractInconclusive
				if fail {
					want = ContractFail
				}
				if report.Verdict != want {
					t.Fatalf("got %+v want %s", report, want)
				}
				if fail {
					if len(report.Findings) != 1 {
						t.Fatalf("findings: %+v", report.Findings)
					}
					finding := report.Findings[0]
					if finding.ObservedAt != sample.ObservedAt || finding.FromTime != sample.ObservedAt || finding.ToTime != sample.ObservedAt || finding.Correlation != CorrelationExact || finding.Reason != "ExampleReason" || finding.Message != "Example evidence" {
						t.Fatalf("evidence: %+v", finding)
					}
				}
				after, _ := json.Marshal([]any{versions, operators})
				if !bytes.Equal(before, after) {
					t.Fatal("inputs mutated")
				}
			})
		}
	}
}

func TestBracketedFailureRetainsInterval(t *testing.T) {
	versions := loadContractVersions(t)
	second := versions[1]
	second.ObservedAt = second.ObservedAt.Add(2 * time.Minute)
	versions = []upgrade.ClusterVersionObservation{versions[0], versions[1], second, versions[2]}
	at := versions[1].ObservedAt.Add(time.Minute)
	report, err := VerifyNormalUpgradeOperatorConditions(versions, []operator.Observation{contractObservation(at, configv1.ConditionFalse, configv1.ConditionTrue, configv1.ConditionFalse)})
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != ContractFail || len(report.Findings) != 1 {
		t.Fatalf("report: %+v", report)
	}
	f := report.Findings[0]
	if f.Correlation != CorrelationBracketed || f.FromTime != versions[1].ObservedAt || f.ToTime != second.ObservedAt || f.ObservedAt != at {
		t.Fatalf("evidence: %+v", f)
	}
}
