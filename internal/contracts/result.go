package contracts

import (
	"fmt"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/result"
)

func NormalUpgradeResult(
	report UpgradeContractReport,
) result.Contract {
	evidence := make(
		[]result.Evidence,
		0,
		len(report.Findings),
	)

	for _, finding := range report.Findings {
		evidence = append(
			evidence,
			result.Evidence{
				Kind:        "operator-condition",
				Verdict:     ContractInconclusive,
				ObservedAt:  resultTime(finding.ObservedAt),
				From:        resultTime(finding.FromTime),
				To:          resultTime(finding.ToTime),
				Correlation: string(finding.Correlation),
				Reason:      finding.Reason,
				Message:     finding.Message,
				Attributes: map[string]string{
					"condition": string(finding.Condition),
					"status":    string(finding.Status),
				},
			},
		)
	}

	return result.Contract{
		Name:    report.Contract,
		Verdict: report.Verdict,
		Details: &result.Details{
			Counts: map[string]int{
				"upgradeSamples":      report.UpgradeSamples,
				"evaluatedSamples":    report.EvaluatedSamples,
				"ambiguousSamples":    report.AmbiguousSamples,
				"outsideSamples":      report.OutsideSamples,
				"missingConditions":   report.MissingConditions,
				"unknownPhaseSamples": report.UnknownPhaseSamples,
			},
		},
		Evidence: evidence,
	}
}

func ProgressingResult(
	report ProgressingReport,
) result.Contract {
	evidence := make(
		[]result.Evidence,
		0,
		len(report.Episodes),
	)

	for _, episode := range report.Episodes {
		attributes := map[string]string{
			"observedSpan": episode.ObservedSpan.String(),
		}

		if episode.MaximumSpan > 0 {
			attributes["maximumSpan"] =
				episode.MaximumSpan.String()
		}

		if !episode.BeforeFalse.IsZero() {
			attributes["precedingFalse"] =
				episode.BeforeFalse.Format(time.RFC3339Nano)
		}

		if !episode.AfterFalse.IsZero() {
			attributes["followingFalse"] =
				episode.AfterFalse.Format(time.RFC3339Nano)
		}

		evidence = append(
			evidence,
			result.Evidence{
				Kind:       "progressing-episode",
				Verdict:    episode.Verdict,
				From:       resultTime(episode.FirstTrue),
				To:         resultTime(episode.LastTrue),
				Attributes: attributes,
			},
		)
	}

	return result.Contract{
		Name:    "observed-progressing-duration",
		Verdict: report.Verdict,
		Details: &result.Details{
			Counts: map[string]int{
				"observations":      report.Observations,
				"missingConditions": report.MissingConditions,
			},
			Values: map[string]string{
				"maximumDuration": report.Limit.String(),
			},
		},
		Evidence: evidence,
	}
}

func ProgressingUpgradeResult(
	report ProgressingUpgradeReport,
) result.Contract {
	evidence := make(
		[]result.Evidence,
		0,
		len(report.Evidence),
	)

	for _, finding := range report.Evidence {
		episode := finding.Episode

		attributes := map[string]string{
			"observedSpan":     episode.ObservedSpan.String(),
			"startCorrelation": string(finding.Start.Kind),
			"endCorrelation":   string(finding.End.Kind),
		}

		if !finding.Start.FromTime.IsZero() {
			attributes["startVersionFrom"] =
				finding.Start.FromTime.Format(time.RFC3339Nano)
		}

		if !finding.Start.ToTime.IsZero() {
			attributes["startVersionTo"] =
				finding.Start.ToTime.Format(time.RFC3339Nano)
		}

		if !finding.End.FromTime.IsZero() {
			attributes["endVersionFrom"] =
				finding.End.FromTime.Format(time.RFC3339Nano)
		}

		if !finding.End.ToTime.IsZero() {
			attributes["endVersionTo"] =
				finding.End.ToTime.Format(time.RFC3339Nano)
		}

		if !episode.BeforeFalse.IsZero() {
			attributes["precedingFalse"] =
				episode.BeforeFalse.Format(time.RFC3339Nano)
		}

		if !episode.AfterFalse.IsZero() {
			attributes["followingFalse"] =
				episode.AfterFalse.Format(time.RFC3339Nano)
		}

		evidence = append(
			evidence,
			result.Evidence{
				Kind:       "progressing-upgrade-episode",
				Verdict:    episode.Verdict,
				From:       resultTime(episode.FirstTrue),
				To:         resultTime(episode.LastTrue),
				Source:     report.Policy.Source,
				Attributes: attributes,
			},
		)
	}

	return result.Contract{
		Name:    "operator-progressing-duration",
		Verdict: report.Verdict,
		Details: &result.Details{
			Counts: map[string]int{
				"evaluatedSamples":  report.EvaluatedSamples,
				"uncertainSamples":  report.UncertainSamples,
				"missingConditions": report.MissingConditions,
				"discontinuities":   report.Discontinuities,
			},
			Values: map[string]string{
				"maximumDuration":       report.Policy.Limit.String(),
				"maximumObservationGap": report.Policy.MaxObservationGap.String(),
				"operator":              report.Policy.Operator,
				"targetVersion":         report.Policy.TargetVersion,
				"thresholdSource":       report.Policy.Source,
			},
			Flags: map[string]bool{
				"policyApplicable": report.PolicyApplicable,
			},
		},
		Evidence: evidence,
	}
}

func VersionConsistencyResult(
	report VersionConsistencyReport,
) result.Contract {
	evidence := make(
		[]result.Evidence,
		0,
		len(report.Evidence),
	)

	for _, finding := range report.Evidence {
		evidence = append(
			evidence,
			result.Evidence{
				Kind:       "operator-version",
				Verdict:    finding.Verdict,
				ObservedAt: resultTime(finding.ObservedAt),
				From:       resultTime(finding.WindowFrom),
				To:         resultTime(finding.WindowTo),
				Expected:   finding.ExpectedVersion,
				Actual:     finding.ReportedVersion,
			},
		)
	}

	return result.Contract{
		Name:    report.Contract,
		Verdict: report.Verdict,
		Details: &result.Details{
			Counts: map[string]int{
				"completedTargets": report.CompletedTargets,
				"evaluatedSamples": report.EvaluatedSamples,
				"missingTargets":   report.MissingTargets,
				"missingVersions":  report.MissingVersions,
				"uncoveredTargets": report.UncoveredTargets,
			},
		},
		Evidence: evidence,
	}
}

func ClusterUpgradeResult(
	report ClusterUpgradeReport,
) result.Report {
	operators := make(
		[]result.OperatorResult,
		0,
		len(report.Operators),
	)

	for _, operatorReport := range report.Operators {
		operators = append(
			operators,
			result.OperatorResult{
				Name:    operatorReport.Operator,
				Verdict: operatorReport.Verdict,
				Contracts: []result.Contract{
					NormalUpgradeResult(
						operatorReport.Conditions,
					),
					VersionConsistencyResult(
						operatorReport.Version,
					),
				},
			},
		)
	}

	return result.Report{
		Verdict: report.Verdict,
		Details: &result.Details{
			Counts: map[string]int{
				"operators":             len(report.Operators),
				"passedOperators":       report.PassedOperators,
				"failedOperators":       report.FailedOperators,
				"inconclusiveOperators": report.InconclusiveOperators,
			},
		},
		Operators: operators,
	}
}

func resultTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}

	result := value.UTC()
	return &result
}

func MachineConfigPoolLifecycleResult(
	report MachineConfigPoolEvidenceReport,
) result.Contract {
	evidence := make([]result.Evidence, 0, len(report.Evidence))
	for _, finding := range report.Evidence {
		state := finding.State
		attributes := map[string]string{
			"phase":                   string(state.Phase),
			"generation":              fmt.Sprint(state.Generation),
			"observedGeneration":      fmt.Sprint(state.ObservedGeneration),
			"generationObserved":      fmt.Sprint(state.GenerationObserved),
			"currentConfiguration":    state.CurrentConfiguration,
			"desiredConfiguration":    state.DesiredConfiguration,
			"machineCount":            fmt.Sprint(state.MachineCount),
			"updatedMachineCount":     fmt.Sprint(state.UpdatedMachineCount),
			"readyMachineCount":       fmt.Sprint(state.ReadyMachineCount),
			"unavailableMachineCount": fmt.Sprint(state.UnavailableMachineCount),
			"degradedMachineCount":    fmt.Sprint(state.DegradedMachineCount),
			"upgradePhase":            string(finding.Correlation.Phase),
			"desiredVersion":          finding.Correlation.DesiredVersion,
			"desiredImage":            finding.Correlation.DesiredImage,
			"applicable":              fmt.Sprint(finding.Applicable),
		}
		evidence = append(evidence, result.Evidence{
			Kind:        "machine-config-pool-state",
			ObservedAt:  resultTime(state.ObservedAt),
			From:        resultTime(finding.Correlation.FromTime),
			To:          resultTime(finding.Correlation.ToTime),
			Correlation: string(finding.Correlation.Kind),
			Attributes:  attributes,
		})
	}
	return result.Contract{
		Name:    report.Contract,
		Verdict: report.Verdict,
		Details: &result.Details{Counts: map[string]int{
			"observations":     report.Observations,
			"evaluatedSamples": report.EvaluatedSamples,
			"uncertainSamples": report.UncertainSamples,
			"convergedSamples": report.ConvergedSamples,
		}},
		Evidence: evidence,
	}
}

func NodeLifecycleResult(
	report NodeEvidenceReport,
) result.Contract {
	evidence := make([]result.Evidence, 0, len(report.Evidence))
	for _, finding := range report.Evidence {
		state := finding.State
		attributes := map[string]string{
			"ready":                string(state.Ready),
			"currentMachineConfig": state.CurrentMachineConfig,
			"desiredMachineConfig": state.DesiredMachineConfig,
			"kubeletVersion":       state.KubeletVersion,
			"configAligned":        fmt.Sprint(state.ConfigAligned),
			"upgradePhase":         string(finding.Correlation.Phase),
			"desiredVersion":       finding.Correlation.DesiredVersion,
			"desiredImage":         finding.Correlation.DesiredImage,
			"applicable":           fmt.Sprint(finding.Applicable),
		}
		evidence = append(evidence, result.Evidence{
			Kind:        "node-state",
			ObservedAt:  resultTime(state.ObservedAt),
			From:        resultTime(finding.Correlation.FromTime),
			To:          resultTime(finding.Correlation.ToTime),
			Correlation: string(finding.Correlation.Kind),
			Attributes:  attributes,
		})
	}
	return result.Contract{
		Name:    report.Contract,
		Verdict: report.Verdict,
		Details: &result.Details{Counts: map[string]int{
			"observations":     report.Observations,
			"evaluatedSamples": report.EvaluatedSamples,
			"uncertainSamples": report.UncertainSamples,
			"convergedSamples": report.ConvergedSamples,
		}},
		Evidence: evidence,
	}
}

func OperatorConditionPolicyResult(
	report OperatorConditionPolicyReport,
) result.Contract {
	evidence := make([]result.Evidence, 0, len(report.Episodes))
	for _, episode := range report.Episodes {
		attributes := map[string]string{
			"condition":     string(report.Condition),
			"adverseStatus": string(report.AdverseStatus),
			"observedSpan":  episode.ObservedSpan.String(),
		}
		if episode.MaximumSpan > 0 {
			attributes["maximumSpan"] = episode.MaximumSpan.String()
		}
		if !episode.BeforeGood.IsZero() {
			attributes["precedingGood"] = episode.BeforeGood.Format(time.RFC3339Nano)
		}
		if !episode.AfterGood.IsZero() {
			attributes["followingGood"] = episode.AfterGood.Format(time.RFC3339Nano)
		}
		evidence = append(evidence, result.Evidence{
			Kind:       "operator-lifecycle-policy-episode",
			Verdict:    episode.Verdict,
			From:       resultTime(episode.FirstAdverse),
			To:         resultTime(episode.LastAdverse),
			Source:     report.Source,
			Attributes: attributes,
		})
	}
	return result.Contract{
		Name:    report.Contract,
		Verdict: report.Verdict,
		Details: &result.Details{
			Counts: map[string]int{
				"evaluatedSamples":  report.EvaluatedSamples,
				"uncertainSamples":  report.UncertainSamples,
				"missingConditions": report.MissingConditions,
				"discontinuities":   report.Discontinuities,
				"episodes":          len(report.Episodes),
			},
			Values: map[string]string{
				"condition":               string(report.Condition),
				"adverseStatus":           string(report.AdverseStatus),
				"maximumObservedDuration": report.Limit.String(),
				"maximumObservationGap":   report.MaxObservationGap.String(),
				"targetVersion":           report.TargetVersion,
				"targetImage":             report.TargetImage,
				"thresholdSource":         report.Source,
			},
			Flags: map[string]bool{
				"policyApplicable": report.PolicyApplicable,
			},
		},
		Evidence: evidence,
	}
}

func PostCompletionPolicyResult(
	report PostCompletionPolicyReport,
) result.Contract {
	evidence := make([]result.Evidence, 0, len(report.Evidence))
	for _, finding := range report.Evidence {
		attributes := map[string]string{
			"state":          finding.State,
			"applicable":     fmt.Sprint(finding.Applicable),
			"compliant":      fmt.Sprint(finding.Compliant),
			"uncertain":      fmt.Sprint(finding.Uncertain),
			"upgradePhase":   string(finding.Correlation.Phase),
			"desiredVersion": finding.Correlation.DesiredVersion,
			"desiredImage":   finding.Correlation.DesiredImage,
		}
		for key, value := range finding.Attributes {
			attributes[key] = value
		}
		verdict := ContractVerdict("")
		if finding.Applicable {
			switch {
			case finding.Uncertain:
				verdict = ContractInconclusive
			case finding.Compliant:
				verdict = ContractPass
			default:
				verdict = ContractFail
			}
		}
		evidence = append(evidence, result.Evidence{
			Kind:        "post-completion-lifecycle-policy-sample",
			Verdict:     verdict,
			ObservedAt:  resultTime(finding.ObservedAt),
			From:        resultTime(finding.Correlation.FromTime),
			To:          resultTime(finding.Correlation.ToTime),
			Correlation: string(finding.Correlation.Kind),
			Source:      report.Source,
			Attributes:  attributes,
		})
	}
	values := map[string]string{
		"targetVersion":             report.TargetVersion,
		"targetImage":               report.TargetImage,
		"postCompletionGracePeriod": report.GracePeriod.String(),
		"thresholdSource":           report.Source,
	}
	if !report.CompletionAt.IsZero() {
		values["completionObservedAt"] = report.CompletionAt.Format(time.RFC3339Nano)
	}
	if !report.Deadline.IsZero() {
		values["evaluationDeadline"] = report.Deadline.Format(time.RFC3339Nano)
	}
	return result.Contract{
		Name:    report.Contract,
		Verdict: report.Verdict,
		Details: &result.Details{
			Counts: map[string]int{
				"evaluatedSamples": report.EvaluatedSamples,
				"compliantSamples": report.CompliantSamples,
				"violatingSamples": report.ViolatingSamples,
				"uncertainSamples": report.UncertainSamples,
			},
			Values: values,
			Flags: map[string]bool{
				"policyApplicable": report.PolicyApplicable,
			},
		},
		Evidence: evidence,
	}
}
