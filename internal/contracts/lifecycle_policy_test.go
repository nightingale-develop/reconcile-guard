package contracts

import (
	"testing"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/machineconfig"
	nodehistory "github.com/nightingale-develop/reconcile-guard/internal/node"
	"github.com/nightingale-develop/reconcile-guard/internal/operator"
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
