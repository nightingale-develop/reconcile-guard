package contracts

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	configv1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func progressingSamples(statuses string) []operator.Observation {
	observations := make([]operator.Observation, len(statuses))
	for i, status := range statuses {
		o := &observations[i]
		o.ObservedAt = time.Date(2026, 9, 26, 0, i, 0, 0, time.UTC)
		o.Operator.APIVersion = "config.openshift.io/v1"
		o.Operator.Kind = "ClusterOperator"
		o.Operator.Name = "ingress"
		if status != '-' {
			value := map[rune]configv1.ConditionStatus{'T': configv1.ConditionTrue, 'F': configv1.ConditionFalse, 'U': configv1.ConditionUnknown}[status]
			o.Operator.Status.Conditions = []configv1.ClusterOperatorStatusCondition{{Type: configv1.OperatorProgressing, Status: value}}
		}
	}
	return observations
}

func TestVerifyProgressingBoundsAndGaps(t *testing.T) {
	for _, tc := range []struct {
		name, statuses string
		limit          time.Duration
		verdict        ContractVerdict
		missing        int
		episodes       []ContractVerdict
	}{
		{"only false samples", "FFF", time.Minute, ContractPass, 0, nil},
		{"bounded equality passes", "FTTF", 3 * time.Minute, ContractPass, 0, []ContractVerdict{ContractPass}},
		{"uncertain bounds", "FTTF", 2 * time.Minute, ContractInconclusive, 0, []ContractVerdict{ContractInconclusive}},
		{"observed equality is not failure", "TT", time.Minute, ContractInconclusive, 0, []ContractVerdict{ContractInconclusive}},
		{"observed excess fails", "TTT", time.Minute, ContractFail, 0, []ContractVerdict{ContractFail}},
		{"left censored", "TF", 10 * time.Minute, ContractInconclusive, 0, []ContractVerdict{ContractInconclusive}},
		{"right censored", "FT", 10 * time.Minute, ContractInconclusive, 0, []ContractVerdict{ContractInconclusive}},
		{"isolated true", "T", time.Minute, ContractInconclusive, 0, []ContractVerdict{ContractInconclusive}},
		{"missing splits", "FT-TF", time.Minute, ContractInconclusive, 1, []ContractVerdict{ContractInconclusive, ContractInconclusive}},
		{"unknown splits", "FTUTF", time.Minute, ContractInconclusive, 1, []ContractVerdict{ContractInconclusive, ContractInconclusive}},
		{"gap loses preceding boundary", "F-TF", 10 * time.Minute, ContractInconclusive, 1, []ContractVerdict{ContractInconclusive}},
		{"gap after bounded run", "FTF-", 2 * time.Minute, ContractInconclusive, 1, []ContractVerdict{ContractPass}},
		{"all missing", "--", time.Minute, ContractInconclusive, 2, nil},
		{"multiple bounded runs", "FTFTF", 2 * time.Minute, ContractPass, 0, []ContractVerdict{ContractPass, ContractPass}},
		{"failure overrides earlier gap", "-TTT", time.Minute, ContractFail, 1, []ContractVerdict{ContractFail}},
		{"failure overrides later gap", "TTT-", time.Minute, ContractFail, 1, []ContractVerdict{ContractFail}},
		{"failure before inconclusive run", "TTTFT", time.Minute, ContractFail, 0, []ContractVerdict{ContractFail, ContractInconclusive}},
		{"failure overrides inconclusive run", "TFTTT", time.Minute, ContractFail, 0, []ContractVerdict{ContractInconclusive, ContractFail}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := VerifyProgressing(progressingSamples(tc.statuses), tc.limit)
			if err != nil {
				t.Fatal(err)
			}
			if got.Operator != "ingress" || got.Observations != len(tc.statuses) || got.Limit != tc.limit || got.Verdict != tc.verdict || got.MissingConditions != tc.missing {
				t.Fatalf("unexpected report: %+v", got)
			}
			if len(got.Episodes) != len(tc.episodes) {
				t.Fatalf("episodes = %+v", got.Episodes)
			}
			for i, want := range tc.episodes {
				if got.Episodes[i].Verdict != want {
					t.Errorf("episode %d verdict = %s, want %s", i, got.Episodes[i].Verdict, want)
				}
			}
		})
	}
}

func TestVerifyProgressingEvidenceAndNoMutation(t *testing.T) {
	observations := progressingSamples("FFTTF")
	for i := range observations {
		observations[i].Operator.Status.Conditions[0].LastTransitionTime = metav1.NewTime(time.Date(2000+i, 1, 1, 0, 0, 0, 0, time.UTC))
	}
	before := make([]operator.Observation, len(observations))
	for i, o := range observations {
		before[i] = operator.Observation{ObservedAt: o.ObservedAt, Operator: *o.Operator.DeepCopy()}
	}
	got, err := VerifyProgressing(observations, 3*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	want := ProgressingEpisode{FirstTrue: observations[2].ObservedAt, LastTrue: observations[3].ObservedAt, BeforeFalse: observations[1].ObservedAt, AfterFalse: observations[4].ObservedAt, ObservedSpan: time.Minute, MaximumSpan: 3 * time.Minute, Verdict: ContractPass}
	if len(got.Episodes) != 1 || got.Episodes[0] != want {
		t.Fatalf("evidence = %+v, want %+v", got.Episodes, want)
	}
	if !reflect.DeepEqual(before, observations) {
		t.Fatal("input observations mutated")
	}
}

func TestVerifyProgressingValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*[]operator.Observation)
		limit  time.Duration
		want   string
	}{
		{"zero limit", nil, 0, "must be positive"},
		{"negative limit", nil, -time.Minute, "must be positive"},
		{"empty", func(o *[]operator.Observation) { *o = nil }, time.Minute, "no observations"},
		{"timestamp", func(o *[]operator.Observation) { (*o)[0].ObservedAt = time.Time{} }, time.Minute, "observedAt is missing"},
		{"same time", func(o *[]operator.Observation) { (*o)[1].ObservedAt = (*o)[0].ObservedAt }, time.Minute, "must be later"},
		{"backward", func(o *[]operator.Observation) { (*o)[1].ObservedAt = (*o)[0].ObservedAt.Add(-time.Second) }, time.Minute, "must be later"},
		{"name changes", func(o *[]operator.Observation) { (*o)[1].Operator.Name = "other" }, time.Minute, "operator changed"},
		{"missing name", func(o *[]operator.Observation) { (*o)[0].Operator.Name = "" }, time.Minute, "name is missing"},
		{"api version", func(o *[]operator.Observation) { (*o)[0].Operator.APIVersion = "v1" }, time.Minute, "unsupported resource"},
		{"kind", func(o *[]operator.Observation) { (*o)[0].Operator.Kind = "Pod" }, time.Minute, "unsupported resource"},
		{"invalid progressing", func(o *[]operator.Observation) { (*o)[0].Operator.Status.Conditions[0].Status = "Broken" }, time.Minute, "invalid Progressing status"},
		{"duplicate progressing", func(o *[]operator.Observation) {
			c := (*o)[0].Operator.Status.Conditions[0]
			(*o)[0].Operator.Status.Conditions = append((*o)[0].Operator.Status.Conditions, c)
		}, time.Minute, "duplicate Progressing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			observations := progressingSamples("TT")
			if tc.change != nil {
				tc.change(&observations)
			}
			got, err := VerifyProgressing(observations, tc.limit)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			if !reflect.DeepEqual(got, ProgressingReport{}) {
				t.Fatalf("partial report on invalid input: %+v", got)
			}
		})
	}
}

func TestVerifyProgressingElapsedTime(t *testing.T) {
	observations := progressingSamples("FTTF")
	start := observations[0].ObservedAt
	for i, offset := range []time.Duration{0, 100 * time.Millisecond, 1600 * time.Millisecond, 2100 * time.Millisecond} {
		observations[i].ObservedAt = start.Add(offset)
	}
	for _, tc := range []struct {
		limit time.Duration
		want  ContractVerdict
	}{
		{2100 * time.Millisecond, ContractPass},
		{1500 * time.Millisecond, ContractInconclusive},
		{1500*time.Millisecond - time.Nanosecond, ContractFail},
	} {
		got, err := VerifyProgressing(observations, tc.limit)
		if err != nil {
			t.Fatal(err)
		}
		if got.Verdict != tc.want || len(got.Episodes) != 1 {
			t.Fatalf("limit %s: %+v", tc.limit, got)
		}
		if got.Episodes[0].ObservedSpan != 1500*time.Millisecond || got.Episodes[0].MaximumSpan != 2100*time.Millisecond {
			t.Fatalf("incorrect elapsed evidence: %+v", got.Episodes[0])
		}
	}
}

func TestVerifyProgressingDurationRange(t *testing.T) {
	const maximum = time.Duration(1<<63 - 1)
	for _, tc := range []struct {
		name      string
		excess    time.Duration
		wantError bool
	}{
		{"maximum representable span", 0, false},
		{"one nanosecond beyond range", time.Nanosecond, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			observations := progressingSamples("TT")
			observations[1].ObservedAt = observations[0].ObservedAt.Add(maximum).Add(tc.excess)
			got, err := VerifyProgressing(observations, maximum)
			if tc.wantError {
				if err == nil || !strings.Contains(err.Error(), "history duration exceeds supported range") {
					t.Fatalf("error = %v", err)
				}
				if !reflect.DeepEqual(got, ProgressingReport{}) {
					t.Fatalf("partial report: %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Verdict != ContractInconclusive || len(got.Episodes) != 1 || got.Episodes[0].ObservedSpan != maximum {
				t.Fatalf("report: %+v", got)
			}
		})
	}
}
