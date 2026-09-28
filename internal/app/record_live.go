package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/nightingale-develop/reconcile-guard/internal/collector"
	"github.com/nightingale-develop/reconcile-guard/internal/recording"
)

func (c cli) recordLive(args []string) int {
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

	recorder, err :=
		recording.NewJSONLRecorder(
			options.outputDirectory,
		)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}

	liveRecorder, err :=
		collector.NewLiveRecorder(
			config,
			recorder,
		)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
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
		"Output:",
		options.outputDirectory,
	)

	fmt.Fprintln(
		c.stdout,
		"Press Ctrl+C to stop",
	)

	if err := liveRecorder.Run(ctx); err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}

	fmt.Fprintln(
		c.stdout,
		"Recording stopped",
	)

	return 0
}
