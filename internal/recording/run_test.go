package recording

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
	configv1 "github.com/openshift/api/config/v1"
)

func TestRunLifecycle(t *testing.T) {
	for _, status := range []RunStatus{RunStatusStopped, RunStatusFailed} {
		t.Run(string(status), func(t *testing.T) {
			start := time.Date(2026, 9, 29, 12, 0, 0, 123, time.FixedZone("local", 7200))
			r, err := StartRun(t.TempDir(), "test-version", "https://api.example", start)
			if err != nil {
				t.Fatal(err)
			}
			m, err := ReadRunManifest(r.Directory())
			if err != nil {
				t.Fatal(err)
			}
			if m.SchemaVersion != "1" || m.Status != RunStatusRecording || m.Command != "record-live" || m.ToolVersion != "test-version" || m.Source.Server != "https://api.example" || !m.StartedAt.Equal(start) || m.StartedAt.Location() != time.UTC || m.EndedAt != nil {
				t.Fatalf("initial manifest: %+v", m)
			}
			if filepath.Base(r.Directory()) != m.RunID || m.Files.ClusterVersion != "cluster-version.jsonl" || m.Files.OperatorsDirectory != "operators" {
				t.Fatalf("paths: %+v", m)
			}
			data, err := os.ReadFile(filepath.Join(r.Directory(), "run.json"))
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(data, &fields); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"endedAt", "error", "operators"} {
				if _, ok := fields[key]; ok {
					t.Errorf("initial optional %s present", key)
				}
			}
			names := []string{"network", "ingress"}
			var runErr error
			if status == RunStatusFailed {
				runErr = errors.New("disk failure")
			}
			end := start.Add(time.Minute)
			if err := r.Finish(status, end, RunSnapshot{ClusterID: "cluster-a", Operators: names}, runErr); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(names, []string{"network", "ingress"}) {
				t.Fatal("Finish mutated input")
			}
			m, err = ReadRunManifest(r.Directory())
			if err != nil {
				t.Fatal(err)
			}
			if m.Status != status || m.EndedAt == nil || !m.EndedAt.Equal(end) || m.EndedAt.Location() != time.UTC || m.Source.ClusterID != "cluster-a" || !reflect.DeepEqual(m.Operators, []string{"ingress", "network"}) {
				t.Fatalf("final manifest: %+v", m)
			}
			if (status == RunStatusFailed && m.Error != "disk failure") || (status == RunStatusStopped && m.Error != "") {
				t.Fatalf("error=%q", m.Error)
			}
			if _, err := os.Stat(filepath.Join(r.Directory(), "run.json.tmp")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("temporary manifest remains: %v", err)
			}
		})
	}
}

func TestRunCommandCompatibility(t *testing.T) {
	start := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	for _, command := range []string{"record-live", "observe-upgrade"} {
		t.Run(command, func(t *testing.T) {
			run, err := StartRunForCommand(t.TempDir(), "0.2.0", "https://api.example", start, command)
			if err != nil {
				t.Fatal(err)
			}
			if err := run.Finish(RunStatusStopped, start.Add(time.Minute), RunSnapshot{}, nil); err != nil {
				t.Fatal(err)
			}
			manifest, err := ReadRunManifest(run.Directory())
			if err != nil {
				t.Fatal(err)
			}
			if manifest.Command != command || manifest.SchemaVersion != "1" || manifest.Status != RunStatusStopped {
				t.Fatalf("manifest=%+v", manifest)
			}
		})
	}
	for _, command := range []string{"", "unknown"} {
		directory := t.TempDir()
		if _, err := StartRunForCommand(directory, "0.2.0", "server", start, command); err == nil {
			t.Fatalf("accepted command %q", command)
		}
		entries, err := os.ReadDir(directory)
		if err != nil || len(entries) != 0 {
			t.Fatalf("invalid command created artifacts: entries=%v err=%v", entries, err)
		}
	}
}

func TestRunManifestSnapshotDoesNotAlias(t *testing.T) {
	r, err := StartRun(t.TempDir(), "test", "server", time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	end := r.Manifest().StartedAt.Add(time.Minute)
	if err := r.Finish(RunStatusStopped, end, RunSnapshot{Operators: []string{"ingress"}}, nil); err != nil {
		t.Fatal(err)
	}
	snapshot := r.Manifest()
	snapshot.Operators[0] = "tampered"
	*snapshot.EndedAt = time.Time{}
	got := r.Manifest()
	if got.Operators[0] != "ingress" || !got.EndedAt.Equal(end) {
		t.Fatalf("caller mutated Run through snapshot: %+v", got)
	}
}

func TestRunErrors(t *testing.T) {
	start := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	if _, err := StartRun("", "test", "server", start); err == nil {
		t.Fatal("accepted empty directory")
	}
	dir := t.TempDir()
	r, err := StartRun(dir, "test", "server", start)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := StartRun(dir, "test", "server", start); !errors.Is(err, os.ErrExist) {
		t.Fatalf("collision error=%v", err)
	}
	if err := r.Finish(RunStatusRecording, start, RunSnapshot{}, nil); err == nil {
		t.Fatal("accepted recording as final status")
	}
	before, err := os.ReadFile(filepath.Join(r.Directory(), "run.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(r.Directory(), "run.json.tmp"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := r.Finish(RunStatusStopped, start.Add(time.Minute), RunSnapshot{}, nil); err == nil {
		t.Fatal("lost manifest write error")
	}
	after, err := os.ReadFile(filepath.Join(r.Directory(), "run.json"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Manifest().Status != RunStatusRecording {
		t.Error("failed write changed in-memory status")
	}
	if string(before) != string(after) {
		t.Fatal("failed replacement changed existing manifest")
	}
}

func TestReadRunManifestErrors(t *testing.T) {
	for _, tc := range []struct{ name, data, want string }{
		{"missing", "", "read run manifest"},
		{"malformed", "{", "decode run manifest"},
		{"schema", `{"schemaVersion":"2","runId":"run"}`, "unsupported run schema"},
		{"id", `{"schemaVersion":"1"}`, "runId"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.data != "" {
				if err := os.WriteFile(filepath.Join(dir, "run.json"), []byte(tc.data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			_, err := ReadRunManifest(dir)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v, want %q", err, tc.want)
			}
		})
	}
}

func TestRunRecorderTracksOnlyWrittenObservations(t *testing.T) {
	dir := t.TempDir()
	r, err := NewRunRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	v := configv1.ClusterVersion{}
	v.APIVersion = "config.openshift.io/v1"
	v.Kind = "ClusterVersion"
	v.Name = "version"
	v.Spec.ClusterID = "cluster-a"
	if err := os.Mkdir(filepath.Join(dir, "cluster-version.jsonl"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := r.AppendClusterVersion(upgrade.ClusterVersionObservation{ObservedAt: at, ClusterVersion: v}); err == nil {
		t.Fatal("lost write error")
	}
	if got := r.Snapshot().ClusterID; got != "" {
		t.Errorf("failed write recorded cluster ID %q", got)
	}
	if err := os.Remove(filepath.Join(dir, "cluster-version.jsonl")); err != nil {
		t.Fatal(err)
	}
	if err := r.AppendClusterVersion(upgrade.ClusterVersionObservation{ObservedAt: at, ClusterVersion: v}); err != nil {
		t.Fatal(err)
	}
	v.Spec.ClusterID = "cluster-b"
	if err := r.AppendClusterVersion(upgrade.ClusterVersionObservation{ObservedAt: at.Add(time.Second), ClusterVersion: v}); err == nil || !strings.Contains(err.Error(), "cluster ID changed") {
		t.Fatalf("cluster switch error=%v", err)
	}
	versions, err := upgrade.ReadHistory(filepath.Join(dir, "cluster-version.jsonl"))
	if err != nil || len(versions) != 1 {
		t.Fatalf("versions=%v error=%v", versions, err)
	}
	for i, name := range []string{"network", "ingress", "ingress", "../invalid"} {
		o := configv1.ClusterOperator{}
		o.APIVersion = "config.openshift.io/v1"
		o.Kind = "ClusterOperator"
		o.Name = name
		err := r.AppendOperator(operator.Observation{ObservedAt: at.Add(time.Duration(i) * time.Second), Operator: o})
		if (name == "../invalid") != (err != nil) {
			t.Fatalf("append %q: %v", name, err)
		}
	}
	snapshot := r.Snapshot()
	if snapshot.ClusterID != "cluster-a" || !reflect.DeepEqual(snapshot.Operators, []string{"ingress", "network"}) {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	snapshot.Operators[0] = "changed"
	if r.Snapshot().Operators[0] != "ingress" {
		t.Fatal("snapshot aliases recorder")
	}
	observations, err := operator.ReadHistory(filepath.Join(dir, "operators", "ingress.jsonl"))
	if err != nil || len(observations) != 2 {
		t.Fatalf("observations=%v err=%v", observations, err)
	}
}

func TestRunFinishValidation(t *testing.T) {
	start := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		status RunStatus
		end    time.Time
		err    error
	}{
		{"zero end", RunStatusStopped, time.Time{}, nil},
		{"end before start", RunStatusStopped, start.Add(-time.Second), nil},
		{"failed without error", RunStatusFailed, start, nil},
		{"stopped with error", RunStatusStopped, start, errors.New("failure")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := StartRun(t.TempDir(), "test", "server", start)
			if err != nil {
				t.Fatal(err)
			}
			if err := r.Finish(tc.status, tc.end, RunSnapshot{}, tc.err); err == nil {
				t.Fatal("accepted invalid finalization")
			}
			if r.Manifest().Status != RunStatusRecording {
				t.Fatal("invalid finalization changed status")
			}
		})
	}
	r, err := StartRun(t.TempDir(), "test", "server", start)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Finish(RunStatusStopped, start, RunSnapshot{}, nil); err != nil {
		t.Fatal(err)
	}
	if err := r.Finish(RunStatusFailed, start.Add(time.Second), RunSnapshot{}, errors.New("late")); err == nil {
		t.Fatal("overwrote finalized run")
	}
	if _, err := StartRun(t.TempDir(), "test", "server", time.Time{}); err == nil {
		t.Fatal("accepted zero start")
	}
}

func TestReadRunManifestValidation(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		change     func(*RunManifest)
	}{
		{"missing start", "startedAt", func(m *RunManifest) { m.StartedAt = time.Time{} }},
		{"missing end", "endedAt", func(m *RunManifest) { m.EndedAt = nil }},
		{"backwards end", "endedAt", func(m *RunManifest) { at := m.StartedAt.Add(-time.Second); m.EndedAt = &at }},
		{"unknown status", "status", func(m *RunManifest) { m.Status = "bogus" }},
		{"absolute version path", "path", func(m *RunManifest) { m.Files.ClusterVersion = "/tmp/outside.jsonl" }},
		{"operator directory escape", "path", func(m *RunManifest) { m.Files.OperatorsDirectory = "../operators" }},
		{"empty path", "path", func(m *RunManifest) { m.Files.ClusterVersion = "" }},
		{"duplicate operator", "duplicate", func(m *RunManifest) { m.Operators = []string{"ingress", "ingress"} }},
		{"operator name escape", "operator", func(m *RunManifest) { m.Operators = []string{"../ingress"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
			r, err := StartRun(t.TempDir(), "test", "server", start)
			if err != nil {
				t.Fatal(err)
			}
			if err := r.Finish(RunStatusStopped, start, RunSnapshot{}, nil); err != nil {
				t.Fatal(err)
			}
			m := r.Manifest()
			tc.change(&m)
			data, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(r.Directory(), "run.json"), data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := ReadRunManifest(r.Directory()); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v want %q", err, tc.want)
			}
		})
	}
}
