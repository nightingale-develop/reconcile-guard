package app

import (
	"fmt"
	"sort"

	"github.com/nightingale-develop/reconcile-guard/internal/machineconfig"
	"github.com/nightingale-develop/reconcile-guard/internal/regression"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
	corev1 "k8s.io/api/core/v1"
)

const (
	upgradeObservedSpanTiming     = "upgrade-observed-span"
	poolStableObservedDelayTiming = "post-completion-stable-observed-delay"
	nodeReadyObservedDelayTiming  = "post-completion-ready-observed-delay"
	nodeConfigObservedDelayTiming = "post-completion-config-aligned-observed-delay"
)

func buildRegressionTimings(input runInput, auxiliary auxiliaryRunReport) ([]regression.ObservedTiming, error) {
	states, err := upgrade.AnalyzePhases(input.Versions)
	if err != nil {
		return nil, fmt.Errorf("ClusterVersion timeline: %w", err)
	}
	if len(states) == 0 {
		return []regression.ObservedTiming{}, nil
	}

	final := states[len(states)-1]
	completionIndex := finalTargetCompletion(states, final.DesiredVersion, final.DesiredImage)
	if completionIndex < 0 {
		return []regression.ObservedTiming{}, nil
	}
	completion := states[completionIndex]

	var timings []regression.ObservedTiming
	if startIndex := contiguousUpdatingStart(states, completionIndex, final.DesiredVersion, final.DesiredImage); startIndex >= 0 {
		timings = append(timings, regression.ObservedTiming{
			Name:         upgradeObservedSpanTiming,
			ResourceKind: "ClusterVersion",
			ResourceName: "version",
			From:         states[startIndex].ObservedAt,
			To:           completion.ObservedAt,
		})
	}

	for _, report := range auxiliary.MachineConfigPools {
		for _, finding := range report.Evidence {
			if !finding.Applicable || finding.Correlation.DesiredVersion != final.DesiredVersion || finding.Correlation.DesiredImage != final.DesiredImage || finding.State.Phase != machineconfig.PhaseStable || finding.State.ObservedAt.Before(completion.ObservedAt) {
				continue
			}
			timings = append(timings, regression.ObservedTiming{
				Name:         poolStableObservedDelayTiming,
				ResourceKind: "MachineConfigPool",
				ResourceName: report.Pool,
				From:         completion.ObservedAt,
				To:           finding.State.ObservedAt,
			})
			break
		}
	}

	for _, report := range auxiliary.Nodes {
		readyAdded, configAdded := false, false
		for _, finding := range report.Evidence {
			if !finding.Applicable || finding.Correlation.DesiredVersion != final.DesiredVersion || finding.Correlation.DesiredImage != final.DesiredImage || finding.State.ObservedAt.Before(completion.ObservedAt) {
				continue
			}
			if !readyAdded && finding.State.Ready == corev1.ConditionTrue {
				timings = append(timings, regression.ObservedTiming{
					Name:         nodeReadyObservedDelayTiming,
					ResourceKind: "Node",
					ResourceName: report.Node,
					From:         completion.ObservedAt,
					To:           finding.State.ObservedAt,
				})
				readyAdded = true
			}
			if !configAdded && finding.State.ConfigAligned {
				timings = append(timings, regression.ObservedTiming{
					Name:         nodeConfigObservedDelayTiming,
					ResourceKind: "Node",
					ResourceName: report.Node,
					From:         completion.ObservedAt,
					To:           finding.State.ObservedAt,
				})
				configAdded = true
			}
			if readyAdded && configAdded {
				break
			}
		}
	}

	sort.Slice(timings, func(i, j int) bool {
		if timings[i].ResourceKind != timings[j].ResourceKind {
			return timings[i].ResourceKind < timings[j].ResourceKind
		}
		if timings[i].ResourceName != timings[j].ResourceName {
			return timings[i].ResourceName < timings[j].ResourceName
		}
		return timings[i].Name < timings[j].Name
	})
	return timings, nil
}

func finalTargetCompletion(states []upgrade.UpgradeState, version, image string) int {
	for i := len(states) - 1; i >= 0; i-- {
		state := states[i]
		if state.Phase == upgrade.UpgradePhaseCompleted && state.DesiredVersion == version && state.DesiredImage == image {
			return i
		}
	}
	return -1
}

func contiguousUpdatingStart(states []upgrade.UpgradeState, completionIndex int, version, image string) int {
	if completionIndex <= 0 {
		return -1
	}
	start := completionIndex - 1
	if states[start].Phase != upgrade.UpgradePhaseUpdating || states[start].DesiredVersion != version || states[start].DesiredImage != image {
		return -1
	}
	for start > 0 {
		previous := states[start-1]
		if previous.Phase != upgrade.UpgradePhaseUpdating || previous.DesiredVersion != version || previous.DesiredImage != image {
			break
		}
		start--
	}
	return start
}
