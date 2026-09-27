package contracts

import (
	"strings"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"

	"github.com/nightingale-develop/reconcile-guard/internal/operator"
)

func withOperatorVersion(
	observation operator.Observation,
	version string,
) operator.Observation {
	result := observation
	result.Operator = *observation.Operator.DeepCopy()

	if version != "" {
		result.Operator.Status.Versions = []configv1.OperandVersion{
			{
				Name:    "operator",
				Version: version,
			},
		}
	}

	return result
}

func versionContractObservations(
	t *testing.T,
) ([]operator.Observation, []time.Time) {
	t.Helper()

	versions := loadContractVersions(t)

	observations := []operator.Observation{
		withOperatorVersion(
			contractObservation(
				versions[0].ObservedAt,
				configv1.ConditionTrue,
				configv1.ConditionFalse,
				configv1.ConditionFalse,
			),
			"4.19.0",
		),
		withOperatorVersion(
			contractObservation(
				versions[1].ObservedAt,
				configv1.ConditionTrue,
				configv1.ConditionTrue,
				configv1.ConditionFalse,
			),
			"4.19.0",
		),
		withOperatorVersion(
			contractObservation(
				versions[2].ObservedAt,
				configv1.ConditionTrue,
				configv1.ConditionFalse,
				configv1.ConditionFalse,
			),
			"4.20.0",
		),
	}

	return observations, []time.Time{
		versions[0].ObservedAt,
		versions[1].ObservedAt,
		versions[2].ObservedAt,
	}
}

func TestVersionConsistencyPass(t *testing.T) {
	versions := loadContractVersions(t)
	observations, _ := versionContractObservations(t)

	report, err := VerifyOperatorVersionConsistency(
		versions,
		observations,
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

	if report.CompletedTargets != 1 ||
		report.EvaluatedSamples != 1 ||
		len(report.Evidence) != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}

	if report.Evidence[0].ExpectedVersion != "4.20.0" ||
		report.Evidence[0].ReportedVersion != "4.20.0" {
		t.Fatalf(
			"unexpected evidence: %+v",
			report.Evidence[0],
		)
	}
}

func TestVersionConsistencyAllowsOldVersionWhileUpdating(
	t *testing.T,
) {
	versions := loadContractVersions(t)
	observations, _ := versionContractObservations(t)

	observations[1].Operator.Status.Versions[0].Version =
		"4.18.99"

	report, err := VerifyOperatorVersionConsistency(
		versions,
		observations,
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

	if report.EvaluatedSamples != 1 {
		t.Fatalf(
			"evaluated samples = %d, want 1",
			report.EvaluatedSamples,
		)
	}
}

func TestVersionConsistencyFail(t *testing.T) {
	versions := loadContractVersions(t)
	observations, _ := versionContractObservations(t)

	observations[2].Operator.Status.Versions[0].Version =
		"4.19.0"

	report, err := VerifyOperatorVersionConsistency(
		versions,
		observations,
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

	if len(report.Evidence) != 1 ||
		report.Evidence[0].Verdict != ContractFail {
		t.Fatalf(
			"unexpected evidence: %+v",
			report.Evidence,
		)
	}
}

func TestVersionConsistencyMissingVersion(t *testing.T) {
	versions := loadContractVersions(t)
	observations, _ := versionContractObservations(t)

	observations[2].Operator.Status.Versions = nil

	report, err := VerifyOperatorVersionConsistency(
		versions,
		observations,
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

	if report.MissingVersions != 1 {
		t.Fatalf(
			"missing versions = %d, want 1",
			report.MissingVersions,
		)
	}
}

func TestVersionConsistencyWithoutCompletedTarget(
	t *testing.T,
) {
	versions := loadContractVersions(t)
	observations, _ := versionContractObservations(t)

	report, err := VerifyOperatorVersionConsistency(
		versions[:1],
		observations[:1],
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

	if report.CompletedTargets != 0 {
		t.Fatalf(
			"completed targets = %d, want 0",
			report.CompletedTargets,
		)
	}
}

func TestVersionConsistencyBracketedAfterCompletion(
	t *testing.T,
) {
	versions := loadContractVersions(t)

	stable := versions[2]
	stable.ObservedAt =
		stable.ObservedAt.Add(5 * time.Minute)

	versions = append(versions, stable)

	observedAt :=
		versions[2].ObservedAt.Add(2 * time.Minute)

	observation := withOperatorVersion(
		contractObservation(
			observedAt,
			configv1.ConditionTrue,
			configv1.ConditionFalse,
			configv1.ConditionFalse,
		),
		"4.20.0",
	)

	report, err := VerifyOperatorVersionConsistency(
		versions,
		[]operator.Observation{observation},
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

	evidence := report.Evidence[0]

	if evidence.WindowFrom != versions[2].ObservedAt ||
		evidence.WindowTo != stable.ObservedAt {
		t.Fatalf(
			"unexpected evidence window: %+v",
			evidence,
		)
	}
}

func TestVersionConsistencyBracketedMismatchFails(
	t *testing.T,
) {
	versions := loadContractVersions(t)

	stable := versions[2]
	stable.ObservedAt =
		stable.ObservedAt.Add(5 * time.Minute)

	versions = append(versions, stable)

	observation := withOperatorVersion(
		contractObservation(
			versions[2].ObservedAt.Add(2*time.Minute),
			configv1.ConditionTrue,
			configv1.ConditionFalse,
			configv1.ConditionFalse,
		),
		"4.19.0",
	)

	report, err := VerifyOperatorVersionConsistency(
		versions,
		[]operator.Observation{observation},
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
}

func TestVersionConsistencyDuplicateOperatorVersion(
	t *testing.T,
) {
	versions := loadContractVersions(t)
	observations, _ := versionContractObservations(t)

	observations[2].Operator.Status.Versions =
		[]configv1.OperandVersion{
			{
				Name:    "operator",
				Version: "4.20.0",
			},
			{
				Name:    "operator",
				Version: "4.20.0",
			},
		}

	_, err := VerifyOperatorVersionConsistency(
		versions,
		observations,
	)

	if err == nil ||
		!strings.Contains(
			err.Error(),
			"duplicate operator version entry",
		) {
		t.Fatalf("unexpected error: %v", err)
	}
}
