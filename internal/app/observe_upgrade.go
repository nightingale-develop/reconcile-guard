package app

import (
	"context"
	"fmt"
	"io"

	"github.com/nightingale-develop/reconcile-guard/internal/collector"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
)

func (c cli) observeUpgrade(args []string) int {
	return c.runLive(args, "observe-upgrade")
}

type observingSink struct {
	collector.ObservationSink
	observer *upgradeObserver
	ctx      context.Context
}

func (s *observingSink) AppendClusterVersion(observation upgrade.ClusterVersionObservation) error {
	if err := s.ObservationSink.AppendClusterVersion(observation); err != nil {
		return err
	}
	if s.observer != nil && s.ctx.Err() == nil {
		return s.observer.observe(observation)
	}
	return nil
}

type upgradeObserver struct {
	previous      *upgrade.ClusterVersionObservation
	targetVersion string
	targetImage   string
	active        bool
	completed     bool
	waiting       bool
	baseline      bool
	stdout        io.Writer
	stop          context.CancelFunc
}

func (o *upgradeObserver) observe(observation upgrade.ClusterVersionObservation) error {
	if o.completed {
		return nil
	}
	observations := []upgrade.ClusterVersionObservation{observation}
	if o.previous != nil {
		observations = []upgrade.ClusterVersionObservation{*o.previous, observation}
	}
	states, err := upgrade.AnalyzePhases(observations)
	if err != nil {
		return fmt.Errorf("observe upgrade: %w", err)
	}
	state := states[len(states)-1]
	if o.active && (state.DesiredVersion != o.targetVersion || state.DesiredImage != o.targetImage) {
		fmt.Fprintln(o.stdout, "Upgrade target changed; waiting for UPDATING evidence for the new target.")
		o.active = false
		o.targetVersion, o.targetImage = "", ""
	}
	switch state.Phase {
	case upgrade.UpgradePhaseStable:
		o.baseline = true
		if !o.active && !o.waiting {
			fmt.Fprintln(o.stdout, "Waiting for upgrade...")
			o.waiting = true
		}
	case upgrade.UpgradePhaseUpdating:
		if !o.active {
			if !o.baseline {
				fmt.Fprintln(o.stdout, "Upgrade already in progress; pre-upgrade evidence may be incomplete.")
			}
			o.active = true
			o.waiting = false
			o.targetVersion, o.targetImage = state.DesiredVersion, state.DesiredImage
			fmt.Fprintf(o.stdout, "Upgrade detected:\n  target version: %s\n  target image: %s\n", o.targetVersion, o.targetImage)
		}
	case upgrade.UpgradePhaseCompleted:
		if o.active && state.DesiredVersion == o.targetVersion && state.DesiredImage == o.targetImage {
			o.completed = true
			o.stop()
		}
	}
	o.previous = &observation
	return nil
}

func (c cli) summarizeObservedRun(directory string, observer *upgradeObserver) int {
	input, report, err := verifyRecordedRun(directory)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}
	if observer.completed {
		fmt.Fprintln(c.stdout, "Upgrade completion observed")
	} else {
		fmt.Fprintln(c.stdout, "Observation stopped before upgrade completion was observed.")
	}
	fmt.Fprintln(c.stdout, "Run:", input.Manifest.RunID)
	fmt.Fprintln(c.stdout, "Cluster ID:", input.Manifest.Source.ClusterID)
	fmt.Fprintf(c.stdout, "Observed target version: %q\nObserved target image: %q\n", observer.targetVersion, observer.targetImage)
	fmt.Fprintln(c.stdout, "Completion observed:", observer.completed)
	fmt.Fprintln(c.stdout, "Final snapshot: captured")
	fmt.Fprintln(c.stdout, "Verdict:", report.Verdict)
	fmt.Fprintf(c.stdout, "Operators: %d (PASS=%d FAIL=%d INCONCLUSIVE=%d)\n", len(report.Operators), report.PassedOperators, report.FailedOperators, report.InconclusiveOperators)
	return contractExitCode(report.Verdict)
}
