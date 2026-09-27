package contracts

import (
	"strings"
	"testing"

	configv1 "github.com/openshift/api/config/v1"

	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
)

func clusterOperatorHistory(
	name string,
	versions []upgrade.ClusterVersionObservation,
	completedVersion string,
) []operator.Observation {
	oldVersion :=
		versions[0].ClusterVersion.Status.Desired.Version

	observations := []operator.Observation{
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

	for i := range observations {
		observations[i].Operator.Name = name
	}

	observations[0].Operator.Status.Versions =
		[]configv1.OperandVersion{
			{
				Name:    "operator",
				Version: oldVersion,
			},
		}

	observations[1].Operator.Status.Versions =
		[]configv1.OperandVersion{
			{
				Name:    "operator",
				Version: oldVersion,
			},
		}

	if completedVersion != "" {
		observations[2].Operator.Status.Versions =
			[]configv1.OperandVersion{
				{
					Name:    "operator",
					Version: completedVersion,
				},
			}
	}

	return observations
}

func TestClusterUpgradePass(t *testing.T) {
	versions := loadContractVersions(t)

	target :=
		versions[len(versions)-1].
			ClusterVersion.Status.Desired.Version

	histories := [][]operator.Observation{
		clusterOperatorHistory(
			"ingress",
			versions,
			target,
		),
		clusterOperatorHistory(
			"network",
			versions,
			target,
		),
	}

	report, err := VerifyClusterUpgrade(
		versions,
		histories,
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

	if report.PassedOperators != 2 ||
		report.FailedOperators != 0 ||
		report.InconclusiveOperators != 0 {
		t.Fatalf("unexpected report: %+v", report)
	}

	if len(report.Operators) != 2 {
		t.Fatalf(
			"operators = %d, want 2",
			len(report.Operators),
		)
	}
}

func TestClusterUpgradeFail(t *testing.T) {
	versions := loadContractVersions(t)

	target :=
		versions[len(versions)-1].
			ClusterVersion.Status.Desired.Version

	ingress := clusterOperatorHistory(
		"ingress",
		versions,
		target,
	)

	network := clusterOperatorHistory(
		"network",
		versions,
		"4.19.0",
	)

	report, err := VerifyClusterUpgrade(
		versions,
		[][]operator.Observation{
			ingress,
			network,
		},
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

	if report.PassedOperators != 1 ||
		report.FailedOperators != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}

	if report.Operators[1].Version.Verdict !=
		ContractFail {
		t.Fatalf(
			"version verdict = %s, want %s",
			report.Operators[1].Version.Verdict,
			ContractFail,
		)
	}
}

func TestClusterUpgradeInconclusive(t *testing.T) {
	versions := loadContractVersions(t)

	target :=
		versions[len(versions)-1].
			ClusterVersion.Status.Desired.Version

	ingress := clusterOperatorHistory(
		"ingress",
		versions,
		target,
	)

	network := clusterOperatorHistory(
		"network",
		versions,
		"",
	)

	report, err := VerifyClusterUpgrade(
		versions,
		[][]operator.Observation{
			ingress,
			network,
		},
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

	if report.PassedOperators != 1 ||
		report.InconclusiveOperators != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestClusterUpgradeFailHasPrecedence(
	t *testing.T,
) {
	versions := loadContractVersions(t)

	target :=
		versions[len(versions)-1].
			ClusterVersion.Status.Desired.Version

	pass := clusterOperatorHistory(
		"ingress",
		versions,
		target,
	)

	fail := clusterOperatorHistory(
		"network",
		versions,
		"4.19.0",
	)

	inconclusive := clusterOperatorHistory(
		"authentication",
		versions,
		"",
	)

	report, err := VerifyClusterUpgrade(
		versions,
		[][]operator.Observation{
			pass,
			fail,
			inconclusive,
		},
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

func TestClusterUpgradeRejectsDuplicateOperator(
	t *testing.T,
) {
	versions := loadContractVersions(t)

	target :=
		versions[len(versions)-1].
			ClusterVersion.Status.Desired.Version

	first := clusterOperatorHistory(
		"ingress",
		versions,
		target,
	)

	second := clusterOperatorHistory(
		"ingress",
		versions,
		target,
	)

	_, err := VerifyClusterUpgrade(
		versions,
		[][]operator.Observation{
			first,
			second,
		},
	)

	if err == nil ||
		!strings.Contains(
			err.Error(),
			`duplicate operator history for "ingress"`,
		) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClusterUpgradeRequiresOperatorHistory(
	t *testing.T,
) {
	versions := loadContractVersions(t)

	_, err := VerifyClusterUpgrade(
		versions,
		nil,
	)

	if err == nil {
		t.Fatal("expected error")
	}
}
