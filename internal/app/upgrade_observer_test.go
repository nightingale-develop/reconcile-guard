package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
)

func TestUpgradeObserverDoesNotBridgeUncertainCompletion(t *testing.T) {
	for _, change := range []string{"unknown gap", "version changed", "image changed"} {
		t.Run(change, func(t *testing.T) {
			observations, err := upgrade.ReadHistory("../../testdata/upgrade/cluster-version.jsonl")
			if err != nil {
				t.Fatal(err)
			}
			updating, uncertain, stable := observations[2], observations[3], observations[4]
			switch change {
			case "unknown gap":
				uncertain.ClusterVersion.Status.ObservedGeneration = 0
			case "version changed":
				stable.ClusterVersion.Status.Desired.Version = "4.21.0"
				stable.ClusterVersion.Status.History[0].Version = "4.21.0"
			case "image changed":
				stable.ClusterVersion.Status.Desired.Image = "example.invalid/release@sha256:new"
				stable.ClusterVersion.Status.History[0].Image = stable.ClusterVersion.Status.Desired.Image
			}
			var out bytes.Buffer
			stops := 0
			observer := upgradeObserver{stdout: &out, stop: func() { stops++ }}
			for _, observation := range []upgrade.ClusterVersionObservation{updating, uncertain, stable} {
				if err := observer.observe(observation); err != nil {
					t.Fatal(err)
				}
			}
			if stops != 0 || observer.completed {
				t.Fatalf("uncertain completion stopped observer: %s", &out)
			}
			if change != "unknown gap" && (observer.active || !strings.Contains(out.String(), "target changed")) {
				t.Fatalf("old target retained: %+v output=%s", observer, &out)
			}

			next := updating
			next.ClusterVersion = *updating.ClusterVersion.DeepCopy()
			next.ObservedAt = stable.ObservedAt.Add(time.Minute)
			next.ClusterVersion.Status.Desired = stable.ClusterVersion.Status.Desired
			next.ClusterVersion.Status.History[0].Version = stable.ClusterVersion.Status.Desired.Version
			next.ClusterVersion.Status.History[0].Image = stable.ClusterVersion.Status.Desired.Image
			stable.ObservedAt = next.ObservedAt.Add(time.Minute)
			for _, observation := range []upgrade.ClusterVersionObservation{next, stable} {
				if err := observer.observe(observation); err != nil {
					t.Fatal(err)
				}
			}
			if stops != 1 || !observer.completed || observer.targetImage != stable.ClusterVersion.Status.Desired.Image || observer.targetVersion != stable.ClusterVersion.Status.Desired.Version {
				t.Fatalf("new target failed to complete: %+v", observer)
			}
		})
	}
}

func TestUpgradeObserverRejectsInvalidTimeline(t *testing.T) {
	for _, offset := range []time.Duration{0, -time.Second} {
		t.Run(offset.String(), func(t *testing.T) {
			observations, err := upgrade.ReadHistory("../../testdata/upgrade/cluster-version.jsonl")
			if err != nil {
				t.Fatal(err)
			}
			observer := upgradeObserver{stdout: io.Discard, stop: func() { t.Error("stopped on invalid timeline") }}
			if err := observer.observe(observations[2]); err != nil {
				t.Fatal(err)
			}
			observations[4].ObservedAt = observations[2].ObservedAt.Add(offset)
			if err := observer.observe(observations[4]); err == nil {
				t.Fatal("accepted duplicate/out-of-order timestamp")
			}
		})
	}
}

type observerTestSink struct {
	written int
	err     error
}

func (s *observerTestSink) AppendClusterVersion(upgrade.ClusterVersionObservation) error {
	if s.err == nil {
		s.written++
	}
	return s.err
}

func (s *observerTestSink) AppendOperator(operator.Observation) error { return s.err }

func TestObservingSinkDoesNotCompleteAfterCancellationOrWriteFailure(t *testing.T) {
	for _, failure := range []string{"cancelled", "write failure"} {
		t.Run(failure, func(t *testing.T) {
			observations, err := upgrade.ReadHistory("../../testdata/upgrade/cluster-version.jsonl")
			if err != nil {
				t.Fatal(err)
			}
			observer := &upgradeObserver{stdout: io.Discard, stop: func() { t.Error("completed after cancellation/write failure") }}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			store := &observerTestSink{}
			sink := observingSink{ObservationSink: store, observer: observer, ctx: ctx}
			if err := sink.AppendClusterVersion(observations[2]); err != nil {
				t.Fatal(err)
			}
			if failure == "cancelled" {
				cancel()
			} else {
				store.err = errors.New("disk failure")
			}
			if err := sink.AppendClusterVersion(observations[4]); !errors.Is(err, store.err) {
				t.Fatalf("lost write error: %v", err)
			}
			wantWritten := 2
			if store.err != nil {
				wantWritten = 1
			}
			if observer.completed || store.written != wantWritten {
				t.Fatalf("completed=%v written=%d", observer.completed, store.written)
			}
		})
	}
}
