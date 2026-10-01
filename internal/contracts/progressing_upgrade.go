package contracts

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
)

type ProgressingPolicy struct {
	Limit             time.Duration
	MaxObservationGap time.Duration
	Operator          string
	TargetVersion     string
	Source            string
}

type ProgressingUpgradeEvidence struct {
	Episode ProgressingEpisode
	Start   CorrelatedObservation
	End     CorrelatedObservation
}

type ProgressingUpgradeReport struct {
	Operator          string
	Policy            ProgressingPolicy
	Verdict           ContractVerdict
	EvaluatedSamples  int
	UncertainSamples  int
	MissingConditions int
	Discontinuities   int
	PolicyApplicable  bool
	Evidence          []ProgressingUpgradeEvidence
}

func VerifyUpgradeProgressing(versions []upgrade.ClusterVersionObservation, observations []operator.Observation, policy ProgressingPolicy) (ProgressingUpgradeReport, error) {
	if policy.Limit <= 0 || policy.MaxObservationGap <= 0 {
		return ProgressingUpgradeReport{}, fmt.Errorf("policy durations must be positive")
	}
	states, err := upgrade.AnalyzePhases(versions)
	if err != nil {
		return ProgressingUpgradeReport{}, err
	}
	history, err := operator.AnalyzeHistory(observations)
	if err != nil {
		return ProgressingUpgradeReport{}, err
	}
	if observations[len(observations)-1].ObservedAt.After(observations[0].ObservedAt.Add(time.Duration(1<<63 - 1))) {
		return ProgressingUpgradeReport{}, fmt.Errorf("history duration exceeds supported range")
	}
	samples, err := Correlate(states, observations)
	if err != nil {
		return ProgressingUpgradeReport{}, err
	}
	report := ProgressingUpgradeReport{Operator: history.Operator, Policy: policy, Verdict: ContractInconclusive,
		PolicyApplicable: policy.Operator == history.Operator && strings.TrimSpace(policy.Source) != "" && strings.TrimSpace(policy.TargetVersion) != ""}

	blocks := make([]int, len(states))
	block := 0
	for i, state := range states {
		if state.Phase != upgrade.UpgradePhaseUpdating {
			continue
		}
		if i == 0 || blocks[i-1] == 0 || state.ObservedAt.After(states[i-1].ObservedAt.Add(policy.MaxObservationGap)) ||
			versions[i].ClusterVersion.Status.Desired.Version != versions[i-1].ClusterVersion.Status.Desired.Version ||
			versions[i].ClusterVersion.Status.Desired.Image != versions[i-1].ClusterVersion.Status.Desired.Image {
			block++
		}
		blocks[i] = block
	}
	var segment []operator.Observation
	var segmentSamples []CorrelatedObservation
	var previousBlock int
	inconclusive, failed := false, false
	flush := func() error {
		if len(segment) == 0 {
			return nil
		}
		result, err := VerifyProgressing(segment, policy.Limit)
		if err != nil {
			return err
		}
		report.MissingConditions += result.MissingConditions
		if result.Verdict == ContractInconclusive {
			inconclusive = true
		}
		if result.Verdict == ContractFail {
			failed = true
		}
		for _, episode := range result.Episodes {
			finding := ProgressingUpgradeEvidence{Episode: episode}
			for _, sample := range segmentSamples {
				if sample.Observation.ObservedAt.Equal(episode.FirstTrue) {
					finding.Start = sample
				}
				if sample.Observation.ObservedAt.Equal(episode.LastTrue) {
					finding.End = sample
				}
			}
			if !report.PolicyApplicable {
				finding.Episode.Verdict = ContractInconclusive
			}
			report.Evidence = append(report.Evidence, finding)
		}
		segment = nil
		segmentSamples = nil
		return nil
	}
	for _, sample := range samples {
		index := sort.Search(len(states), func(i int) bool { return !states[i].ObservedAt.Before(sample.Observation.ObservedAt) })
		uncertain := sample.Kind == CorrelationAmbiguous || sample.Kind == CorrelationOutside || sample.Phase == upgrade.UpgradePhaseUnknown
		eligible := !uncertain && sample.Phase == upgrade.UpgradePhaseUpdating
		currentBlock := 0
		if eligible {
			currentBlock = blocks[index]
			if sample.Kind == CorrelationBracketed && (blocks[index-1] != currentBlock || currentBlock == 0) {
				uncertain = true
			}
			if states[index].DesiredVersion != policy.TargetVersion {
				uncertain = true
			}
		}
		if !eligible || uncertain {
			if err := flush(); err != nil {
				return ProgressingUpgradeReport{}, err
			}
			previousBlock = 0
			if uncertain {
				report.UncertainSamples++
				inconclusive = true
			}
			continue
		}
		if len(segment) > 0 && (currentBlock != previousBlock || sample.Observation.ObservedAt.After(segment[len(segment)-1].ObservedAt.Add(policy.MaxObservationGap))) {
			if err := flush(); err != nil {
				return ProgressingUpgradeReport{}, err
			}
			report.Discontinuities++
			inconclusive = true
		}
		report.EvaluatedSamples++
		segment = append(segment, sample.Observation)
		segmentSamples = append(segmentSamples, sample)
		previousBlock = currentBlock
	}
	if err := flush(); err != nil {
		return ProgressingUpgradeReport{}, err
	}
	switch {
	case !report.PolicyApplicable:
	case failed:
		report.Verdict = ContractFail
	case report.EvaluatedSamples > 0 && !inconclusive:
		report.Verdict = ContractPass
	}
	return report, nil
}
