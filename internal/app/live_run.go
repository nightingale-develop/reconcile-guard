package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/collector"
	"github.com/nightingale-develop/reconcile-guard/internal/recording"
	appversion "github.com/nightingale-develop/reconcile-guard/internal/version"
)

func (c cli) runLive(args []string, command string) int {
	options, err := parseCaptureLiveArgs(args)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}
	config, err := loadLiveConfig(options.kubeconfig)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}
	run, err := recording.StartRunForCommand(options.outputDirectory, appversion.Current, config.Host, time.Now().UTC(), command)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}
	fail := func(snapshot recording.RunSnapshot, err error) int {
		finishErr := run.Finish(recording.RunStatusFailed, time.Now().UTC(), snapshot, err)
		fmt.Fprintln(c.stderr, "Error:", err)
		if finishErr != nil {
			fmt.Fprintln(c.stderr, "Error finalizing run:", finishErr)
		}
		return 1
	}
	recorder, err := recording.NewRunRecorder(run.Directory())
	if err != nil {
		return fail(recording.RunSnapshot{}, err)
	}
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(signalCtx)
	defer cancel()
	var observer *upgradeObserver
	sink := &observingSink{ObservationSink: recorder, ctx: ctx}
	if command == "observe-upgrade" {
		observer = &upgradeObserver{stdout: c.stdout, stop: cancel}
		sink.observer = observer
	}
	liveRecorder, err := collector.NewLiveRecorder(config, sink)
	if err != nil {
		return fail(recorder.Snapshot(), err)
	}
	if observer != nil {
		fmt.Fprintln(c.stdout, "Observing OpenShift upgrade")
	} else {
		fmt.Fprintln(c.stdout, "Recording live OpenShift observations")
	}
	fmt.Fprintln(c.stdout, "Run:", run.Manifest().RunID)
	fmt.Fprintln(c.stdout, "Output:", run.Directory())
	fmt.Fprintln(c.stdout, "Press Ctrl+C to stop")
	runErr := liveRecorder.Run(ctx)
	stop()

	sink.observer = nil
	if runErr == nil {
		finalCtx, finalCancel := context.WithTimeout(context.Background(), 30*time.Second)
		runErr = liveRecorder.CaptureFinal(finalCtx)
		finalCancel()
	}
	if runErr != nil {
		return fail(recorder.Snapshot(), runErr)
	}
	if err := run.Finish(recording.RunStatusStopped, time.Now().UTC(), recorder.Snapshot(), nil); err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}
	if observer != nil {
		return c.summarizeObservedRun(run.Directory(), observer)
	}
	fmt.Fprintln(c.stdout, "Recording stopped")
	return 0
}
