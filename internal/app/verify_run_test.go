package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/recording"
	"github.com/nightingale-develop/reconcile-guard/internal/result"
)

func verifyRunFixture(t *testing.T) string {
	t.Helper()
	start := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	run, err := recording.StartRun(t.TempDir(), "test", "https://api.example", start)
	if err != nil {
		t.Fatal(err)
	}
	dir := run.Directory()
	if err := os.Mkdir(filepath.Join(dir, "operators"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, file := range []struct{ src, dst string }{
		{"cluster-version-history.jsonl", "cluster-version.jsonl"},
		{"ingress-version-history.jsonl", "operators/ingress.jsonl"},
		{"network-version-history.jsonl", "operators/network.jsonl"},
	} {
		data, err := os.ReadFile(filepath.Join("../../examples", file.src))
		if err != nil {
			t.Fatal(err)
		}
		writeRunTestFile(t, filepath.Join(dir, file.dst), data)
	}
	if err := run.Finish(recording.RunStatusStopped, start.Add(time.Hour), recording.RunSnapshot{Operators: []string{"network", "ingress"}}, nil); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeRunTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
func changeRunManifest(t *testing.T, dir string, change func(*recording.RunManifest)) {
	t.Helper()
	m, err := recording.ReadRunManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	change(&m)
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	writeRunTestFile(t, filepath.Join(dir, "run.json"), data)
}
func changeRunHistory(t *testing.T, dir, old, new string) {
	t.Helper()
	path := filepath.Join(dir, "operators", "ingress.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(old)) {
		t.Fatalf("fixture does not contain %q", old)
	}
	writeRunTestFile(t, path, bytes.ReplaceAll(data, []byte(old), []byte(new)))
}

func TestVerifyRunVerdicts(t *testing.T) {
	for _, tc := range []struct {
		name, old, new, verdict string
		code                    int
	}{
		{name: "pass", verdict: "PASS", code: 0},
		{name: "fail", old: `"name":"operator","version":"4.20.0"`, new: `"name":"operator","version":"4.19.0"`, verdict: "FAIL", code: 2},
		{name: "missing version evidence", old: `"versions":[{"name":"operator","version":"4.20.0"}]`, new: `"versions":[]`, verdict: "INCONCLUSIVE", code: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := verifyRunFixture(t)
			if tc.old != "" {
				changeRunHistory(t, dir, tc.old, tc.new)
			}
			for _, format := range []string{"text", "json"} {
				t.Run(format, func(t *testing.T) {
					var out, stderr bytes.Buffer
					code := Run([]string{"verify-run", dir, "--output", format}, &out, &stderr)
					if code != tc.code || stderr.Len() != 0 {
						t.Fatalf("exit=%d want=%d stdout=%s stderr=%s", code, tc.code, &out, &stderr)
					}
					if format == "text" {
						if !strings.Contains(out.String(), "Aggregate verdict: "+tc.verdict) || !strings.Contains(out.String(), "Operators: 2") {
							t.Fatalf("output=%s", &out)
						}
						return
					}
					var doc result.Document
					if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
						t.Fatal(err)
					}
					if doc.SchemaVersion != "1" || doc.Command != "verify-run" || string(doc.Result.Verdict) != tc.verdict || len(doc.Result.Operators) != 2 {
						t.Fatalf("document=%+v", doc)
					}
					if doc.Result.Operators[0].Name != "ingress" || doc.Result.Operators[1].Name != "network" {
						t.Fatalf("operator order=%+v", doc.Result.Operators)
					}
				})
			}
		})
	}
}

func TestVerifyLegacyRunWithoutAuxiliaryFields(t *testing.T) {
	directory := verifyRunFixture(t)
	changeRunManifest(t, directory, func(m *recording.RunManifest) {
		m.Files.MachineConfigPoolsDirectory, m.Files.NodesDirectory = "", ""
		m.MachineConfigPools, m.Nodes = nil, nil
	})
	for _, format := range []string{"text", "json"} {
		var out, stderr bytes.Buffer
		if code := Run([]string{"verify-run", directory, "--output", format}, &out, &stderr); code != 0 || stderr.Len() != 0 {
			t.Fatalf("legacy verification exit=%d output=%s error=%s", code, &out, &stderr)
		}
	}
}

func TestVerifyRunRejectsIncompleteOrInvalidInputs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*testing.T, string)
	}{
		{"recording", func(t *testing.T, d string) {
			changeRunManifest(t, d, func(m *recording.RunManifest) { m.Status = recording.RunStatusRecording; m.EndedAt = nil })
		}},
		{"failed", func(t *testing.T, d string) {
			changeRunManifest(t, d, func(m *recording.RunManifest) { m.Status = recording.RunStatusFailed; m.Error = "watch failed" })
		}},
		{"unknown status", func(t *testing.T, d string) {
			changeRunManifest(t, d, func(m *recording.RunManifest) { m.Status = "bogus" })
		}},
		{"missing declared history", func(t *testing.T, d string) {
			if err := os.Remove(filepath.Join(d, "operators", "network.jsonl")); err != nil {
				t.Fatal(err)
			}
		}},
		{"unlisted history", func(t *testing.T, d string) {
			changeRunManifest(t, d, func(m *recording.RunManifest) { m.Operators = []string{"ingress"} })
		}},
		{"duplicate declared operator", func(t *testing.T, d string) {
			changeRunManifest(t, d, func(m *recording.RunManifest) { m.Operators = []string{"ingress", "network", "network"} })
		}},
		{"operator filename mismatch", func(t *testing.T, d string) { changeRunHistory(t, d, `"name":"ingress"`, `"name":"authentication"`) }},
		{"missing manifest", func(t *testing.T, d string) {
			if err := os.Remove(filepath.Join(d, "run.json")); err != nil {
				t.Fatal(err)
			}
		}},
		{"malformed history", func(t *testing.T, d string) {
			writeRunTestFile(t, filepath.Join(d, "operators", "network.jsonl"), []byte("{\n"))
		}},
		{"empty version history", func(t *testing.T, d string) { writeRunTestFile(t, filepath.Join(d, "cluster-version.jsonl"), nil) }},
		{"no operators", func(t *testing.T, d string) {
			if err := os.RemoveAll(filepath.Join(d, "operators")); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(d, "operators"), 0755); err != nil {
				t.Fatal(err)
			}
			changeRunManifest(t, d, func(m *recording.RunManifest) { m.Operators = nil })
		}},
		{"path escape", func(t *testing.T, d string) {
			data, err := os.ReadFile(filepath.Join(d, "cluster-version.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			writeRunTestFile(t, filepath.Join(filepath.Dir(d), "outside.jsonl"), data)
			changeRunManifest(t, d, func(m *recording.RunManifest) { m.Files.ClusterVersion = "../outside.jsonl" })
		}},
		{"symlink escape", func(t *testing.T, d string) {
			data, err := os.ReadFile(filepath.Join(d, "cluster-version.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			outside := filepath.Join(t.TempDir(), "outside.jsonl")
			writeRunTestFile(t, outside, data)
			path := filepath.Join(d, "cluster-version.jsonl")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, path); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := verifyRunFixture(t)
			tc.change(t, dir)
			for _, format := range []string{"text", "json"} {
				var out, stderr bytes.Buffer
				code := Run([]string{"verify-run", dir, "--output", format}, &out, &stderr)
				if code != 1 || out.Len() != 0 || stderr.Len() == 0 {
					t.Errorf("%s: exit=%d stdout=%s stderr=%s", format, code, &out, &stderr)
				}
			}
		})
	}
}

func TestVerifyRunUsage(t *testing.T) {
	for _, args := range [][]string{{"verify-run"}, {"verify-run", "one", "two"}, {"verify-run", "missing", "--output=xml"}} {
		var out, stderr bytes.Buffer
		if code := Run(args, &out, &stderr); code != 1 || out.Len() != 0 || stderr.Len() == 0 {
			t.Fatalf("args=%v exit=%d stdout=%s stderr=%s", args, code, &out, &stderr)
		}
	}
}

func TestVerifyRunRejectsMixedClusterEvidence(t *testing.T) {
	for _, tc := range []struct{ name, first, rest, manifest string }{
		{"manifest mismatch", "cluster-a", "cluster-a", "cluster-b"},
		{"history changes cluster", "cluster-a", "cluster-b", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := verifyRunFixture(t)
			path := filepath.Join(dir, "cluster-version.jsonl")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
			var output bytes.Buffer
			for i, line := range lines {
				var observation map[string]any
				if err := json.Unmarshal(line, &observation); err != nil {
					t.Fatal(err)
				}
				version := observation["clusterVersion"].(map[string]any)
				spec, ok := version["spec"].(map[string]any)
				if !ok {
					spec = map[string]any{}
					version["spec"] = spec
				}
				id := tc.rest
				if i == 0 {
					id = tc.first
				}
				spec["clusterID"] = id
				if err := json.NewEncoder(&output).Encode(observation); err != nil {
					t.Fatal(err)
				}
			}
			writeRunTestFile(t, path, output.Bytes())
			changeRunManifest(t, dir, func(m *recording.RunManifest) { m.Source.ClusterID = tc.manifest })
			for _, format := range []string{"text", "json"} {
				var out, stderr bytes.Buffer
				code := Run([]string{"verify-run", dir, "--output", format}, &out, &stderr)
				if code != 1 || out.Len() != 0 || !strings.Contains(strings.ToLower(stderr.String()), "cluster") {
					t.Fatalf("%s: exit=%d stdout=%s stderr=%s", format, code, &out, &stderr)
				}
			}
		})
	}
}

func TestVerifyRunIncludesMachineConfigPoolAndNodeEvidence(t *testing.T) {
	dir := verifyRunFixture(t)
	if err := os.Mkdir(filepath.Join(dir, "machine-config-pools"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "nodes"), 0755); err != nil {
		t.Fatal(err)
	}

	pool := `{"observedAt":"2026-09-25T10:10:00Z","machineConfigPool":{"apiVersion":"machineconfiguration.openshift.io/v1","kind":"MachineConfigPool","metadata":{"name":"master"},"spec":{"configuration":{"name":"rendered-b"}},"status":{"configuration":{"name":"rendered-b"},"machineCount":1,"updatedMachineCount":1,"readyMachineCount":1,"unavailableMachineCount":0,"degradedMachineCount":0,"conditions":[{"type":"Updated","status":"True"},{"type":"Updating","status":"False"},{"type":"Degraded","status":"False"}]}}}` + "\n"
	node := `{"observedAt":"2026-09-25T10:10:00Z","node":{"apiVersion":"v1","kind":"Node","metadata":{"name":"node-0","annotations":{"machineconfiguration.openshift.io/currentConfig":"rendered-b","machineconfiguration.openshift.io/desiredConfig":"rendered-b"}},"status":{"conditions":[{"type":"Ready","status":"True"}],"nodeInfo":{"kubeletVersion":"v1.34.4"}}}}` + "\n"
	writeRunTestFile(t, filepath.Join(dir, "machine-config-pools", "master.jsonl"), []byte(pool))
	writeRunTestFile(t, filepath.Join(dir, "nodes", "node-0.jsonl"), []byte(node))
	changeRunManifest(t, dir, func(m *recording.RunManifest) {
		m.MachineConfigPools = []string{"master"}
		m.Nodes = []string{"node-0"}
	})

	for _, format := range []string{"text", "json"} {
		var out, stderr bytes.Buffer
		if code := Run([]string{"verify-run", dir, "--output", format}, &out, &stderr); code != 0 || stderr.Len() != 0 {
			t.Fatalf("%s: exit=%d stdout=%s stderr=%s", format, code, &out, &stderr)
		}
		if format == "text" {
			for _, want := range []string{"MachineConfigPool lifecycle evidence:", "Pool: master", "Node lifecycle evidence:", "Node: node-0", "evidence-only"} {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("missing %q in %s", want, &out)
				}
			}
			continue
		}
		var doc result.Document
		if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
			t.Fatal(err)
		}
		if len(doc.Result.MachineConfigPools) != 1 || len(doc.Result.Nodes) != 1 {
			t.Fatalf("document=%+v", doc)
		}
		if doc.Result.MachineConfigPools[0].Verdict != result.VerdictPass || doc.Result.Nodes[0].Verdict != result.VerdictPass {
			t.Fatalf("auxiliary verdicts=%+v %+v", doc.Result.MachineConfigPools, doc.Result.Nodes)
		}
	}
}
