package contracts

import (
	"fmt"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	configv1 "github.com/openshift/api/config/v1"
)

type ProgressingEpisode struct {
	FirstTrue    time.Time
	LastTrue     time.Time
	BeforeFalse  time.Time
	AfterFalse   time.Time
	ObservedSpan time.Duration
	MaximumSpan  time.Duration
	Verdict      ContractVerdict
}

type ProgressingReport struct {
	Operator          string
	Limit             time.Duration
	Observations      int
	MissingConditions int
	Episodes          []ProgressingEpisode
	Verdict           ContractVerdict
}

// VerifyProgressing checks a user-supplied sampling policy, not an OpenShift timeout.
func VerifyProgressing(observations []operator.Observation, limit time.Duration) (ProgressingReport, error) {
	if limit <= 0 {
		return ProgressingReport{}, fmt.Errorf("maximum progressing duration must be positive")
	}
	history, err := operator.AnalyzeHistory(observations)
	if err != nil {
		return ProgressingReport{}, err
	}
	// Reject timestamps whose elapsed duration cannot be represented without saturation.
	if observations[len(observations)-1].ObservedAt.After(observations[0].ObservedAt.Add(time.Duration(1<<63 - 1))) {
		return ProgressingReport{}, fmt.Errorf("history duration exceeds supported range")
	}
	report := ProgressingReport{Operator: history.Operator, Limit: limit, Observations: len(observations), Verdict: ContractPass}
	var episode *ProgressingEpisode
	var previousFalse time.Time
	finish := func(afterFalse time.Time) {
		if episode == nil {
			return
		}
		episode.AfterFalse = afterFalse
		episode.ObservedSpan = episode.LastTrue.Sub(episode.FirstTrue)
		episode.Verdict = ContractInconclusive
		if !episode.BeforeFalse.IsZero() && !afterFalse.IsZero() {
			episode.MaximumSpan = afterFalse.Sub(episode.BeforeFalse)
			if episode.MaximumSpan <= limit {
				episode.Verdict = ContractPass
			}
		}
		// Consecutive True samples describe an observed run, not proof of continuous state.
		if episode.ObservedSpan > limit {
			episode.Verdict = ContractFail
		}
		report.Episodes = append(report.Episodes, *episode)
		episode = nil
	}
	for _, observation := range observations {
		condition, found := findOperatorCondition(observation.Operator.Status.Conditions, configv1.OperatorProgressing)
		if !found || condition.Status == configv1.ConditionUnknown {
			finish(time.Time{})
			previousFalse = time.Time{}
			report.MissingConditions++
			continue
		}
		if condition.Status == configv1.ConditionFalse {
			finish(observation.ObservedAt)
			previousFalse = observation.ObservedAt
			continue
		}
		if episode == nil {
			episode = &ProgressingEpisode{FirstTrue: observation.ObservedAt, BeforeFalse: previousFalse}
		}
		episode.LastTrue = observation.ObservedAt
		previousFalse = time.Time{}
	}
	finish(time.Time{})
	if report.MissingConditions > 0 {
		report.Verdict = ContractInconclusive
	}
	for _, episode := range report.Episodes {
		if episode.Verdict == ContractFail {
			report.Verdict = ContractFail
			break
		}
		if episode.Verdict == ContractInconclusive {
			report.Verdict = ContractInconclusive
		}
	}
	return report, nil
}
