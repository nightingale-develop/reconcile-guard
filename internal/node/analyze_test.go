package node

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func nodeObservation(at time.Time, ready corev1.ConditionStatus, current, desired, kubelet string) Observation {
	return Observation{
		ObservedAt: at,
		Node: corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-0",
				Annotations: map[string]string{
					CurrentMachineConfigAnnotation: current,
					DesiredMachineConfigAnnotation: desired,
				},
			},
			TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Node"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: ready}},
				NodeInfo:   corev1.NodeSystemInfo{KubeletVersion: kubelet},
			},
		},
	}
}

func TestAnalyzeHistoryRejectsDuplicateReadyCondition(t *testing.T) {
	observation := nodeObservation(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC), corev1.ConditionTrue, "rendered-a", "rendered-a", "v1.34.4")
	observation.Node.Status.Conditions = append(observation.Node.Status.Conditions, corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionFalse})
	if _, err := AnalyzeHistory([]Observation{observation}); err == nil {
		t.Fatal("accepted duplicate NodeReady condition")
	}
}

func TestAnalyzeHistoryRejectsUnsupportedTypeMeta(t *testing.T) {
	observation := nodeObservation(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC), corev1.ConditionTrue, "rendered-a", "rendered-a", "v1.34.4")
	observation.Node.Kind = "Pod"
	if _, err := AnalyzeHistory([]Observation{observation}); err == nil {
		t.Fatal("accepted unsupported Node kind")
	}
}

func TestAnalyzeLifecycle(t *testing.T) {
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	observations := []Observation{
		nodeObservation(base, corev1.ConditionTrue, "rendered-a", "rendered-a", "v1.34.4"),
		nodeObservation(base.Add(time.Minute), corev1.ConditionTrue, "rendered-a", "rendered-b", "v1.34.4"),
		nodeObservation(base.Add(2*time.Minute), corev1.ConditionTrue, "rendered-b", "rendered-b", "v1.35.0"),
	}
	report, err := AnalyzeLifecycle(observations)
	if err != nil {
		t.Fatal(err)
	}
	if report.Observations != 3 || len(report.Transitions) != 2 {
		t.Fatalf("report=%+v", report)
	}
	if !report.States[0].ConfigAligned || report.States[1].ConfigAligned || !report.States[2].ConfigAligned {
		t.Fatalf("states=%+v", report.States)
	}
	if report.Transitions[1].KubeletVersionFrom != "v1.34.4" || report.Transitions[1].KubeletVersionTo != "v1.35.0" {
		t.Fatalf("transition=%+v", report.Transitions[1])
	}
}

func TestAnalyzeLifecycleRejectsInvalidReady(t *testing.T) {
	for _, status := range []corev1.ConditionStatus{"", "invalid"} {
		observation := nodeObservation(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC), status, "a", "a", "v1.34.4")
		if _, err := AnalyzeLifecycle([]Observation{observation}); err == nil {
			t.Fatalf("accepted invalid Ready %q", status)
		}
	}
}
