package main

import (
	"fmt"
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

func analyzeUpgradePhases(
	observations []ClusterVersionObservation,
) ([]UpgradeState, error) {
	if _, err := analyzeClusterVersionHistory(observations); err != nil {
		return nil, err
	}

	states := make([]UpgradeState, 0, len(observations))

	for _, observation := range observations {
		phase := classifyUpgradePhase(observation.ClusterVersion)

		if phase == UpgradePhaseStable &&
			len(states) > 0 &&
			states[len(states)-1].Phase == UpgradePhaseUpdating {
			phase = UpgradePhaseCompleted
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
	progressing, hasProgressing := conditionStatus(
		version.Status.Conditions,
		configv1.OperatorProgressing,
	)

	if !hasProgressing || len(version.Status.History) == 0 {
		return UpgradePhaseUnknown
	}

	latest := version.Status.History[0]

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

func printUpgradeTimeline(states []UpgradeState) {
	fmt.Println("Upgrade phases:")

	for _, state := range states {
		fmt.Printf(
			"  %s  %-9s desired=%q\n",
			state.ObservedAt.Format(time.RFC3339Nano),
			state.Phase,
			state.DesiredVersion,
		)
	}

	fmt.Println(
		"Verdict: NOT EVALUATED (phase reconstruction only)",
	)
}
