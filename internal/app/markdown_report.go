package app

import (
	"bytes"
	"fmt"
	"html"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/contracts"
	"github.com/nightingale-develop/reconcile-guard/internal/policy"
	"github.com/nightingale-develop/reconcile-guard/internal/result"
	"github.com/nightingale-develop/reconcile-guard/internal/timeline"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
)

type reportRunOptions struct {
	runDirectory string
	policyPath   string
	filePath     string
}

func (c cli) reportRun(args []string) int {
	options, err := parseReportRunOptions(args)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}

	input, verification, err := verifyRecordedRun(options.runDirectory)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}
	auxiliary, err := verifyAuxiliaryRunEvidence(input)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}
	timelineReport, err := timeline.Build(input.Versions, input.Histories, input.MachineConfigPools, input.Nodes)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}

	var configured *policy.UpgradePolicy
	var policyReport *result.Report
	if options.policyPath != "" {
		parsed, err := readLifecyclePolicy(options.policyPath)
		if err != nil {
			fmt.Fprintln(c.stderr, "Error:", err)
			return 1
		}
		report, err := evaluateLifecyclePolicy(input, parsed)
		if err != nil {
			fmt.Fprintln(c.stderr, "Error:", err)
			return 1
		}
		configured = &parsed
		policyReport = &report
	}

	markdown, err := renderMarkdownRunReport(input, verification, auxiliary, timelineReport, configured, policyReport)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}

	if options.filePath == "" {
		if _, err := c.stdout.Write(markdown); err != nil {
			fmt.Fprintln(c.stderr, "Error: write Markdown report:", err)
			return 1
		}
		return 0
	}
	if err := os.WriteFile(options.filePath, markdown, 0644); err != nil {
		fmt.Fprintln(c.stderr, "Error: write Markdown report:", err)
		return 1
	}
	fmt.Fprintln(c.stdout, "Markdown report:", options.filePath)
	return 0
}

func parseReportRunOptions(args []string) (reportRunOptions, error) {
	if len(args) < 2 {
		return reportRunOptions{}, fmt.Errorf("usage: reconcile-guard report-run <run-directory> [--policy <policy.yaml>] [--file <report.md>]")
	}
	options := reportRunOptions{runDirectory: args[1]}
	for i := 2; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--policy":
			if options.policyPath != "" {
				return reportRunOptions{}, fmt.Errorf("--policy may only be specified once")
			}
			if i+1 >= len(args) || args[i+1] == "" {
				return reportRunOptions{}, fmt.Errorf("--policy requires a path")
			}
			i++
			options.policyPath = args[i]
		case strings.HasPrefix(arg, "--policy="):
			if options.policyPath != "" {
				return reportRunOptions{}, fmt.Errorf("--policy may only be specified once")
			}
			options.policyPath = strings.TrimPrefix(arg, "--policy=")
			if options.policyPath == "" {
				return reportRunOptions{}, fmt.Errorf("--policy requires a path")
			}
		case arg == "--file":
			if options.filePath != "" {
				return reportRunOptions{}, fmt.Errorf("--file may only be specified once")
			}
			if i+1 >= len(args) || args[i+1] == "" {
				return reportRunOptions{}, fmt.Errorf("--file requires a path")
			}
			i++
			options.filePath = args[i]
		case strings.HasPrefix(arg, "--file="):
			if options.filePath != "" {
				return reportRunOptions{}, fmt.Errorf("--file may only be specified once")
			}
			options.filePath = strings.TrimPrefix(arg, "--file=")
			if options.filePath == "" {
				return reportRunOptions{}, fmt.Errorf("--file requires a path")
			}
		default:
			return reportRunOptions{}, fmt.Errorf("unknown report-run option %q", arg)
		}
	}
	return options, nil
}

func renderMarkdownRunReport(
	input runInput,
	verification contracts.ClusterUpgradeReport,
	auxiliary auxiliaryRunReport,
	timelineReport timeline.Report,
	configured *policy.UpgradePolicy,
	policyReport *result.Report,
) ([]byte, error) {
	states, err := upgrade.AnalyzePhases(input.Versions)
	if err != nil {
		return nil, fmt.Errorf("ClusterVersion timeline: %w", err)
	}

	var out bytes.Buffer
	fmt.Fprintln(&out, "# ReconcileGuard run report")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "> Generated from recorded evidence. Transition intervals are observation bounds; the report does not infer an exact transition time or root cause.")
	fmt.Fprintln(&out)

	fmt.Fprintln(&out, "## Run")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "| Field | Value |")
	fmt.Fprintln(&out, "| --- | --- |")
	markdownRow(&out, "Run ID", input.Manifest.RunID)
	markdownRow(&out, "Cluster ID", input.Manifest.Source.ClusterID)
	markdownRow(&out, "API server", input.Manifest.Source.Server)
	markdownRow(&out, "Recorded by", input.Manifest.Command)
	markdownRow(&out, "Started", formatTime(input.Manifest.StartedAt))
	if input.Manifest.EndedAt != nil {
		markdownRow(&out, "Ended", formatTime(*input.Manifest.EndedAt))
		markdownRow(&out, "Recording duration", input.Manifest.EndedAt.Sub(input.Manifest.StartedAt).String())
	}
	if len(states) > 0 {
		last := states[len(states)-1]
		markdownRow(&out, "Last observed desired version", last.DesiredVersion)
		markdownRow(&out, "Last observed desired image", last.DesiredImage)
		markdownRow(&out, "Last reconstructed phase", string(last.Phase))
	}
	fmt.Fprintln(&out)

	fmt.Fprintln(&out, "## Verification summary")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "| Metric | Value |")
	fmt.Fprintln(&out, "| --- | ---: |")
	markdownRow(&out, "Aggregate verdict", string(verification.Verdict))
	markdownRow(&out, "Operators", fmt.Sprint(len(verification.Operators)))
	markdownRow(&out, "Passed operators", fmt.Sprint(verification.PassedOperators))
	markdownRow(&out, "Failed operators", fmt.Sprint(verification.FailedOperators))
	markdownRow(&out, "Inconclusive operators", fmt.Sprint(verification.InconclusiveOperators))
	fmt.Fprintln(&out)

	fmt.Fprintln(&out, "### ClusterOperators")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "| Operator | Verdict | Conditions | Version consistency |")
	fmt.Fprintln(&out, "| --- | --- | --- | --- |")
	for _, item := range verification.Operators {
		fmt.Fprintf(&out, "| %s | %s | %s | %s |\n",
			markdownCell(item.Operator), item.Verdict, item.Conditions.Verdict, item.Version.Verdict)
	}
	fmt.Fprintln(&out)

	fmt.Fprintln(&out, "### MachineConfigPools")
	fmt.Fprintln(&out)
	if len(auxiliary.MachineConfigPools) == 0 {
		fmt.Fprintln(&out, "No MachineConfigPool histories were recorded.")
	} else {
		fmt.Fprintln(&out, "| Pool | Evidence verdict | Last phase | Current config | Desired config | Updated | Ready | Degraded |")
		fmt.Fprintln(&out, "| --- | --- | --- | --- | --- | ---: | ---: | ---: |")
		for _, item := range auxiliary.MachineConfigPools {
			phase, current, desired, updated, ready, degraded := "", "", "", "", "", ""
			if len(item.Evidence) > 0 {
				last := item.Evidence[len(item.Evidence)-1].State
				phase = string(last.Phase)
				current = last.CurrentConfiguration
				desired = last.DesiredConfiguration
				updated = fmt.Sprintf("%d/%d", last.UpdatedMachineCount, last.MachineCount)
				ready = fmt.Sprintf("%d/%d", last.ReadyMachineCount, last.MachineCount)
				degraded = fmt.Sprint(last.DegradedMachineCount)
			}
			fmt.Fprintf(&out, "| %s | %s | %s | %s | %s | %s | %s | %s |\n",
				markdownCell(item.Pool), item.Verdict, markdownCell(phase), markdownCell(current), markdownCell(desired), markdownCell(updated), markdownCell(ready), markdownCell(degraded))
		}
	}
	fmt.Fprintln(&out)

	fmt.Fprintln(&out, "### Nodes")
	fmt.Fprintln(&out)
	if len(auxiliary.Nodes) == 0 {
		fmt.Fprintln(&out, "No Node histories were recorded.")
	} else {
		fmt.Fprintln(&out, "| Node | Evidence verdict | Ready | Config aligned | Current config | Desired config | Kubelet |")
		fmt.Fprintln(&out, "| --- | --- | --- | --- | --- | --- | --- |")
		for _, item := range auxiliary.Nodes {
			ready, aligned, current, desired, kubelet := "", "", "", "", ""
			if len(item.Evidence) > 0 {
				last := item.Evidence[len(item.Evidence)-1].State
				ready = string(last.Ready)
				aligned = fmt.Sprint(last.ConfigAligned)
				current = last.CurrentMachineConfig
				desired = last.DesiredMachineConfig
				kubelet = last.KubeletVersion
			}
			fmt.Fprintf(&out, "| %s | %s | %s | %s | %s | %s | %s |\n",
				markdownCell(item.Node), item.Verdict, markdownCell(ready), markdownCell(aligned), markdownCell(current), markdownCell(desired), markdownCell(kubelet))
		}
	}
	fmt.Fprintln(&out)

	if configured != nil && policyReport != nil {
		renderMarkdownPolicy(&out, *configured, *policyReport)
	}

	fmt.Fprintln(&out, "## Timeline")
	fmt.Fprintln(&out)
	if len(timelineReport.Events) == 0 {
		fmt.Fprintln(&out, "No lifecycle transitions were reconstructed from the recorded observations.")
	} else {
		fmt.Fprintln(&out, "| Time or observation interval | Resource | Event |")
		fmt.Fprintln(&out, "| --- | --- | --- |")
		for _, event := range timelineReport.Events {
			resource := event.ResourceKind + "/" + event.ResourceName
			fmt.Fprintf(&out, "| %s | %s | %s |\n",
				markdownCell(timelineEventTime(event)), markdownCell(resource), markdownCell(event.Summary))
		}
	}
	fmt.Fprintln(&out)

	fmt.Fprintln(&out, "## Scope")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "- The report is derived only from observations present in this run.")
	fmt.Fprintln(&out, "- PASS does not establish overall cluster health or complete event delivery.")
	fmt.Fprintln(&out, "- Missing evidence, ambiguous correlation, and sampling gaps remain INCONCLUSIVE where the underlying contracts require it.")
	fmt.Fprintln(&out, "- Policy thresholds, when supplied, come from the policy source and are not treated as OpenShift guarantees.")

	return out.Bytes(), nil
}

func renderMarkdownPolicy(out *bytes.Buffer, configured policy.UpgradePolicy, report result.Report) {
	fmt.Fprintln(out, "## Lifecycle policy")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "| Field | Value |")
	fmt.Fprintln(out, "| --- | --- |")
	markdownRow(out, "Verdict", string(report.Verdict))
	markdownRow(out, "Source", configured.Source)
	markdownRow(out, "Target version", configured.TargetVersion)
	if configured.TargetImage != "" {
		markdownRow(out, "Target image", configured.TargetImage)
	}
	if configured.MaxObservationGap.Set() {
		markdownRow(out, "Maximum observation gap", configured.MaxObservationGap.Duration.String())
	}
	fmt.Fprintln(out)

	type row struct {
		kind, name, contract string
		verdict              result.Verdict
	}
	var rows []row
	for _, item := range report.Operators {
		for _, contract := range item.Contracts {
			rows = append(rows, row{"ClusterOperator", item.Name, contract.Name, contract.Verdict})
		}
	}
	for _, item := range report.MachineConfigPools {
		for _, contract := range item.Contracts {
			rows = append(rows, row{"MachineConfigPool", item.Name, contract.Name, contract.Verdict})
		}
	}
	for _, item := range report.Nodes {
		for _, contract := range item.Contracts {
			rows = append(rows, row{"Node", item.Name, contract.Name, contract.Verdict})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].kind != rows[j].kind {
			return rows[i].kind < rows[j].kind
		}
		if rows[i].name != rows[j].name {
			return rows[i].name < rows[j].name
		}
		return rows[i].contract < rows[j].contract
	})
	if len(rows) == 0 {
		fmt.Fprintln(out, "No lifecycle policy contracts were applicable to recorded resources.")
		fmt.Fprintln(out)
		return
	}
	fmt.Fprintln(out, "| Resource | Contract | Verdict |")
	fmt.Fprintln(out, "| --- | --- | --- |")
	for _, item := range rows {
		fmt.Fprintf(out, "| %s | %s | %s |\n",
			markdownCell(item.kind+"/"+item.name), markdownCell(item.contract), item.verdict)
	}
	fmt.Fprintln(out)
}

func markdownRow(out *bytes.Buffer, field, value string) {
	fmt.Fprintf(out, "| %s | %s |\n", markdownCell(field), markdownCell(value))
}

func markdownCell(value string) string {
	value = html.EscapeString(value)
	value = strings.ReplaceAll(value, "\\", "\\\\")
	for _, character := range []string{"`", "*", "_", "[", "]"} {
		value = strings.ReplaceAll(value, character, "\\"+character)
	}
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
