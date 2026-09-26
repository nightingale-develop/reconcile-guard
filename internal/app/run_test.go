package app

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nightingale-develop/reconcile-guard/internal/contracts"
)

func TestCheckFileRejectsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.json")

	err := os.WriteFile(path, []byte("{invalid"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	_, err = (cli{stdout: io.Discard, stderr: io.Discard}).checkFile(path)

	if err == nil {
		t.Fatal("expected JSON decoding error")
	}

	if !strings.Contains(err.Error(), "decode JSON") {
		t.Fatalf("unexpected error: %v", err)
	}
}
func TestContractExitCode(t *testing.T) {
	tests := []struct {
		verdict contracts.ContractVerdict
		want    int
	}{
		{contracts.ContractPass, 0},
		{contracts.ContractFail, 2},
		{contracts.ContractInconclusive, 3},
	}

	for _, tc := range tests {
		if got := contractExitCode(tc.verdict); got != tc.want {
			t.Errorf(
				"exit code = %d, want %d",
				got,
				tc.want,
			)
		}
	}
}

func TestRun(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "bad.jsonl")
	if err := os.WriteFile(bad, []byte("{bad"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name        string
		args        []string
		code        int
		out, stderr string
	}{
		{"help", []string{"help"}, 0, "verify-upgrade", ""},
		{"no command", nil, 0, "Usage:", ""},
		{"version", []string{"version"}, 0, "v0.1.0-dev", ""},
		{"unknown", []string{"bogus"}, 1, "", "Unknown command:"},
		{"check ok", []string{"check", "../../examples/ingress-ok.json"}, 0, "NOT DEGRADED (reported)", ""},
		{"check degraded", []string{"check", "../../examples/ingress.json"}, 2, "Result: DEGRADED", ""},
		{"check version", []string{"check-version", "../../examples/cluster-version.json"}, 0, "snapshot report only", ""},
		{"replay", []string{"replay", "../../examples/ingress-history.jsonl"}, 0, "Observed transitions: 2", ""},
		{"replay version", []string{"replay-version", "../../examples/cluster-version-history.jsonl"}, 0, "COMPLETED", ""},
		{"verify", []string{"verify-upgrade", "../../examples/cluster-version-history.jsonl", "../../examples/ingress-upgrade-history.jsonl"}, 0, "Verdict: PASS", ""},
		{"verify outside", []string{"verify-upgrade", "../../examples/cluster-version-history.jsonl", "../../examples/ingress-history.jsonl"}, 3, "Verdict: INCONCLUSIVE", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, stderr bytes.Buffer
			code := Run(tc.args, &out, &stderr)
			if code != tc.code || !strings.Contains(out.String(), tc.out) || !strings.Contains(stderr.String(), tc.stderr) {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
			}
			if tc.stderr == "" && stderr.Len() != 0 {
				t.Fatalf("unexpected stderr: %s", &stderr)
			}
			if tc.out == "" && out.Len() != 0 {
				t.Fatalf("unexpected stdout: %s", &out)
			}
		})
	}
	for _, command := range []string{"check", "check-version", "replay", "replay-version", "verify-upgrade"} {
		for _, args := range [][]string{{command}, {command, bad}, {command, "missing-file.jsonl"}} {
			if command == "verify-upgrade" && len(args) > 1 {
				args = append(args, bad)
			}
			var out, stderr bytes.Buffer
			if code := Run(args, &out, &stderr); code != 1 || out.Len() != 0 || stderr.Len() == 0 {
				t.Fatalf("%v: exit %d stdout=%s stderr=%s", args, code, &out, &stderr)
			}
		}
	}
	// Concrete failure must reach the CLI with evidence and exit code 2.
	data, err := os.ReadFile("../../examples/ingress-upgrade-history.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.ReplaceAll(data, []byte(`"type":"Degraded","status":"False"`), []byte(`"type":"Degraded","status":"True"`))
	fail := filepath.Join(t.TempDir(), "failure.jsonl")
	if err := os.WriteFile(fail, data, 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	code := Run([]string{"verify-upgrade", "../../examples/cluster-version-history.jsonl", fail}, &out, &stderr)
	if code != 2 || !strings.Contains(out.String(), "Degraded=True") || !strings.Contains(out.String(), "correlation=EXACT") || stderr.Len() != 0 {
		t.Fatalf("exit %d: %s %s", code, &out, &stderr)
	}
	// Snapshot Unknown remains diagnostic exit code 3.
	var snapshot map[string]any
	data, err = os.ReadFile("../../examples/ingress-ok.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot["status"] = map[string]any{}
	data, err = json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(fail, data, 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	stderr.Reset()
	if code = Run([]string{"check", fail}, &out, &stderr); code != 3 {
		t.Fatalf("missing Degraded: exit %d", code)
	}
}
