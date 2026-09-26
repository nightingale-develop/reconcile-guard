package main

import (
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
)

func contractObservation(
	observedAt time.Time,
	available configv1.ConditionStatus,
	progressing configv1.ConditionStatus,
	degraded configv1.ConditionStatus,
) Observation {
	return Observation{
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

func loadContractVersions(t *testing.T) []ClusterVersionObservation {
	t.Helper()

	observations, err := readClusterVersionHistory(
		"examples/cluster-version-history.jsonl",
	)
	if err != nil {
		t.Fatal(err)
	}

	return observations
}

func TestUpgradeContractPass(t *testing.T) {
	versions := loadContractVersions(t)

	operators := []Observation{
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

	report, err := verifyNormalUpgradeOperatorConditions(
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
			operators := []Observation{
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
				verifyNormalUpgradeOperatorConditions(
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

func TestUpgradeContractInconclusiveWithoutSample(
	t *testing.T,
) {
	versions := loadContractVersions(t)

	operators := []Observation{
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

	report, err := verifyNormalUpgradeOperatorConditions(
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

	if report.MissingSamples != 1 {
		t.Fatalf(
			"missing samples = %d, want 1",
			report.MissingSamples,
		)
	}
}

func TestUpgradeContractInconclusiveWithUnknownCondition(
	t *testing.T,
) {
	versions := loadContractVersions(t)

	operators := []Observation{
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

	report, err := verifyNormalUpgradeOperatorConditions(
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

func TestContractExitCode(t *testing.T) {
	tests := []struct {
		verdict ContractVerdict
		want    int
	}{
		{ContractPass, 0},
		{ContractFail, 2},
		{ContractInconclusive, 3},
	}

	for _, tc := range tests {
		if got := contractExitCode(tc.verdict); got != tc.want {
			t.Errorf(
				"exit code = %d, want %d",
				got,
				tc.want,
			)
		}
	}
}
