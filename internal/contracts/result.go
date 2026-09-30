package contracts

import (
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
