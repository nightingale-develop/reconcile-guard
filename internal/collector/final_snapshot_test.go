package collector

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/contracts"
	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clienttesting "k8s.io/client-go/testing"
)

func TestFinalSnapshotMakesUnchangedVersionEvaluable(t *testing.T) {
	for _, tc := range []struct {
		name, version, change string
		want                  contracts.ContractVerdict
	}{
		{"matching", "4.20.0", "", contracts.ContractPass},
		{"mismatching", "4.19.0", "", contracts.ContractFail},
		{"target changes", "4.20.0", "version", contracts.ContractInconclusive},
		{"image changes", "4.20.0", "image", contracts.ContractInconclusive},
	} {
		t.Run(tc.name, func(t *testing.T) {
			versions, err := upgrade.ReadHistory("../../testdata/upgrade/cluster-version.jsonl")
			if err != nil {
				t.Fatal(err)
			}
			operators, err := operator.ReadHistory("../../testdata/upgrade/ingress.jsonl")
			if err != nil {
				t.Fatal(err)
			}
			versions = versions[:5]
			operators = operators[:2]
			operators[1].Operator.Status.Versions[0].Version = tc.version
			sink := &memorySink{versions: versions, operators: operators}
			client := liveClient(t, &versions[4].ClusterVersion, &operators[1].Operator)
			gets := 0
			client.PrependReactor("get", "clusterversions", func(clienttesting.Action) (bool, runtime.Object, error) {
				gets++
				v := versions[4].ClusterVersion.DeepCopy()
				if gets == 2 {
					switch tc.change {
					case "version":
						v.Status.Desired.Version = "4.21.0"
						v.Status.History[0].Version = "4.21.0"
					case "image":
						v.Status.Desired.Image = "different"
						v.Status.History[0].Image = "different"
					}
				}
				data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(v)
				return true, &unstructured.Unstructured{Object: data}, err
			})
			report, err := contracts.VerifyOperatorVersionConsistency(sink.versions, sink.operators)
			if err != nil {
				t.Fatal(err)
			}
			if report.Verdict != contracts.ContractInconclusive || report.UncoveredTargets != 1 || report.EvaluatedSamples != 0 {
				t.Fatalf("without snapshot=%+v", report)
			}
			now := versions[4].ObservedAt.Add(time.Minute)
			r := newLiveRecorder(client, sink, func() time.Time { now = now.Add(time.Second); return now })
			r.clock.Observe("clusterversion/version", versions[4].ObservedAt)
			r.clock.Observe("clusteroperator/ingress", operators[1].ObservedAt)
			if err := r.CaptureFinal(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(sink.versions) != 7 || len(sink.operators) != 3 {
				t.Fatal("snapshot missing")
			}
			last := sink.operators[2]
			if !reflect.DeepEqual(last.Operator, operators[1].Operator) || !last.ObservedAt.After(versions[4].ObservedAt) {
				t.Fatal("unchanged object not freshly observed")
			}
			if !last.ObservedAt.After(sink.versions[5].ObservedAt) || !sink.versions[6].ObservedAt.After(last.ObservedAt) {
				t.Fatal("missing CV bracket")
			}
			report, err = contracts.VerifyOperatorVersionConsistency(sink.versions, sink.operators)
			if err != nil {
				t.Fatal(err)
			}
			if report.Verdict != tc.want {
				t.Fatalf("final snapshot=%+v", report)
			}
			if tc.change == "" && (report.EvaluatedSamples != 1 || report.UncoveredTargets != 0) {
				t.Fatalf("not evaluated: %+v", report)
			}
			actions := client.Actions()
			if len(actions) != 3 || actions[0].GetVerb() != "get" || actions[1].GetVerb() != "list" || actions[2].GetVerb() != "get" {
				t.Fatalf("unexpected calls: %v", actions)
			}
		})
	}
}

func TestFinalSnapshotMonotonicAndSorted(t *testing.T) {
	client := liveClient(t, liveObject("ClusterVersion", "version", "1"), liveObject("ClusterOperator", "z", "1"), liveObject("ClusterOperator", "a", "1"))
	sink := &memorySink{}
	now := time.Date(2026, 9, 30, 10, 0, 0, 0, time.FixedZone("local", 3600))
	r := newLiveRecorder(client, sink, func() time.Time { return now })
	for i := 0; i < 2; i++ {
		if err := r.CaptureFinal(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if len(sink.versions) != 4 || len(sink.operators) != 4 {
		t.Fatal("deduplicated explicit snapshots")
	}
	for i := 1; i < len(sink.versions); i++ {
		if !sink.versions[i].ObservedAt.After(sink.versions[i-1].ObservedAt) {
			t.Fatal("CV timestamps not increasing")
		}
	}
	for i, name := range []string{"a", "z"} {
		if sink.operators[i].Operator.Name != name || sink.operators[i+2].Operator.Name != name || !sink.operators[i+2].ObservedAt.After(sink.operators[i].ObservedAt) || sink.operators[i].ObservedAt.Location() != time.UTC {
			t.Fatal("operator order/timestamps")
		}
	}
}

func TestFinalSnapshotErrors(t *testing.T) {
	for _, stage := range []string{"get", "list", "closing get", "version write", "operator write", "cancelled"} {
		t.Run(stage, func(t *testing.T) {
			client := liveClient(t, liveObject("ClusterVersion", "version", "1"), liveObject("ClusterOperator", "ingress", "1"))
			want := errors.New("snapshot failure")
			gets := 0
			client.PrependReactor("get", "clusterversions", func(clienttesting.Action) (bool, runtime.Object, error) {
				gets++
				if stage == "get" || (stage == "closing get" && gets == 2) {
					return true, nil, want
				}
				return false, nil, nil
			})
			client.PrependReactor("list", "clusteroperators", func(clienttesting.Action) (bool, runtime.Object, error) {
				if stage == "list" {
					return true, nil, want
				}
				return false, nil, nil
			})
			var sink ObservationSink = &memorySink{}
			if stage == "version write" || stage == "operator write" {
				sink = failingSink{version: stage == "version write", err: want}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if stage == "cancelled" {
				cancel()
				want = context.Canceled
			}
			err := newLiveRecorder(client, sink, time.Now).CaptureFinal(ctx)
			if !errors.Is(err, want) {
				t.Fatalf("error=%v want=%v", err, want)
			}
		})
	}
}
