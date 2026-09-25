package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	configv1 "github.com/openshift/api/config/v1"
)

func main() {
	os.Exit(run())
}

func run() int {
	if len(os.Args) < 2 {
		printUsage()
		return 0
	}

	switch os.Args[1] {
	case "version":
		fmt.Println("ReconcileGuard v0.1.0-dev")
		return 0

	case "help":
		printUsage()
		return 0

	case "check":
		if len(os.Args) != 3 {
			fmt.Fprintln(
				os.Stderr,
				"Usage: reconcile-guard check <file>",
			)
			return 1
		}

		code, err := checkFile(os.Args[2])
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			return 1
		}

		return code

	case "check-version":
		if len(os.Args) != 3 {
			fmt.Fprintln(os.Stderr, "Usage: reconcile-guard check-version <file.json>")
			return 1
		}
		if err := checkVersionFile(os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			return 1
		}
		return 0

	case "replay-version":
		if len(os.Args) != 3 {
			fmt.Fprintln(
				os.Stderr,
				"Usage: reconcile-guard replay-version <file.jsonl>",
			)
			return 1
		}

		observations, err := readClusterVersionHistory(os.Args[2])
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			return 1
		}

		report, err := analyzeClusterVersionHistory(observations)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			return 1
		}

		printClusterVersionHistoryReport(report)
		return 0

	case "replay":
		if len(os.Args) != 3 {
			fmt.Fprintln(
				os.Stderr,
				"Usage: reconcile-guard replay <file.jsonl>",
			)
			return 1
		}

		observations, err := readHistory(os.Args[2])
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			return 1
		}

		report, err := analyzeHistory(observations)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			return 1
		}

		printHistoryReport(report)

		return 0

	default:
		fmt.Fprintln(
			os.Stderr,
			"Unknown command:",
			os.Args[1],
		)
		return 1
	}
}

func checkFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read file %q: %w", path, err)
	}

	var operator configv1.ClusterOperator

	if err := json.Unmarshal(data, &operator); err != nil {
		return 0, fmt.Errorf("decode JSON: %w", err)
	}

	result, err := analyzeOperator(operator)
	if err != nil {
		return 0, err
	}

	fmt.Println("Operator:", result.Name)

	if result.HasDegraded {
		fmt.Println(
			"Degraded:",
			result.Degraded.Status,
		)

		if result.Degraded.Status == configv1.ConditionTrue {
			fmt.Println(
				"Reason:",
				result.Degraded.Reason,
			)
			fmt.Println(
				"Message:",
				result.Degraded.Message,
			)
		}
	}

	fmt.Println("Result:", result.Result)

	return result.ExitCode, nil
}

func printHistoryReport(report HistoryReport) {
	fmt.Println("Operator:", report.Operator)
	fmt.Println("Observations:", report.Observations)
	fmt.Println(
		"Observed transitions:",
		len(report.Transitions),
	)

	for _, transition := range report.Transitions {
		fmt.Printf(
			"  %s: %s -> %s (between %s and %s)\n",
			transition.Condition,
			transition.From,
			transition.To,
			transition.FromTime.Format(time.RFC3339Nano),
			transition.ToTime.Format(time.RFC3339Nano),
		)
	}

	fmt.Println(
		"Uncompared adjacent condition pairs:",
		report.UncomparedPairs,
	)

	fmt.Println(
		"Verdict: NOT EVALUATED (transition report only)",
	)
}

func printUsage() {
	fmt.Println(
		"ReconcileGuard - OpenShift operator diagnostics",
	)
	fmt.Println()

	fmt.Println("Usage:")
	fmt.Println("  reconcile-guard <command>")
	fmt.Println()

	fmt.Println("Available commands:")
	fmt.Println(
		"  version       Show application version",
	)
	fmt.Println(
		"  help          Show this help message",
	)
	fmt.Println(
		"  check <file>  Check an OpenShift ClusterOperator",
	)
	fmt.Println("  check-version <file.json>  Inspect a saved OpenShift ClusterVersion")
	fmt.Println("  replay-version <file.jsonl>  Summarize ClusterVersion observations")
	fmt.Println(
		"  replay <file.jsonl>  Show reported condition changes across snapshots",
	)
}
