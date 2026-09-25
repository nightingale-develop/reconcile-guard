package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAnalyzeClusterVersionHistory(t *testing.T) {
	fresh := func() []ClusterVersionObservation {
		t.Helper()
		obs, err := readClusterVersionHistory("examples/cluster-version-history.jsonl")
		if err != nil {
			t.Fatal(err)
		}
		return obs
	}
	got, err := analyzeClusterVersionHistory(fresh())
	if err != nil || got != (ClusterVersionHistoryReport{Name: "version", Observations: 3, DesiredVersion: "4.20.0"}) {
		t.Fatalf("got %+v, %v", got, err)
	}
	for _, tc := range []struct {
		name string
		edit func([]ClusterVersionObservation) []ClusterVersionObservation
		want string
	}{
		{"empty", func(o []ClusterVersionObservation) []ClusterVersionObservation { return nil }, "no observations"},
		{"missing timestamp", func(o []ClusterVersionObservation) []ClusterVersionObservation {
			o[1].ObservedAt = time.Time{}
			return o
		}, "observedAt is missing"},
		{"backward timestamp", func(o []ClusterVersionObservation) []ClusterVersionObservation {
			o[1].ObservedAt = o[0].ObservedAt.Add(-time.Second)
			return o
		}, "must be later"},
		{"same timestamp", func(o []ClusterVersionObservation) []ClusterVersionObservation {
			o[1].ObservedAt = o[0].ObservedAt
			return o
		}, "must be later"},
		{"different name", func(o []ClusterVersionObservation) []ClusterVersionObservation {
			o[1].ClusterVersion.Name = "other"
			return o
		}, "name changed"},
		{"invalid apiVersion", func(o []ClusterVersionObservation) []ClusterVersionObservation {
			o[1].ClusterVersion.APIVersion = "v1"
			return o
		}, "unsupported resource"},
		{"invalid kind", func(o []ClusterVersionObservation) []ClusterVersionObservation {
			o[1].ClusterVersion.Kind = "ClusterOperator"
			return o
		}, "unsupported resource"},
		{"missing name", func(o []ClusterVersionObservation) []ClusterVersionObservation {
			o[1].ClusterVersion.Name = ""
			return o
		}, "name is missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := analyzeClusterVersionHistory(tc.edit(fresh()))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v, want %q", err, tc.want)
			}
		})
	}
	obs := fresh()
	obs[2].ClusterVersion.Status.Desired.Version = ""
	got, err = analyzeClusterVersionHistory(obs)
	if err != nil || got.DesiredVersion != "" {
		t.Fatalf("must use last observation even if empty: %+v, %v", got, err)
	}
	got, err = analyzeClusterVersionHistory(fresh()[:1])
	if err != nil || got.Observations != 1 || got.DesiredVersion != "4.19.0" {
		t.Fatalf("single observation: %+v, %v", got, err)
	}
}

func TestReadClusterVersionHistory(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
		count             int
	}{
		{"blank lines", "\n{\"observedAt\":\"2026-09-25T10:00:00Z\",\"clusterVersion\":{}}\n\n", "", 1},
		{"malformed JSONL", "\n{}\n{broken", "line 3: decode JSON", 0},
		{"invalid timestamp", `{"observedAt":"yesterday"}`, "line 1: decode JSON", 0},
		{"empty JSONL", "", "no observations", 0},
		{"whitespace JSONL", " \n\t\n", "no observations", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "history.jsonl")
			if err := os.WriteFile(path, []byte(tc.input), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := readClusterVersionHistory(path)
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("error %v, want %q", err, tc.want)
				}
				return
			}
			if err != nil || len(got) != tc.count {
				t.Fatalf("got %d observations, %v", len(got), err)
			}
		})
	}
	if _, err := readClusterVersionHistory(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected read error")
	}
}
