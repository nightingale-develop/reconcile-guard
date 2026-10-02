package app

import (
	"fmt"
	"io"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/contracts"
	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
	appversion "github.com/nightingale-develop/reconcile-guard/internal/version"
)

func (c cli) run(args []string) int {
	args, format, outputSpecified, err :=
		parseOutputOption(args)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}

	if len(args) == 0 {
		c.printUsage()
		return 0
	}

	if outputSpecified &&
		!supportsStructuredOutput(args[0]) {
		fmt.Fprintln(
			c.stderr,
			"Error: --output is supported only by verify commands, compare-runs, or timeline-run",
		)
		return 1
	}

	c.output = format

	switch args[0] {
	case "version":
		fmt.Fprintln(
			c.stdout,
			"ReconcileGuard v"+appversion.Current,
		)
		return 0

	case "help":
		c.printUsage()
		return 0

	case "check":
		if len(args) != 2 {
			fmt.Fprintln(
				c.stderr,
				"Usage: reconcile-guard check <file>",
			)
			return 1
		}

		code, err := c.checkFile(args[1])
		if err != nil {
			fmt.Fprintln(c.stderr, "Error:", err)
			return 1
		}

		return code

	case "check-version":
		if len(args) != 2 {
			fmt.Fprintln(
				c.stderr,
				"Usage: reconcile-guard check-version <file.json>",
			)
			return 1
		}

		if err := c.checkVersionFile(args[1]); err != nil {
			fmt.Fprintln(c.stderr, "Error:", err)
			return 1
		}

		return 0

	case "replay-version":
		if len(args) != 2 {
			fmt.Fprintln(
				c.stderr,
				"Usage: reconcile-guard replay-version <file.jsonl>",
			)
			return 1
		}

		observations, err :=
			upgrade.ReadHistory(args[1])
		if err != nil {
			fmt.Fprintln(c.stderr, "Error:", err)
			return 1
		}

		report, err :=
			upgrade.AnalyzeHistory(observations)
		if err != nil {
			fmt.Fprintln(c.stderr, "Error:", err)
			return 1
		}

		states, err :=
			upgrade.AnalyzePhases(observations)
		if err != nil {
			fmt.Fprintln(c.stderr, "Error:", err)
			return 1
		}

		c.printClusterVersionHistoryReport(report)
		c.printUpgradeTimeline(states)

		return 0

	case "replay":
		if len(args) != 2 {
			fmt.Fprintln(
				c.stderr,
				"Usage: reconcile-guard replay <file.jsonl>",
			)
			return 1
		}

		observations, err := operator.ReadHistory(args[1])
		if err != nil {
			fmt.Fprintln(c.stderr, "Error:", err)
			return 1
		}

		report, err := operator.AnalyzeHistory(observations)
		if err != nil {
			fmt.Fprintln(c.stderr, "Error:", err)
			return 1
		}

		c.printHistoryReport(report)

		return 0

	case "verify-progressing-upgrade":
		return c.verifyUpgradeProgressing(args)

	case "verify-lifecycle-policy":
		return c.verifyLifecyclePolicy(args)

	case "verify-version-upgrade":
		return c.verifyUpgradeVersion(args)

	case "verify-cluster-upgrade":
		return c.verifyClusterUpgrade(args)

	case "verify-progressing":
		if len(args) != 3 {
			fmt.Fprintln(c.stderr, "Usage: reconcile-guard verify-progressing <operator-history.jsonl> <max-duration>")
			return 1
		}
		limit, err := time.ParseDuration(args[2])
		if err != nil || limit <= 0 {
			fmt.Fprintln(c.stderr, "Error: max-duration must be a positive Go duration, for example 30m")
			return 1
		}
		observations, err := operator.ReadHistory(args[1])
		if err != nil {
			fmt.Fprintln(c.stderr, "Error:", err)
			return 1
		}
		report, err := contracts.VerifyProgressing(observations, limit)
		if err != nil {
			fmt.Fprintln(c.stderr, "Error:", err)
			return 1
		}
		if c.output == outputJSON {
			err := c.writeSingleContractJSON(
				"verify-progressing",
				report.Operator,
				contracts.ProgressingResult(report),
			)
			if err != nil {
				fmt.Fprintln(c.stderr, "Error:", err)
				return 1
			}
		} else {
			c.printProgressingReport(report)
		}

		return contractExitCode(report.Verdict)

	case "verify-upgrade":
		if len(args) != 3 {
			fmt.Fprintln(
				c.stderr,
				"Usage: reconcile-guard verify-upgrade <cluster-version-history.jsonl> <operator-history.jsonl>",
			)
			return 1
		}

		versionObservations, err :=
			upgrade.ReadHistory(args[1])
		if err != nil {
			fmt.Fprintln(c.stderr, "Error:", err)
			return 1
		}

		operatorObservations, err :=
			operator.ReadHistory(args[2])
		if err != nil {
			fmt.Fprintln(c.stderr, "Error:", err)
			return 1
		}

		report, err :=
			contracts.VerifyNormalUpgradeOperatorConditions(
				versionObservations,
				operatorObservations,
			)
		if err != nil {
			fmt.Fprintln(c.stderr, "Error:", err)
			return 1
		}

		if c.output == outputJSON {
			err := c.writeSingleContractJSON(
				"verify-upgrade",
				report.Operator,
				contracts.NormalUpgradeResult(report),
			)
			if err != nil {
				fmt.Fprintln(c.stderr, "Error:", err)
				return 1
			}
		} else {
			c.printUpgradeContractReport(report)
		}

		return contractExitCode(report.Verdict)

	case "capture-live":
		return c.captureLive(args[1:])

	case "record-live":
		return c.recordLive(args[1:])

	case "observe-upgrade":
		return c.observeUpgrade(args[1:])

	case "verify-run":
		return c.verifyRun(args)

	case "timeline-run":
		return c.timelineRun(args)

	case "report-run":
		return c.reportRun(args)

	case "compare-runs":
		return c.compareRuns(args)

	default:
		fmt.Fprintln(
			c.stderr,
			"Unknown command:",
			args[0],
		)
		return 1
	}
}

func (c cli) checkFile(path string) (int, error) {
	snapshot, err := operator.ReadSnapshot(path)
	if err != nil {
		return 0, err
	}

	result, err := operator.Analyze(snapshot)
	if err != nil {
		return 0, err
	}

	c.printOperatorAnalysis(result)
	return result.ExitCode, nil
}

func (c cli) checkVersionFile(path string) error {
	version, err := upgrade.ReadClusterVersion(path)
	if err != nil {
		return err
	}

	report, err := upgrade.AnalyzeClusterVersion(version)
	if err != nil {
		return err
	}

	c.printVersionReport(report)
	return nil
}

func contractExitCode(verdict contracts.ContractVerdict) int {
	switch verdict {
	case contracts.ContractPass:
		return 0
	case contracts.ContractFail:
		return 2
	case contracts.ContractInconclusive:
		return 3
	default:
		return 1
	}
}

type cli struct {
	stdout io.Writer
	stderr io.Writer
	output outputFormat
}

func Run(args []string, stdout, stderr io.Writer) int {
	output := &outputWriter{writer: stdout}
	code := (cli{stdout: output, stderr: stderr}).run(args)
	if output.err != nil {
		if code != 1 {
			fmt.Fprintln(stderr, "Error: write output:", output.err)
		}
		return 1
	}
	return code
}

type outputWriter struct {
	writer io.Writer
	err    error
}

func (w *outputWriter) Write(data []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.writer.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	w.err = err
	return n, err
}
