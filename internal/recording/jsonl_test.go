package recording

import (
	"os"
	"path/filepath"
	"sync"
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

func TestJSONLRecorderConcurrentStreams(t *testing.T) {
	dir := t.TempDir()
	r, err := NewJSONLRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup
	for _, name := range []string{"version", "ingress", "network"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				at := start.Add(time.Duration(i) * time.Nanosecond)
				var err error
				if name == "version" {
					v := configv1.ClusterVersion{}
					v.APIVersion = "config.openshift.io/v1"
					v.Kind = "ClusterVersion"
					v.Name = "version"
					err = r.AppendClusterVersion(upgrade.ClusterVersionObservation{ObservedAt: at, ClusterVersion: v})
				} else {
					o := configv1.ClusterOperator{}
					o.APIVersion = "config.openshift.io/v1"
					o.Kind = "ClusterOperator"
					o.Name = name
					err = r.AppendOperator(operator.Observation{ObservedAt: at, Operator: o})
				}
				if err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	v, err := upgrade.ReadHistory(filepath.Join(dir, "cluster-version.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 20 {
		t.Fatalf("versions=%d", len(v))
	}
	if _, err = upgrade.AnalyzeHistory(v); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ingress", "network"} {
		o, err := operator.ReadHistory(filepath.Join(dir, "operators", name+".jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		if len(o) != 20 {
			t.Fatalf("%s observations=%d", name, len(o))
		}
		if _, err = operator.AnalyzeHistory(o); err != nil {
			t.Fatal(err)
		}
	}
}

func TestJSONLRecorderErrors(t *testing.T) {
	dir := t.TempDir()
	r, err := NewJSONLRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "cluster-version.jsonl"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := r.AppendClusterVersion(upgrade.ClusterVersionObservation{}); err == nil {
		t.Fatal("accepted directory as output file")
	}
	o := configv1.ClusterOperator{}
	o.Name = "../escape"
	if err := r.AppendOperator(operator.Observation{Operator: o}); err == nil {
		t.Fatal("accepted invalid name")
	}
	o.Name = "ingress"
	if err := r.AppendOperator(operator.Observation{Operator: o, ObservedAt: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}); err == nil {
		t.Fatal("lost serialization error")
	}
}
