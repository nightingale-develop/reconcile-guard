package app

import (
	"fmt"
	"io"

	"github.com/nightingale-develop/reconcile-guard/internal/contracts"
	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
)

func (c cli) run(args []string) int {
	if len(args) == 0 {
		c.printUsage()
		return 0
	}

	switch args[0] {
	case "version":
		fmt.Fprintln(c.stdout, "ReconcileGuard v0.1.0-dev")
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

		c.printUpgradeContractReport(report)

		return contractExitCode(report.Verdict)

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
}

func Run(args []string, stdout, stderr io.Writer) int {
	return (cli{stdout: stdout, stderr: stderr}).run(args)
}
