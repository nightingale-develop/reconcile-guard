package contracts

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/nightingale-develop/reconcile-guard/internal/machineconfig"
	nodehistory "github.com/nightingale-develop/reconcile-guard/internal/node"
	machineconfigv1 "github.com/openshift/api/machineconfiguration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func evidencePool(t *testing.T, atIndex int, stable bool) machineconfig.Observation {
	t.Helper()
	updated, updating, current, desired := "False", "True", "rendered-a", "rendered-b"
	updatedCount := 0
	if stable {
		updated, updating, current = "True", "False", "rendered-b"
		updatedCount = 1
	}
	data := fmt.Sprintf(`{
		"apiVersion":"machineconfiguration.openshift.io/v1",
		"kind":"MachineConfigPool",
		"metadata":{"name":"master","generation":1},
		"spec":{"configuration":{"name":%q}},
		"status":{"observedGeneration":1,"configuration":{"name":%q},"machineCount":1,"updatedMachineCount":%d,"readyMachineCount":1,"unavailableMachineCount":0,"degradedMachineCount":0,"conditions":[{"type":"Updated","status":%q},{"type":"Updating","status":%q},{"type":"Degraded","status":"False"}]}
	}`, desired, current, updatedCount, updated, updating)
	var pool machineconfigv1.MachineConfigPool
	if err := json.Unmarshal([]byte(data), &pool); err != nil {
		t.Fatal(err)
	}
	versions, _ := lifecycleInputs(t)
	return machineconfig.Observation{ObservedAt: versions[atIndex].ObservedAt, Pool: pool}
}

func evidenceNode(t *testing.T, atIndex int, ready corev1.ConditionStatus, current, desired string) nodehistory.Observation {
	versions, _ := lifecycleInputs(t)
	return nodehistory.Observation{
		ObservedAt: versions[atIndex].ObservedAt,
		Node: corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-0", Annotations: map[string]string{
				nodehistory.CurrentMachineConfigAnnotation: current,
				nodehistory.DesiredMachineConfigAnnotation: desired,
			}},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: ready}},
				NodeInfo:   corev1.NodeSystemInfo{KubeletVersion: "v1.34.4"},
			},
		},
	}
}

func TestMachineConfigPoolLifecycleEvidence(t *testing.T) {
	versions, _ := lifecycleInputs(t)
	report, err := VerifyMachineConfigPoolLifecycleEvidence(versions, []machineconfig.Observation{
		evidencePool(t, 3, false),
		evidencePool(t, 4, true),
		evidencePool(t, 5, true),
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != ContractPass || report.ConvergedSamples == 0 || report.EvaluatedSamples == 0 {
		t.Fatalf("report=%+v", report)
	}
}

func TestMachineConfigPoolLifecycleEvidenceWithoutCompletionIsInconclusive(t *testing.T) {
	versions, _ := lifecycleInputs(t)
	report, err := VerifyMachineConfigPoolLifecycleEvidence(versions, []machineconfig.Observation{evidencePool(t, 3, false)})
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != ContractInconclusive || report.EvaluatedSamples != 0 {
		t.Fatalf("report=%+v", report)
	}
}

func TestMachineConfigPoolLifecycleEvidenceDoesNotFailDegraded(t *testing.T) {
	versions, _ := lifecycleInputs(t)
	observation := evidencePool(t, 4, false)
	for i := range observation.Pool.Status.Conditions {
		if observation.Pool.Status.Conditions[i].Type == machineconfigv1.MachineConfigPoolDegraded {
			observation.Pool.Status.Conditions[i].Status = corev1.ConditionTrue
		}
	}
	observation.Pool.Status.DegradedMachineCount = 1
	report, err := VerifyMachineConfigPoolLifecycleEvidence(versions, []machineconfig.Observation{observation})
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != ContractInconclusive {
		t.Fatalf("verdict=%s", report.Verdict)
	}
}

func TestNodeLifecycleEvidence(t *testing.T) {
	versions, _ := lifecycleInputs(t)
	report, err := VerifyNodeLifecycleEvidence(versions, []nodehistory.Observation{
		evidenceNode(t, 3, corev1.ConditionTrue, "rendered-a", "rendered-b"),
		evidenceNode(t, 4, corev1.ConditionTrue, "rendered-b", "rendered-b"),
		evidenceNode(t, 5, corev1.ConditionTrue, "rendered-b", "rendered-b"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != ContractPass || report.ConvergedSamples == 0 || report.EvaluatedSamples == 0 {
		t.Fatalf("report=%+v", report)
	}
}

func TestNodeLifecycleEvidenceDoesNotFailNotReady(t *testing.T) {
	versions, _ := lifecycleInputs(t)
	report, err := VerifyNodeLifecycleEvidence(versions, []nodehistory.Observation{
		evidenceNode(t, 4, corev1.ConditionFalse, "rendered-a", "rendered-b"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != ContractInconclusive {
		t.Fatalf("verdict=%s", report.Verdict)
	}
}
