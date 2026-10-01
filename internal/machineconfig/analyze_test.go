package machineconfig

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	machineconfigv1 "github.com/openshift/api/machineconfiguration/v1"
)

func poolObservation(t *testing.T, at time.Time, updated, updating, degraded string, machineCount, updatedCount, readyCount, unavailableCount, degradedCount int32, current, desired string) Observation {
	t.Helper()
	data := fmt.Sprintf(`{
		"apiVersion":"machineconfiguration.openshift.io/v1",
		"kind":"MachineConfigPool",
		"metadata":{"name":"master"},
		"spec":{"configuration":{"name":%q}},
		"status":{
			"configuration":{"name":%q},
			"machineCount":%d,
			"updatedMachineCount":%d,
			"readyMachineCount":%d,
			"unavailableMachineCount":%d,
			"degradedMachineCount":%d,
			"conditions":[
				{"type":"Updated","status":%q},
				{"type":"Updating","status":%q},
				{"type":"Degraded","status":%q}
			]
		}
	}`, desired, current, machineCount, updatedCount, readyCount, unavailableCount, degradedCount, updated, updating, degraded)
	var pool machineconfigv1.MachineConfigPool
	if err := json.Unmarshal([]byte(data), &pool); err != nil {
		t.Fatal(err)
	}
	return Observation{ObservedAt: at, Pool: pool}
}

func TestAnalyzeLifecyclePhases(t *testing.T) {
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		obs  Observation
		want Phase
	}{
		{"stable", poolObservation(t, base, "True", "False", "False", 1, 1, 1, 0, 0, "rendered-a", "rendered-a"), PhaseStable},
		{"updating", poolObservation(t, base, "False", "True", "False", 1, 0, 1, 0, 0, "rendered-a", "rendered-b"), PhaseUpdating},
		{"contradictory updated and updating", poolObservation(t, base, "True", "True", "False", 1, 1, 1, 0, 0, "rendered-a", "rendered-b"), PhaseUnknown},
		{"degraded condition", poolObservation(t, base, "False", "False", "True", 1, 0, 0, 1, 1, "rendered-a", "rendered-b"), PhaseDegraded},
		{"degraded count", poolObservation(t, base, "False", "False", "False", 1, 0, 0, 1, 1, "rendered-a", "rendered-b"), PhaseDegraded},
		{"inconsistent stable counters", poolObservation(t, base, "True", "False", "False", 1, 0, 1, 0, 0, "rendered-a", "rendered-a"), PhaseUnknown},
		{"configuration mismatch", poolObservation(t, base, "True", "False", "False", 1, 1, 1, 0, 0, "rendered-a", "rendered-b"), PhaseUnknown},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			report, err := AnalyzeLifecycle([]Observation{tc.obs})
			if err != nil {
				t.Fatal(err)
			}
			if got := report.States[0].Phase; got != tc.want {
				t.Fatalf("phase=%s want=%s", got, tc.want)
			}
		})
	}
}

func TestAnalyzeLifecycleTransitions(t *testing.T) {
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	observations := []Observation{
		poolObservation(t, base, "True", "False", "False", 1, 1, 1, 0, 0, "rendered-a", "rendered-a"),
		poolObservation(t, base.Add(time.Minute), "False", "True", "False", 1, 0, 1, 0, 0, "rendered-a", "rendered-b"),
		poolObservation(t, base.Add(2*time.Minute), "True", "False", "False", 1, 1, 1, 0, 0, "rendered-b", "rendered-b"),
	}
	report, err := AnalyzeLifecycle(observations)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Transitions) != 2 || report.Transitions[0].From != PhaseStable || report.Transitions[0].To != PhaseUpdating || report.Transitions[1].To != PhaseStable {
		t.Fatalf("transitions=%+v", report.Transitions)
	}
}

func TestAnalyzeLifecycleStaleGenerationIsUnknown(t *testing.T) {
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	observation := poolObservation(t, base, "True", "False", "False", 1, 1, 1, 0, 0, "rendered-a", "rendered-a")
	observation.Pool.Generation = 2
	observation.Pool.Status.ObservedGeneration = 1
	report, err := AnalyzeLifecycle([]Observation{observation})
	if err != nil {
		t.Fatal(err)
	}
	if report.States[0].Phase != PhaseUnknown || report.States[0].GenerationObserved {
		t.Fatalf("state=%+v", report.States[0])
	}
}
