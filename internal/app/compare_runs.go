package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nightingale-develop/reconcile-guard/internal/contracts"
	"github.com/nightingale-develop/reconcile-guard/internal/regression"
	"github.com/nightingale-develop/reconcile-guard/internal/result"
)

func (c cli) compareRuns(args []string) int {
	if len(args) != 3 {
		fmt.Fprintln(c.stderr, "Usage: reconcile-guard compare-runs <baseline-run-directory> <candidate-run-directory>")
		return 1
	}
	baseline, err := loadRunInput(args[1])
	if err != nil {
		fmt.Fprintln(c.stderr, "Error: baseline:", err)
		return 1
	}
	candidate, err := loadRunInput(args[2])
	if err != nil {
		fmt.Fprintln(c.stderr, "Error: candidate:", err)
		return 1
	}
	before, err := contracts.VerifyClusterUpgrade(baseline.Versions, baseline.Histories)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error: baseline:", err)
		return 1
	}
	after, err := contracts.VerifyClusterUpgrade(candidate.Versions, candidate.Histories)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error: candidate:", err)
		return 1
	}
	comparison, err := regression.Compare(before, after)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}
	comparison.Baseline = summarizeRun(baseline, before.Verdict)
	comparison.Candidate = summarizeRun(candidate, after.Verdict)
	if c.output == outputJSON {
		encoder := json.NewEncoder(c.stdout)
		encoder.SetIndent("", "  ")
		err = encoder.Encode(regression.NewDocument(comparison))
	} else {
		err = c.printComparison(comparison)
	}
	if err != nil {
		fmt.Fprintln(c.stderr, "Error: write comparison output:", err)
		return 1
	}
	return contractExitCode(comparison.Verdict)
}

func summarizeRun(input runInput, verdict result.Verdict) regression.RunSummary {
	return regression.RunSummary{
		RunID:               input.Manifest.RunID,
		ClusterID:           input.Manifest.Source.ClusterID,
		FinalDesiredVersion: input.Versions[len(input.Versions)-1].ClusterVersion.Status.Desired.Version,
		VerificationVerdict: verdict,
	}
}

func (c cli) printComparison(report regression.Report) error {
	var text bytes.Buffer
	for _, item := range []struct {
		label   string
		summary regression.RunSummary
	}{
		{"Baseline", report.Baseline}, {"Candidate", report.Candidate},
	} {
		fmt.Fprintf(&text, "%s run: %s\n%s cluster: %s\n%s final desired version: %s\n%s verification: %s\n\n",
			item.label, item.summary.RunID, item.label, item.summary.ClusterID,
			item.label, item.summary.FinalDesiredVersion, item.label, item.summary.VerificationVerdict)
	}
	fmt.Fprintf(&text, "Common operators: %d\n", report.Scope.CommonOperators)
	fmt.Fprintf(&text, "Baseline-only operators: %d [%s]\n", len(report.Scope.BaselineOnlyOperators), strings.Join(report.Scope.BaselineOnlyOperators, ", "))
	fmt.Fprintf(&text, "Candidate-only operators: %d [%s]\n", len(report.Scope.CandidateOnlyOperators), strings.Join(report.Scope.CandidateOnlyOperators, ", "))
	for _, operator := range report.Operators {
		fmt.Fprintf(&text, "\n%s: %s (baseline=%s, candidate=%s)\n", operator.Name, operator.Verdict, operator.BaselineVerdict, operator.CandidateVerdict)
		for _, contract := range operator.Contracts {
			before, after := string(contract.BaselineVerdict), string(contract.CandidateVerdict)
			if before == "" {
				before = "MISSING"
			}
			if after == "" {
				after = "MISSING"
			}
			fmt.Fprintf(&text, "  %s: %s -> %s [%s]\n", contract.Name, before, after, contract.Change)
		}
	}
	fmt.Fprintf(&text, "\nComparison verdict: %s\n", report.Verdict)
	fmt.Fprintln(&text, "PASS means no regression was detected among comparable contracts; it does not mean the candidate run itself passed verification.")
	_, err := text.WriteTo(c.stdout)
	return err
}
