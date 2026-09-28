package app

import (
	"context"
	"fmt"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/collector"
	"github.com/nightingale-develop/reconcile-guard/internal/recording"

	"k8s.io/client-go/tools/clientcmd"
)

type captureLiveOptions struct {
	outputDirectory string
	kubeconfig      string
}

func (c cli) captureLive(args []string) int {
	options, err := parseCaptureLiveArgs(args)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}

	loadingRules :=
		clientcmd.NewDefaultClientConfigLoadingRules()

	if options.kubeconfig != "" {
		loadingRules.ExplicitPath =
			options.kubeconfig
	}

	clientConfig :=
		clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			loadingRules,
			&clientcmd.ConfigOverrides{},
		)

	config, err := clientConfig.ClientConfig()
	if err != nil {
		fmt.Fprintln(
			c.stderr,
			"Error: load kubeconfig:",
			err,
		)
		return 1
	}

	liveCollector, err :=
		collector.NewLiveCollector(config)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	defer cancel()

	capture, err := liveCollector.Capture(ctx)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}

	if err := recording.AppendCapture(
		options.outputDirectory,
		capture,
	); err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}

	fmt.Fprintln(
		c.stdout,
		"ClusterVersion:",
		capture.ClusterVersion.ClusterVersion.Name,
	)

	fmt.Fprintln(
		c.stdout,
		"ClusterOperators:",
		len(capture.Operators),
	)

	fmt.Fprintln(
		c.stdout,
		"Output:",
		options.outputDirectory,
	)

	return 0
}

func parseCaptureLiveArgs(
	args []string,
) (captureLiveOptions, error) {
	var options captureLiveOptions

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--kubeconfig":
			if i+1 >= len(args) {
				return options, fmt.Errorf(
					"--kubeconfig requires a path",
				)
			}

			i++
			options.kubeconfig = args[i]

		default:
			if len(args[i]) > 0 &&
				args[i][0] == '-' {
				return options, fmt.Errorf(
					"unknown option %q",
					args[i],
				)
			}

			if options.outputDirectory != "" {
				return options, fmt.Errorf(
					"only one output directory is allowed",
				)
			}

			options.outputDirectory = args[i]
		}
	}

	if options.outputDirectory == "" {
		return options, fmt.Errorf(
			"output directory is required",
		)
	}

	return options, nil
}
