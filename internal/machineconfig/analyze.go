package machineconfig

import (
	"time"

	machineconfigv1 "github.com/openshift/api/machineconfiguration/v1"
	corev1 "k8s.io/api/core/v1"
)

type Phase string

const (
	PhaseStable   Phase = "STABLE"
	PhaseUpdating Phase = "UPDATING"
	PhaseDegraded Phase = "DEGRADED"
	PhaseUnknown  Phase = "UNKNOWN"
)

type State struct {
	ObservedAt              time.Time
	Phase                   Phase
	Generation              int64
	ObservedGeneration      int64
	GenerationObserved      bool
	CurrentConfiguration    string
	DesiredConfiguration    string
	MachineCount            int32
	UpdatedMachineCount     int32
	ReadyMachineCount       int32
	UnavailableMachineCount int32
	DegradedMachineCount    int32
}

type Transition struct {
	From       Phase
	To         Phase
	FromTime   time.Time
	ToTime     time.Time
	FromConfig string
	ToConfig   string
}

type LifecycleReport struct {
	Pool         string
	Observations int
	States       []State
	Transitions  []Transition
}

func AnalyzeLifecycle(observations []Observation) (LifecycleReport, error) {
	history, err := AnalyzeHistory(observations)
	if err != nil {
		return LifecycleReport{}, err
	}

	report := LifecycleReport{
		Pool:         history.Pool,
		Observations: history.Observations,
		States:       make([]State, 0, len(observations)),
	}

	for i, observation := range observations {
		state := classify(observation)
		report.States = append(report.States, state)

		if i == 0 {
			continue
		}

		previous := report.States[i-1]
		if previous.Phase == state.Phase && previous.CurrentConfiguration == state.CurrentConfiguration && previous.DesiredConfiguration == state.DesiredConfiguration {
			continue
		}

		report.Transitions = append(report.Transitions, Transition{
			From:       previous.Phase,
			To:         state.Phase,
			FromTime:   previous.ObservedAt,
			ToTime:     state.ObservedAt,
			FromConfig: previous.CurrentConfiguration,
			ToConfig:   state.CurrentConfiguration,
		})
	}

	return report, nil
}

func classify(observation Observation) State {
	pool := observation.Pool
	state := State{
		ObservedAt:              observation.ObservedAt,
		CurrentConfiguration:    pool.Status.Configuration.Name,
		DesiredConfiguration:    pool.Spec.Configuration.Name,
		MachineCount:            pool.Status.MachineCount,
		UpdatedMachineCount:     pool.Status.UpdatedMachineCount,
		ReadyMachineCount:       pool.Status.ReadyMachineCount,
		UnavailableMachineCount: pool.Status.UnavailableMachineCount,
		DegradedMachineCount:    pool.Status.DegradedMachineCount,
		Phase:                   PhaseUnknown,
		Generation:              pool.Generation,
		ObservedGeneration:      pool.Status.ObservedGeneration,
		GenerationObserved:      pool.Generation == pool.Status.ObservedGeneration,
	}

	if pool.Generation != pool.Status.ObservedGeneration {
		return state
	}

	updated, hasUpdated := poolCondition(pool.Status.Conditions, machineconfigv1.MachineConfigPoolUpdated)
	updating, hasUpdating := poolCondition(pool.Status.Conditions, machineconfigv1.MachineConfigPoolUpdating)
	degraded, hasDegraded := poolCondition(pool.Status.Conditions, machineconfigv1.MachineConfigPoolDegraded)

	if (hasDegraded && degraded == corev1.ConditionTrue) || pool.Status.DegradedMachineCount > 0 {
		state.Phase = PhaseDegraded
		return state
	}

	if hasUpdated && hasUpdating && updated == corev1.ConditionTrue && updating == corev1.ConditionTrue {
		return state
	}

	if hasUpdating && updating == corev1.ConditionTrue {
		state.Phase = PhaseUpdating
		return state
	}

	if !hasUpdated || !hasUpdating || !hasDegraded || updated == corev1.ConditionUnknown || updating == corev1.ConditionUnknown || degraded == corev1.ConditionUnknown {
		return state
	}

	if updated == corev1.ConditionTrue && updating == corev1.ConditionFalse && degraded == corev1.ConditionFalse && state.CurrentConfiguration != "" && state.CurrentConfiguration == state.DesiredConfiguration && pool.Status.UpdatedMachineCount == pool.Status.MachineCount && pool.Status.ReadyMachineCount == pool.Status.MachineCount && pool.Status.UnavailableMachineCount == 0 && pool.Status.DegradedMachineCount == 0 {
		state.Phase = PhaseStable
	}

	return state
}

func poolCondition(conditions []machineconfigv1.MachineConfigPoolCondition, conditionType machineconfigv1.MachineConfigPoolConditionType) (corev1.ConditionStatus, bool) {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return condition.Status, true
		}
	}
	return corev1.ConditionUnknown, false
}
