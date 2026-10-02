package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/timeline"
)

func (c cli) timelineRun(args []string) int {
	if len(args) != 2 {
		fmt.Fprintln(c.stderr, "Usage: reconcile-guard timeline-run <run-directory>")
		return 1
	}

	input, err := loadRunInput(args[1])
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}
	report, err := timeline.Build(input.Versions, input.Histories, input.MachineConfigPools, input.Nodes)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}

	if c.output == outputJSON {
		encoder := json.NewEncoder(c.stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(timeline.NewDocument("timeline-run", input.Manifest.RunID, input.Manifest.Source.ClusterID, report)); err != nil {
			fmt.Fprintln(c.stderr, "Error: encode JSON output:", err)
			return 1
		}
		return 0
	}

	var text bytes.Buffer
	fmt.Fprintln(&text, "Run:", input.Manifest.RunID)
	fmt.Fprintln(&text, "Cluster ID:", input.Manifest.Source.ClusterID)
	fmt.Fprintln(&text, "Timeline events:", len(report.Events))
	for _, event := range report.Events {
		fmt.Fprintf(&text, "  %s  %s/%s  %s\n", timelineEventTime(event), event.ResourceKind, event.ResourceName, event.Summary)
	}
	fmt.Fprintln(&text, "Transition timestamps are observation bounds, not inferred exact transition times.")
	if _, err := text.WriteTo(c.stdout); err != nil {
		fmt.Fprintln(c.stderr, "Error: write timeline output:", err)
		return 1
	}
	return 0
}

func timelineEventTime(event timeline.Event) string {
	if event.ObservedAt != nil {
		return event.ObservedAt.UTC().Format(time.RFC3339Nano)
	}
	if event.From != nil && event.To != nil {
		return fmt.Sprintf("[%s, %s]", event.From.UTC().Format(time.RFC3339Nano), event.To.UTC().Format(time.RFC3339Nano))
	}
	if event.To != nil {
		return event.To.UTC().Format(time.RFC3339Nano)
	}
	if event.From != nil {
		return event.From.UTC().Format(time.RFC3339Nano)
	}
	return "unknown-time"
}
