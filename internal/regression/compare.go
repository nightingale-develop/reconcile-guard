package regression

import (
	"fmt"
	"sort"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/contracts"
	"github.com/nightingale-develop/reconcile-guard/internal/result"
)

const (
	conditionsContract = "normal-upgrade-operator-conditions"
	versionContract    = "operator-version-consistency"
)

type Evidence struct {
	Verification       contracts.ClusterUpgradeReport
	MachineConfigPools []contracts.MachineConfigPoolEvidenceReport
	Nodes              []contracts.NodeEvidenceReport
	Timings            []ObservedTiming
	Policy             *result.Report
}

func Compare(baseline, candidate contracts.ClusterUpgradeReport) (Report, error) {
	return CompareEvidence(
		Evidence{Verification: baseline},
		Evidence{Verification: candidate},
	)
}

func CompareEvidence(baseline, candidate Evidence) (Report, error) {
	report, err := compareVerification(baseline.Verification, candidate.Verification)
	if err != nil {
		return Report{}, err
	}

	if err := compareAuxiliary(&report, baseline, candidate); err != nil {
		return Report{}, err
	}

	report.Timings, err = compareTimings(baseline.Timings, candidate.Timings)
	if err != nil {
		return Report{}, err
	}

	if (baseline.Policy == nil) != (candidate.Policy == nil) {
		return Report{}, fmt.Errorf("lifecycle policy evidence must be supplied for both runs or neither run")
	}
	if baseline.Policy != nil {
		policyComparison, err := comparePolicyReports(*baseline.Policy, *candidate.Policy)
		if err != nil {
			return Report{}, err
		}
		report.Policy = &policyComparison
		report.Verdict = mergeComparisonVerdicts(report.Verdict, policyComparison.Verdict)
	}

	return report, nil
}

func compareVerification(baseline, candidate contracts.ClusterUpgradeReport) (Report, error) {
	before, err := indexOperators(baseline)
	if err != nil {
		return Report{}, fmt.Errorf("baseline: %w", err)
	}
	after, err := indexOperators(candidate)
	if err != nil {
		return Report{}, fmt.Errorf("candidate: %w", err)
	}
	report := Report{
		Verdict:   result.VerdictPass,
		Baseline:  RunSummary{VerificationVerdict: baseline.Verdict},
		Candidate: RunSummary{VerificationVerdict: candidate.Verdict},
		Scope: Scope{
			BaselineOnlyOperators:           []string{},
			CandidateOnlyOperators:          []string{},
			BaselineOnlyMachineConfigPools:  []string{},
			CandidateOnlyMachineConfigPools: []string{},
			BaselineOnlyNodes:               []string{},
			CandidateOnlyNodes:              []string{},
		},
		Operators: []OperatorComparison{},
	}
	common := make([]string, 0)
	for name := range before {
		if _, ok := after[name]; ok {
			common = append(common, name)
		} else {
			report.Scope.BaselineOnlyOperators = append(report.Scope.BaselineOnlyOperators, name)
		}
	}
	for name := range after {
		if _, ok := before[name]; !ok {
			report.Scope.CandidateOnlyOperators = append(report.Scope.CandidateOnlyOperators, name)
		}
	}
	sort.Strings(common)
	sort.Strings(report.Scope.BaselineOnlyOperators)
	sort.Strings(report.Scope.CandidateOnlyOperators)
	report.Scope.CommonOperators = len(common)
	if len(common) == 0 || len(report.Scope.BaselineOnlyOperators) != 0 || len(report.Scope.CandidateOnlyOperators) != 0 {
		report.Verdict = result.VerdictInconclusive
	}
	for _, name := range common {
		b, c := before[name], after[name]
		comparison := OperatorComparison{
			Name:             name,
			BaselineVerdict:  b.Verdict,
			CandidateVerdict: c.Verdict,
			Contracts: []ContractComparison{
				compareContract(conditionsContract, b.Conditions.Verdict, c.Conditions.Verdict),
				compareContract(versionContract, b.Version.Verdict, c.Version.Verdict),
			},
		}
		comparison.Verdict = comparisonVerdict(comparison.Contracts)
		report.Verdict = mergeComparisonVerdicts(report.Verdict, comparison.Verdict)
		report.Operators = append(report.Operators, comparison)
	}
	return report, nil
}

func compareAuxiliary(report *Report, baseline, candidate Evidence) error {
	beforePools, err := indexPoolEvidence(baseline.MachineConfigPools)
	if err != nil {
		return fmt.Errorf("baseline MachineConfigPools: %w", err)
	}
	afterPools, err := indexPoolEvidence(candidate.MachineConfigPools)
	if err != nil {
		return fmt.Errorf("candidate MachineConfigPools: %w", err)
	}
	poolCommon, poolBeforeOnly, poolAfterOnly := compareNameScope(beforePools, afterPools)
	report.Scope.CommonMachineConfigPools = len(poolCommon)
	report.Scope.BaselineOnlyMachineConfigPools = poolBeforeOnly
	report.Scope.CandidateOnlyMachineConfigPools = poolAfterOnly
	for _, name := range poolCommon {
		before, after := beforePools[name], afterPools[name]
		contract := compareContract(contracts.MachineConfigPoolLifecycleContract, before.Verdict, after.Verdict)
		report.MachineConfigPools = append(report.MachineConfigPools, ResourceComparison{
			Name:             name,
			BaselineVerdict:  before.Verdict,
			CandidateVerdict: after.Verdict,
			Verdict:          comparisonVerdict([]ContractComparison{contract}),
			Contracts:        []ContractComparison{contract},
		})
	}

	beforeNodes, err := indexNodeEvidence(baseline.Nodes)
	if err != nil {
		return fmt.Errorf("baseline Nodes: %w", err)
	}
	afterNodes, err := indexNodeEvidence(candidate.Nodes)
	if err != nil {
		return fmt.Errorf("candidate Nodes: %w", err)
	}
	nodeCommon, nodeBeforeOnly, nodeAfterOnly := compareNameScope(beforeNodes, afterNodes)
	report.Scope.CommonNodes = len(nodeCommon)
	report.Scope.BaselineOnlyNodes = nodeBeforeOnly
	report.Scope.CandidateOnlyNodes = nodeAfterOnly
	for _, name := range nodeCommon {
		before, after := beforeNodes[name], afterNodes[name]
		contract := compareContract(contracts.NodeLifecycleContract, before.Verdict, after.Verdict)
		report.Nodes = append(report.Nodes, ResourceComparison{
			Name:             name,
			BaselineVerdict:  before.Verdict,
			CandidateVerdict: after.Verdict,
			Verdict:          comparisonVerdict([]ContractComparison{contract}),
			Contracts:        []ContractComparison{contract},
		})
	}

	// MCP/Node checks from verify-run are evidence-only. Their comparison is
	// reported, but it does not create a regression verdict without an
	// explicit lifecycle policy.
	return nil
}

func comparePolicyReports(baseline, candidate result.Report) (PolicyComparison, error) {
	if !validVerdict(baseline.Verdict) {
		return PolicyComparison{}, fmt.Errorf("baseline policy: invalid verdict %q", baseline.Verdict)
	}
	if !validVerdict(candidate.Verdict) {
		return PolicyComparison{}, fmt.Errorf("candidate policy: invalid verdict %q", candidate.Verdict)
	}

	comparison := PolicyComparison{
		BaselineVerdict:  baseline.Verdict,
		CandidateVerdict: candidate.Verdict,
		Verdict:          result.VerdictPass,
		Operators:        []OperatorComparison{},
	}
	if baseline.Verdict == result.VerdictInconclusive || candidate.Verdict == result.VerdictInconclusive {
		comparison.Verdict = result.VerdictInconclusive
	}

	operators, err := comparePolicyOperators(baseline.Operators, candidate.Operators)
	if err != nil {
		return PolicyComparison{}, err
	}
	pools, err := comparePolicyResources("MachineConfigPool", baseline.MachineConfigPools, candidate.MachineConfigPools)
	if err != nil {
		return PolicyComparison{}, err
	}
	nodes, err := comparePolicyResources("Node", baseline.Nodes, candidate.Nodes)
	if err != nil {
		return PolicyComparison{}, err
	}
	comparison.Operators = operators
	comparison.MachineConfigPools = pools
	comparison.Nodes = nodes

	compared := 0
	for _, item := range comparison.Operators {
		compared++
		comparison.Verdict = mergeComparisonVerdicts(comparison.Verdict, item.Verdict)
	}
	for _, group := range [][]ResourceComparison{comparison.MachineConfigPools, comparison.Nodes} {
		for _, item := range group {
			compared++
			comparison.Verdict = mergeComparisonVerdicts(comparison.Verdict, item.Verdict)
		}
	}
	if compared == 0 {
		comparison.Verdict = result.VerdictInconclusive
	}
	return comparison, nil
}

func comparePolicyOperators(baseline, candidate []result.OperatorResult) ([]OperatorComparison, error) {
	before, err := indexPolicyOperators(baseline)
	if err != nil {
		return nil, fmt.Errorf("baseline policy operators: %w", err)
	}
	after, err := indexPolicyOperators(candidate)
	if err != nil {
		return nil, fmt.Errorf("candidate policy operators: %w", err)
	}
	names := unionNames(before, after)
	items := make([]OperatorComparison, 0, len(names))
	for _, name := range names {
		b, bok := before[name]
		c, cok := after[name]
		contracts := compareResultContracts(contractSlice(b.Contracts, bok), contractSlice(c.Contracts, cok))
		item := OperatorComparison{Name: name, Contracts: contracts, Verdict: comparisonVerdict(contracts)}
		if bok {
			item.BaselineVerdict = b.Verdict
		}
		if cok {
			item.CandidateVerdict = c.Verdict
		}
		items = append(items, item)
	}
	return items, nil
}

func comparePolicyResources(kind string, baseline, candidate []result.ResourceResult) ([]ResourceComparison, error) {
	before, err := indexPolicyResources(baseline)
	if err != nil {
		return nil, fmt.Errorf("baseline policy %s resources: %w", kind, err)
	}
	after, err := indexPolicyResources(candidate)
	if err != nil {
		return nil, fmt.Errorf("candidate policy %s resources: %w", kind, err)
	}
	names := unionNames(before, after)
	items := make([]ResourceComparison, 0, len(names))
	for _, name := range names {
		b, bok := before[name]
		c, cok := after[name]
		contracts := compareResultContracts(contractSlice(b.Contracts, bok), contractSlice(c.Contracts, cok))
		item := ResourceComparison{Name: name, Contracts: contracts, Verdict: comparisonVerdict(contracts)}
		if bok {
			item.BaselineVerdict = b.Verdict
		}
		if cok {
			item.CandidateVerdict = c.Verdict
		}
		items = append(items, item)
	}
	return items, nil
}

func compareResultContracts(baseline, candidate []result.Contract) []ContractComparison {
	before := make(map[string]result.Contract, len(baseline))
	after := make(map[string]result.Contract, len(candidate))
	for _, contract := range baseline {
		before[contract.Name] = contract
	}
	for _, contract := range candidate {
		after[contract.Name] = contract
	}
	names := unionNames(before, after)
	comparisons := make([]ContractComparison, 0, len(names))
	for _, name := range names {
		b, bok := before[name]
		c, cok := after[name]
		comparison := compareContract(name, contractVerdict(b, bok), contractVerdict(c, cok))
		baselineSpan, baselineOK := maximumObservedSpan(b, bok)
		candidateSpan, candidateOK := maximumObservedSpan(c, cok)
		if baselineOK {
			comparison.BaselineObservedSpan = baselineSpan.String()
		}
		if candidateOK {
			comparison.CandidateObservedSpan = candidateSpan.String()
		}
		if baselineOK && candidateOK {
			comparison.ObservedSpanDelta = formatSignedDuration(candidateSpan - baselineSpan)
		}
		comparisons = append(comparisons, comparison)
	}
	return comparisons
}

func contractVerdict(contract result.Contract, present bool) result.Verdict {
	if !present {
		return ""
	}
	return contract.Verdict
}

func maximumObservedSpan(contract result.Contract, present bool) (time.Duration, bool) {
	if !present {
		return 0, false
	}
	var maximum time.Duration
	found := false
	for _, evidence := range contract.Evidence {
		raw := evidence.Attributes["observedSpan"]
		if raw == "" {
			continue
		}
		value, err := time.ParseDuration(raw)
		if err != nil || value < 0 {
			continue
		}
		if !found || value > maximum {
			maximum = value
			found = true
		}
	}
	return maximum, found
}

func compareTimings(baseline, candidate []ObservedTiming) ([]TimingComparison, error) {
	before, err := indexTimings(baseline)
	if err != nil {
		return nil, fmt.Errorf("baseline timings: %w", err)
	}
	after, err := indexTimings(candidate)
	if err != nil {
		return nil, fmt.Errorf("candidate timings: %w", err)
	}
	keys := unionNames(before, after)
	comparisons := make([]TimingComparison, 0, len(keys))
	for _, key := range keys {
		b, bok := before[key]
		c, cok := after[key]
		item := TimingComparison{}
		if bok {
			item.Name = b.Name
			item.ResourceKind = b.ResourceKind
			item.ResourceName = b.ResourceName
			value := timingValue(b)
			item.Baseline = &value
		}
		if cok {
			if !bok {
				item.Name = c.Name
				item.ResourceKind = c.ResourceKind
				item.ResourceName = c.ResourceName
			}
			value := timingValue(c)
			item.Candidate = &value
		}
		if bok && cok {
			item.Delta = formatSignedDuration(c.To.Sub(c.From) - b.To.Sub(b.From))
		}
		comparisons = append(comparisons, item)
	}
	return comparisons, nil
}

func timingValue(item ObservedTiming) TimingValue {
	return TimingValue{From: item.From.UTC(), To: item.To.UTC(), Duration: item.To.Sub(item.From).String()}
}

func formatSignedDuration(value time.Duration) string {
	if value > 0 {
		return "+" + value.String()
	}
	return value.String()
}

func indexTimings(items []ObservedTiming) (map[string]ObservedTiming, error) {
	result := make(map[string]ObservedTiming, len(items))
	for i, item := range items {
		if item.Name == "" || item.ResourceKind == "" {
			return nil, fmt.Errorf("timing %d: name and resource kind are required", i+1)
		}
		if item.From.IsZero() || item.To.IsZero() || item.To.Before(item.From) {
			return nil, fmt.Errorf("timing %s/%s %q has invalid observation bounds", item.ResourceKind, item.ResourceName, item.Name)
		}
		key := timingKey(item)
		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("duplicate timing %s/%s %q", item.ResourceKind, item.ResourceName, item.Name)
		}
		result[key] = item
	}
	return result, nil
}

func timingKey(item ObservedTiming) string {
	return item.ResourceKind + "\x00" + item.ResourceName + "\x00" + item.Name
}

func compareContract(name string, baseline, candidate result.Verdict) ContractComparison {
	change := ChangeInconclusive
	switch {
	case baseline == "" || candidate == "", baseline == result.VerdictInconclusive || candidate == result.VerdictInconclusive:
	case baseline == candidate:
		change = ChangeUnchanged
	case baseline == result.VerdictPass && candidate == result.VerdictFail:
		change = ChangeRegression
	case baseline == result.VerdictFail && candidate == result.VerdictPass:
		change = ChangeImprovement
	}
	return ContractComparison{Name: name, BaselineVerdict: baseline, CandidateVerdict: candidate, Change: change}
}

func comparisonVerdict(contracts []ContractComparison) result.Verdict {
	if len(contracts) == 0 {
		return result.VerdictInconclusive
	}
	verdict := result.VerdictPass
	for _, contract := range contracts {
		switch contract.Change {
		case ChangeRegression:
			return result.VerdictFail
		case ChangeInconclusive:
			verdict = result.VerdictInconclusive
		}
	}
	return verdict
}

func mergeComparisonVerdicts(current, next result.Verdict) result.Verdict {
	if current == result.VerdictFail || next == result.VerdictFail {
		return result.VerdictFail
	}
	if current == result.VerdictInconclusive || next == result.VerdictInconclusive {
		return result.VerdictInconclusive
	}
	return result.VerdictPass
}

func indexOperators(report contracts.ClusterUpgradeReport) (map[string]contracts.OperatorUpgradeReport, error) {
	if !validVerdict(report.Verdict) {
		return nil, fmt.Errorf("invalid verification verdict %q", report.Verdict)
	}
	operators := make(map[string]contracts.OperatorUpgradeReport, len(report.Operators))
	for _, operator := range report.Operators {
		if operator.Operator == "" {
			return nil, fmt.Errorf("operator name is missing")
		}
		if _, exists := operators[operator.Operator]; exists {
			return nil, fmt.Errorf("duplicate operator %q", operator.Operator)
		}
		if !validVerdict(operator.Verdict) {
			return nil, fmt.Errorf("operator %q: invalid verdict %q", operator.Operator, operator.Verdict)
		}
		for _, contract := range []struct {
			name, expected string
			verdict        result.Verdict
		}{
			{operator.Conditions.Contract, conditionsContract, operator.Conditions.Verdict},
			{operator.Version.Contract, versionContract, operator.Version.Verdict},
		} {
			if contract.name == "" && contract.verdict == "" {
				continue
			}
			if contract.name != contract.expected || !validVerdict(contract.verdict) {
				return nil, fmt.Errorf("operator %q: invalid %s contract name/verdict %q/%q", operator.Operator, contract.expected, contract.name, contract.verdict)
			}
		}
		operators[operator.Operator] = operator
	}
	return operators, nil
}

func indexPoolEvidence(reports []contracts.MachineConfigPoolEvidenceReport) (map[string]contracts.MachineConfigPoolEvidenceReport, error) {
	indexed := make(map[string]contracts.MachineConfigPoolEvidenceReport, len(reports))
	for _, report := range reports {
		if report.Pool == "" || report.Contract != contracts.MachineConfigPoolLifecycleContract || !validVerdict(result.Verdict(report.Verdict)) {
			return nil, fmt.Errorf("invalid MachineConfigPool evidence for %q", report.Pool)
		}
		if _, exists := indexed[report.Pool]; exists {
			return nil, fmt.Errorf("duplicate MachineConfigPool %q", report.Pool)
		}
		indexed[report.Pool] = report
	}
	return indexed, nil
}

func indexNodeEvidence(reports []contracts.NodeEvidenceReport) (map[string]contracts.NodeEvidenceReport, error) {
	indexed := make(map[string]contracts.NodeEvidenceReport, len(reports))
	for _, report := range reports {
		if report.Node == "" || report.Contract != contracts.NodeLifecycleContract || !validVerdict(result.Verdict(report.Verdict)) {
			return nil, fmt.Errorf("invalid Node evidence for %q", report.Node)
		}
		if _, exists := indexed[report.Node]; exists {
			return nil, fmt.Errorf("duplicate Node %q", report.Node)
		}
		indexed[report.Node] = report
	}
	return indexed, nil
}

func indexPolicyOperators(items []result.OperatorResult) (map[string]result.OperatorResult, error) {
	indexed := make(map[string]result.OperatorResult, len(items))
	for _, item := range items {
		if item.Name == "" || !validVerdict(item.Verdict) {
			return nil, fmt.Errorf("invalid operator result %q/%q", item.Name, item.Verdict)
		}
		if _, exists := indexed[item.Name]; exists {
			return nil, fmt.Errorf("duplicate operator %q", item.Name)
		}
		if err := validateResultContracts(item.Name, item.Contracts); err != nil {
			return nil, err
		}
		indexed[item.Name] = item
	}
	return indexed, nil
}

func indexPolicyResources(items []result.ResourceResult) (map[string]result.ResourceResult, error) {
	indexed := make(map[string]result.ResourceResult, len(items))
	for _, item := range items {
		if item.Name == "" || !validVerdict(item.Verdict) {
			return nil, fmt.Errorf("invalid resource result %q/%q", item.Name, item.Verdict)
		}
		if _, exists := indexed[item.Name]; exists {
			return nil, fmt.Errorf("duplicate resource %q", item.Name)
		}
		if err := validateResultContracts(item.Name, item.Contracts); err != nil {
			return nil, err
		}
		indexed[item.Name] = item
	}
	return indexed, nil
}

func validateResultContracts(resource string, contracts []result.Contract) error {
	seen := make(map[string]bool, len(contracts))
	for _, contract := range contracts {
		if contract.Name == "" || !validVerdict(contract.Verdict) {
			return fmt.Errorf("resource %q has invalid contract %q/%q", resource, contract.Name, contract.Verdict)
		}
		if seen[contract.Name] {
			return fmt.Errorf("resource %q has duplicate contract %q", resource, contract.Name)
		}
		seen[contract.Name] = true
	}
	return nil
}

func contractSlice(contracts []result.Contract, present bool) []result.Contract {
	if !present {
		return nil
	}
	return contracts
}

func compareNameScope[T any](baseline, candidate map[string]T) (common, baselineOnly, candidateOnly []string) {
	for name := range baseline {
		if _, ok := candidate[name]; ok {
			common = append(common, name)
		} else {
			baselineOnly = append(baselineOnly, name)
		}
	}
	for name := range candidate {
		if _, ok := baseline[name]; !ok {
			candidateOnly = append(candidateOnly, name)
		}
	}
	sort.Strings(common)
	sort.Strings(baselineOnly)
	sort.Strings(candidateOnly)
	if baselineOnly == nil {
		baselineOnly = []string{}
	}
	if candidateOnly == nil {
		candidateOnly = []string{}
	}
	return common, baselineOnly, candidateOnly
}

func unionNames[T any](baseline, candidate map[string]T) []string {
	seen := make(map[string]bool, len(baseline)+len(candidate))
	for name := range baseline {
		seen[name] = true
	}
	for name := range candidate {
		seen[name] = true
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func validVerdict(verdict result.Verdict) bool {
	return verdict == result.VerdictPass || verdict == result.VerdictFail || verdict == result.VerdictInconclusive
}
