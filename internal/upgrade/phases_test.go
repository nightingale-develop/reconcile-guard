package upgrade

import (
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAnalyzeUpgradePhases(t *testing.T) {
	observations, err := ReadHistory(
		"../../examples/cluster-version-history.jsonl",
	)
	if err != nil {
		t.Fatal(err)
	}

	states, err := AnalyzePhases(observations)
	if err != nil {
		t.Fatal(err)
	}

	want := []UpgradePhase{
		UpgradePhaseStable,
		UpgradePhaseUpdating,
		UpgradePhaseCompleted,
	}

	if len(states) != len(want) {
		t.Fatalf(
			"states = %d, want %d",
			len(states),
			len(want),
		)
	}

	for i, phase := range want {
		if states[i].Phase != phase {
			t.Errorf(
				"state %d = %s, want %s",
				i,
				states[i].Phase,
				phase,
			)
		}
	}

	if states[2].DesiredVersion != "4.20.0" {
		t.Errorf(
			"desired version = %q, want %q",
			states[2].DesiredVersion,
			"4.20.0",
		)
	}
}

func TestUpgradePhaseReturnsStableAfterCompletion(
	t *testing.T,
) {
	observations, err := ReadHistory(
		"../../examples/cluster-version-history.jsonl",
	)
	if err != nil {
		t.Fatal(err)
	}

	last := observations[len(observations)-1]
	last.ObservedAt = last.ObservedAt.Add(5 * time.Minute)

	observations = append(observations, last)

	states, err := AnalyzePhases(observations)
	if err != nil {
		t.Fatal(err)
	}

	if states[2].Phase != UpgradePhaseCompleted ||
		states[3].Phase != UpgradePhaseStable {
		t.Fatalf("unexpected phases: %+v", states)
	}
}

func TestUpgradePhaseUnknown(t *testing.T) {
	observations, err := ReadHistory(
		"../../examples/cluster-version-history.jsonl",
	)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		change func(*configv1.ClusterVersion)
	}{
		{
			name: "missing Progressing",
			change: func(version *configv1.ClusterVersion) {
				version.Status.Conditions =
					[]configv1.ClusterOperatorStatusCondition{
						{
							Type:   configv1.OperatorAvailable,
							Status: configv1.ConditionTrue,
						},
					}
			},
		},
		{
			name: "Progressing Unknown",
			change: func(version *configv1.ClusterVersion) {
				for i := range version.Status.Conditions {
					if version.Status.Conditions[i].Type ==
						configv1.OperatorProgressing {
						version.Status.Conditions[i].Status =
							configv1.ConditionUnknown
					}
				}
			},
		},
		{
			name: "missing history",
			change: func(version *configv1.ClusterVersion) {
				version.Status.History = nil
			},
		},
		{
			name: "desired version mismatch",
			change: func(version *configv1.ClusterVersion) {
				version.Status.Desired.Version = "9.9.9"
			},
		},
		{
			name: "completed but unavailable",
			change: func(version *configv1.ClusterVersion) {
				for i := range version.Status.Conditions {
					if version.Status.Conditions[i].Type ==
						configv1.OperatorAvailable {
						version.Status.Conditions[i].Status =
							configv1.ConditionFalse
					}
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			version :=
				observations[2].ClusterVersion.DeepCopy()

			tc.change(version)

			got := classifyUpgradePhase(*version)

			if got != UpgradePhaseUnknown {
				t.Fatalf(
					"phase = %s, want %s",
					got,
					UpgradePhaseUnknown,
				)
			}
		})
	}
}

func TestAnalyzeUpgradePhasesRejectsInvalidHistory(
	t *testing.T,
) {
	_, err := AnalyzePhases(nil)

	if err == nil {
		t.Fatal("expected history validation error")
	}
}

func TestPhaseRejectsContradictoryEvidence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func([]ClusterVersionObservation)
	}{
		{"zero completion", func(o []ClusterVersionObservation) {
			o[2].ClusterVersion.Status.History[0].CompletionTime = &metav1.Time{}
		}},
		{"completion before start", func(o []ClusterVersionObservation) {
			o[2].ClusterVersion.Status.History[0].CompletionTime = &metav1.Time{Time: o[0].ObservedAt}
		}},
		{"future completion", func(o []ClusterVersionObservation) {
			o[2].ClusterVersion.Status.History[0].CompletionTime = &metav1.Time{Time: o[2].ObservedAt.Add(time.Hour)}
		}},
		{"missing start", func(o []ClusterVersionObservation) { o[2].ClusterVersion.Status.History[0].StartedTime = metav1.Time{} }},
		{"future start", func(o []ClusterVersionObservation) {
			o[2].ClusterVersion.Status.History[0].StartedTime = metav1.Time{Time: o[2].ObservedAt.Add(time.Hour)}
		}},
		{"old generation", func(o []ClusterVersionObservation) {
			o[2].ClusterVersion.Generation = 2
			o[2].ClusterVersion.Status.ObservedGeneration = 1
		}},
		{"wrong history order", func(o []ClusterVersionObservation) {
			o[2].ClusterVersion.Status.History[1].StartedTime = metav1.Time{Time: o[2].ObservedAt}
		}},
		{"new target", func(o []ClusterVersionObservation) {
			o[2].ClusterVersion.Status.Desired.Version = "4.21.0"
			o[2].ClusterVersion.Status.History[0].Version = "4.21.0"
		}},
		{"image mismatch", func(o []ClusterVersionObservation) { o[2].ClusterVersion.Status.Desired.Image = "different" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			observations, err := ReadHistory("../../examples/cluster-version-history.jsonl")
			if err != nil {
				t.Fatal(err)
			}
			tc.change(observations)
			states, err := AnalyzePhases(observations)
			if err != nil {
				t.Fatal(err)
			}
			if states[2].Phase != UpgradePhaseUnknown {
				t.Fatalf("phase=%s", states[2].Phase)
			}
		})
	}
}
