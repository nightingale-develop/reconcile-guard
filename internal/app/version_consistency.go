package app

import (
	"fmt"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/contracts"
	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
)

func (c cli) verifyUpgradeVersion(args []string) int {
	if len(args) != 3 {
		fmt.Fprintln(
			c.stderr,
			"Usage: reconcile-guard verify-version-upgrade <cluster-version-history.jsonl> <operator-history.jsonl>",
		)
		return 1
	}

	versions, err := upgrade.ReadHistory(args[1])
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}

	observations, err := operator.ReadHistory(args[2])
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}

	report, err :=
		contracts.VerifyOperatorVersionConsistency(
			versions,
			observations,
		)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}

	c.printVersionConsistencyReport(report)

	return contractExitCode(report.Verdict)
}

func (c cli) printVersionConsistencyReport(
	report contracts.VersionConsistencyReport,
) {
	fmt.Fprintln(c.stdout, "Contract:", report.Contract)
	fmt.Fprintln(c.stdout, "Operator:", report.Operator)
	fmt.Fprintln(c.stdout, "Completed targets:", report.CompletedTargets)
	fmt.Fprintln(c.stdout, "Evaluated post-upgrade samples:", report.EvaluatedSamples)
	fmt.Fprintln(c.stdout, "Missing target versions:", report.MissingTargets)
	fmt.Fprintln(c.stdout, "Missing operator versions:", report.MissingVersions)
	fmt.Fprintln(c.stdout, "Uncovered targets:", report.UncoveredTargets)
	fmt.Fprintln(c.stdout, "Verdict:", report.Verdict)

	if len(report.Evidence) == 0 {
		return
	}

	fmt.Fprintln(c.stdout, "Evidence:")

	for _, evidence := range report.Evidence {
		fmt.Fprintf(
			c.stdout,
			"  %s observed=%s expected=%q reported=%q window=[%s,%s]\n",
			evidence.Verdict,
			evidence.ObservedAt.Format(time.RFC3339Nano),
			evidence.ExpectedVersion,
			evidence.ReportedVersion,
			evidence.WindowFrom.Format(time.RFC3339Nano),
			evidence.WindowTo.Format(time.RFC3339Nano),
		)
	}
}
