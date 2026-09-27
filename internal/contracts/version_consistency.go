package contracts

import (
	"fmt"
	"strings"
	"time"

	configv1 "github.com/openshift/api/config/v1"

	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
)

const operatorVersionConsistencyContract = "operator-version-consistency"

type VersionConsistencyEvidence struct {
	ObservedAt      time.Time
	WindowFrom      time.Time
	WindowTo        time.Time
	ExpectedVersion string
	ReportedVersion string
	Verdict         ContractVerdict
}

type VersionConsistencyReport struct {
	Contract         string
	Operator         string
	Verdict          ContractVerdict
	CompletedTargets int
	EvaluatedSamples int
	MissingTargets   int
	MissingVersions  int
	UncoveredTargets int
	Evidence         []VersionConsistencyEvidence
}

type completedTargetWindow struct {
	FromTime      time.Time
	ToTime        time.Time
	TargetVersion string
}

func VerifyOperatorVersionConsistency(
	versions []upgrade.ClusterVersionObservation,
	observations []operator.Observation,
) (VersionConsistencyReport, error) {
	states, err := upgrade.AnalyzePhases(versions)
	if err != nil {
		return VersionConsistencyReport{}, err
	}

	history, err := operator.AnalyzeHistory(observations)
	if err != nil {
		return VersionConsistencyReport{}, err
	}

	report := VersionConsistencyReport{
		Contract: operatorVersionConsistencyContract,
		Operator: history.Operator,
		Verdict:  ContractInconclusive,
	}

	var windows []completedTargetWindow

	for i, state := range states {
		if state.Phase != upgrade.UpgradePhaseCompleted {
			continue
		}

		report.CompletedTargets++

		if strings.TrimSpace(state.DesiredVersion) == "" {
			report.MissingTargets++
			continue
		}

		window := completedTargetWindow{
			FromTime:      state.ObservedAt,
			ToTime:        state.ObservedAt,
			TargetVersion: state.DesiredVersion,
		}

		for j := i + 1; j < len(states); j++ {
			next := states[j]

			if next.Phase != upgrade.UpgradePhaseStable ||
				next.DesiredVersion != state.DesiredVersion {
				break
			}

			window.ToTime = next.ObservedAt
		}

		windows = append(windows, window)
	}

	covered := make([]bool, len(windows))
	failed := false

	for _, observation := range observations {
		windowIndex := findCompletedTargetWindow(
			windows,
			observation.ObservedAt,
		)

		if windowIndex < 0 {
			continue
		}

		covered[windowIndex] = true
		window := windows[windowIndex]

		reportedVersion, found, err := findOperatorVersion(
			observation.Operator.Status.Versions,
		)
		if err != nil {
			return VersionConsistencyReport{}, fmt.Errorf(
				"observation %s: %w",
				observation.ObservedAt.Format(time.RFC3339Nano),
				err,
			)
		}

		evidence := VersionConsistencyEvidence{
			ObservedAt:      observation.ObservedAt,
			WindowFrom:      window.FromTime,
			WindowTo:        window.ToTime,
			ExpectedVersion: window.TargetVersion,
			ReportedVersion: reportedVersion,
			Verdict:         ContractInconclusive,
		}

		if !found {
			report.MissingVersions++
			report.Evidence = append(report.Evidence, evidence)
			continue
		}

		report.EvaluatedSamples++

		if reportedVersion != window.TargetVersion {
			evidence.Verdict = ContractFail
			failed = true
		} else {
			evidence.Verdict = ContractPass
		}

		report.Evidence = append(report.Evidence, evidence)
	}

	for i := range windows {
		if !covered[i] {
			report.UncoveredTargets++
		}
	}

	switch {
	case failed:
		report.Verdict = ContractFail

	case report.CompletedTargets == 0,
		report.EvaluatedSamples == 0,
		report.MissingTargets > 0,
		report.MissingVersions > 0,
		report.UncoveredTargets > 0:
		report.Verdict = ContractInconclusive

	default:
		report.Verdict = ContractPass
	}

	return report, nil
}

func findCompletedTargetWindow(
	windows []completedTargetWindow,
	observedAt time.Time,
) int {
	for i, window := range windows {
		if observedAt.Before(window.FromTime) ||
			observedAt.After(window.ToTime) {
			continue
		}

		return i
	}

	return -1
}

func findOperatorVersion(
	versions []configv1.OperandVersion,
) (string, bool, error) {
	var result string
	found := false

	for _, version := range versions {
		if version.Name != "operator" {
			continue
		}

		if found {
			return "", false, fmt.Errorf(
				"duplicate operator version entry",
			)
		}

		found = true
		result = version.Version
	}

	if !found || strings.TrimSpace(result) == "" {
		return "", false, nil
	}

	return result, true, nil
}
