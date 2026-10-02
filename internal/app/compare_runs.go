package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nightingale-develop/reconcile-guard/internal/contracts"
	"github.com/nightingale-develop/reconcile-guard/internal/policy"
	"github.com/nightingale-develop/reconcile-guard/internal/regression"
	"github.com/nightingale-develop/reconcile-guard/internal/result"
)

type compareRunsOptions struct {
	baselineDirectory  string
	candidateDirectory string
	policyPath         string
}

func (c cli) compareRuns(args []string) int {
	options, err := parseCompareRunsOptions(args)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}

	baseline, err := loadRunInput(options.baselineDirectory)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error: baseline:", err)
		return 1
	}
	candidate, err := loadRunInput(options.candidateDirectory)
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

	baselineAuxiliary, err := verifyAuxiliaryRunEvidence(baseline)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error: baseline:", err)
		return 1
	}
	candidateAuxiliary, err := verifyAuxiliaryRunEvidence(candidate)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error: candidate:", err)
		return 1
	}
	baselineTimings, err := buildRegressionTimings(baseline, baselineAuxiliary)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error: baseline:", err)
		return 1
	}
	candidateTimings, err := buildRegressionTimings(candidate, candidateAuxiliary)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error: candidate:", err)
		return 1
	}

	baselineEvidence := regression.Evidence{
		Verification:       before,
		MachineConfigPools: baselineAuxiliary.MachineConfigPools,
		Nodes:              baselineAuxiliary.Nodes,
		Timings:            baselineTimings,
	}
	candidateEvidence := regression.Evidence{
		Verification:       after,
		MachineConfigPools: candidateAuxiliary.MachineConfigPools,
		Nodes:              candidateAuxiliary.Nodes,
		Timings:            candidateTimings,
	}

	var configured *policy.UpgradePolicy
	if options.policyPath != "" {
		parsed, err := readLifecyclePolicy(options.policyPath)
		if err != nil {
			fmt.Fprintln(c.stderr, "Error:", err)
			return 1
		}
		baselinePolicy, err := evaluateLifecyclePolicy(baseline, parsed)
		if err != nil {
			fmt.Fprintln(c.stderr, "Error: baseline policy:", err)
			return 1
		}
		candidatePolicy, err := evaluateLifecyclePolicy(candidate, parsed)
		if err != nil {
			fmt.Fprintln(c.stderr, "Error: candidate policy:", err)
			return 1
		}
		baselineEvidence.Policy = &baselinePolicy
		candidateEvidence.Policy = &candidatePolicy
		configured = &parsed
	}

	comparison, err := regression.CompareEvidence(baselineEvidence, candidateEvidence)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}
	comparison.Baseline = summarizeRun(baseline, before.Verdict)
	comparison.Candidate = summarizeRun(candidate, after.Verdict)
	comparison.Scope.SameFinalTarget = comparison.Baseline.FinalDesiredVersion == comparison.Candidate.FinalDesiredVersion && comparison.Baseline.FinalDesiredImage == comparison.Candidate.FinalDesiredImage
	if !comparison.Scope.SameFinalTarget {
		for i := range comparison.Timings {
			comparison.Timings[i].Delta = ""
		}
	}
	if comparison.Policy != nil && configured != nil {
		comparison.Policy.Source = configured.Source
		comparison.Policy.TargetVersion = configured.TargetVersion
		comparison.Policy.TargetImage = configured.TargetImage
	}

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

func parseCompareRunsOptions(args []string) (compareRunsOptions, error) {
	if len(args) < 3 {
		return compareRunsOptions{}, fmt.Errorf("usage: reconcile-guard compare-runs <baseline-run-directory> <candidate-run-directory> [--policy <policy.yaml>]")
	}
	options := compareRunsOptions{baselineDirectory: args[1], candidateDirectory: args[2]}
	for i := 3; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--policy":
			if options.policyPath != "" {
				return compareRunsOptions{}, fmt.Errorf("--policy may only be specified once")
			}
			if i+1 >= len(args) || args[i+1] == "" {
				return compareRunsOptions{}, fmt.Errorf("--policy requires a path")
			}
			i++
			options.policyPath = args[i]
		case strings.HasPrefix(arg, "--policy="):
			if options.policyPath != "" {
				return compareRunsOptions{}, fmt.Errorf("--policy may only be specified once")
			}
			options.policyPath = strings.TrimPrefix(arg, "--policy=")
			if options.policyPath == "" {
				return compareRunsOptions{}, fmt.Errorf("--policy requires a path")
			}
		default:
			return compareRunsOptions{}, fmt.Errorf("unknown compare-runs option %q", arg)
		}
	}
	return options, nil
}

func summarizeRun(input runInput, verdict result.Verdict) regression.RunSummary {
	last := input.Versions[len(input.Versions)-1].ClusterVersion.Status.Desired
	return regression.RunSummary{
		RunID:               input.Manifest.RunID,
		ClusterID:           input.Manifest.Source.ClusterID,
		FinalDesiredVersion: last.Version,
		FinalDesiredImage:   last.Image,
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
		fmt.Fprintf(&text, "%s run: %s\n%s cluster: %s\n%s final desired version: %s\n%s final desired image: %s\n%s verification: %s\n\n",
			item.label, item.summary.RunID, item.label, item.summary.ClusterID,
			item.label, item.summary.FinalDesiredVersion, item.label, item.summary.FinalDesiredImage,
			item.label, item.summary.VerificationVerdict)
	}
	fmt.Fprintf(&text, "Final targets match: %t\n", report.Scope.SameFinalTarget)
	fmt.Fprintf(&text, "Common operators: %d\n", report.Scope.CommonOperators)
	fmt.Fprintf(&text, "Baseline-only operators: %d [%s]\n", len(report.Scope.BaselineOnlyOperators), strings.Join(report.Scope.BaselineOnlyOperators, ", "))
	fmt.Fprintf(&text, "Candidate-only operators: %d [%s]\n", len(report.Scope.CandidateOnlyOperators), strings.Join(report.Scope.CandidateOnlyOperators, ", "))
	for _, operator := range report.Operators {
		printComparisonItem(&text, operator.Name, operator.Verdict, operator.BaselineVerdict, operator.CandidateVerdict, operator.Contracts)
	}

	if report.Scope.CommonMachineConfigPools > 0 || len(report.Scope.BaselineOnlyMachineConfigPools) > 0 || len(report.Scope.CandidateOnlyMachineConfigPools) > 0 {
		fmt.Fprintln(&text, "\nMachineConfigPool evidence comparison:")
		fmt.Fprintf(&text, "  Common: %d\n", report.Scope.CommonMachineConfigPools)
		fmt.Fprintf(&text, "  Baseline-only: %d [%s]\n", len(report.Scope.BaselineOnlyMachineConfigPools), strings.Join(report.Scope.BaselineOnlyMachineConfigPools, ", "))
		fmt.Fprintf(&text, "  Candidate-only: %d [%s]\n", len(report.Scope.CandidateOnlyMachineConfigPools), strings.Join(report.Scope.CandidateOnlyMachineConfigPools, ", "))
		for _, resource := range report.MachineConfigPools {
			printComparisonItem(&text, "MachineConfigPool/"+resource.Name, resource.Verdict, resource.BaselineVerdict, resource.CandidateVerdict, resource.Contracts)
		}
	}
	if report.Scope.CommonNodes > 0 || len(report.Scope.BaselineOnlyNodes) > 0 || len(report.Scope.CandidateOnlyNodes) > 0 {
		fmt.Fprintln(&text, "\nNode evidence comparison:")
		fmt.Fprintf(&text, "  Common: %d\n", report.Scope.CommonNodes)
		fmt.Fprintf(&text, "  Baseline-only: %d [%s]\n", len(report.Scope.BaselineOnlyNodes), strings.Join(report.Scope.BaselineOnlyNodes, ", "))
		fmt.Fprintf(&text, "  Candidate-only: %d [%s]\n", len(report.Scope.CandidateOnlyNodes), strings.Join(report.Scope.CandidateOnlyNodes, ", "))
		for _, resource := range report.Nodes {
			printComparisonItem(&text, "Node/"+resource.Name, resource.Verdict, resource.BaselineVerdict, resource.CandidateVerdict, resource.Contracts)
		}
	}
	if len(report.MachineConfigPools) > 0 || len(report.Nodes) > 0 {
		fmt.Fprintln(&text, "Auxiliary MCP/Node comparisons are evidence-only and do not change the comparison verdict without an explicit policy.")
	}

	if len(report.Timings) > 0 {
		fmt.Fprintln(&text, "\nObserved timing comparison (descriptive only):")
		for _, timing := range report.Timings {
			resource := timing.ResourceKind
			if timing.ResourceName != "" {
				resource += "/" + timing.ResourceName
			}
			baseline := "MISSING"
			candidate := "MISSING"
			if timing.Baseline != nil {
				baseline = timing.Baseline.Duration
			}
			if timing.Candidate != nil {
				candidate = timing.Candidate.Duration
			}
			fmt.Fprintf(&text, "  %s %s: baseline=%s candidate=%s", resource, timing.Name, baseline, candidate)
			if timing.Delta != "" {
				fmt.Fprintf(&text, " delta=%s", timing.Delta)
			}
			fmt.Fprintln(&text)
		}
		fmt.Fprintln(&text, "Observed timing deltas are measurements between recorded samples; they are not regression verdicts or exact transition durations.")
	}

	if report.Policy != nil {
		printPolicyComparison(&text, *report.Policy)
	}

	fmt.Fprintf(&text, "\nComparison verdict: %s\n", report.Verdict)
	fmt.Fprintln(&text, "PASS means no regression was detected among comparable contracts; it does not mean the candidate run itself passed verification.")
	if report.Policy != nil {
		fmt.Fprintln(&text, "Policy regressions are based only on the supplied explicit lifecycle policy; its thresholds are not OpenShift guarantees.")
	}
	_, err := text.WriteTo(c.stdout)
	return err
}

func printComparisonItem(text *bytes.Buffer, name string, verdict, baselineVerdict, candidateVerdict result.Verdict, contracts []regression.ContractComparison) {
	before, after := printableVerdict(baselineVerdict), printableVerdict(candidateVerdict)
	fmt.Fprintf(text, "\n%s: %s (baseline=%s, candidate=%s)\n", name, verdict, before, after)
	for _, contract := range contracts {
		fmt.Fprintf(text, "  %s: %s -> %s [%s]", contract.Name, printableVerdict(contract.BaselineVerdict), printableVerdict(contract.CandidateVerdict), contract.Change)
		if contract.BaselineObservedSpan != "" || contract.CandidateObservedSpan != "" {
			baselineSpan, candidateSpan := contract.BaselineObservedSpan, contract.CandidateObservedSpan
			if baselineSpan == "" {
				baselineSpan = "MISSING"
			}
			if candidateSpan == "" {
				candidateSpan = "MISSING"
			}
			fmt.Fprintf(text, " observed-span=%s -> %s", baselineSpan, candidateSpan)
			if contract.ObservedSpanDelta != "" {
				fmt.Fprintf(text, " delta=%s", contract.ObservedSpanDelta)
			}
		}
		fmt.Fprintln(text)
	}
}

func printableVerdict(verdict result.Verdict) string {
	if verdict == "" {
		return "MISSING"
	}
	return string(verdict)
}

func printPolicyComparison(text *bytes.Buffer, report regression.PolicyComparison) {
	fmt.Fprintln(text, "\nLifecycle policy comparison:")
	fmt.Fprintln(text, "  Source:", report.Source)
	fmt.Fprintln(text, "  Target version:", report.TargetVersion)
	if report.TargetImage != "" {
		fmt.Fprintln(text, "  Target image:", report.TargetImage)
	}
	fmt.Fprintf(text, "  Baseline policy verdict: %s\n", report.BaselineVerdict)
	fmt.Fprintf(text, "  Candidate policy verdict: %s\n", report.CandidateVerdict)
	fmt.Fprintf(text, "  Policy comparison verdict: %s\n", report.Verdict)
	for _, item := range report.Operators {
		printComparisonItem(text, "  ClusterOperator/"+item.Name, item.Verdict, item.BaselineVerdict, item.CandidateVerdict, item.Contracts)
	}
	for _, item := range report.MachineConfigPools {
		printComparisonItem(text, "  MachineConfigPool/"+item.Name, item.Verdict, item.BaselineVerdict, item.CandidateVerdict, item.Contracts)
	}
	for _, item := range report.Nodes {
		printComparisonItem(text, "  Node/"+item.Name, item.Verdict, item.BaselineVerdict, item.CandidateVerdict, item.Contracts)
	}
}
