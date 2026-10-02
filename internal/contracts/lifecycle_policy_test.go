package contracts

import (
	"testing"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/machineconfig"
	nodehistory "github.com/nightingale-develop/reconcile-guard/internal/node"
	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
)

func policyOperatorObservation(t *testing.T, at time.Time, conditionType configv1.ClusterStatusConditionType, status configv1.ConditionStatus) operator.Observation {
	t.Helper()
	_, observations := lifecycleInputs(t)
	item := observations[1]
	item.ObservedAt = at
	copy := item.Operator.DeepCopy()
	for i := range copy.Status.Conditions {
		if copy.Status.Conditions[i].Type == conditionType {
			copy.Status.Conditions[i].Status = status
		}
	}
	item.Operator = *copy
	return item
}

func TestOperatorConditionPolicyFailsObservedDuration(t *testing.T) {
	versions, _ := lifecycleInputs(t)
	start := versions[2].ObservedAt.Add(10 * time.Second)
	observations := []operator.Observation{
		policyOperatorObservation(t, start, configv1.OperatorDegraded, configv1.ConditionFalse),
		policyOperatorObservation(t, start.Add(10*time.Second), configv1.OperatorDegraded, configv1.ConditionTrue),
		policyOperatorObservation(t, start.Add(80*time.Second), configv1.OperatorDegraded, configv1.ConditionTrue),
	}
	report, err := VerifyOperatorConditionPolicy(versions, observations, OperatorConditionPolicy{
		Contract:          OperatorDegradedPolicyContract,
		Condition:         configv1.OperatorDegraded,
		AdverseStatus:     configv1.ConditionTrue,
		Limit:             time.Minute,
		MaxObservationGap: 2 * time.Minute,
		Operator:          "ingress",
		TargetVersion:     "4.20.0",
		Source:            "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != ContractFail || len(report.Episodes) != 1 || report.Episodes[0].ObservedSpan <= time.Minute {
		t.Fatalf("report=%+v", report)
	}
}

func TestOperatorConditionPolicyPassesKnownGoodSamples(t *testing.T) {
	versions, _ := lifecycleInputs(t)
	observations := []operator.Observation{
		policyOperatorObservation(t, versions[2].ObservedAt.Add(10*time.Second), configv1.OperatorAvailable, configv1.ConditionTrue),
		policyOperatorObservation(t, versions[3].ObservedAt.Add(-10*time.Second), configv1.OperatorAvailable, configv1.ConditionTrue),
	}
	report, err := VerifyOperatorConditionPolicy(versions, observations, OperatorConditionPolicy{
		Contract:          OperatorAvailabilityLossPolicyContract,
		Condition:         configv1.OperatorAvailable,
		AdverseStatus:     configv1.ConditionFalse,
		Limit:             time.Minute,
		MaxObservationGap: 2 * time.Minute,
		Operator:          "ingress",
		TargetVersion:     "4.20.0",
		Source:            "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != ContractPass || report.EvaluatedSamples != 2 {
		t.Fatalf("report=%+v", report)
	}
}

func TestOperatorConditionPolicyGapAndImageMismatchRemainInconclusive(t *testing.T) {
	versions, _ := lifecycleInputs(t)
	observations := []operator.Observation{
		policyOperatorObservation(t, versions[2].ObservedAt.Add(10*time.Second), configv1.OperatorDegraded, configv1.ConditionFalse),
		policyOperatorObservation(t, versions[3].ObservedAt.Add(-10*time.Second), configv1.OperatorDegraded, configv1.ConditionTrue),
	}
	base := OperatorConditionPolicy{Contract: OperatorDegradedPolicyContract, Condition: configv1.OperatorDegraded, AdverseStatus: configv1.ConditionTrue, Limit: time.Minute, MaxObservationGap: 30 * time.Second, Operator: "ingress", TargetVersion: "4.20.0", Source: "test"}
	gapReport, err := VerifyOperatorConditionPolicy(versions, observations, base)
	if err != nil {
		t.Fatal(err)
	}
	if gapReport.Verdict != ContractInconclusive || gapReport.Discontinuities != 0 || gapReport.UncertainSamples != 2 {
		t.Fatalf("gap report=%+v", gapReport)
	}

	base.MaxObservationGap = 2 * time.Minute
	base.TargetImage = "image:not-recorded"
	imageReport, err := VerifyOperatorConditionPolicy(versions, observations, base)
	if err != nil {
		t.Fatal(err)
	}
	if imageReport.Verdict != ContractInconclusive || imageReport.EvaluatedSamples != 0 {
		t.Fatalf("image report=%+v", imageReport)
	}
}

func TestOperatorConditionPolicyDoesNotBridgeClusterVersionBlocks(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func([]upgrade.ClusterVersionObservation)
	}{
		{"target change and return", func(versions []upgrade.ClusterVersionObservation) {
			versions[1].ClusterVersion.Status.Desired.Version = "4.21.0"
			versions[1].ClusterVersion.Status.History[0].Version = "4.21.0"
		}},
		{"unknown phase", func(versions []upgrade.ClusterVersionObservation) {
			versions[1].ClusterVersion.Status.Conditions = nil
		}},
		{"same version different image", func(versions []upgrade.ClusterVersionObservation) {
			versions[1].ClusterVersion.Status.Desired.Image = "other-image"
			versions[1].ClusterVersion.Status.History[0].Image = "other-image"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			versions := durationUpgradeVersions(t, 0, 1, 2)
			tc.change(versions)
			observations := []operator.Observation{
				policyOperatorObservation(t, versions[0].ObservedAt, configv1.OperatorDegraded, configv1.ConditionTrue),
				policyOperatorObservation(t, versions[2].ObservedAt, configv1.OperatorDegraded, configv1.ConditionTrue),
			}
			policy := durationUpgradePolicy()
			policy.Limit = time.Minute
			policy.MaxObservationGap = 5 * time.Minute
			got, err := VerifyOperatorConditionPolicy(versions, observations, OperatorConditionPolicy{
				Contract:          OperatorDegradedPolicyContract,
				Condition:         configv1.OperatorDegraded,
				AdverseStatus:     configv1.ConditionTrue,
				Limit:             policy.Limit,
				MaxObservationGap: policy.MaxObservationGap,
				Operator:          "ingress",
				TargetVersion:     "4.20.0",
				Source:            "test",
			})
			if err != nil {
				t.Fatal(err)
			}
			if got.Verdict != ContractInconclusive || got.Discontinuities != 1 {
				t.Fatalf("report=%+v", got)
			}
		})
	}
}

func TestOperatorConditionPolicyBracketGapBoundary(t *testing.T) {
	for _, tc := range []struct {
		name   string
		span   []int
		gap    time.Duration
		want   ContractVerdict
		uncert int
	}{
		{"exact", []int{0, 1}, time.Minute, ContractPass, 0},
		{"above", []int{0, 2}, time.Minute, ContractInconclusive, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			versions := durationUpgradeVersions(t, tc.span...)
			observations := []operator.Observation{
				policyOperatorObservation(t, versions[0].ObservedAt.Add(15*time.Second), configv1.OperatorDegraded, configv1.ConditionFalse),
				policyOperatorObservation(t, versions[0].ObservedAt.Add(45*time.Second), configv1.OperatorDegraded, configv1.ConditionFalse),
			}
			policy := OperatorConditionPolicy{
				Contract:          OperatorDegradedPolicyContract,
				Condition:         configv1.OperatorDegraded,
				AdverseStatus:     configv1.ConditionTrue,
				Limit:             time.Minute,
				MaxObservationGap: tc.gap,
				Operator:          "ingress",
				TargetVersion:     "4.20.0",
				Source:            "test",
			}
			got, err := VerifyOperatorConditionPolicy(versions, observations, policy)
			if err != nil {
				t.Fatal(err)
			}
			if got.Verdict != tc.want || got.UncertainSamples != tc.uncert {
				t.Fatalf("report=%+v", got)
			}
		})
	}
}

func TestMachineConfigPoolPostCompletionPolicy(t *testing.T) {
	versions, _ := lifecycleInputs(t)
	for _, tc := range []struct {
		name    string
		stable  bool
		verdict ContractVerdict
	}{
		{"stable after grace", true, ContractPass},
		{"updating after grace", false, ContractFail},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report, err := VerifyMachineConfigPoolPostCompletionPolicy(
				versions,
				[]machineconfig.Observation{evidencePool(t, 5, tc.stable)},
				"4.20.0", "", "test", time.Minute,
			)
			if err != nil {
				t.Fatal(err)
			}
			if report.Verdict != tc.verdict || !report.PolicyApplicable || report.EvaluatedSamples != 1 {
				t.Fatalf("report=%+v", report)
			}
		})
	}
}

func TestMachineConfigPoolGraceBoundaryIsApplicable(t *testing.T) {
	versions, _ := lifecycleInputs(t)
	report, err := VerifyMachineConfigPoolPostCompletionPolicy(versions, []machineconfig.Observation{evidencePool(t, 6, false)}, "4.20.0", "", "test", 3*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !report.PolicyApplicable || report.EvaluatedSamples != 1 || report.Verdict != ContractFail {
		t.Fatalf("report=%+v", report)
	}
}

func TestNodePostCompletionPolicies(t *testing.T) {
	versions, _ := lifecycleInputs(t)
	ready, err := VerifyNodeReadyPostCompletionPolicy(
		versions,
		[]nodehistory.Observation{evidenceNode(t, 5, corev1.ConditionFalse, "rendered-b", "rendered-b")},
		"4.20.0", "", "test", time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}
	if ready.Verdict != ContractFail {
		t.Fatalf("ready=%+v", ready)
	}

	config, err := VerifyNodeConfigPostCompletionPolicy(
		versions,
		[]nodehistory.Observation{evidenceNode(t, 5, corev1.ConditionTrue, "rendered-a", "rendered-b")},
		"4.20.0", "", "test", time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}
	if config.Verdict != ContractFail {
		t.Fatalf("config=%+v", config)
	}
}

func TestPostCompletionPolicyStartsNewGraceForRepeatedCompletion(t *testing.T) {
	versions, _ := lifecycleInputs(t)
	versions[5].ClusterVersion = *versions[3].ClusterVersion.DeepCopy()
	observation := evidenceNode(t, 6, corev1.ConditionFalse, "b", "b")
	report, err := VerifyNodeReadyPostCompletionPolicy(versions, []nodehistory.Observation{observation}, "4.20.0", "", "test", time.Minute)
	if err != nil || report.Verdict != ContractInconclusive || report.EvaluatedSamples != 0 {
		t.Fatalf("reused expired grace period after another completion: %+v %v", report, err)
	}
}

func TestOperatorConditionPolicyDurationBoundary(t *testing.T) {
	versions := durationUpgradeVersions(t, 0, 1, 2)
	for _, extra := range []time.Duration{0, time.Nanosecond} {
		observations := []operator.Observation{
			policyOperatorObservation(t, versions[0].ObservedAt, configv1.OperatorDegraded, configv1.ConditionTrue),
			policyOperatorObservation(t, versions[0].ObservedAt.Add(time.Minute+extra), configv1.OperatorDegraded, configv1.ConditionTrue),
		}
		report, err := VerifyOperatorConditionPolicy(versions, observations, OperatorConditionPolicy{Contract: OperatorDegradedPolicyContract, Condition: configv1.OperatorDegraded, AdverseStatus: configv1.ConditionTrue, Limit: time.Minute, MaxObservationGap: 2 * time.Minute, Operator: "ingress", TargetVersion: "4.20.0", Source: "test"})
		want := ContractInconclusive
		if extra > 0 {
			want = ContractFail
		}
		if err != nil || report.Verdict != want {
			t.Fatalf("extra=%s report=%+v err=%v", extra, report, err)
		}
	}
}
