package app

import (
	"fmt"

	"github.com/nightingale-develop/reconcile-guard/internal/contracts"
	"github.com/nightingale-develop/reconcile-guard/internal/result"
)

type auxiliaryRunReport struct {
	MachineConfigPools []contracts.MachineConfigPoolEvidenceReport
	Nodes              []contracts.NodeEvidenceReport
}

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
	auxiliary, err := verifyAuxiliaryRunEvidence(input)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}

	if c.output == outputJSON {
		resultReport := contracts.ClusterUpgradeResult(report)
		for _, pool := range auxiliary.MachineConfigPools {
			contract := contracts.MachineConfigPoolLifecycleResult(pool)
			resultReport.MachineConfigPools = append(resultReport.MachineConfigPools, result.ResourceResult{
				Name:      pool.Pool,
				Verdict:   contract.Verdict,
				Contracts: []result.Contract{contract},
			})
		}
		for _, node := range auxiliary.Nodes {
			contract := contracts.NodeLifecycleResult(node)
			resultReport.Nodes = append(resultReport.Nodes, result.ResourceResult{
				Name:      node.Node,
				Verdict:   contract.Verdict,
				Contracts: []result.Contract{contract},
			})
		}
		if err := c.writeJSON("verify-run", resultReport); err != nil {
			fmt.Fprintln(c.stderr, "Error:", err)
			return 1
		}
	} else {
		fmt.Fprintln(c.stdout, "Run:", input.Manifest.RunID)
		fmt.Fprintln(c.stdout, "Cluster ID:", input.Manifest.Source.ClusterID)
		c.printClusterUpgradeReport(report)
		c.printAuxiliaryRunReport(auxiliary)
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

func verifyAuxiliaryRunEvidence(input runInput) (auxiliaryRunReport, error) {
	var report auxiliaryRunReport
	for i, history := range input.MachineConfigPools {
		result, err := contracts.VerifyMachineConfigPoolLifecycleEvidence(input.Versions, history)
		if err != nil {
			return auxiliaryRunReport{}, fmt.Errorf("MachineConfigPool history %d: %w", i+1, err)
		}
		report.MachineConfigPools = append(report.MachineConfigPools, result)
	}
	for i, history := range input.Nodes {
		result, err := contracts.VerifyNodeLifecycleEvidence(input.Versions, history)
		if err != nil {
			return auxiliaryRunReport{}, fmt.Errorf("Node history %d: %w", i+1, err)
		}
		report.Nodes = append(report.Nodes, result)
	}
	return report, nil
}

func (c cli) printAuxiliaryRunReport(report auxiliaryRunReport) {
	if len(report.MachineConfigPools) == 0 && len(report.Nodes) == 0 {
		return
	}

	fmt.Fprintln(c.stdout)
	fmt.Fprintln(c.stdout, "MachineConfigPool lifecycle evidence:")
	if len(report.MachineConfigPools) == 0 {
		fmt.Fprintln(c.stdout, "  none recorded")
	}
	for _, pool := range report.MachineConfigPools {
		fmt.Fprintf(c.stdout, "  Pool: %s\n", pool.Pool)
		fmt.Fprintf(c.stdout, "    %s: %s\n", pool.Contract, pool.Verdict)
		fmt.Fprintf(c.stdout, "    Observations: %d, evaluated post-completion: %d, converged: %d, uncertain: %d\n", pool.Observations, pool.EvaluatedSamples, pool.ConvergedSamples, pool.UncertainSamples)
		if len(pool.Evidence) > 0 {
			last := pool.Evidence[len(pool.Evidence)-1].State
			fmt.Fprintf(c.stdout, "    Last state: %s current=%q desired=%q updated=%d/%d ready=%d/%d degraded=%d\n", last.Phase, last.CurrentConfiguration, last.DesiredConfiguration, last.UpdatedMachineCount, last.MachineCount, last.ReadyMachineCount, last.MachineCount, last.DegradedMachineCount)
		}
	}

	fmt.Fprintln(c.stdout, "Node lifecycle evidence:")
	if len(report.Nodes) == 0 {
		fmt.Fprintln(c.stdout, "  none recorded")
	}
	for _, node := range report.Nodes {
		fmt.Fprintf(c.stdout, "  Node: %s\n", node.Node)
		fmt.Fprintf(c.stdout, "    %s: %s\n", node.Contract, node.Verdict)
		fmt.Fprintf(c.stdout, "    Observations: %d, evaluated post-completion: %d, converged: %d, uncertain: %d\n", node.Observations, node.EvaluatedSamples, node.ConvergedSamples, node.UncertainSamples)
		if len(node.Evidence) > 0 {
			last := node.Evidence[len(node.Evidence)-1].State
			fmt.Fprintf(c.stdout, "    Last state: Ready=%s currentConfig=%q desiredConfig=%q aligned=%t kubelet=%q\n", last.Ready, last.CurrentMachineConfig, last.DesiredMachineConfig, last.ConfigAligned, last.KubeletVersion)
		}
	}
	fmt.Fprintln(c.stdout, "Auxiliary MCP/Node verdicts are evidence-only and do not change the aggregate operator verdict.")
}
