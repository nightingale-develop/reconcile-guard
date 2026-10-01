package app

import (
	"fmt"

	"github.com/nightingale-develop/reconcile-guard/internal/contracts"
)

func (c cli) verifyRun(
	args []string,
) int {
	if len(args) != 2 {
		fmt.Fprintln(
			c.stderr,
			"Usage: reconcile-guard verify-run <run-directory>",
		)
		return 1
	}

	input, report, err := verifyRecordedRun(args[1])
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}

	if c.output == outputJSON {
		if err := c.writeJSON(
			"verify-run",
			contracts.ClusterUpgradeResult(report),
		); err != nil {
			fmt.Fprintln(c.stderr, "Error:", err)
			return 1
		}
	} else {
		fmt.Fprintln(
			c.stdout,
			"Run:",
			input.Manifest.RunID,
		)

		fmt.Fprintln(
			c.stdout,
			"Cluster ID:",
			input.Manifest.Source.ClusterID,
		)

		c.printClusterUpgradeReport(report)
	}

	return contractExitCode(report.Verdict)
}

func verifyRecordedRun(directory string) (runInput, contracts.ClusterUpgradeReport, error) {
	input, err := loadRunInput(directory)
	if err != nil {
		return runInput{}, contracts.ClusterUpgradeReport{}, err
	}
	report, err := contracts.VerifyClusterUpgrade(input.Versions, input.Histories)
	return input, report, err
}
