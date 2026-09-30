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

func (c cli) recordLive(args []string) int {
	options, err := parseCaptureLiveArgs(args)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}

	config, err :=
		loadLiveConfig(options.kubeconfig)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}

	startedAt := time.Now().UTC()

	run, err := recording.StartRun(
		options.outputDirectory,
		appversion.Current,
		config.Host,
		startedAt,
	)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}

	recorder, err :=
		recording.NewRunRecorder(
			run.Directory(),
		)
	if err != nil {
		finishErr := run.Finish(
			recording.RunStatusFailed,
			time.Now().UTC(),
			recording.RunSnapshot{},
			err,
		)

		fmt.Fprintln(c.stderr, "Error:", err)

		if finishErr != nil {
			fmt.Fprintln(
				c.stderr,
				"Error finalizing run:",
				finishErr,
			)
		}

		return 1
	}

	liveRecorder, err :=
		collector.NewLiveRecorder(
			config,
			recorder,
		)
	if err != nil {
		finishErr := run.Finish(
			recording.RunStatusFailed,
			time.Now().UTC(),
			recorder.Snapshot(),
			err,
		)

		fmt.Fprintln(c.stderr, "Error:", err)
		if finishErr != nil {
			fmt.Fprintln(c.stderr, "Error finalizing run:", finishErr)
		}
		return 1
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	fmt.Fprintln(
		c.stdout,
		"Recording live OpenShift observations",
	)

	fmt.Fprintln(
		c.stdout,
		"Run:",
		run.Manifest().RunID,
	)

	fmt.Fprintln(
		c.stdout,
		"Output:",
		run.Directory(),
	)

	fmt.Fprintln(
		c.stdout,
		"Press Ctrl+C to stop",
	)

	runErr := liveRecorder.Run(ctx)
	stop()
	if runErr == nil {
		finalCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		runErr = liveRecorder.CaptureFinal(finalCtx)
		cancel()
	}

	if runErr != nil {
		finishErr := run.Finish(
			recording.RunStatusFailed,
			time.Now().UTC(),
			recorder.Snapshot(),
			runErr,
		)

		fmt.Fprintln(
			c.stderr,
			"Error:",
			runErr,
		)

		if finishErr != nil {
			fmt.Fprintln(
				c.stderr,
				"Error finalizing run:",
				finishErr,
			)
		}

		return 1
	}

	if err := run.Finish(
		recording.RunStatusStopped,
		time.Now().UTC(),
		recorder.Snapshot(),
		nil,
	); err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}

	fmt.Fprintln(
		c.stdout,
		"Recording stopped",
	)

	return 0
}
