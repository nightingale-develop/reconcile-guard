package node

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

type State struct {
	ObservedAt           time.Time
	Ready                corev1.ConditionStatus
	CurrentMachineConfig string
	DesiredMachineConfig string
	KubeletVersion       string
	ConfigAligned        bool
}

type Transition struct {
	FromTime           time.Time
	ToTime             time.Time
	ReadyFrom          corev1.ConditionStatus
	ReadyTo            corev1.ConditionStatus
	CurrentConfigFrom  string
	CurrentConfigTo    string
	DesiredConfigFrom  string
	DesiredConfigTo    string
	KubeletVersionFrom string
	KubeletVersionTo   string
}

type LifecycleReport struct {
	Node         string
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
		Node:         history.Node,
		Observations: history.Observations,
		States:       make([]State, 0, len(observations)),
	}

	for i, observation := range observations {
		current := CurrentMachineConfig(observation.Node)
		desired := DesiredMachineConfig(observation.Node)
		state := State{
			ObservedAt:           observation.ObservedAt,
			Ready:                ReadyStatus(observation.Node),
			CurrentMachineConfig: current,
			DesiredMachineConfig: desired,
			KubeletVersion:       observation.Node.Status.NodeInfo.KubeletVersion,
			ConfigAligned:        current != "" && desired != "" && current == desired,
		}
		report.States = append(report.States, state)

		if i == 0 {
			continue
		}
		previous := report.States[i-1]
		if previous.Ready == state.Ready && previous.CurrentMachineConfig == state.CurrentMachineConfig && previous.DesiredMachineConfig == state.DesiredMachineConfig && previous.KubeletVersion == state.KubeletVersion {
			continue
		}
		report.Transitions = append(report.Transitions, Transition{
			FromTime:           previous.ObservedAt,
			ToTime:             state.ObservedAt,
			ReadyFrom:          previous.Ready,
			ReadyTo:            state.Ready,
			CurrentConfigFrom:  previous.CurrentMachineConfig,
			CurrentConfigTo:    state.CurrentMachineConfig,
			DesiredConfigFrom:  previous.DesiredMachineConfig,
			DesiredConfigTo:    state.DesiredMachineConfig,
			KubeletVersionFrom: previous.KubeletVersion,
			KubeletVersionTo:   state.KubeletVersion,
		})
	}

	return report, nil
}
