package contracts

import (
	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
	configv1 "github.com/openshift/api/config/v1"
	"testing"
	"time"
)

func lifecycleInputs(t *testing.T) ([]upgrade.ClusterVersionObservation, []operator.Observation) {
	t.Helper()
	v, err := upgrade.ReadHistory("../../testdata/upgrade/cluster-version.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	o, err := operator.ReadHistory("../../testdata/upgrade/ingress.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	return v, o
}

func TestSyntheticUpgradeLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name                string
		change              func([]upgrade.ClusterVersionObservation, []operator.Observation)
		conditions, version ContractVerdict
	}{
		{"late operator observation", func(v []upgrade.ClusterVersionObservation, o []operator.Observation) {}, ContractPass, ContractPass},
		{"operator rolls out before cluster completion", func(v []upgrade.ClusterVersionObservation, o []operator.Observation) {
			o[1].Operator.Status.Versions[0].Version = "4.20.0"
		}, ContractPass, ContractPass},
		{"temporary degraded recovers", func(v []upgrade.ClusterVersionObservation, o []operator.Observation) {
			o[1].Operator.Status.Conditions[2].Status = configv1.ConditionTrue
		}, ContractFail, ContractPass},
		{"old operator after completion", func(v []upgrade.ClusterVersionObservation, o []operator.Observation) {
			o[2].Operator.Status.Versions[0].Version = "4.19.0"
		}, ContractPass, ContractFail},
		{"missing condition", func(v []upgrade.ClusterVersionObservation, o []operator.Observation) {
			o[1].Operator.Status.Conditions = o[1].Operator.Status.Conditions[1:]
		}, ContractInconclusive, ContractPass},
		{"missing operator version", func(v []upgrade.ClusterVersionObservation, o []operator.Observation) {
			o[2].Operator.Status.Versions = o[2].Operator.Status.Versions[1:]
		}, ContractPass, ContractInconclusive},
		{"stale generation", func(v []upgrade.ClusterVersionObservation, o []operator.Observation) {
			v[3].ClusterVersion.Status.ObservedGeneration = 1
		}, ContractInconclusive, ContractInconclusive},
		{"phase boundary", func(v []upgrade.ClusterVersionObservation, o []operator.Observation) {
			o[1].ObservedAt = v[1].ObservedAt.Add(30 * time.Second)
		}, ContractInconclusive, ContractPass},
		{"no post completion bracket", func(v []upgrade.ClusterVersionObservation, o []operator.Observation) {
			o[2].ObservedAt = v[6].ObservedAt.Add(time.Second)
		}, ContractInconclusive, ContractInconclusive},
		{"release version changes during update", func(v []upgrade.ClusterVersionObservation, o []operator.Observation) {
			for i := 3; i < len(v); i++ {
				v[i].ClusterVersion.Status.Desired.Version = "4.21.0"
				v[i].ClusterVersion.Status.History[0].Version = "4.21.0"
			}
			o[2].Operator.Status.Versions[0].Version = "4.21.0"
		}, ContractInconclusive, ContractPass},
		{"release image changes during update", func(v []upgrade.ClusterVersionObservation, o []operator.Observation) {
			for i := 3; i < len(v); i++ {
				v[i].ClusterVersion.Status.Desired.Image = "example.invalid/another-release"
				v[i].ClusterVersion.Status.History[0].Image = "example.invalid/another-release"
			}
		}, ContractInconclusive, ContractPass},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v, o := lifecycleInputs(t)
			tc.change(v, o)
			r, err := VerifyClusterUpgrade(v, [][]operator.Observation{o})
			if err != nil {
				t.Fatal(err)
			}
			got := r.Operators[0]
			if got.Conditions.Verdict != tc.conditions || got.Version.Verdict != tc.version {
				t.Fatalf("conditions=%+v version=%+v", got.Conditions, got.Version)
			}
			if tc.name == "temporary degraded recovers" && (len(got.Conditions.Findings) != 1 || got.Conditions.Findings[0].Condition != configv1.OperatorDegraded) {
				t.Fatalf("lost transient failure: %+v", got.Conditions)
			}
		})
	}
}

func TestCompletedWindowStopsAtReleaseImageChange(t *testing.T) {
	v, o := lifecycleInputs(t)
	for i := 5; i < len(v); i++ {
		v[i].ClusterVersion.Status.Desired.Image = "example.invalid/new-image"
		v[i].ClusterVersion.Status.History[0].Image = "example.invalid/new-image"
	}
	r, err := VerifyOperatorVersionConsistency(v, o)
	if err != nil {
		t.Fatal(err)
	}
	if r.Verdict != ContractInconclusive || r.UncoveredTargets != 1 || r.EvaluatedSamples != 0 {
		t.Fatalf("window crossed image boundary: %+v", r)
	}
}

func TestUpgradeLifecycleRejectsObservationOrder(t *testing.T) {
	for _, stream := range []string{"version", "operator"} {
		for _, duplicate := range []bool{true, false} {
			v, o := lifecycleInputs(t)
			if stream == "version" {
				if duplicate {
					v[1].ObservedAt = v[0].ObservedAt
				} else {
					v[0], v[1] = v[1], v[0]
				}
			} else {
				if duplicate {
					o[1].ObservedAt = o[0].ObservedAt
				} else {
					o[0], o[1] = o[1], o[0]
				}
			}
			if _, err := VerifyClusterUpgrade(v, [][]operator.Observation{o}); err == nil {
				t.Fatalf("accepted %s duplicate=%v", stream, duplicate)
			}
		}
	}
}
