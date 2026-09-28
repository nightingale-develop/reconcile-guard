package recording

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"

	"github.com/nightingale-develop/reconcile-guard/internal/collector"
	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
)

func TestAppendCapture(t *testing.T) {
	directory := t.TempDir()

	observedAt := time.Date(
		2026,
		time.September,
		28,
		10,
		0,
		0,
		0,
		time.UTC,
	)

	version := configv1.ClusterVersion{}
	version.Name = "version"

	clusterOperator := configv1.ClusterOperator{}
	clusterOperator.Name = "ingress"

	capture := collector.Capture{
		ClusterVersion: upgrade.ClusterVersionObservation{
			ObservedAt:     observedAt,
			ClusterVersion: version,
		},
		Operators: []operator.Observation{
			{
				ObservedAt: observedAt,
				Operator:   clusterOperator,
			},
		},
	}

	if err := AppendCapture(
		directory,
		capture,
	); err != nil {
		t.Fatal(err)
	}

	if err := AppendCapture(
		directory,
		capture,
	); err != nil {
		t.Fatal(err)
	}

	paths := []string{
		filepath.Join(
			directory,
			"cluster-version.jsonl",
		),
		filepath.Join(
			directory,
			"operators",
			"ingress.jsonl",
		),
	}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		lines := 0

		for _, value := range data {
			if value == '\n' {
				lines++
			}
		}

		if lines != 2 {
			t.Fatalf(
				"%s lines = %d, want 2",
				path,
				lines,
			)
		}
	}
}
