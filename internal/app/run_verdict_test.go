package app

import (
	"bytes"
	"encoding/json"
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nightingale-develop/reconcile-guard/internal/result"
)

func TestRunVerdictReasonsAcrossFormats(t *testing.T) {
	for _, tc := range []struct {
		name, path, old, replacement, code, kind, nameOfResource, observedAt, correlation string
		missing                                                                           bool
	}{
		{name: "missing histories", missing: true, code: "missing-histories", kind: "MachineConfigPool"},
		{name: "node not ready", path: "nodes/node-0.jsonl", old: `"type":"Ready","status":"True"`, replacement: `"type":"Ready","status":"False"`, code: "node-not-ready", kind: "Node", nameOfResource: "node-0", observedAt: "2026-09-25T10:10:00Z", correlation: "EXACT"},
		{name: "node config mismatch", path: "nodes/node-0.jsonl", old: `"machineconfiguration.openshift.io/desiredConfig":"rendered-b"`, replacement: `"machineconfiguration.openshift.io/desiredConfig":"rendered-c"`, code: "node-config-not-aligned", kind: "Node", nameOfResource: "node-0", observedAt: "2026-09-25T10:10:00Z", correlation: "EXACT"},
		{name: "pool updating", path: "machine-config-pools/master.jsonl", old: `"type":"Updated","status":"True"},{"type":"Updating","status":"False"`, replacement: `"type":"Updated","status":"False"},{"type":"Updating","status":"True"`, code: "pool-not-stable", kind: "MachineConfigPool", nameOfResource: "master", observedAt: "2026-09-25T10:10:00Z", correlation: "EXACT"},
		{name: "ambiguous node sample", path: "nodes/node-0.jsonl", old: "2026-09-25T10:10:00Z", replacement: "2026-09-25T10:07:00Z", code: "last-observation-not-applicable", kind: "Node", nameOfResource: "node-0", observedAt: "2026-09-25T10:07:00Z", correlation: "AMBIGUOUS"},
		{name: "outside pool sample", path: "machine-config-pools/master.jsonl", old: "2026-09-25T10:10:00Z", replacement: "2026-09-25T10:11:00Z", code: "last-observation-not-applicable", kind: "MachineConfigPool", nameOfResource: "master", observedAt: "2026-09-25T10:11:00Z", correlation: "OUTSIDE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var dir string
			if tc.missing {
				dir = verifyRunFixture(t)
			} else {
				dir = verifyRunWithAuxiliaryFixture(t)
			}
			if tc.path != "" {
				path := filepath.Join(dir, tc.path)
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Contains(data, []byte(tc.old)) {
					t.Fatal("fixture lacks replacement target")
				}
				writeRunTestFile(t, path, bytes.ReplaceAll(data, []byte(tc.old), []byte(tc.replacement)))
			}
			var out, stderr bytes.Buffer
			if code := Run([]string{"verify-run", dir, "--output=json"}, &out, &stderr); code != 3 || stderr.Len() != 0 {
				t.Fatalf("exit=%d stdout=%s stderr=%s", code, &out, &stderr)
			}
			var doc struct {
				SchemaVersion string `json:"schemaVersion"`
				Result        struct {
					Verdict string `json:"verdict"`
					Reasons []struct {
						Code, Message, ResourceKind, ResourceName, ObservedAt, Correlation string
					} `json:"reasons"`
				} `json:"result"`
			}
			if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
				t.Fatal(err)
			}
			if doc.SchemaVersion != "1" || doc.Result.Verdict != "INCONCLUSIVE" || len(doc.Result.Reasons) == 0 {
				t.Fatalf("missing verdict reasons: %s", &out)
			}
			reason := doc.Result.Reasons[0]
			if reason.Code != tc.code || reason.ResourceKind != tc.kind || reason.ResourceName != tc.nameOfResource || reason.ObservedAt != tc.observedAt || reason.Correlation != tc.correlation || reason.Message == "" {
				t.Fatalf("reason=%+v", reason)
			}
			if tc.missing && (len(doc.Result.Reasons) != 2 || doc.Result.Reasons[1].ResourceKind != "Node") {
				t.Fatalf("missing Node reason: %+v", doc.Result.Reasons)
			}
			for _, command := range []string{"verify-run", "report-run"} {
				out.Reset()
				stderr.Reset()
				wantCode := 3
				if command == "report-run" {
					wantCode = 0
				}
				if code := Run([]string{command, dir}, &out, &stderr); code != wantCode || stderr.Len() != 0 {
					t.Fatalf("%s exit=%d stdout=%s stderr=%s", command, code, &out, &stderr)
				}
				for _, want := range []string{reason.Code, reason.Message, reason.ResourceKind, reason.ResourceName, reason.ObservedAt, reason.Correlation} {
					if !strings.Contains(html.UnescapeString(out.String()), want) {
						t.Fatalf("%s missing %q: %s", command, want, &out)
					}
				}
			}
		})
	}
}

func TestRunVerdictReasonsPreserveOperatorPrecedence(t *testing.T) {
	for _, tc := range []struct {
		verdict string
		code    int
	}{{"PASS", 0}, {"FAIL", 2}, {"INCONCLUSIVE", 3}} {
		t.Run(tc.verdict, func(t *testing.T) {
			dir := verifyRunWithAuxiliaryFixture(t)
			setComparisonFixtureVerdict(t, dir, tc.verdict)
			if tc.verdict != "PASS" {
				path := filepath.Join(dir, "nodes/node-0.jsonl")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				writeRunTestFile(t, path, bytes.ReplaceAll(data, []byte(`"type":"Ready","status":"True"`), []byte(`"type":"Ready","status":"False"`)))
			}
			var out, stderr bytes.Buffer
			if code := Run([]string{"verify-run", dir, "--output=json"}, &out, &stderr); code != tc.code || stderr.Len() != 0 {
				t.Fatalf("exit=%d stdout=%s stderr=%s", code, &out, &stderr)
			}
			var doc result.Document
			if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
				t.Fatal(err)
			}
			if string(doc.Result.Verdict) != tc.verdict || doc.Result.Details.Values["operatorVerdict"] != tc.verdict || len(doc.Result.Operators) != 2 {
				t.Fatalf("result=%+v", doc.Result)
			}
			if tc.verdict == "PASS" {
				if bytes.Contains(out.Bytes(), []byte(`"reasons"`)) {
					t.Fatalf("PASS contains reasons: %s", &out)
				}
			} else if len(doc.Result.Reasons) != 1 || doc.Result.Reasons[0].Code != "operator-contracts-not-passed" || doc.Result.Reasons[0].ResourceName != "ingress" {
				t.Fatalf("operator precedence reasons=%+v", doc.Result.Reasons)
			}
			first := append([]byte(nil), out.Bytes()...)
			out.Reset()
			if code := Run([]string{"verify-run", dir, "--output=json"}, &out, &stderr); code != tc.code || !bytes.Equal(first, out.Bytes()) {
				t.Fatalf("non-deterministic verification output: code=%d", code)
			}
		})
	}
}
