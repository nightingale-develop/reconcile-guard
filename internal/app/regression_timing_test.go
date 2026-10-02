package app

import (
	"testing"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
	corev1 "k8s.io/api/core/v1"
)

func TestRegressionTimingUsesContiguousUpdatingEvidence(t *testing.T) {
	start := time.Date(2026, 10, 2, 10, 0, 0, 0, time.UTC)
	states := []upgrade.UpgradeState{
		{ObservedAt: start, Phase: upgrade.UpgradePhaseUpdating, DesiredVersion: "4.20.0", DesiredImage: "image-a"},
		{ObservedAt: start.Add(time.Minute), Phase: upgrade.UpgradePhaseUnknown, DesiredVersion: "4.20.0", DesiredImage: "image-a"},
		{ObservedAt: start.Add(2 * time.Minute), Phase: upgrade.UpgradePhaseUpdating, DesiredVersion: "4.20.0", DesiredImage: "image-a"},
		{ObservedAt: start.Add(3 * time.Minute), Phase: upgrade.UpgradePhaseCompleted, DesiredVersion: "4.20.0", DesiredImage: "image-a"},
	}
	completion := finalTargetCompletion(states, "4.20.0", "image-a")
	if completion != 3 {
		t.Fatalf("completion=%d", completion)
	}
	if got := contiguousUpdatingStart(states, completion, "4.20.0", "image-a"); got != 2 {
		t.Fatalf("start=%d, want 2; timing must not bridge UNKNOWN", got)
	}
}

func TestRegressionTimingDoesNotBridgeTargetChange(t *testing.T) {
	start := time.Date(2026, 10, 2, 10, 0, 0, 0, time.UTC)
	states := []upgrade.UpgradeState{
		{ObservedAt: start, Phase: upgrade.UpgradePhaseUpdating, DesiredVersion: "4.20.0", DesiredImage: "image-a"},
		{ObservedAt: start.Add(time.Minute), Phase: upgrade.UpgradePhaseUpdating, DesiredVersion: "4.21.0", DesiredImage: "image-b"},
		{ObservedAt: start.Add(2 * time.Minute), Phase: upgrade.UpgradePhaseCompleted, DesiredVersion: "4.21.0", DesiredImage: "image-b"},
	}
	completion := finalTargetCompletion(states, "4.21.0", "image-b")
	if completion != 2 {
		t.Fatalf("completion=%d", completion)
	}
	if got := contiguousUpdatingStart(states, completion, "4.21.0", "image-b"); got != 1 {
		t.Fatalf("start=%d, want 1; timing must not include the previous target", got)
	}
	if got := finalTargetCompletion(states, "4.21.0", "image-a"); got != -1 {
		t.Fatalf("completion with wrong image=%d", got)
	}
}

func TestBuildRegressionTimingsRequiresObservedCompletion(t *testing.T) {
	for _, change := range []string{"stable only", "unknown before completion", "image change before completion"} {
		t.Run(change, func(t *testing.T) {
			input, err := loadRunInput(lifecyclePolicyAuxiliaryRunFixture(t))
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "stable only":
				input.Versions = input.Versions[:2]
			case "unknown before completion":
				input.Versions[3].ClusterVersion.Status.Conditions = nil
			case "image change before completion":
				input.Versions[3].ClusterVersion.Status.Desired.Image = "example.invalid/release:other"
				input.Versions[3].ClusterVersion.Status.History[0].Image = "example.invalid/release:other"
			}
			auxiliary, err := verifyAuxiliaryRunEvidence(input)
			if err != nil {
				t.Fatal(err)
			}
			timings, err := buildRegressionTimings(input, auxiliary)
			if err != nil || len(timings) != 0 {
				t.Fatalf("timings invented without observed completion: %+v %v", timings, err)
			}
		})
	}
}

func TestBuildRegressionTimingsKeepsNodeReadyAndConfigIndependent(t *testing.T) {
	input, err := loadRunInput(lifecyclePolicyAuxiliaryRunFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	input.Nodes[0][0].Node.Status.Conditions[0].Status = corev1.ConditionUnknown
	auxiliary, err := verifyAuxiliaryRunEvidence(input)
	if err != nil {
		t.Fatal(err)
	}
	timings, err := buildRegressionTimings(input, auxiliary)
	if err != nil {
		t.Fatal(err)
	}
	foundConfig := false
	for _, timing := range timings {
		if timing.Name == nodeReadyObservedDelayTiming {
			t.Fatal("UNKNOWN Ready produced a readiness timing")
		}
		if timing.Name == nodeConfigObservedDelayTiming {
			foundConfig = true
			if !timing.From.Equal(input.Versions[4].ObservedAt) || !timing.To.Equal(input.Nodes[0][0].ObservedAt) {
				t.Fatalf("timing must retain observation bounds: %+v", timing)
			}
		}
	}
	if !foundConfig {
		t.Fatal("lost independently observed configuration alignment")
	}
}
