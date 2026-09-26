package upgrade

import (
	"time"

	configv1 "github.com/openshift/api/config/v1"
)

type UpgradePhase string

const (
	UpgradePhaseStable    UpgradePhase = "STABLE"
	UpgradePhaseUpdating  UpgradePhase = "UPDATING"
	UpgradePhaseCompleted UpgradePhase = "COMPLETED"
	UpgradePhaseUnknown   UpgradePhase = "UNKNOWN"
)

type UpgradeState struct {
	ObservedAt     time.Time
	Phase          UpgradePhase
	DesiredVersion string
}

func AnalyzePhases(
	observations []ClusterVersionObservation,
) ([]UpgradeState, error) {
	if _, err := AnalyzeHistory(observations); err != nil {
		return nil, err
	}

	states := make([]UpgradeState, 0, len(observations))

	for i, observation := range observations {
		phase := classifyUpgradePhase(observation.ClusterVersion)

		if phase != UpgradePhaseUnknown {
			latest := observation.ClusterVersion.Status.History[0]
			if latest.StartedTime.Time.After(observation.ObservedAt) ||
				(latest.CompletionTime != nil && latest.CompletionTime.Time.After(observation.ObservedAt)) {
				phase = UpgradePhaseUnknown
			}
		}

		if phase == UpgradePhaseStable &&
			len(states) > 0 &&
			states[len(states)-1].Phase == UpgradePhaseUpdating {
			previous := observations[i-1].ClusterVersion.Status.Desired
			current := observation.ClusterVersion.Status.Desired
			if previous.Version == current.Version && previous.Image == current.Image {
				phase = UpgradePhaseCompleted
			} else {
				phase = UpgradePhaseUnknown
			}
		}

		states = append(states, UpgradeState{
			ObservedAt:     observation.ObservedAt,
			Phase:          phase,
			DesiredVersion: observation.ClusterVersion.Status.Desired.Version,
		})
	}

	return states, nil
}

func classifyUpgradePhase(
	version configv1.ClusterVersion,
) UpgradePhase {
	if version.Generation != version.Status.ObservedGeneration {
		return UpgradePhaseUnknown
	}
	progressing, hasProgressing := conditionStatus(
		version.Status.Conditions,
		configv1.OperatorProgressing,
	)

	if !hasProgressing || len(version.Status.History) == 0 {
		return UpgradePhaseUnknown
	}

	latest := version.Status.History[0]

	if latest.StartedTime.IsZero() ||
		(latest.CompletionTime != nil && (latest.CompletionTime.IsZero() || latest.CompletionTime.Before(&latest.StartedTime))) {
		return UpgradePhaseUnknown
	}
	for i := 1; i < len(version.Status.History); i++ {
		if version.Status.History[i].StartedTime.After(version.Status.History[i-1].StartedTime.Time) {
			return UpgradePhaseUnknown
		}
	}
	if !historyMatchesDesired(version.Status.Desired, latest) {
		return UpgradePhaseUnknown
	}

	switch progressing {
	case configv1.ConditionTrue:
		if latest.State == configv1.PartialUpdate &&
			latest.CompletionTime == nil {
			return UpgradePhaseUpdating
		}

	case configv1.ConditionFalse:
		available, hasAvailable := conditionStatus(
			version.Status.Conditions,
			configv1.OperatorAvailable,
		)

		if hasAvailable &&
			available == configv1.ConditionTrue &&
			latest.State == configv1.CompletedUpdate &&
			latest.CompletionTime != nil {
			return UpgradePhaseStable
		}
	}

	return UpgradePhaseUnknown
}

func conditionStatus(
	conditions []configv1.ClusterOperatorStatusCondition,
	conditionType configv1.ClusterStatusConditionType,
) (configv1.ConditionStatus, bool) {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return condition.Status, true
		}
	}

	return "", false
}

func historyMatchesDesired(
	desired configv1.Release,
	history configv1.UpdateHistory,
) bool {
	compared := false

	if desired.Version != "" && history.Version != "" {
		compared = true

		if desired.Version != history.Version {
			return false
		}
	}

	if desired.Image != "" && history.Image != "" {
		compared = true

		if desired.Image != history.Image {
			return false
		}
	}

	return compared
}
