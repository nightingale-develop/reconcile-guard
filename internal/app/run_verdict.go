package app

import (
	"fmt"
	"io"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/contracts"
	"github.com/nightingale-develop/reconcile-guard/internal/machineconfig"
	"github.com/nightingale-develop/reconcile-guard/internal/result"
	corev1 "k8s.io/api/core/v1"
)

type runAssessment struct {
	Verdict result.Verdict
	Reasons []result.VerdictReason
}

func recordedRunVerdict(operators contracts.ClusterUpgradeReport, auxiliary auxiliaryRunReport) contracts.ContractVerdict {
	return assessRecordedRun(operators, auxiliary).Verdict
}

func assessRecordedRun(operators contracts.ClusterUpgradeReport, auxiliary auxiliaryRunReport) runAssessment {
	assessment := runAssessment{Verdict: operators.Verdict}
	if operators.Verdict != contracts.ContractPass {
		for _, operator := range operators.Operators {
			if operator.Verdict != operators.Verdict {
				continue
			}
			assessment.Reasons = append(assessment.Reasons, result.VerdictReason{
				Code: "operator-contracts-not-passed", ResourceKind: "ClusterOperator", ResourceName: operator.Operator,
				Message: fmt.Sprintf("Operator contracts: conditions=%s, version consistency=%s. See contract evidence.", operator.Conditions.Verdict, operator.Version.Verdict),
			})
		}
		return assessment
	}
	if len(auxiliary.MachineConfigPools) == 0 {
		assessment.Reasons = append(assessment.Reasons, result.VerdictReason{
			Code: "missing-histories", ResourceKind: "MachineConfigPool", Message: "No MachineConfigPool histories were recorded.",
		})
	}
	if len(auxiliary.Nodes) == 0 {
		assessment.Reasons = append(assessment.Reasons, result.VerdictReason{
			Code: "missing-histories", ResourceKind: "Node", Message: "No Node histories were recorded.",
		})
	}
	for _, pool := range auxiliary.MachineConfigPools {
		reason := result.VerdictReason{ResourceKind: "MachineConfigPool", ResourceName: pool.Pool}
		if len(pool.Evidence) == 0 {
			reason.Code, reason.Message = "missing-evidence", "No lifecycle observations are available."
		} else {
			last := pool.Evidence[len(pool.Evidence)-1]
			reason.ObservedAt = verdictObservationTime(last.State.ObservedAt)
			reason.Correlation = string(last.Correlation.Kind)
			switch {
			case !last.Applicable:
				reason.Code, reason.Message = "last-observation-not-applicable", "The last observation is not confidently correlated within a post-completion window for the same target."
			case last.State.Phase != machineconfig.PhaseStable:
				reason.Code, reason.Message = "pool-not-stable", fmt.Sprintf("The last observed pool phase is %s; STABLE is required.", last.State.Phase)
			case pool.Verdict != contracts.ContractPass:
				reason.Code, reason.Message = "lifecycle-evidence-not-passed", "The lifecycle evidence contract did not establish convergence."
			}
		}
		if reason.Code != "" {
			assessment.Reasons = append(assessment.Reasons, reason)
		}
	}
	for _, node := range auxiliary.Nodes {
		reason := result.VerdictReason{ResourceKind: "Node", ResourceName: node.Node}
		if len(node.Evidence) == 0 {
			reason.Code, reason.Message = "missing-evidence", "No lifecycle observations are available."
		} else {
			last := node.Evidence[len(node.Evidence)-1]
			reason.ObservedAt = verdictObservationTime(last.State.ObservedAt)
			reason.Correlation = string(last.Correlation.Kind)
			switch {
			case !last.Applicable:
				reason.Code, reason.Message = "last-observation-not-applicable", "The last observation is not confidently correlated within a post-completion window for the same target."
			case last.State.Ready != corev1.ConditionTrue:
				reason.Code, reason.Message = "node-not-ready", fmt.Sprintf("The last observed Ready status is %q; True is required.", last.State.Ready)
			case !last.State.ConfigAligned:
				reason.Code, reason.Message = "node-config-not-aligned", fmt.Sprintf("The last observed MachineConfig annotations are missing or unequal: current=%q, desired=%q.", last.State.CurrentMachineConfig, last.State.DesiredMachineConfig)
			case node.Verdict != contracts.ContractPass:
				reason.Code, reason.Message = "lifecycle-evidence-not-passed", "The lifecycle evidence contract did not establish convergence."
			}
		}
		if reason.Code != "" {
			assessment.Reasons = append(assessment.Reasons, reason)
		}
	}
	if len(assessment.Reasons) > 0 {
		assessment.Verdict = contracts.ContractInconclusive
	}
	return assessment
}

func verdictObservationTime(at time.Time) *time.Time {
	if at.IsZero() {
		return nil
	}
	at = at.UTC()
	return &at
}

func verdictReasonResource(reason result.VerdictReason) string {
	if reason.ResourceName != "" {
		return reason.ResourceKind + "/" + reason.ResourceName
	}
	return reason.ResourceKind
}

func printRunVerdictReasons(out io.Writer, reasons []result.VerdictReason) {
	if len(reasons) == 0 {
		return
	}
	fmt.Fprintln(out, "Run verdict reasons:")
	for _, reason := range reasons {
		fmt.Fprintf(out, "  %s [%s]: %s", verdictReasonResource(reason), reason.Code, reason.Message)
		if reason.ObservedAt != nil {
			fmt.Fprintf(out, " observedAt=%s", reason.ObservedAt.Format(time.RFC3339Nano))
		}
		if reason.Correlation != "" {
			fmt.Fprintf(out, " correlation=%s", reason.Correlation)
		}
		fmt.Fprintln(out)
	}
}
