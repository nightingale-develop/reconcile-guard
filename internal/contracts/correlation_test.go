package contracts

import (
	"reflect"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"

	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
)

func correlationTime(value string) time.Time {
	result, err := time.Parse(
		time.RFC3339,
		"2026-09-26T"+value+":00Z",
	)
	if err != nil {
		panic(err)
	}

	return result
}

func correlationObservation(
	observedAt time.Time,
) operator.Observation {
	return operator.Observation{
		ObservedAt: observedAt,
		Operator: testOperator(
			configv1.ClusterOperatorStatusCondition{
				Type:   configv1.OperatorAvailable,
				Status: configv1.ConditionTrue,
			},
			configv1.ClusterOperatorStatusCondition{
				Type:   configv1.OperatorProgressing,
				Status: configv1.ConditionTrue,
			},
			configv1.ClusterOperatorStatusCondition{
				Type:   configv1.OperatorDegraded,
				Status: configv1.ConditionFalse,
			},
		),
	}
}

func TestCorrelationExact(t *testing.T) {
	at := correlationTime("10:05")

	states := []upgrade.UpgradeState{
		{
			ObservedAt: at,
			Phase:      upgrade.UpgradePhaseUpdating,
		},
	}

	got := correlateOperatorObservation(
		states,
		correlationObservation(at),
	)

	if got.Kind != CorrelationExact {
		t.Fatalf(
			"kind = %s, want %s",
			got.Kind,
			CorrelationExact,
		)
	}

	if got.Phase != upgrade.UpgradePhaseUpdating {
		t.Fatalf(
			"phase = %s, want %s",
			got.Phase,
			upgrade.UpgradePhaseUpdating,
		)
	}
}

func TestCorrelationBracketed(t *testing.T) {
	states := []upgrade.UpgradeState{
		{
			ObservedAt: correlationTime("10:05"),
			Phase:      upgrade.UpgradePhaseUpdating,
		},
		{
			ObservedAt: correlationTime("10:10"),
			Phase:      upgrade.UpgradePhaseUpdating,
		},
	}

	got := correlateOperatorObservation(
		states,
		correlationObservation(
			correlationTime("10:07"),
		),
	)

	if got.Kind != CorrelationBracketed {
		t.Fatalf(
			"kind = %s, want %s",
			got.Kind,
			CorrelationBracketed,
		)
	}

	if got.Phase != upgrade.UpgradePhaseUpdating {
		t.Fatalf(
			"phase = %s, want %s",
			got.Phase,
			upgrade.UpgradePhaseUpdating,
		)
	}
}

func TestCorrelationAmbiguous(t *testing.T) {
	states := []upgrade.UpgradeState{
		{
			ObservedAt: correlationTime("10:05"),
			Phase:      upgrade.UpgradePhaseUpdating,
		},
		{
			ObservedAt: correlationTime("10:10"),
			Phase:      upgrade.UpgradePhaseCompleted,
		},
	}

	got := correlateOperatorObservation(
		states,
		correlationObservation(
			correlationTime("10:07"),
		),
	)

	if got.Kind != CorrelationAmbiguous {
		t.Fatalf(
			"kind = %s, want %s",
			got.Kind,
			CorrelationAmbiguous,
		)
	}

	if got.Phase != upgrade.UpgradePhaseUnknown {
		t.Fatalf(
			"phase = %s, want %s",
			got.Phase,
			upgrade.UpgradePhaseUnknown,
		)
	}
}

func TestCorrelationOutsideRange(t *testing.T) {
	states := []upgrade.UpgradeState{
		{
			ObservedAt: correlationTime("10:05"),
			Phase:      upgrade.UpgradePhaseUpdating,
		},
		{
			ObservedAt: correlationTime("10:10"),
			Phase:      upgrade.UpgradePhaseUpdating,
		},
	}

	tests := []time.Time{
		correlationTime("10:00"),
		correlationTime("10:15"),
	}

	for _, observedAt := range tests {
		got := correlateOperatorObservation(
			states,
			correlationObservation(observedAt),
		)

		if got.Kind != CorrelationOutside {
			t.Fatalf(
				"kind = %s, want %s",
				got.Kind,
				CorrelationOutside,
			)
		}
	}
}

func TestCorrelationUnknownPhaseIsAmbiguous(t *testing.T) {
	states := []upgrade.UpgradeState{
		{
			ObservedAt: correlationTime("10:05"),
			Phase:      upgrade.UpgradePhaseUnknown,
		},
		{
			ObservedAt: correlationTime("10:10"),
			Phase:      upgrade.UpgradePhaseUnknown,
		},
	}

	got := correlateOperatorObservation(
		states,
		correlationObservation(
			correlationTime("10:07"),
		),
	)

	if got.Kind != CorrelationAmbiguous {
		t.Fatalf(
			"kind = %s, want %s",
			got.Kind,
			CorrelationAmbiguous,
		)
	}
}

func TestCorrelationValidationAndBoundaries(t *testing.T) {
	first, last := correlationTime("10:05"), correlationTime("10:10")
	states := []upgrade.UpgradeState{{ObservedAt: first, Phase: upgrade.UpgradePhaseUpdating}, {ObservedAt: last, Phase: upgrade.UpgradePhaseUpdating}}
	observations := []operator.Observation{correlationObservation(first), correlationObservation(first.Add(time.Minute)), correlationObservation(first.Add(2 * time.Minute)), correlationObservation(last)}
	before := append([]upgrade.UpgradeState(nil), states...)
	got, err := Correlate(states, observations)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []CorrelationKind{CorrelationExact, CorrelationBracketed, CorrelationBracketed, CorrelationExact} {
		if got[i].Kind != want {
			t.Fatalf("sample %d: %+v", i, got[i])
		}
	}
	if got[1].FromTime != first || got[1].ToTime != last || !reflect.DeepEqual(states, before) {
		t.Fatalf("bounds or mutation: %+v", got)
	}
	got, err = Correlate(nil, observations[:1])
	if err != nil || got[0].Kind != CorrelationOutside {
		t.Fatalf("empty: %+v %v", got, err)
	}
	got, err = Correlate([]upgrade.UpgradeState{{ObservedAt: first, Phase: upgrade.UpgradePhaseUnknown}}, observations[:1])
	if err != nil || got[0].Kind != CorrelationExact || got[0].Phase != upgrade.UpgradePhaseUnknown {
		t.Fatalf("exact unknown: %+v %v", got, err)
	}
	for _, invalid := range [][]upgrade.UpgradeState{
		{states[1], states[0]}, {states[0], states[0]}, {{Phase: upgrade.UpgradePhaseUpdating}}, {{ObservedAt: first, Phase: "invalid"}},
	} {
		if _, err := Correlate(invalid, observations); err == nil {
			t.Fatalf("accepted invalid states: %+v", invalid)
		}
	}
	for _, invalid := range [][]operator.Observation{{observations[1], observations[0]}, {observations[0], observations[0]}, {{}}} {
		if _, err := Correlate(states, invalid); err == nil {
			t.Fatalf("accepted invalid observations: %+v", invalid)
		}
	}
}
