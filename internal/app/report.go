package app

import (
	"fmt"
	"time"

	configv1 "github.com/openshift/api/config/v1"

	"github.com/nightingale-develop/reconcile-guard/internal/contracts"
	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
)

func (c cli) printOperatorAnalysis(result operator.Analysis) {
	fmt.Fprintln(c.stdout, "Operator:", result.Name)

	if result.HasDegraded {
		fmt.Fprintln(c.stdout,
			"Degraded:",
			result.Degraded.Status,
		)

		if result.Degraded.Status ==
			configv1.ConditionTrue {
			fmt.Fprintln(c.stdout,
				"Reason:",
				result.Degraded.Reason,
			)
			fmt.Fprintln(c.stdout,
				"Message:",
				result.Degraded.Message,
			)
		}
	}

	fmt.Fprintln(c.stdout, "Result:", result.Result)

}

func (c cli) printHistoryReport(report operator.HistoryReport) {
	fmt.Fprintln(c.stdout, "Operator:", report.Operator)
	fmt.Fprintln(c.stdout, "Observations:", report.Observations)
	fmt.Fprintln(c.stdout,
		"Observed transitions:",
		len(report.Transitions),
	)

	for _, transition := range report.Transitions {
		fmt.Fprintf(c.stdout,
			"  %s: %s -> %s (between %s and %s)\n",
			transition.Condition,
			transition.From,
			transition.To,
			transition.FromTime.Format(
				time.RFC3339Nano,
			),
			transition.ToTime.Format(
				time.RFC3339Nano,
			),
		)
	}

	fmt.Fprintln(c.stdout,
		"Uncompared adjacent condition pairs:",
		report.UncomparedPairs,
	)

	fmt.Fprintln(c.stdout,
		"Verdict: NOT EVALUATED (transition report only)",
	)
}

func (c cli) printUsage() {
	fmt.Fprintln(c.stdout,
		"ReconcileGuard - OpenShift operator diagnostics",
	)
	fmt.Fprintln(c.stdout)

	fmt.Fprintln(c.stdout, "Usage:")
	fmt.Fprintln(c.stdout, "  reconcile-guard <command>")
	fmt.Fprintln(c.stdout)

	fmt.Fprintln(c.stdout, "Available commands:")
	fmt.Fprintln(c.stdout,
		"  version       Show application version",
	)
	fmt.Fprintln(c.stdout,
		"  help          Show this help message",
	)
	fmt.Fprintln(c.stdout,
		"  check <file>  Check an OpenShift ClusterOperator",
	)
	fmt.Fprintln(c.stdout,
		"  check-version <file.json>  Inspect a saved OpenShift ClusterVersion",
	)
	fmt.Fprintln(c.stdout,
		"  replay-version <file.jsonl>  Reconstruct OpenShift upgrade phases",
	)
	fmt.Fprintln(c.stdout,
		"  replay <file.jsonl>  Show reported condition changes across snapshots",
	)
	fmt.Fprintln(c.stdout,
		"  verify-upgrade <cluster-version-history.jsonl> <operator-history.jsonl>  Verify operator upgrade conditions",
	)
}

func (c cli) printVersionReport(report upgrade.ClusterVersionReport) {
	fmt.Fprintln(c.stdout, "ClusterVersion:", report.Name)
	fmt.Fprintf(c.stdout, "Desired version (reported): %q\n", report.Desired.Version)
	fmt.Fprintf(c.stdout, "Desired image (reported): %q\n", report.Desired.Image)
	fmt.Fprintln(c.stdout, "Conditions:", len(report.Conditions))

	for _, condition := range report.Conditions {
		fmt.Fprintf(c.stdout,
			"  %s: %s (reason=%q, message=%q)\n",
			condition.Type,
			condition.Status,
			condition.Reason,
			condition.Message,
		)
	}

	fmt.Fprintln(c.stdout, "Update history entries:", len(report.History))

	for _, entry := range report.History {
		started := "not reported"
		completed := "not reported"

		if !entry.StartedTime.IsZero() {
			started = entry.StartedTime.Time.Format(time.RFC3339Nano)
		}

		if entry.CompletionTime != nil && !entry.CompletionTime.IsZero() {
			completed = entry.CompletionTime.Time.Format(time.RFC3339Nano)
		}

		fmt.Fprintf(c.stdout,
			"  version=%q image=%q state=%q started=%s completed=%s\n",
			entry.Version,
			entry.Image,
			entry.State,
			started,
			completed,
		)
	}

	fmt.Fprintln(c.stdout, "Verdict: NOT EVALUATED (snapshot report only)")

}

func (c cli) printClusterVersionHistoryReport(
	report upgrade.ClusterVersionHistoryReport,
) {
	fmt.Fprintln(c.stdout, "ClusterVersion:", report.Name)
	fmt.Fprintln(c.stdout, "Observations:", report.Observations)
	fmt.Fprintf(c.stdout,
		"Desired version: %q\n",
		report.DesiredVersion,
	)
}

func (c cli) printUpgradeTimeline(states []upgrade.UpgradeState) {
	fmt.Fprintln(c.stdout, "Upgrade phases:")

	for _, state := range states {
		fmt.Fprintf(c.stdout,
			"  %s  %-9s desired=%q\n",
			state.ObservedAt.Format(time.RFC3339Nano),
			state.Phase,
			state.DesiredVersion,
		)
	}

	fmt.Fprintln(c.stdout,
		"Verdict: NOT EVALUATED (phase reconstruction only)",
	)
}

func (c cli) printUpgradeContractReport(
	report contracts.UpgradeContractReport,
) {
	fmt.Fprintln(c.stdout, "Contract:", report.Contract)
	fmt.Fprintln(c.stdout, "Operator:", report.Operator)
	fmt.Fprintln(c.stdout, "Verdict:", report.Verdict)
	fmt.Fprintln(c.stdout, "Upgrade samples:", report.UpgradeSamples)
	fmt.Fprintln(c.stdout, "Evaluated samples:", report.EvaluatedSamples)
	fmt.Fprintln(c.stdout, "Ambiguous samples:", report.AmbiguousSamples)
	fmt.Fprintln(c.stdout, "Outside samples:", report.OutsideSamples)
	fmt.Fprintln(c.stdout, "Missing conditions:", report.MissingConditions)
	fmt.Fprintln(c.stdout,
		"Unknown phase samples:",
		report.UnknownPhaseSamples,
	)

	if len(report.Findings) == 0 {
		return
	}

	fmt.Fprintln(c.stdout, "Evidence:")

	for _, finding := range report.Findings {
		fmt.Fprintf(c.stdout,
			"  %s %s=%s",
			finding.ObservedAt.Format(time.RFC3339Nano),
			finding.Condition,
			finding.Status,
		)

		if finding.Reason != "" {
			fmt.Fprintf(c.stdout,
				" reason=%q",
				finding.Reason,
			)
		}

		if finding.Message != "" {
			fmt.Fprintf(c.stdout,
				" message=%q",
				finding.Message,
			)
		}

		fmt.Fprintf(c.stdout, " correlation=%s interval=[%s,%s]\n", finding.Correlation,
			finding.FromTime.Format(time.RFC3339Nano), finding.ToTime.Format(time.RFC3339Nano))
	}
}
