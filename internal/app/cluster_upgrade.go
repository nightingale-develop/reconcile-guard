package app

import (
	"fmt"

	"github.com/nightingale-develop/reconcile-guard/internal/contracts"
	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
)

func (c cli) verifyClusterUpgrade(
	args []string,
) int {
	if len(args) < 3 {
		fmt.Fprintln(
			c.stderr,
			"Usage: reconcile-guard verify-cluster-upgrade <cluster-version-history.jsonl> <operator-history.jsonl> [operator-history.jsonl...]",
		)
		return 1
	}

	versions, err := upgrade.ReadHistory(args[1])
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}

	histories := make(
		[][]operator.Observation,
		0,
		len(args)-2,
	)

	for _, path := range args[2:] {
		observations, err :=
			operator.ReadHistory(path)
		if err != nil {
			fmt.Fprintf(
				c.stderr,
				"Error: operator history %q: %v\n",
				path,
				err,
			)
			return 1
		}

		histories = append(
			histories,
			observations,
		)
	}

	report, err :=
		contracts.VerifyClusterUpgrade(
			versions,
			histories,
		)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}

	c.printClusterUpgradeReport(report)

	return contractExitCode(report.Verdict)
}

func (c cli) printClusterUpgradeReport(
	report contracts.ClusterUpgradeReport,
) {
	fmt.Fprintln(
		c.stdout,
		"Aggregate verdict:",
		report.Verdict,
	)

	fmt.Fprintln(
		c.stdout,
		"Operators:",
		len(report.Operators),
	)

	fmt.Fprintln(
		c.stdout,
		"Passed:",
		report.PassedOperators,
	)

	fmt.Fprintln(
		c.stdout,
		"Failed:",
		report.FailedOperators,
	)

	fmt.Fprintln(
		c.stdout,
		"Inconclusive:",
		report.InconclusiveOperators,
	)

	fmt.Fprintln(c.stdout)

	for _, operatorReport := range report.Operators {
		fmt.Fprintf(
			c.stdout,
			"Operator: %s\n",
			operatorReport.Operator,
		)

		fmt.Fprintf(
			c.stdout,
			"  Verdict: %s\n",
			operatorReport.Verdict,
		)

		fmt.Fprintf(
			c.stdout,
			"  %s: %s\n",
			operatorReport.Conditions.Contract,
			operatorReport.Conditions.Verdict,
		)

		fmt.Fprintf(
			c.stdout,
			"  %s: %s\n",
			operatorReport.Version.Contract,
			operatorReport.Version.Verdict,
		)
	}

	fmt.Fprintln(
		c.stdout,
		"Scope: supplied operator histories only; PASS does not mean every ClusterOperator in the cluster was evaluated.",
	)
}
