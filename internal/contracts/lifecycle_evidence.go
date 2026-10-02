package contracts

import (
	"fmt"
	"sort"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/machineconfig"
	nodehistory "github.com/nightingale-develop/reconcile-guard/internal/node"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
	corev1 "k8s.io/api/core/v1"
)

const (
	MachineConfigPoolLifecycleContract = "machine-config-pool-lifecycle-evidence"
	NodeLifecycleContract              = "node-lifecycle-evidence"
)

type TimelineCorrelation struct {
	Phase          upgrade.UpgradePhase
	Kind           CorrelationKind
	FromTime       time.Time
	ToTime         time.Time
	DesiredVersion string
	DesiredImage   string
}

type MachineConfigPoolFinding struct {
	State       machineconfig.State
	Correlation TimelineCorrelation
	Applicable  bool
}

type MachineConfigPoolEvidenceReport struct {
	Contract         string
	Pool             string
	Verdict          ContractVerdict
	Observations     int
	EvaluatedSamples int
	UncertainSamples int
	ConvergedSamples int
	Evidence         []MachineConfigPoolFinding
}

type NodeFinding struct {
	State       nodehistory.State
	Correlation TimelineCorrelation
	Applicable  bool
}

type NodeEvidenceReport struct {
	Contract         string
	Node             string
	Verdict          ContractVerdict
	Observations     int
	EvaluatedSamples int
	UncertainSamples int
	ConvergedSamples int
	Evidence         []NodeFinding
}

func VerifyMachineConfigPoolLifecycleEvidence(versions []upgrade.ClusterVersionObservation, observations []machineconfig.Observation) (MachineConfigPoolEvidenceReport, error) {
	states, err := upgrade.AnalyzePhases(versions)
	if err != nil {
		return MachineConfigPoolEvidenceReport{}, err
	}
	lifecycle, err := machineconfig.AnalyzeLifecycle(observations)
	if err != nil {
		return MachineConfigPoolEvidenceReport{}, err
	}

	report := MachineConfigPoolEvidenceReport{
		Contract:     MachineConfigPoolLifecycleContract,
		Pool:         lifecycle.Pool,
		Verdict:      ContractInconclusive,
		Observations: lifecycle.Observations,
	}
	windows := completedLifecycleWindows(states)

	for _, state := range lifecycle.States {
		correlation, err := CorrelateTime(states, state.ObservedAt)
		if err != nil {
			return MachineConfigPoolEvidenceReport{}, err
		}
		finding := MachineConfigPoolFinding{State: state, Correlation: correlation}
		if correlation.Kind != CorrelationExact && correlation.Kind != CorrelationBracketed {
			report.UncertainSamples++
			report.Evidence = append(report.Evidence, finding)
			continue
		}
		if _, ok := lifecycleWindowAt(windows, state.ObservedAt, correlation.DesiredVersion, correlation.DesiredImage); !ok {
			report.Evidence = append(report.Evidence, finding)
			continue
		}
		finding.Applicable = true
		report.EvaluatedSamples++
		if state.Phase == machineconfig.PhaseStable {
			report.ConvergedSamples++
		}
		report.Evidence = append(report.Evidence, finding)
	}

	if report.ConvergedSamples > 0 {
		report.Verdict = ContractPass
	}
	return report, nil
}

func VerifyNodeLifecycleEvidence(versions []upgrade.ClusterVersionObservation, observations []nodehistory.Observation) (NodeEvidenceReport, error) {
	states, err := upgrade.AnalyzePhases(versions)
	if err != nil {
		return NodeEvidenceReport{}, err
	}
	lifecycle, err := nodehistory.AnalyzeLifecycle(observations)
	if err != nil {
		return NodeEvidenceReport{}, err
	}

	report := NodeEvidenceReport{
		Contract:     NodeLifecycleContract,
		Node:         lifecycle.Node,
		Verdict:      ContractInconclusive,
		Observations: lifecycle.Observations,
	}
	windows := completedLifecycleWindows(states)

	for _, state := range lifecycle.States {
		correlation, err := CorrelateTime(states, state.ObservedAt)
		if err != nil {
			return NodeEvidenceReport{}, err
		}
		finding := NodeFinding{State: state, Correlation: correlation}
		if correlation.Kind != CorrelationExact && correlation.Kind != CorrelationBracketed {
			report.UncertainSamples++
			report.Evidence = append(report.Evidence, finding)
			continue
		}
		if _, ok := lifecycleWindowAt(windows, state.ObservedAt, correlation.DesiredVersion, correlation.DesiredImage); !ok {
			report.Evidence = append(report.Evidence, finding)
			continue
		}
		finding.Applicable = true
		report.EvaluatedSamples++
		if state.Ready == corev1.ConditionTrue && state.ConfigAligned {
			report.ConvergedSamples++
		}
		report.Evidence = append(report.Evidence, finding)
	}

	if report.ConvergedSamples > 0 {
		report.Verdict = ContractPass
	}
	return report, nil
}

func CorrelateTime(states []upgrade.UpgradeState, observedAt time.Time) (TimelineCorrelation, error) {
	for i, state := range states {
		if state.ObservedAt.IsZero() || (i > 0 && !state.ObservedAt.After(states[i-1].ObservedAt)) {
			return TimelineCorrelation{}, fmt.Errorf("state %d: timestamps must be nonzero and strictly increasing", i+1)
		}
	}
	if observedAt.IsZero() {
		return TimelineCorrelation{}, fmt.Errorf("observation timestamp is missing")
	}

	result := TimelineCorrelation{Phase: upgrade.UpgradePhaseUnknown, Kind: CorrelationOutside}
	if len(states) == 0 {
		return result, nil
	}
	index := sort.Search(len(states), func(i int) bool { return !states[i].ObservedAt.Before(observedAt) })
	if index < len(states) && states[index].ObservedAt.Equal(observedAt) {
		state := states[index]
		result.Phase = state.Phase
		result.Kind = CorrelationExact
		result.FromTime = state.ObservedAt
		result.ToTime = state.ObservedAt
		result.DesiredVersion = state.DesiredVersion
		result.DesiredImage = state.DesiredImage
		return result, nil
	}
	if index == 0 || index == len(states) {
		return result, nil
	}
	before, after := states[index-1], states[index]
	result.FromTime, result.ToTime = before.ObservedAt, after.ObservedAt
	if before.Phase == upgrade.UpgradePhaseUnknown || after.Phase == upgrade.UpgradePhaseUnknown || before.Phase != after.Phase || before.DesiredVersion != after.DesiredVersion || before.DesiredImage != after.DesiredImage {
		result.Kind = CorrelationAmbiguous
		return result, nil
	}
	result.Phase = before.Phase
	result.Kind = CorrelationBracketed
	result.DesiredVersion = before.DesiredVersion
	result.DesiredImage = before.DesiredImage
	return result, nil
}

type lifecycleCompletionWindow struct {
	From, To       time.Time
	Version, Image string
}

func completedLifecycleWindows(states []upgrade.UpgradeState) []lifecycleCompletionWindow {
	var windows []lifecycleCompletionWindow
	for i, state := range states {
		if state.Phase != upgrade.UpgradePhaseCompleted {
			continue
		}
		window := lifecycleCompletionWindow{From: state.ObservedAt, To: state.ObservedAt, Version: state.DesiredVersion, Image: state.DesiredImage}
		for _, next := range states[i+1:] {
			if next.Phase != upgrade.UpgradePhaseStable || next.DesiredVersion != window.Version || next.DesiredImage != window.Image {
				break
			}
			window.To = next.ObservedAt
		}
		windows = append(windows, window)
	}
	return windows
}

func lifecycleWindowAt(windows []lifecycleCompletionWindow, at time.Time, version, image string) (lifecycleCompletionWindow, bool) {
	for _, window := range windows {
		if window.Version == version && window.Image == image && !at.Before(window.From) && !at.After(window.To) {
			return window, true
		}
	}
	return lifecycleCompletionWindow{}, false
}
