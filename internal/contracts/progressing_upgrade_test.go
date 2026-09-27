package contracts

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
)

func durationUpgradeVersions(t *testing.T, minutes ...int) []upgrade.ClusterVersionObservation {
	t.Helper()
	base := loadContractVersions(t)[1]
	out := make([]upgrade.ClusterVersionObservation, len(minutes))
	for i, m := range minutes {
		out[i] = upgrade.ClusterVersionObservation{ObservedAt: base.ObservedAt.Add(time.Duration(m) * time.Minute), ClusterVersion: *base.ClusterVersion.DeepCopy()}
	}
	return out
}
func durationUpgradePolicy() ProgressingPolicy {
	return ProgressingPolicy{Limit: 2 * time.Minute, MaxObservationGap: 5 * time.Minute, Operator: "ingress", TargetVersion: "4.20.0", Source: "project policy v1"}
}
func durationUpgradeOperators(at time.Time, statuses string) []operator.Observation {
	out := progressingSamples(statuses)
	for i := range out {
		out[i].ObservedAt = at.Add(time.Duration(i) * time.Minute)
	}
	return out
}

func TestUpgradeProgressingScopeAndEvidence(t *testing.T) {
	versions := durationUpgradeVersions(t, 0, 4)
	observations := durationUpgradeOperators(versions[0].ObservedAt, "FTTTF")
	policy := durationUpgradePolicy()
	policy.Limit = 4 * time.Minute
	before, err := json.Marshal([]any{versions, observations})
	if err != nil {
		t.Fatal(err)
	}
	got, err := VerifyUpgradeProgressing(versions, observations, policy)
	if err != nil {
		t.Fatal(err)
	}
	if got.Verdict != ContractPass || got.EvaluatedSamples != 5 || !got.PolicyApplicable || len(got.Evidence) != 1 {
		t.Fatalf("report: %+v", got)
	}
	e := got.Evidence[0]
	if e.Start.Kind != CorrelationBracketed || e.End.Kind != CorrelationBracketed || e.Start.FromTime != versions[0].ObservedAt || e.End.ToTime != versions[1].ObservedAt || e.Episode.ObservedSpan != 2*time.Minute || e.Episode.MaximumSpan != 4*time.Minute {
		t.Fatalf("evidence: %+v", e)
	}
	after, err := json.Marshal([]any{versions, observations})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("inputs mutated")
	}
	// Stable/completed samples must not supply False boundaries to the updating run.
	full := loadContractVersions(t)
	observations = durationUpgradeOperators(full[0].ObservedAt, "FTF")
	for i := range observations {
		observations[i].ObservedAt = full[i].ObservedAt
	}
	got, err = VerifyUpgradeProgressing(full, observations, policy)
	if err != nil {
		t.Fatal(err)
	}
	if got.Verdict != ContractInconclusive || got.EvaluatedSamples != 1 || len(got.Evidence) != 1 || !got.Evidence[0].Episode.BeforeFalse.IsZero() || !got.Evidence[0].Episode.AfterFalse.IsZero() {
		t.Fatalf("scope: %+v", got)
	}
	got, err = VerifyUpgradeProgressing(full, []operator.Observation{observations[0], observations[2]}, policy)
	if err != nil || got.Verdict != ContractInconclusive || got.EvaluatedSamples != 0 {
		t.Fatalf("no eligible: %+v %v", got, err)
	}
}

func TestUpgradeProgressingBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name                                  string
		change                                func([]upgrade.ClusterVersionObservation, []operator.Observation, *ProgressingPolicy)
		want                                  ContractVerdict
		evaluated, uncertain, missing, breaks int
	}{
		{"exact fail", nil, ContractFail, 3, 0, 0, 0},
		{"operator gap", func(v []upgrade.ClusterVersionObservation, o []operator.Observation, p *ProgressingPolicy) {
			p.MaxObservationGap = 30 * time.Second
		}, ContractInconclusive, 3, 0, 0, 2},
		{"target image change", func(v []upgrade.ClusterVersionObservation, o []operator.Observation, p *ProgressingPolicy) {
			v[1].ClusterVersion.Status.Desired.Image = "new"
			v[1].ClusterVersion.Status.History[0].Image = "new"
		}, ContractInconclusive, 3, 0, 0, 2},
		{"target version change", func(v []upgrade.ClusterVersionObservation, o []operator.Observation, p *ProgressingPolicy) {
			v[1].ClusterVersion.Status.Desired.Version = "4.21.0"
			v[1].ClusterVersion.Status.History[0].Version = "4.21.0"
		}, ContractInconclusive, 2, 1, 0, 0},
		{"unknown phase", func(v []upgrade.ClusterVersionObservation, o []operator.Observation, p *ProgressingPolicy) {
			v[1].ClusterVersion.Status.Conditions = nil
		}, ContractInconclusive, 2, 1, 0, 0},
		{"missing condition", func(v []upgrade.ClusterVersionObservation, o []operator.Observation, p *ProgressingPolicy) {
			o[1].Operator.Status.Conditions = nil
		}, ContractInconclusive, 3, 0, 1, 0},
		{"unknown condition", func(v []upgrade.ClusterVersionObservation, o []operator.Observation, p *ProgressingPolicy) {
			o[1].Operator.Status.Conditions[0].Status = "Unknown"
		}, ContractInconclusive, 3, 0, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := durationUpgradeVersions(t, 0, 1, 2)
			o := durationUpgradeOperators(v[0].ObservedAt, "TTT")
			p := durationUpgradePolicy()
			p.Limit = time.Minute
			if tc.change != nil {
				tc.change(v, o, &p)
			}
			got, err := VerifyUpgradeProgressing(v, o, p)
			if err != nil {
				t.Fatal(err)
			}
			if got.Verdict != tc.want || got.EvaluatedSamples != tc.evaluated || got.UncertainSamples != tc.uncertain || got.MissingConditions != tc.missing || got.Discontinuities != tc.breaks {
				t.Fatalf("report: %+v", got)
			}
		})
	}
}

func TestUpgradeProgressingDoesNotBridgeVersionGaps(t *testing.T) {
	for _, kind := range []string{"gap", "image", "phase"} {
		t.Run(kind, func(t *testing.T) {
			v := durationUpgradeVersions(t, 0, 4)
			o := durationUpgradeOperators(v[0].ObservedAt, "TTT")
			p := durationUpgradePolicy()
			p.Limit = time.Minute
			switch kind {
			case "gap":
				p.MaxObservationGap = 2 * time.Minute
			case "image":
				v[1].ClusterVersion.Status.Desired.Image = "new"
				v[1].ClusterVersion.Status.History[0].Image = "new"
			case "phase":
				v[1].ClusterVersion.Status.Conditions = nil
			}
			got, err := VerifyUpgradeProgressing(v, o, p)
			if err != nil {
				t.Fatal(err)
			}
			if got.Verdict != ContractInconclusive || got.EvaluatedSamples != 1 || got.UncertainSamples != 2 {
				t.Fatalf("report: %+v", got)
			}
		})
	}
	// Dense CV samples cannot hide a large operator sampling gap.
	v := durationUpgradeVersions(t, 0, 1, 2, 3, 4)
	o := durationUpgradeOperators(v[0].ObservedAt, "TT")
	o[1].ObservedAt = v[4].ObservedAt
	p := durationUpgradePolicy()
	p.Limit = time.Minute
	p.MaxObservationGap = time.Minute
	got, err := VerifyUpgradeProgressing(v, o, p)
	if err != nil || got.Verdict != ContractInconclusive || got.Discontinuities != 1 {
		t.Fatalf("operator gap: %+v %v", got, err)
	}
}

func TestUpgradeProgressingPolicyAndPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name       string
		change     func(*ProgressingPolicy)
		applicable bool
	}{
		{"matching", func(p *ProgressingPolicy) {}, true},
		{"operator mismatch", func(p *ProgressingPolicy) { p.Operator = "network" }, false},
		{"no source", func(p *ProgressingPolicy) { p.Source = " \t" }, false},
		{"no target", func(p *ProgressingPolicy) { p.TargetVersion = "" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := durationUpgradeVersions(t, 0, 1, 2, 3)
			o := durationUpgradeOperators(v[0].ObservedAt, "TTT-")
			p := durationUpgradePolicy()
			p.Limit = time.Minute
			tc.change(&p)
			got, err := VerifyUpgradeProgressing(v, o, p)
			if err != nil {
				t.Fatal(err)
			}
			want := ContractInconclusive
			if tc.applicable {
				want = ContractFail
			}
			if tc.name == "no target" {
				if got.PolicyApplicable || got.Verdict != ContractInconclusive || got.EvaluatedSamples != 0 || got.UncertainSamples != 4 || len(got.Evidence) != 0 {
					t.Fatalf("absent target: %+v", got)
				}
				return
			}
			if got.PolicyApplicable != tc.applicable || got.Verdict != want || len(got.Evidence) != 1 || got.Evidence[0].Episode.Verdict != want {
				t.Fatalf("report: %+v", got)
			}
		})
	}
	v := durationUpgradeVersions(t, 0, 1)
	o := durationUpgradeOperators(v[0].ObservedAt, "FF")
	p := durationUpgradePolicy()
	p.TargetVersion = "4.99.0"
	got, err := VerifyUpgradeProgressing(v, o, p)
	if err != nil || got.Verdict != ContractInconclusive || got.EvaluatedSamples != 0 {
		t.Fatalf("target mismatch: %+v %v", got, err)
	}
	for _, gap := range []bool{false, true} {
		p := durationUpgradePolicy()
		if gap {
			p.MaxObservationGap = 0
		} else {
			p.Limit = -time.Second
		}
		if _, err := VerifyUpgradeProgressing(v, o, p); err == nil || !strings.Contains(err.Error(), "durations must be positive") {
			t.Fatalf("error: %v", err)
		}
	}
}

func TestUpgradeProgressingHiddenPhaseBoundary(t *testing.T) {
	versions := durationUpgradeVersions(t, 0, 1, 2)
	completed := loadContractVersions(t)[2].ClusterVersion.DeepCopy()
	completed.Status.History[0].CompletionTime.Time = versions[1].ObservedAt
	versions[1].ClusterVersion = *completed
	states, err := upgrade.AnalyzePhases(versions)
	if err != nil {
		t.Fatal(err)
	}
	if states[1].Phase != upgrade.UpgradePhaseCompleted {
		t.Fatalf("middle phase = %s, want COMPLETED", states[1].Phase)
	}
	observations := durationUpgradeOperators(versions[0].ObservedAt, "TT")
	observations[1].ObservedAt = versions[2].ObservedAt
	policy := durationUpgradePolicy()
	policy.Limit = time.Minute
	got, err := VerifyUpgradeProgressing(versions, observations, policy)
	if err != nil {
		t.Fatal(err)
	}
	if got.Verdict != ContractInconclusive || got.Discontinuities != 1 || len(got.Evidence) != 2 {
		t.Fatalf("bridged hidden completed phase: %+v", got)
	}
	for _, e := range got.Evidence {
		if e.Episode.ObservedSpan != 0 || e.Episode.Verdict != ContractInconclusive {
			t.Fatalf("unexpected episode: %+v", e)
		}
	}
}

func TestUpgradeProgressingUncertainSamplePrecedence(t *testing.T) {
	for _, kind := range []string{"outside", "ambiguous"} {
		for _, fail := range []bool{false, true} {
			t.Run(kind+map[bool]string{false: "/pass", true: "/fail"}[fail], func(t *testing.T) {
				versions := durationUpgradeVersions(t, 0, 1, 2, 3, 4)
				versions[4].ClusterVersion.Status.Conditions = nil
				statuses := "FTF"
				if fail {
					statuses = "TTT"
				}
				observations := durationUpgradeOperators(versions[0].ObservedAt, statuses)
				extra := durationUpgradeOperators(versions[0].ObservedAt, "F")[0]
				if kind == "outside" {
					extra.ObservedAt = versions[4].ObservedAt.Add(time.Minute)
				} else {
					extra.ObservedAt = versions[3].ObservedAt.Add(30 * time.Second)
				}
				observations = append(observations, extra)
				policy := durationUpgradePolicy()
				if fail {
					policy.Limit = time.Minute
				}
				got, err := VerifyUpgradeProgressing(versions, observations, policy)
				if err != nil {
					t.Fatal(err)
				}
				want := ContractInconclusive
				if fail {
					want = ContractFail
				}
				if got.Verdict != want || got.UncertainSamples != 1 || got.EvaluatedSamples != 3 || len(got.Evidence) != 1 {
					t.Fatalf("report: %+v", got)
				}
			})
		}
	}
}

func TestUpgradeProgressingGapAndRangeBoundaries(t *testing.T) {
	for _, excess := range []time.Duration{0, time.Nanosecond} {
		versions := durationUpgradeVersions(t, 0, 1)
		versions[1].ObservedAt = versions[1].ObservedAt.Add(excess)
		observations := durationUpgradeOperators(versions[0].ObservedAt, "TT")
		observations[1].ObservedAt = versions[1].ObservedAt
		policy := durationUpgradePolicy()
		policy.MaxObservationGap = time.Minute
		policy.Limit = 30 * time.Second
		got, err := VerifyUpgradeProgressing(versions, observations, policy)
		if err != nil {
			t.Fatal(err)
		}
		want, breaks := ContractFail, 0
		if excess > 0 {
			want, breaks = ContractInconclusive, 1
		}
		if got.Verdict != want || got.Discontinuities != breaks {
			t.Fatalf("excess=%s: %+v", excess, got)
		}
	}
	versions := durationUpgradeVersions(t, 0, 1)
	observations := durationUpgradeOperators(versions[0].ObservedAt, "TT")
	observations[1].ObservedAt = observations[0].ObservedAt.Add(time.Duration(1<<63 - 1)).Add(time.Nanosecond)
	_, err := VerifyUpgradeProgressing(versions, observations, durationUpgradePolicy())
	if err == nil || !strings.Contains(err.Error(), "history duration exceeds supported range") {
		t.Fatalf("range error: %v", err)
	}
}
