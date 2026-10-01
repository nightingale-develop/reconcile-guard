package result

import "time"

const SchemaVersion = "1"

type Verdict string

const (
	VerdictPass         Verdict = "PASS"
	VerdictFail         Verdict = "FAIL"
	VerdictInconclusive Verdict = "INCONCLUSIVE"
)

type Document struct {
	SchemaVersion string `json:"schemaVersion"`
	Command       string `json:"command"`
	Result        Report `json:"result"`
}

type Report struct {
	Verdict            Verdict          `json:"verdict"`
	Details            *Details         `json:"details,omitempty"`
	Operators          []OperatorResult `json:"operators"`
	MachineConfigPools []ResourceResult `json:"machineConfigPools,omitempty"`
	Nodes              []ResourceResult `json:"nodes,omitempty"`
}

type OperatorResult struct {
	Name      string     `json:"name"`
	Verdict   Verdict    `json:"verdict"`
	Contracts []Contract `json:"contracts"`
}

type ResourceResult struct {
	Name      string     `json:"name"`
	Verdict   Verdict    `json:"verdict"`
	Contracts []Contract `json:"contracts"`
}

type Contract struct {
	Name     string     `json:"name"`
	Verdict  Verdict    `json:"verdict"`
	Details  *Details   `json:"details,omitempty"`
	Evidence []Evidence `json:"evidence,omitempty"`
}

type Details struct {
	Counts map[string]int    `json:"counts,omitempty"`
	Values map[string]string `json:"values,omitempty"`
	Flags  map[string]bool   `json:"flags,omitempty"`
}

type Evidence struct {
	Kind        string            `json:"kind"`
	Verdict     Verdict           `json:"verdict,omitempty"`
	ObservedAt  *time.Time        `json:"observedAt,omitempty"`
	From        *time.Time        `json:"from,omitempty"`
	To          *time.Time        `json:"to,omitempty"`
	Expected    string            `json:"expected,omitempty"`
	Actual      string            `json:"actual,omitempty"`
	Correlation string            `json:"correlation,omitempty"`
	Reason      string            `json:"reason,omitempty"`
	Message     string            `json:"message,omitempty"`
	Source      string            `json:"source,omitempty"`
	Attributes  map[string]string `json:"attributes,omitempty"`
}

func NewDocument(command string, report Report) Document {
	return Document{
		SchemaVersion: SchemaVersion,
		Command:       command,
		Result:        report,
	}
}

func SingleOperator(
	name string,
	contract Contract,
) Report {
	return Report{
		Verdict: contract.Verdict,
		Operators: []OperatorResult{
			{
				Name:      name,
				Verdict:   contract.Verdict,
				Contracts: []Contract{contract},
			},
		},
	}
}
