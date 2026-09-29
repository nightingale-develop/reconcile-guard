package regression

import "github.com/nightingale-develop/reconcile-guard/internal/result"

const SchemaVersion = "1"

type Change string

const (
	ChangeUnchanged    Change = "UNCHANGED"
	ChangeRegression   Change = "REGRESSION"
	ChangeImprovement  Change = "IMPROVEMENT"
	ChangeInconclusive Change = "INCONCLUSIVE"
)

type ContractComparison struct {
	Name             string         `json:"name"`
	BaselineVerdict  result.Verdict `json:"baselineVerdict,omitempty"`
	CandidateVerdict result.Verdict `json:"candidateVerdict,omitempty"`
	Change           Change         `json:"change"`
}

type OperatorComparison struct {
	Name             string               `json:"name"`
	BaselineVerdict  result.Verdict       `json:"baselineVerdict"`
	CandidateVerdict result.Verdict       `json:"candidateVerdict"`
	Verdict          result.Verdict       `json:"verdict"`
	Contracts        []ContractComparison `json:"contracts"`
}

type RunSummary struct {
	RunID               string         `json:"runId"`
	ClusterID           string         `json:"clusterId"`
	FinalDesiredVersion string         `json:"finalDesiredVersion"`
	VerificationVerdict result.Verdict `json:"verificationVerdict"`
}

type Scope struct {
	CommonOperators        int      `json:"commonOperators"`
	BaselineOnlyOperators  []string `json:"baselineOnlyOperators"`
	CandidateOnlyOperators []string `json:"candidateOnlyOperators"`
}

type Report struct {
	Verdict   result.Verdict       `json:"verdict"`
	Baseline  RunSummary           `json:"baseline"`
	Candidate RunSummary           `json:"candidate"`
	Scope     Scope                `json:"scope"`
	Operators []OperatorComparison `json:"operators"`
}

type Document struct {
	SchemaVersion string `json:"schemaVersion"`
	Command       string `json:"command"`
	Comparison    Report `json:"comparison"`
}

func NewDocument(report Report) Document {
	return Document{SchemaVersion: SchemaVersion, Command: "compare-runs", Comparison: report}
}
