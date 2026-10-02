package regression

import (
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/result"
)

const SchemaVersion = "1"

type Change string

const (
	ChangeUnchanged    Change = "UNCHANGED"
	ChangeRegression   Change = "REGRESSION"
	ChangeImprovement  Change = "IMPROVEMENT"
	ChangeInconclusive Change = "INCONCLUSIVE"
)

type ContractComparison struct {
	Name                  string         `json:"name"`
	BaselineVerdict       result.Verdict `json:"baselineVerdict,omitempty"`
	CandidateVerdict      result.Verdict `json:"candidateVerdict,omitempty"`
	Change                Change         `json:"change"`
	BaselineObservedSpan  string         `json:"baselineObservedSpan,omitempty"`
	CandidateObservedSpan string         `json:"candidateObservedSpan,omitempty"`
	ObservedSpanDelta     string         `json:"observedSpanDelta,omitempty"`
}

type OperatorComparison struct {
	Name             string               `json:"name"`
	BaselineVerdict  result.Verdict       `json:"baselineVerdict"`
	CandidateVerdict result.Verdict       `json:"candidateVerdict"`
	Verdict          result.Verdict       `json:"verdict"`
	Contracts        []ContractComparison `json:"contracts"`
}

type ResourceComparison struct {
	Name             string               `json:"name"`
	BaselineVerdict  result.Verdict       `json:"baselineVerdict,omitempty"`
	CandidateVerdict result.Verdict       `json:"candidateVerdict,omitempty"`
	Verdict          result.Verdict       `json:"verdict"`
	Contracts        []ContractComparison `json:"contracts"`
}

type RunSummary struct {
	RunID               string         `json:"runId"`
	ClusterID           string         `json:"clusterId"`
	FinalDesiredVersion string         `json:"finalDesiredVersion"`
	FinalDesiredImage   string         `json:"finalDesiredImage,omitempty"`
	VerificationVerdict result.Verdict `json:"verificationVerdict"`
}

type Scope struct {
	CommonOperators                 int      `json:"commonOperators"`
	BaselineOnlyOperators           []string `json:"baselineOnlyOperators"`
	CandidateOnlyOperators          []string `json:"candidateOnlyOperators"`
	CommonMachineConfigPools        int      `json:"commonMachineConfigPools"`
	BaselineOnlyMachineConfigPools  []string `json:"baselineOnlyMachineConfigPools"`
	CandidateOnlyMachineConfigPools []string `json:"candidateOnlyMachineConfigPools"`
	CommonNodes                     int      `json:"commonNodes"`
	BaselineOnlyNodes               []string `json:"baselineOnlyNodes"`
	CandidateOnlyNodes              []string `json:"candidateOnlyNodes"`
	SameFinalTarget                 bool     `json:"sameFinalTarget"`
}

type ObservedTiming struct {
	Name         string    `json:"name"`
	ResourceKind string    `json:"resourceKind"`
	ResourceName string    `json:"resourceName,omitempty"`
	From         time.Time `json:"from"`
	To           time.Time `json:"to"`
}

type TimingValue struct {
	From     time.Time `json:"from"`
	To       time.Time `json:"to"`
	Duration string    `json:"duration"`
}

type TimingComparison struct {
	Name         string       `json:"name"`
	ResourceKind string       `json:"resourceKind"`
	ResourceName string       `json:"resourceName,omitempty"`
	Baseline     *TimingValue `json:"baseline,omitempty"`
	Candidate    *TimingValue `json:"candidate,omitempty"`
	Delta        string       `json:"delta,omitempty"`
}

type PolicyComparison struct {
	Source             string               `json:"source"`
	TargetVersion      string               `json:"targetVersion"`
	TargetImage        string               `json:"targetImage,omitempty"`
	BaselineVerdict    result.Verdict       `json:"baselineVerdict"`
	CandidateVerdict   result.Verdict       `json:"candidateVerdict"`
	Verdict            result.Verdict       `json:"verdict"`
	Operators          []OperatorComparison `json:"operators"`
	MachineConfigPools []ResourceComparison `json:"machineConfigPools,omitempty"`
	Nodes              []ResourceComparison `json:"nodes,omitempty"`
}

type Report struct {
	Verdict            result.Verdict       `json:"verdict"`
	Baseline           RunSummary           `json:"baseline"`
	Candidate          RunSummary           `json:"candidate"`
	Scope              Scope                `json:"scope"`
	Operators          []OperatorComparison `json:"operators"`
	MachineConfigPools []ResourceComparison `json:"machineConfigPools,omitempty"`
	Nodes              []ResourceComparison `json:"nodes,omitempty"`
	Timings            []TimingComparison   `json:"timings,omitempty"`
	Policy             *PolicyComparison    `json:"policy,omitempty"`
}

type Document struct {
	SchemaVersion string `json:"schemaVersion"`
	Command       string `json:"command"`
	Comparison    Report `json:"comparison"`
}

func NewDocument(report Report) Document {
	return Document{SchemaVersion: SchemaVersion, Command: "compare-runs", Comparison: report}
}
