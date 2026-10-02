package contracts

import (
	"fmt"
	"sort"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/machineconfig"
	nodehistory "github.com/nightingale-develop/reconcile-guard/internal/node"
	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
)

const (
	OperatorAvailabilityLossPolicyContract = "operator-availability-loss-policy"
	OperatorDegradedPolicyContract         = "operator-degraded-policy"
	OperatorProgressingPolicyContract      = "operator-progressing-policy"
	MachineConfigPoolPolicyContract        = "machine-config-pool-post-completion-policy"
	NodeReadyPolicyContract                = "node-ready-post-completion-policy"
	NodeConfigPolicyContract               = "node-config-alignment-post-completion-policy"
)

type OperatorConditionPolicy struct {
	Contract          string
	Condition         configv1.ClusterStatusConditionType
	AdverseStatus     configv1.ConditionStatus
	Limit             time.Duration
	MaxObservationGap time.Duration
	Operator          string
	TargetVersion     string
	TargetImage       string
	Source            string
}

type ConditionPolicyEpisode struct {
	FirstAdverse time.Time
	LastAdverse  time.Time
	BeforeGood   time.Time
	AfterGood    time.Time
	ObservedSpan time.Duration
	MaximumSpan  time.Duration
	Verdict      ContractVerdict
}

type OperatorConditionPolicyReport struct {
	Contract          string
	Operator          string
	Condition         configv1.ClusterStatusConditionType
	AdverseStatus     configv1.ConditionStatus
	Limit             time.Duration
	MaxObservationGap time.Duration
	TargetVersion     string
	TargetImage       string
	Source            string
	Verdict           ContractVerdict
	PolicyApplicable  bool
	EvaluatedSamples  int
	UncertainSamples  int
	MissingConditions int
	Discontinuities   int
	Episodes          []ConditionPolicyEpisode
}

func VerifyOperatorConditionPolicy(versions []upgrade.ClusterVersionObservation, observations []operator.Observation, policy OperatorConditionPolicy) (OperatorConditionPolicyReport, error) {
	if policy.Contract == "" || policy.Condition == "" {
		return OperatorConditionPolicyReport{}, fmt.Errorf("operator lifecycle policy contract and condition are required")
	}
	if policy.Limit <= 0 || policy.MaxObservationGap <= 0 {
		return OperatorConditionPolicyReport{}, fmt.Errorf("operator lifecycle policy durations must be positive")
	}
	states, err := upgrade.AnalyzePhases(versions)
	if err != nil {
		return OperatorConditionPolicyReport{}, err
	}
	history, err := operator.AnalyzeHistory(observations)
	if err != nil {
		return OperatorConditionPolicyReport{}, err
	}

	report := OperatorConditionPolicyReport{
		Contract:          policy.Contract,
		Operator:          history.Operator,
		Condition:         policy.Condition,
		AdverseStatus:     policy.AdverseStatus,
		Limit:             policy.Limit,
		MaxObservationGap: policy.MaxObservationGap,
		TargetVersion:     policy.TargetVersion,
		TargetImage:       policy.TargetImage,
		Source:            policy.Source,
		Verdict:           ContractInconclusive,
		PolicyApplicable:  policy.Operator == history.Operator && policy.TargetVersion != "" && policy.Source != "",
	}
	if !report.PolicyApplicable {
		return report, nil
	}

	var episode *ConditionPolicyEpisode
	var previousGood time.Time
	var previousEligible time.Time
	var previousCorrelation *TimelineCorrelation
	inconclusive := false
	failed := false

	finish := func(afterGood time.Time) {
		if episode == nil {
			return
		}
		episode.AfterGood = afterGood
		episode.ObservedSpan = episode.LastAdverse.Sub(episode.FirstAdverse)
		episode.Verdict = ContractInconclusive
		if !episode.BeforeGood.IsZero() && !afterGood.IsZero() {
			episode.MaximumSpan = afterGood.Sub(episode.BeforeGood)
			if episode.MaximumSpan <= policy.Limit {
				episode.Verdict = ContractPass
			}
		}
		if episode.ObservedSpan > policy.Limit {
			episode.Verdict = ContractFail
		}
		if episode.Verdict == ContractFail {
			failed = true
		} else if episode.Verdict == ContractInconclusive {
			inconclusive = true
		}
		report.Episodes = append(report.Episodes, *episode)
		episode = nil
	}

	for _, observation := range observations {
		correlation, err := CorrelateTime(states, observation.ObservedAt)
		if err != nil {
			return OperatorConditionPolicyReport{}, err
		}
		eligible := (correlation.Kind == CorrelationExact || correlation.Kind == CorrelationBracketed) &&
			correlation.Phase == upgrade.UpgradePhaseUpdating &&
			correlation.DesiredVersion == policy.TargetVersion &&
			(policy.TargetImage == "" || correlation.DesiredImage == policy.TargetImage)
		if !eligible {
			finish(time.Time{})
			previousGood = time.Time{}
			previousEligible = time.Time{}
			previousCorrelation = nil
			if correlation.Kind == CorrelationAmbiguous || (correlation.Phase == upgrade.UpgradePhaseUnknown && correlation.Kind != CorrelationOutside) {
				report.UncertainSamples++
				inconclusive = true
			}
			continue
		}
		if correlation.Kind == CorrelationBracketed && correlation.ToTime.Sub(correlation.FromTime) > policy.MaxObservationGap {
			finish(time.Time{})
			previousGood = time.Time{}
			previousEligible = time.Time{}
			previousCorrelation = nil
			report.UncertainSamples++
			inconclusive = true
			continue
		}
		if !previousEligible.IsZero() && observation.ObservedAt.Sub(previousEligible) > policy.MaxObservationGap {
			finish(time.Time{})
			previousGood = time.Time{}
			report.Discontinuities++
			inconclusive = true
			previousCorrelation = nil
		}
		if previousCorrelation != nil && !policyTimelineContinuous(states, previousCorrelation, &correlation, policy.MaxObservationGap, policy.TargetVersion, policy.TargetImage) {
			finish(time.Time{})
			previousGood = time.Time{}
			report.Discontinuities++
			inconclusive = true
			previousCorrelation = nil
		}
		previousEligible = observation.ObservedAt
		currentCorrelation := correlation
		previousCorrelation = &currentCorrelation
		report.EvaluatedSamples++

		condition, found := findOperatorCondition(observation.Operator.Status.Conditions, policy.Condition)
		if !found || condition.Status == configv1.ConditionUnknown {
			finish(time.Time{})
			previousGood = time.Time{}
			report.MissingConditions++
			inconclusive = true
			continue
		}
		if condition.Status != policy.AdverseStatus {
			finish(observation.ObservedAt)
			previousGood = observation.ObservedAt
			continue
		}
		if episode == nil {
			episode = &ConditionPolicyEpisode{FirstAdverse: observation.ObservedAt, BeforeGood: previousGood}
		}
		episode.LastAdverse = observation.ObservedAt
		previousGood = time.Time{}
	}
	finish(time.Time{})

	switch {
	case failed:
		report.Verdict = ContractFail
	case report.EvaluatedSamples == 0:
		report.Verdict = ContractInconclusive
	case inconclusive:
		report.Verdict = ContractInconclusive
	default:
		report.Verdict = ContractPass
	}
	return report, nil
}

func policyTimelineContinuous(states []upgrade.UpgradeState, previous, current *TimelineCorrelation, maxGap time.Duration, targetVersion, targetImage string) bool {
	start, end, ok := correlationBounds(states, *current)
	if !ok {
		return false
	}
	if previous != nil {
		previousStart, _, previousOK := correlationBounds(states, *previous)
		if !previousOK || previousStart > start {
			return false
		}
		start = previousStart
	}
	for i := start; i <= end; i++ {
		state := states[i]
		if state.Phase != upgrade.UpgradePhaseUpdating || state.DesiredVersion != targetVersion || (targetImage != "" && state.DesiredImage != targetImage) {
			return false
		}
		if i > start && state.DesiredImage != states[start].DesiredImage {
			return false
		}
		if i > start && state.ObservedAt.Sub(states[i-1].ObservedAt) > maxGap {
			return false
		}
	}
	return true
}

func correlationBounds(states []upgrade.UpgradeState, correlation TimelineCorrelation) (int, int, bool) {
	from, to := -1, -1
	for i, state := range states {
		if state.ObservedAt.Equal(correlation.FromTime) {
			from = i
		}
		if state.ObservedAt.Equal(correlation.ToTime) {
			to = i
		}
	}
	if from < 0 || to < from {
		return 0, 0, false
	}
	return from, to, correlation.Kind == CorrelationExact || correlation.Kind == CorrelationBracketed
}

type PostCompletionPolicyReport struct {
	Contract         string
	Resource         string
	TargetVersion    string
	TargetImage      string
	Source           string
	GracePeriod      time.Duration
	Verdict          ContractVerdict
	PolicyApplicable bool
	CompletionAt     time.Time
	Deadline         time.Time
	EvaluatedSamples int
	CompliantSamples int
	ViolatingSamples int
	UncertainSamples int
	Evidence         []PostCompletionPolicyFinding
	windows          []lifecycleCompletionWindow
}

type PostCompletionPolicyFinding struct {
	ObservedAt  time.Time
	Correlation TimelineCorrelation
	Applicable  bool
	Compliant   bool
	Uncertain   bool
	State       string
	Attributes  map[string]string
}

func VerifyMachineConfigPoolPostCompletionPolicy(versions []upgrade.ClusterVersionObservation, observations []machineconfig.Observation, targetVersion, targetImage, source string, grace time.Duration) (PostCompletionPolicyReport, error) {
	lifecycle, err := machineconfig.AnalyzeLifecycle(observations)
	if err != nil {
		return PostCompletionPolicyReport{}, err
	}
	report, states, err := newPostCompletionPolicyReport(versions, MachineConfigPoolPolicyContract, lifecycle.Pool, targetVersion, targetImage, source, grace)
	if err != nil {
		return PostCompletionPolicyReport{}, err
	}
	if !report.PolicyApplicable {
		return report, nil
	}
	for _, state := range lifecycle.States {
		correlation, err := CorrelateTime(states, state.ObservedAt)
		if err != nil {
			return PostCompletionPolicyReport{}, err
		}
		finding := PostCompletionPolicyFinding{
			ObservedAt:  state.ObservedAt,
			Correlation: correlation,
			State:       string(state.Phase),
			Attributes: map[string]string{
				"currentConfiguration": state.CurrentConfiguration,
				"desiredConfiguration": state.DesiredConfiguration,
			},
		}
		evaluatePostCompletionFinding(&report, &finding, correlation, state.ObservedAt, state.Phase == machineconfig.PhaseStable, state.Phase == machineconfig.PhaseUnknown)
		report.Evidence = append(report.Evidence, finding)
	}
	finalizePostCompletionReport(&report)
	return report, nil
}

func VerifyNodeReadyPostCompletionPolicy(versions []upgrade.ClusterVersionObservation, observations []nodehistory.Observation, targetVersion, targetImage, source string, grace time.Duration) (PostCompletionPolicyReport, error) {
	return verifyNodePostCompletionPolicy(versions, observations, NodeReadyPolicyContract, targetVersion, targetImage, source, grace, func(state nodehistory.State) (bool, bool, string, map[string]string) {
		return state.Ready == corev1.ConditionTrue, state.Ready == corev1.ConditionUnknown, string(state.Ready), map[string]string{"ready": string(state.Ready)}
	})
}

func VerifyNodeConfigPostCompletionPolicy(versions []upgrade.ClusterVersionObservation, observations []nodehistory.Observation, targetVersion, targetImage, source string, grace time.Duration) (PostCompletionPolicyReport, error) {
	return verifyNodePostCompletionPolicy(versions, observations, NodeConfigPolicyContract, targetVersion, targetImage, source, grace, func(state nodehistory.State) (bool, bool, string, map[string]string) {
		uncertain := state.CurrentMachineConfig == "" || state.DesiredMachineConfig == ""
		return state.ConfigAligned, uncertain, fmt.Sprint(state.ConfigAligned), map[string]string{
			"currentMachineConfig": state.CurrentMachineConfig,
			"desiredMachineConfig": state.DesiredMachineConfig,
			"configAligned":        fmt.Sprint(state.ConfigAligned),
		}
	})
}

func verifyNodePostCompletionPolicy(versions []upgrade.ClusterVersionObservation, observations []nodehistory.Observation, contract, targetVersion, targetImage, source string, grace time.Duration, evaluate func(nodehistory.State) (bool, bool, string, map[string]string)) (PostCompletionPolicyReport, error) {
	lifecycle, err := nodehistory.AnalyzeLifecycle(observations)
	if err != nil {
		return PostCompletionPolicyReport{}, err
	}
	report, states, err := newPostCompletionPolicyReport(versions, contract, lifecycle.Node, targetVersion, targetImage, source, grace)
	if err != nil {
		return PostCompletionPolicyReport{}, err
	}
	if !report.PolicyApplicable {
		return report, nil
	}
	for _, state := range lifecycle.States {
		correlation, err := CorrelateTime(states, state.ObservedAt)
		if err != nil {
			return PostCompletionPolicyReport{}, err
		}
		compliant, uncertain, stateText, attributes := evaluate(state)
		finding := PostCompletionPolicyFinding{ObservedAt: state.ObservedAt, Correlation: correlation, State: stateText, Attributes: attributes}
		evaluatePostCompletionFinding(&report, &finding, correlation, state.ObservedAt, compliant, uncertain)
		report.Evidence = append(report.Evidence, finding)
	}
	finalizePostCompletionReport(&report)
	return report, nil
}

func newPostCompletionPolicyReport(versions []upgrade.ClusterVersionObservation, contract, resource, targetVersion, targetImage, source string, grace time.Duration) (PostCompletionPolicyReport, []upgrade.UpgradeState, error) {
	if targetVersion == "" || source == "" || grace <= 0 {
		return PostCompletionPolicyReport{}, nil, fmt.Errorf("post-completion lifecycle policy requires targetVersion, source, and a positive grace period")
	}
	states, err := upgrade.AnalyzePhases(versions)
	if err != nil {
		return PostCompletionPolicyReport{}, nil, err
	}
	report := PostCompletionPolicyReport{
		Contract:      contract,
		Resource:      resource,
		TargetVersion: targetVersion,
		TargetImage:   targetImage,
		Source:        source,
		GracePeriod:   grace,
		Verdict:       ContractInconclusive,
	}

	var completions []upgrade.UpgradeState
	images := map[string]bool{}
	for _, state := range states {
		if state.Phase != upgrade.UpgradePhaseCompleted || state.DesiredVersion != targetVersion {
			continue
		}
		if targetImage != "" && state.DesiredImage != targetImage {
			continue
		}
		completions = append(completions, state)
		images[state.DesiredImage] = true
	}
	if len(completions) == 0 || (targetImage == "" && len(images) > 1) {
		return report, states, nil
	}
	sort.Slice(completions, func(i, j int) bool { return completions[i].ObservedAt.Before(completions[j].ObservedAt) })
	chosen := completions[0]
	report.PolicyApplicable = true
	report.windows = completedLifecycleWindows(states)
	report.CompletionAt = chosen.ObservedAt
	report.Deadline = chosen.ObservedAt.Add(grace)
	if report.TargetImage == "" {
		report.TargetImage = chosen.DesiredImage
	}
	return report, states, nil
}

func evaluatePostCompletionFinding(report *PostCompletionPolicyReport, finding *PostCompletionPolicyFinding, correlation TimelineCorrelation, observedAt time.Time, compliant, stateUncertain bool) {
	if observedAt.Before(report.Deadline) {
		return
	}
	matchesTarget := (correlation.Kind == CorrelationExact || correlation.Kind == CorrelationBracketed) &&
		(correlation.Phase == upgrade.UpgradePhaseCompleted || correlation.Phase == upgrade.UpgradePhaseStable) &&
		correlation.DesiredVersion == report.TargetVersion && correlation.DesiredImage == report.TargetImage
	if !matchesTarget {
		if correlation.Kind == CorrelationAmbiguous || correlation.Phase == upgrade.UpgradePhaseUnknown {
			finding.Uncertain = true
			report.UncertainSamples++
		}
		return
	}
	window, ok := lifecycleWindowAt(report.windows, observedAt, report.TargetVersion, report.TargetImage)
	if !ok {
		finding.Uncertain = true
		report.UncertainSamples++
		return
	}
	deadline := window.From.Add(report.GracePeriod)
	if finding.Attributes == nil {
		finding.Attributes = make(map[string]string)
	}
	finding.Attributes["completionObservedAt"] = window.From.UTC().Format(time.RFC3339Nano)
	finding.Attributes["deadline"] = deadline.UTC().Format(time.RFC3339Nano)
	if observedAt.Before(deadline) {
		return
	}
	finding.Applicable = true
	report.EvaluatedSamples++
	if stateUncertain {
		finding.Uncertain = true
		report.UncertainSamples++
		return
	}
	finding.Compliant = compliant
	if compliant {
		report.CompliantSamples++
	} else {
		report.ViolatingSamples++
	}
}

func finalizePostCompletionReport(report *PostCompletionPolicyReport) {
	switch {
	case !report.PolicyApplicable:
		report.Verdict = ContractInconclusive
	case report.ViolatingSamples > 0:
		report.Verdict = ContractFail
	case report.EvaluatedSamples == 0 || report.UncertainSamples > 0:
		report.Verdict = ContractInconclusive
	default:
		report.Verdict = ContractPass
	}
}
