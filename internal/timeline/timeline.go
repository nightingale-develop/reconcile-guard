package timeline

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/machineconfig"
	nodehistory "github.com/nightingale-develop/reconcile-guard/internal/node"
	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
)

const SchemaVersion = "1"

type Event struct {
	Kind         string            `json:"kind"`
	ResourceKind string            `json:"resourceKind"`
	ResourceName string            `json:"resourceName"`
	ObservedAt   *time.Time        `json:"observedAt,omitempty"`
	From         *time.Time        `json:"from,omitempty"`
	To           *time.Time        `json:"to,omitempty"`
	Summary      string            `json:"summary"`
	Attributes   map[string]string `json:"attributes,omitempty"`
}

type Report struct {
	Events []Event `json:"events"`
}

type Document struct {
	SchemaVersion string `json:"schemaVersion"`
	Command       string `json:"command"`
	RunID         string `json:"runId"`
	ClusterID     string `json:"clusterId,omitempty"`
	Timeline      Report `json:"timeline"`
}

func NewDocument(command, runID, clusterID string, report Report) Document {
	return Document{
		SchemaVersion: SchemaVersion,
		Command:       command,
		RunID:         runID,
		ClusterID:     clusterID,
		Timeline:      report,
	}
}

func Build(
	versions []upgrade.ClusterVersionObservation,
	operators [][]operator.Observation,
	pools [][]machineconfig.Observation,
	nodes [][]nodehistory.Observation,
) (Report, error) {
	states, err := upgrade.AnalyzePhases(versions)
	if err != nil {
		return Report{}, fmt.Errorf("ClusterVersion timeline: %w", err)
	}

	var events []Event
	for i, state := range states {
		if i > 0 {
			previous := states[i-1]
			if previous.Phase == state.Phase &&
				previous.DesiredVersion == state.DesiredVersion &&
				previous.DesiredImage == state.DesiredImage {
				continue
			}
		}

		observedAt := utc(state.ObservedAt)
		events = append(events, Event{
			Kind:         "cluster-version-state",
			ResourceKind: "ClusterVersion",
			ResourceName: "version",
			ObservedAt:   &observedAt,
			Summary:      clusterVersionSummary(state),
			Attributes: map[string]string{
				"phase":          string(state.Phase),
				"desiredVersion": state.DesiredVersion,
				"desiredImage":   state.DesiredImage,
			},
		})
	}

	for i, history := range operators {
		report, err := operator.AnalyzeHistory(history)
		if err != nil {
			return Report{}, fmt.Errorf("operator history %d: %w", i+1, err)
		}
		for _, transition := range report.Transitions {
			from, to := utc(transition.FromTime), utc(transition.ToTime)
			events = append(events, Event{
				Kind:         "operator-condition-transition",
				ResourceKind: "ClusterOperator",
				ResourceName: report.Operator,
				From:         &from,
				To:           &to,
				Summary: fmt.Sprintf(
					"%s %s -> %s",
					transition.Condition,
					transition.From,
					transition.To,
				),
				Attributes: map[string]string{
					"condition": string(transition.Condition),
					"from":      string(transition.From),
					"to":        string(transition.To),
				},
			})
		}
	}

	for i, history := range pools {
		report, err := machineconfig.AnalyzeLifecycle(history)
		if err != nil {
			return Report{}, fmt.Errorf("MachineConfigPool history %d: %w", i+1, err)
		}
		for j := 1; j < len(report.States); j++ {
			previous, current := report.States[j-1], report.States[j]
			if samePoolState(previous, current) {
				continue
			}
			from, to := utc(previous.ObservedAt), utc(current.ObservedAt)
			events = append(events, Event{
				Kind:         "machine-config-pool-transition",
				ResourceKind: "MachineConfigPool",
				ResourceName: report.Pool,
				From:         &from,
				To:           &to,
				Summary:      poolSummary(previous, current),
				Attributes: map[string]string{
					"phaseFrom":            string(previous.Phase),
					"phaseTo":              string(current.Phase),
					"currentConfigFrom":    previous.CurrentConfiguration,
					"currentConfigTo":      current.CurrentConfiguration,
					"desiredConfigFrom":    previous.DesiredConfiguration,
					"desiredConfigTo":      current.DesiredConfiguration,
					"degradedMachineCount": fmt.Sprint(current.DegradedMachineCount),
				},
			})
		}
	}

	for i, history := range nodes {
		report, err := nodehistory.AnalyzeLifecycle(history)
		if err != nil {
			return Report{}, fmt.Errorf("Node history %d: %w", i+1, err)
		}
		for _, transition := range report.Transitions {
			from, to := utc(transition.FromTime), utc(transition.ToTime)
			events = append(events, Event{
				Kind:         "node-transition",
				ResourceKind: "Node",
				ResourceName: report.Node,
				From:         &from,
				To:           &to,
				Summary:      nodeSummary(transition),
				Attributes: map[string]string{
					"readyFrom":          string(transition.ReadyFrom),
					"readyTo":            string(transition.ReadyTo),
					"currentConfigFrom":  transition.CurrentConfigFrom,
					"currentConfigTo":    transition.CurrentConfigTo,
					"desiredConfigFrom":  transition.DesiredConfigFrom,
					"desiredConfigTo":    transition.DesiredConfigTo,
					"kubeletVersionFrom": transition.KubeletVersionFrom,
					"kubeletVersionTo":   transition.KubeletVersionTo,
				},
			})
		}
	}

	sort.SliceStable(events, func(i, j int) bool {
		left, right := sortTime(events[i]), sortTime(events[j])
		if !left.Equal(right) {
			return left.Before(right)
		}
		if events[i].ResourceKind != events[j].ResourceKind {
			return events[i].ResourceKind < events[j].ResourceKind
		}
		if events[i].ResourceName != events[j].ResourceName {
			return events[i].ResourceName < events[j].ResourceName
		}
		if events[i].Kind != events[j].Kind {
			return events[i].Kind < events[j].Kind
		}
		return events[i].Summary < events[j].Summary
	})

	return Report{Events: events}, nil
}

func clusterVersionSummary(state upgrade.UpgradeState) string {
	summary := fmt.Sprintf("phase %s", state.Phase)
	if state.DesiredVersion != "" {
		summary += fmt.Sprintf(", desired version %s", state.DesiredVersion)
	}
	if state.DesiredImage != "" {
		summary += fmt.Sprintf(", desired image %s", state.DesiredImage)
	}
	return summary
}

func samePoolState(left, right machineconfig.State) bool {
	return left.Phase == right.Phase &&
		left.CurrentConfiguration == right.CurrentConfiguration &&
		left.DesiredConfiguration == right.DesiredConfiguration &&
		left.MachineCount == right.MachineCount &&
		left.UpdatedMachineCount == right.UpdatedMachineCount &&
		left.ReadyMachineCount == right.ReadyMachineCount &&
		left.UnavailableMachineCount == right.UnavailableMachineCount &&
		left.DegradedMachineCount == right.DegradedMachineCount &&
		left.Generation == right.Generation &&
		left.ObservedGeneration == right.ObservedGeneration
}

func poolSummary(previous, current machineconfig.State) string {
	parts := make([]string, 0, 3)
	if previous.Phase != current.Phase {
		parts = append(parts, fmt.Sprintf("phase %s -> %s", previous.Phase, current.Phase))
	}
	if previous.CurrentConfiguration != current.CurrentConfiguration {
		parts = append(parts, fmt.Sprintf("current config %s -> %s", display(previous.CurrentConfiguration), display(current.CurrentConfiguration)))
	}
	if previous.DesiredConfiguration != current.DesiredConfiguration {
		parts = append(parts, fmt.Sprintf("desired config %s -> %s", display(previous.DesiredConfiguration), display(current.DesiredConfiguration)))
	}
	if len(parts) == 0 {
		parts = append(parts, fmt.Sprintf(
			"counts updated=%d/%d ready=%d/%d unavailable=%d degraded=%d",
			current.UpdatedMachineCount,
			current.MachineCount,
			current.ReadyMachineCount,
			current.MachineCount,
			current.UnavailableMachineCount,
			current.DegradedMachineCount,
		))
	}
	return strings.Join(parts, "; ")
}

func nodeSummary(transition nodehistory.Transition) string {
	parts := make([]string, 0, 4)
	if transition.ReadyFrom != transition.ReadyTo {
		parts = append(parts, fmt.Sprintf("Ready %s -> %s", transition.ReadyFrom, transition.ReadyTo))
	}
	if transition.CurrentConfigFrom != transition.CurrentConfigTo {
		parts = append(parts, fmt.Sprintf("current config %s -> %s", display(transition.CurrentConfigFrom), display(transition.CurrentConfigTo)))
	}
	if transition.DesiredConfigFrom != transition.DesiredConfigTo {
		parts = append(parts, fmt.Sprintf("desired config %s -> %s", display(transition.DesiredConfigFrom), display(transition.DesiredConfigTo)))
	}
	if transition.KubeletVersionFrom != transition.KubeletVersionTo {
		parts = append(parts, fmt.Sprintf("kubelet %s -> %s", display(transition.KubeletVersionFrom), display(transition.KubeletVersionTo)))
	}
	if len(parts) == 0 {
		return "node state changed"
	}
	return strings.Join(parts, "; ")
}

func display(value string) string {
	if value == "" {
		return "<missing>"
	}
	return value
}

func sortTime(event Event) time.Time {
	if event.ObservedAt != nil {
		return *event.ObservedAt
	}
	if event.To != nil {
		return *event.To
	}
	if event.From != nil {
		return *event.From
	}
	return time.Time{}
}

func utc(value time.Time) time.Time {
	return value.UTC()
}
