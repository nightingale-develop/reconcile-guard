package contracts

import (
	"fmt"
	"sort"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
)

type CorrelationKind string

const (
	CorrelationExact     CorrelationKind = "EXACT"
	CorrelationBracketed CorrelationKind = "BRACKETED"
	CorrelationAmbiguous CorrelationKind = "AMBIGUOUS"
	CorrelationOutside   CorrelationKind = "OUTSIDE"
)

type CorrelatedObservation struct {
	Observation operator.Observation
	Phase       upgrade.UpgradePhase
	Kind        CorrelationKind
	FromTime    time.Time
	ToTime      time.Time
}

func Correlate(
	states []upgrade.UpgradeState,
	observations []operator.Observation,
) ([]CorrelatedObservation, error) {
	for i, state := range states {
		if state.ObservedAt.IsZero() || (i > 0 && !state.ObservedAt.After(states[i-1].ObservedAt)) {
			return nil, fmt.Errorf("state %d: timestamps must be nonzero and strictly increasing", i+1)
		}
		switch state.Phase {
		case upgrade.UpgradePhaseStable, upgrade.UpgradePhaseUpdating, upgrade.UpgradePhaseCompleted, upgrade.UpgradePhaseUnknown:
		default:
			return nil, fmt.Errorf("state %d: invalid phase %q", i+1, state.Phase)
		}
	}
	for i, observation := range observations {
		if observation.ObservedAt.IsZero() || (i > 0 && !observation.ObservedAt.After(observations[i-1].ObservedAt)) {
			return nil, fmt.Errorf("observation %d: timestamps must be nonzero and strictly increasing", i+1)
		}
	}
	result := make(
		[]CorrelatedObservation,
		0,
		len(observations),
	)

	for _, observation := range observations {
		result = append(
			result,
			correlateOperatorObservation(
				states,
				observation,
			),
		)
	}

	return result, nil
}

func correlateOperatorObservation(
	states []upgrade.UpgradeState,
	observation operator.Observation,
) CorrelatedObservation {
	result := CorrelatedObservation{
		Observation: observation,
		Phase:       upgrade.UpgradePhaseUnknown,
		Kind:        CorrelationOutside,
	}

	if len(states) == 0 {
		return result
	}

	index := sort.Search(
		len(states),
		func(i int) bool {
			return !states[i].ObservedAt.Before(
				observation.ObservedAt,
			)
		},
	)

	if index < len(states) &&
		states[index].ObservedAt.Equal(
			observation.ObservedAt,
		) {
		result.Phase = states[index].Phase
		result.Kind = CorrelationExact
		result.FromTime = states[index].ObservedAt
		result.ToTime = states[index].ObservedAt

		return result
	}

	if index == 0 || index == len(states) {
		return result
	}

	before := states[index-1]
	after := states[index]

	result.FromTime = before.ObservedAt
	result.ToTime = after.ObservedAt

	if before.Phase == upgrade.UpgradePhaseUnknown ||
		after.Phase == upgrade.UpgradePhaseUnknown ||
		before.Phase != after.Phase {
		result.Kind = CorrelationAmbiguous
		return result
	}

	result.Phase = before.Phase
	result.Kind = CorrelationBracketed

	return result
}
