package main

import (
	"encoding/json"
	"fmt"
	"os"
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
			fmt.Fprintln(os.Stderr, "Usage: reconcile-guard check <file>")
			return 1
		}

		code, err := checkFile(os.Args[2])
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			return 1
		}

		return code

	default:
		fmt.Fprintln(os.Stderr, "Unknown command:", os.Args[1])
		return 1
	}
}

func checkFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read file %q: %w", path, err)
	}

	var operator ClusterOperator

	if err := json.Unmarshal(data, &operator); err != nil {
		return 0, fmt.Errorf("decode JSON: %w", err)
	}

	result, err := analyzeOperator(operator)
	if err != nil {
		return 0, err
	}

	fmt.Println("Operator:", result.Name)

	if result.HasDegraded {
		fmt.Println("Degraded:", result.Degraded.Status)

		if result.Degraded.Status == "True" {
			fmt.Println("Reason:", result.Degraded.Reason)
			fmt.Println("Message:", result.Degraded.Message)
		}
	}

	fmt.Println("Result:", result.Result)

	return result.ExitCode, nil
}

func printUsage() {
	fmt.Println("ReconcileGuard - OpenShift operator diagnostics")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  reconcile-guard <command>")
	fmt.Println()
	fmt.Println("Available commands:")
	fmt.Println("  version       Show application version")
	fmt.Println("  help          Show this help message")
	fmt.Println("  check <file>  Check an OpenShift ClusterOperator")
}
