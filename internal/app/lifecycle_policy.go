package app

import (
	"fmt"
	"os"
	"sort"

	"github.com/nightingale-develop/reconcile-guard/internal/contracts"
	"github.com/nightingale-develop/reconcile-guard/internal/policy"
	"github.com/nightingale-develop/reconcile-guard/internal/result"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
	configv1 "github.com/openshift/api/config/v1"
)

func (c cli) verifyLifecyclePolicy(args []string) int {
	if len(args) != 3 {
		fmt.Fprintln(c.stderr, "Usage: reconcile-guard verify-lifecycle-policy <run-directory> <policy.yaml>")
		return 1
	}
	input, err := loadRunInput(args[1])
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}
	data, err := os.ReadFile(args[2])
	if err != nil {
		fmt.Fprintln(c.stderr, "Error: read policy:", err)
		return 1
	}
	configured, err := policy.Parse(data)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}
	report, err := evaluateLifecyclePolicy(input, configured)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}
	if c.output == outputJSON {
		if err := c.writeJSON("verify-lifecycle-policy", report); err != nil {
			fmt.Fprintln(c.stderr, "Error:", err)
			return 1
		}
	} else {
		c.printLifecyclePolicyReport(configured, report)
	}
	return policyExitCode(report.Verdict)
}

func evaluateLifecyclePolicy(input runInput, configured policy.UpgradePolicy) (result.Report, error) {
	if _, err := upgrade.AnalyzePhases(input.Versions); err != nil {
		return result.Report{}, fmt.Errorf("ClusterVersion timeline: %w", err)
	}
	report := result.Report{
		Verdict: result.VerdictInconclusive,
		Details: &result.Details{
			Counts: map[string]int{},
			Values: map[string]string{
				"policyApiVersion": configured.APIVersion,
				"policyKind":       configured.Kind,
				"targetVersion":    configured.TargetVersion,
				"targetImage":      configured.TargetImage,
				"thresholdSource":  configured.Source,
			},
		},
	}
	if configured.MaxObservationGap.Set() {
		report.Details.Values["maximumObservationGap"] = configured.MaxObservationGap.Duration.String()
	}

	applied := 0
	missingExplicit := 0
	missingDefaultScopes := 0

	seenOperators := map[string]bool{}
	for i, history := range input.Histories {
		if len(history) == 0 {
			return result.Report{}, fmt.Errorf("operator history %d is empty", i+1)
		}
		name := history[0].Operator.Name
		seenOperators[name] = true
		rules := configured.OperatorRulesFor(name)
		var resourceContracts []result.Contract
		appendOperatorRule := func(rule *policy.DurationRule, contractName string, condition configv1.ClusterStatusConditionType, adverse configv1.ConditionStatus) error {
			if rule == nil {
				return nil
			}
			policyReport, err := contracts.VerifyOperatorConditionPolicy(input.Versions, history, contracts.OperatorConditionPolicy{
				Contract:          contractName,
				Condition:         condition,
				AdverseStatus:     adverse,
				Limit:             rule.MaxObservedDuration.Duration,
				MaxObservationGap: configured.MaxObservationGap.Duration,
				Operator:          name,
				TargetVersion:     configured.TargetVersion,
				TargetImage:       configured.TargetImage,
				Source:            configured.Source,
			})
			if err != nil {
				return err
			}
			resourceContracts = append(resourceContracts, contracts.OperatorConditionPolicyResult(policyReport))
			applied++
			return nil
		}
		if err := appendOperatorRule(rules.AvailabilityLoss, contracts.OperatorAvailabilityLossPolicyContract, configv1.OperatorAvailable, configv1.ConditionFalse); err != nil {
			return result.Report{}, fmt.Errorf("operator %s availabilityLoss: %w", name, err)
		}
		if err := appendOperatorRule(rules.Degraded, contracts.OperatorDegradedPolicyContract, configv1.OperatorDegraded, configv1.ConditionTrue); err != nil {
			return result.Report{}, fmt.Errorf("operator %s degraded: %w", name, err)
		}
		if err := appendOperatorRule(rules.Progressing, contracts.OperatorProgressingPolicyContract, configv1.OperatorProgressing, configv1.ConditionTrue); err != nil {
			return result.Report{}, fmt.Errorf("operator %s progressing: %w", name, err)
		}
		if len(resourceContracts) > 0 {
			report.Operators = append(report.Operators, result.OperatorResult{Name: name, Verdict: aggregateContracts(resourceContracts), Contracts: resourceContracts})
		}
	}
	for name := range configured.Operators {
		if !seenOperators[name] {
			report.Operators = append(report.Operators, result.OperatorResult{Name: name, Verdict: result.VerdictInconclusive, Contracts: []result.Contract{missingPolicyResourceContract("operator", name, configured.Source)}})
			missingExplicit++
			applied++
		}
	}

	seenPools := map[string]bool{}
	if configured.Defaults.MachineConfigPool != nil && len(input.MachineConfigPools) == 0 {
		missingDefaultScopes++
	}
	for i, history := range input.MachineConfigPools {
		if len(history) == 0 {
			return result.Report{}, fmt.Errorf("MachineConfigPool history %d is empty", i+1)
		}
		name := history[0].Pool.Name
		seenPools[name] = true
		rule, ok := configured.MachineConfigPoolRuleFor(name)
		if !ok {
			continue
		}
		policyReport, err := contracts.VerifyMachineConfigPoolPostCompletionPolicy(input.Versions, history, configured.TargetVersion, configured.TargetImage, configured.Source, rule.PostCompletionGracePeriod.Duration)
		if err != nil {
			return result.Report{}, fmt.Errorf("MachineConfigPool %s: %w", name, err)
		}
		contract := contracts.PostCompletionPolicyResult(policyReport)
		report.MachineConfigPools = append(report.MachineConfigPools, result.ResourceResult{Name: name, Verdict: contract.Verdict, Contracts: []result.Contract{contract}})
		applied++
	}
	for name := range configured.MachineConfigPools {
		if !seenPools[name] {
			report.MachineConfigPools = append(report.MachineConfigPools, result.ResourceResult{Name: name, Verdict: result.VerdictInconclusive, Contracts: []result.Contract{missingPolicyResourceContract("MachineConfigPool", name, configured.Source)}})
			missingExplicit++
			applied++
		}
	}

	seenNodes := map[string]bool{}
	if configured.Defaults.Node != nil && len(input.Nodes) == 0 {
		missingDefaultScopes++
	}
	for i, history := range input.Nodes {
		if len(history) == 0 {
			return result.Report{}, fmt.Errorf("Node history %d is empty", i+1)
		}
		name := history[0].Node.Name
		seenNodes[name] = true
		rule, ok := configured.NodeRuleFor(name)
		if !ok {
			continue
		}
		var resourceContracts []result.Contract
		if rule.ReadyPostCompletionGracePeriod.Set() {
			policyReport, err := contracts.VerifyNodeReadyPostCompletionPolicy(input.Versions, history, configured.TargetVersion, configured.TargetImage, configured.Source, rule.ReadyPostCompletionGracePeriod.Duration)
			if err != nil {
				return result.Report{}, fmt.Errorf("Node %s Ready: %w", name, err)
			}
			resourceContracts = append(resourceContracts, contracts.PostCompletionPolicyResult(policyReport))
			applied++
		}
		if rule.ConfigAlignedPostCompletionGracePeriod.Set() {
			policyReport, err := contracts.VerifyNodeConfigPostCompletionPolicy(input.Versions, history, configured.TargetVersion, configured.TargetImage, configured.Source, rule.ConfigAlignedPostCompletionGracePeriod.Duration)
			if err != nil {
				return result.Report{}, fmt.Errorf("Node %s config alignment: %w", name, err)
			}
			resourceContracts = append(resourceContracts, contracts.PostCompletionPolicyResult(policyReport))
			applied++
		}
		if len(resourceContracts) > 0 {
			report.Nodes = append(report.Nodes, result.ResourceResult{Name: name, Verdict: aggregateContracts(resourceContracts), Contracts: resourceContracts})
		}
	}
	for name := range configured.Nodes {
		if !seenNodes[name] {
			report.Nodes = append(report.Nodes, result.ResourceResult{Name: name, Verdict: result.VerdictInconclusive, Contracts: []result.Contract{missingPolicyResourceContract("Node", name, configured.Source)}})
			missingExplicit++
			applied++
		}
	}

	sort.Slice(report.Operators, func(i, j int) bool { return report.Operators[i].Name < report.Operators[j].Name })
	sort.Slice(report.MachineConfigPools, func(i, j int) bool { return report.MachineConfigPools[i].Name < report.MachineConfigPools[j].Name })
	sort.Slice(report.Nodes, func(i, j int) bool { return report.Nodes[i].Name < report.Nodes[j].Name })

	report.Details.Counts["appliedContracts"] = applied
	report.Details.Counts["missingExplicitResources"] = missingExplicit
	report.Details.Counts["missingDefaultScopes"] = missingDefaultScopes
	report.Verdict = aggregatePolicyReport(report, applied)
	if report.Verdict == result.VerdictPass && missingDefaultScopes > 0 {
		report.Verdict = result.VerdictInconclusive
	}
	return report, nil
}

func missingPolicyResourceContract(kind, name, source string) result.Contract {
	return result.Contract{
		Name:    "lifecycle-policy-resource-presence",
		Verdict: result.VerdictInconclusive,
		Evidence: []result.Evidence{{
			Kind:    "missing-policy-resource",
			Verdict: result.VerdictInconclusive,
			Reason:  fmt.Sprintf("policy explicitly names %s %q but the run contains no matching history", kind, name),
			Source:  source,
		}},
	}
}

func aggregateContracts(contracts []result.Contract) result.Verdict {
	verdict := result.VerdictPass
	for _, contract := range contracts {
		if contract.Verdict == result.VerdictFail {
			return result.VerdictFail
		}
		if contract.Verdict == result.VerdictInconclusive {
			verdict = result.VerdictInconclusive
		}
	}
	return verdict
}

func aggregatePolicyReport(report result.Report, applied int) result.Verdict {
	if applied == 0 {
		return result.VerdictInconclusive
	}
	verdict := result.VerdictPass
	for _, operator := range report.Operators {
		if operator.Verdict == result.VerdictFail {
			return result.VerdictFail
		}
		if operator.Verdict == result.VerdictInconclusive {
			verdict = result.VerdictInconclusive
		}
	}
	for _, resource := range append(append([]result.ResourceResult{}, report.MachineConfigPools...), report.Nodes...) {
		if resource.Verdict == result.VerdictFail {
			return result.VerdictFail
		}
		if resource.Verdict == result.VerdictInconclusive {
			verdict = result.VerdictInconclusive
		}
	}
	return verdict
}

func policyExitCode(verdict result.Verdict) int {
	switch verdict {
	case result.VerdictPass:
		return 0
	case result.VerdictFail:
		return 2
	case result.VerdictInconclusive:
		return 3
	default:
		return 1
	}
}

func (c cli) printLifecyclePolicyReport(configured policy.UpgradePolicy, report result.Report) {
	fmt.Fprintln(c.stdout, "Policy: explicit user-supplied lifecycle policy")
	fmt.Fprintln(c.stdout, "API:", configured.APIVersion, configured.Kind)
	fmt.Fprintln(c.stdout, "Threshold source:", configured.Source)
	fmt.Fprintln(c.stdout, "Target version:", configured.TargetVersion)
	if configured.TargetImage != "" {
		fmt.Fprintln(c.stdout, "Target image:", configured.TargetImage)
	}
	if configured.MaxObservationGap.Set() {
		fmt.Fprintln(c.stdout, "Maximum observation gap:", configured.MaxObservationGap.Duration)
	}
	fmt.Fprintln(c.stdout, "Verdict:", report.Verdict)
	if report.Details != nil && report.Details.Counts["missingDefaultScopes"] > 0 {
		fmt.Fprintln(c.stdout, "Missing recorded resource scopes required by defaults:", report.Details.Counts["missingDefaultScopes"])
	}

	if len(report.Operators) > 0 {
		fmt.Fprintln(c.stdout, "Operators:")
		for _, item := range report.Operators {
			fmt.Fprintf(c.stdout, "  %s: %s\n", item.Name, item.Verdict)
			for _, contract := range item.Contracts {
				fmt.Fprintf(c.stdout, "    %s: %s\n", contract.Name, contract.Verdict)
				printPolicyContractDetails(c, contract)
			}
		}
	}
	if len(report.MachineConfigPools) > 0 {
		fmt.Fprintln(c.stdout, "MachineConfigPools:")
		for _, item := range report.MachineConfigPools {
			fmt.Fprintf(c.stdout, "  %s: %s\n", item.Name, item.Verdict)
			for _, contract := range item.Contracts {
				fmt.Fprintf(c.stdout, "    %s: %s\n", contract.Name, contract.Verdict)
				printPolicyContractDetails(c, contract)
			}
		}
	}
	if len(report.Nodes) > 0 {
		fmt.Fprintln(c.stdout, "Nodes:")
		for _, item := range report.Nodes {
			fmt.Fprintf(c.stdout, "  %s: %s\n", item.Name, item.Verdict)
			for _, contract := range item.Contracts {
				fmt.Fprintf(c.stdout, "    %s: %s\n", contract.Name, contract.Verdict)
				printPolicyContractDetails(c, contract)
			}
		}
	}
	fmt.Fprintln(c.stdout, "Scope: thresholds come from the supplied policy, not from an OpenShift guarantee. Sampling gaps and missing evidence remain INCONCLUSIVE.")
}

func printPolicyContractDetails(c cli, contract result.Contract) {
	if contract.Details == nil {
		return
	}
	if value := contract.Details.Values["maximumObservedDuration"]; value != "" {
		fmt.Fprintln(c.stdout, "      maximum observed duration:", value)
	}
	if value := contract.Details.Values["postCompletionGracePeriod"]; value != "" {
		fmt.Fprintln(c.stdout, "      post-completion grace period:", value)
	}
	if count, ok := contract.Details.Counts["evaluatedSamples"]; ok {
		fmt.Fprintln(c.stdout, "      evaluated samples:", count)
	}
	if count, ok := contract.Details.Counts["violatingSamples"]; ok {
		fmt.Fprintln(c.stdout, "      violating samples:", count)
	}
}
