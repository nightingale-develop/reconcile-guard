package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/nightingale-develop/reconcile-guard/internal/recording"
	"github.com/nightingale-develop/reconcile-guard/internal/regression"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
)

func setComparisonFixtureVerdict(t *testing.T, dir, verdict string) {
	t.Helper()
	switch verdict {
	case "FAIL":
		changeRunHistory(t, dir, `"name":"operator","version":"4.20.0"`, `"name":"operator","version":"4.19.0"`)
	case "INCONCLUSIVE":
		changeRunHistory(t, dir, `"versions":[{"name":"operator","version":"4.20.0"}]`, `"versions":[]`)
	}
}

func TestCompareRunsVerdicts(t *testing.T) {
	for _, tc := range []struct {
		before, after, verdict, change string
		code                           int
	}{
		{"PASS", "PASS", "PASS", "UNCHANGED", 0},
		{"PASS", "FAIL", "FAIL", "REGRESSION", 2},
		{"FAIL", "PASS", "PASS", "IMPROVEMENT", 0},
		{"FAIL", "FAIL", "PASS", "UNCHANGED", 0},
		{"PASS", "INCONCLUSIVE", "INCONCLUSIVE", "INCONCLUSIVE", 3},
		{"INCONCLUSIVE", "FAIL", "INCONCLUSIVE", "INCONCLUSIVE", 3},
	} {
		t.Run(tc.before+"_"+tc.after, func(t *testing.T) {
			baseline, candidate := verifyRunFixture(t), verifyRunFixture(t)
			setComparisonFixtureVerdict(t, baseline, tc.before)
			setComparisonFixtureVerdict(t, candidate, tc.after)
			for _, options := range [][]string{nil, {"--output", "text"}, {"--output=text"}, {"--output", "json"}, {"--output=json"}} {
				var out, stderr bytes.Buffer
				args := append([]string{"compare-runs", baseline, candidate}, options...)
				if code := Run(args, &out, &stderr); code != tc.code || stderr.Len() != 0 {
					t.Fatalf("%v: code=%d stdout=%s stderr=%s", options, code, &out, &stderr)
				}
				if !strings.Contains(strings.Join(options, " "), "json") {
					for _, want := range []string{"Baseline verification: " + tc.before, "Candidate verification: " + tc.after, "Comparison verdict: " + tc.verdict, "[" + tc.change + "]", "PASS means no regression was detected among comparable contracts; it does not mean the candidate run itself passed verification."} {
						if !strings.Contains(out.String(), want) {
							t.Fatalf("missing %q: %s", want, &out)
						}
					}
					continue
				}
				var doc regression.Document
				if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
					t.Fatal(err)
				}
				r := doc.Comparison
				if doc.SchemaVersion != "1" || doc.Command != "compare-runs" || string(r.Verdict) != tc.verdict || string(r.Baseline.VerificationVerdict) != tc.before || string(r.Candidate.VerificationVerdict) != tc.after {
					t.Fatalf("document=%+v", doc)
				}
				if r.Scope.CommonOperators != 2 || len(r.Operators) != 2 || r.Operators[0].Name != "ingress" || r.Operators[1].Name != "network" {
					t.Fatalf("scope/order=%+v", r)
				}
				ingress := r.Operators[0]
				if string(ingress.BaselineVerdict) != tc.before || string(ingress.CandidateVerdict) != tc.after || len(ingress.Contracts) != 2 || string(ingress.Contracts[1].Change) != tc.change {
					t.Fatalf("ingress=%+v", ingress)
				}
				var raw map[string]json.RawMessage
				if err := json.Unmarshal(out.Bytes(), &raw); err != nil {
					t.Fatal(err)
				}
				if raw["result"] != nil || raw["comparison"] == nil {
					t.Fatalf("wrong envelope: %s", &out)
				}
			}
		})
	}
}

func TestCompareRunsScope(t *testing.T) {
	for _, tc := range []struct {
		side       string
		regression bool
		code       int
	}{{"baseline", false, 3}, {"candidate", false, 3}, {"candidate", true, 2}} {
		t.Run(tc.side+"/"+strconv.Itoa(tc.code), func(t *testing.T) {
			baseline, candidate := verifyRunFixture(t), verifyRunFixture(t)
			reduced := baseline
			if tc.side == "candidate" {
				reduced = candidate
			}
			if err := os.Remove(filepath.Join(reduced, "operators/network.jsonl")); err != nil {
				t.Fatal(err)
			}
			changeRunManifest(t, reduced, func(m *recording.RunManifest) { m.Operators = []string{"ingress"} })
			if tc.regression {
				setComparisonFixtureVerdict(t, candidate, "FAIL")
			}
			var out, stderr bytes.Buffer
			if code := Run([]string{"compare-runs", baseline, candidate, "--output=json"}, &out, &stderr); code != tc.code {
				t.Fatalf("code=%d out=%s err=%s", code, &out, &stderr)
			}
			var doc regression.Document
			if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
				t.Fatal(err)
			}
			scope := doc.Comparison.Scope
			only := scope.CandidateOnlyOperators
			if tc.side == "candidate" {
				only = scope.BaselineOnlyOperators
			}
			if scope.CommonOperators != 1 || len(only) != 1 || only[0] != "network" {
				t.Fatalf("scope=%+v", scope)
			}
		})
	}
}

func TestCompareRunsRejectsInvalidInputs(t *testing.T) {
	for _, side := range []string{"baseline", "candidate"} {
		for _, invalid := range []string{"missing", "failed", "malformed", "escape"} {
			t.Run(side+"_"+invalid, func(t *testing.T) {
				baseline, candidate := verifyRunFixture(t), verifyRunFixture(t)
				dir := baseline
				if side == "candidate" {
					dir = candidate
				}
				switch invalid {
				case "missing":
					if err := os.Remove(filepath.Join(dir, "run.json")); err != nil {
						t.Fatal(err)
					}
				case "failed":
					changeRunManifest(t, dir, func(m *recording.RunManifest) { m.Status = recording.RunStatusFailed; m.Error = "failed" })
				case "malformed":
					writeRunTestFile(t, filepath.Join(dir, "cluster-version.jsonl"), []byte("{invalid"))
				case "escape":
					changeRunManifest(t, dir, func(m *recording.RunManifest) { m.Files.ClusterVersion = "../outside.jsonl" })
				}
				for _, format := range []string{"text", "json"} {
					var out, stderr bytes.Buffer
					if code := Run([]string{"compare-runs", baseline, candidate, "--output=" + format}, &out, &stderr); code != 1 || out.Len() != 0 || !strings.Contains(stderr.String(), side) {
						t.Fatalf("code=%d out=%s err=%s", code, &out, &stderr)
					}
				}
			})
		}
	}
}

func TestCompareRunsDifferentClustersAndVersions(t *testing.T) {
	baseline, candidate := verifyRunFixture(t), verifyRunFixture(t)
	for i, dir := range []string{baseline, candidate} {
		id := []string{"cluster-a", "cluster-b"}[i]
		path := filepath.Join(dir, "cluster-version.jsonl")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var rewritten bytes.Buffer
		for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
			var observation map[string]any
			if err := json.Unmarshal(line, &observation); err != nil {
				t.Fatal(err)
			}
			observation["clusterVersion"].(map[string]any)["spec"] = map[string]any{"clusterID": id}
			if err := json.NewEncoder(&rewritten).Encode(observation); err != nil {
				t.Fatal(err)
			}
		}
		writeRunTestFile(t, path, rewritten.Bytes())
		changeRunManifest(t, dir, func(m *recording.RunManifest) { m.Source.ClusterID = id })
	}
	for _, file := range []string{"cluster-version.jsonl", "operators/ingress.jsonl", "operators/network.jsonl"} {
		path := filepath.Join(candidate, file)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		writeRunTestFile(t, path, bytes.ReplaceAll(data, []byte("4.20.0"), []byte("4.21.0")))
	}
	var out, stderr bytes.Buffer
	if code := Run([]string{"compare-runs", baseline, candidate, "--output=json"}, &out, &stderr); code != 0 {
		t.Fatalf("code=%d out=%s err=%s", code, &out, &stderr)
	}
	var doc regression.Document
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	b, c := doc.Comparison.Baseline, doc.Comparison.Candidate
	if b.ClusterID != "cluster-a" || c.ClusterID != "cluster-b" || b.FinalDesiredVersion != "4.20.0" || c.FinalDesiredVersion != "4.21.0" || b.FinalDesiredImage == "" || c.FinalDesiredImage == "" || b.RunID == "" || c.RunID == "" {
		t.Fatalf("summaries=%+v %+v", b, c)
	}
	if doc.Comparison.Scope.SameFinalTarget {
		t.Fatal("different final versions reported as the same target")
	}
	for _, timing := range doc.Comparison.Timings {
		if timing.Delta != "" {
			t.Fatalf("timing delta reported across different final targets: %+v", timing)
		}
	}
}

func TestCompareRunsUsage(t *testing.T) {
	for _, args := range [][]string{{"compare-runs"}, {"compare-runs", "one"}, {"compare-runs", "one", "two", "three"}, {"compare-runs", "one", "two", "--policy"}, {"compare-runs", "one", "two", "--policy="}, {"compare-runs", "one", "two", "--policy=a", "--policy=b"}, {"compare-runs", "one", "two", "--output=yaml"}, {"compare-runs", "one", "two", "--output=json", "--output=text"}} {
		var out, stderr bytes.Buffer
		if code := Run(args, &out, &stderr); code != 1 || out.Len() != 0 || stderr.Len() == 0 {
			t.Fatalf("args=%v code=%d out=%s err=%s", args, code, &out, &stderr)
		}
	}
	for _, command := range []string{"check", "replay", "replay-version", "capture-live", "record-live", "help", "version"} {
		var out, stderr bytes.Buffer
		if code := Run([]string{command, "--output=json"}, &out, &stderr); code != 1 || !strings.Contains(stderr.String(), "supported only") {
			t.Fatalf("%s code=%d err=%s", command, code, &stderr)
		}
	}
}

type comparisonErrorWriter struct{}

func (comparisonErrorWriter) Write([]byte) (int, error) {
	return 0, errors.New("comparison output unavailable")
}

func TestCompareRunsOutputError(t *testing.T) {
	baseline, candidate := verifyRunFixture(t), verifyRunFixture(t)
	for _, format := range []string{"text", "json"} {
		var stderr bytes.Buffer
		code := Run([]string{"compare-runs", baseline, candidate, "--output=" + format}, comparisonErrorWriter{}, &stderr)
		if code != 1 || !strings.Contains(stderr.String(), "comparison output unavailable") {
			t.Fatalf("%s: code=%d stderr=%s", format, code, &stderr)
		}
	}
}

func TestCompareRunsIncludesAuxiliaryAndObservedTimings(t *testing.T) {
	baseline, candidate := lifecyclePolicyAuxiliaryRunFixture(t), lifecyclePolicyAuxiliaryRunFixture(t)
	var out, stderr bytes.Buffer
	if code := Run([]string{"compare-runs", baseline, candidate, "--output=json"}, &out, &stderr); code != 0 || stderr.Len() != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, &out, &stderr)
	}
	var doc regression.Document
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	report := doc.Comparison
	if !report.Scope.SameFinalTarget || report.Scope.CommonMachineConfigPools != 1 || report.Scope.CommonNodes != 1 {
		t.Fatalf("scope=%+v", report.Scope)
	}
	if len(report.MachineConfigPools) != 1 || len(report.Nodes) != 1 || report.MachineConfigPools[0].Contracts[0].Change != regression.ChangeUnchanged || report.Nodes[0].Contracts[0].Change != regression.ChangeUnchanged {
		t.Fatalf("auxiliary comparison=%+v %+v", report.MachineConfigPools, report.Nodes)
	}
	if len(report.Timings) != 4 {
		t.Fatalf("timings=%+v", report.Timings)
	}
	for _, timing := range report.Timings {
		if timing.Baseline == nil || timing.Candidate == nil || timing.Delta != "0s" {
			t.Fatalf("timing=%+v", timing)
		}
	}
	if report.Timings[0].Name != upgradeObservedSpanTiming || report.Timings[0].Baseline.Duration != "4m0s" {
		t.Fatalf("upgrade timing=%+v", report.Timings[0])
	}
}

func TestCompareRunsPolicyRegression(t *testing.T) {
	baseline, candidate := lifecyclePolicyAuxiliaryRunFixture(t), lifecyclePolicyAuxiliaryRunFixture(t)
	nodePath := filepath.Join(candidate, "nodes", "node-0.jsonl")
	data, err := os.ReadFile(nodePath)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte(`"type":"Ready","status":"True"`), []byte(`"type":"Ready","status":"False"`), 1)
	writeRunTestFile(t, nodePath, data)

	policyPath := writeLifecyclePolicy(t, `
apiVersion: reconcileguard.io/v1alpha1
kind: UpgradePolicy
targetVersion: 4.20.0
source: compare-policy-test
defaults:
  node:
    readyPostCompletionGracePeriod: 1m
`)

	for _, option := range [][]string{{"--policy", policyPath}, {"--policy=" + policyPath}} {
		var out, stderr bytes.Buffer
		args := append([]string{"compare-runs", baseline, candidate, "--output=json"}, option...)
		if code := Run(args, &out, &stderr); code != 2 || stderr.Len() != 0 {
			t.Fatalf("%v: exit=%d stdout=%s stderr=%s", option, code, &out, &stderr)
		}
		var doc regression.Document
		if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
			t.Fatal(err)
		}
		comparison := doc.Comparison
		if comparison.Policy == nil || comparison.Policy.Source != "compare-policy-test" || comparison.Policy.BaselineVerdict != "PASS" || comparison.Policy.CandidateVerdict != "FAIL" || comparison.Policy.Verdict != "FAIL" || comparison.Verdict != "FAIL" {
			t.Fatalf("comparison=%+v", comparison)
		}
		if len(comparison.Policy.Nodes) != 1 || comparison.Policy.Nodes[0].Contracts[0].Change != regression.ChangeRegression {
			t.Fatalf("policy nodes=%+v", comparison.Policy.Nodes)
		}
	}
}

func TestCompareRunsAuxiliaryScopeDoesNotCreateRegression(t *testing.T) {
	baseline, candidate := lifecyclePolicyAuxiliaryRunFixture(t), lifecyclePolicyAuxiliaryRunFixture(t)
	if err := os.RemoveAll(filepath.Join(candidate, "nodes")); err != nil {
		t.Fatal(err)
	}
	changeRunManifest(t, candidate, func(m *recording.RunManifest) {
		m.Nodes = nil
		m.Files.NodesDirectory = ""
	})

	var out, stderr bytes.Buffer
	if code := Run([]string{"compare-runs", baseline, candidate, "--output=json"}, &out, &stderr); code != 0 || stderr.Len() != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, &out, &stderr)
	}
	var doc regression.Document
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Comparison.Verdict != "PASS" || len(doc.Comparison.Scope.BaselineOnlyNodes) != 1 || doc.Comparison.Scope.BaselineOnlyNodes[0] != "node-0" {
		t.Fatalf("comparison=%+v", doc.Comparison)
	}
}

func TestCompareRunsPolicyMissingDefaultScopeIsInconclusive(t *testing.T) {
	baseline, candidate := verifyRunFixture(t), verifyRunFixture(t)
	policyPath := writeLifecyclePolicy(t, `apiVersion: reconcileguard.io/v1alpha1
kind: UpgradePolicy
targetVersion: 4.20.0
source: comparison-scope-test
maxObservationGap: 10m
defaults:
  degraded:
    maxObservedDuration: 1m
  machineConfigPool:
    postCompletionGracePeriod: 1m
  node:
    readyPostCompletionGracePeriod: 1m
`)
	for _, format := range []string{"text", "json"} {
		var out, stderr bytes.Buffer
		code := Run([]string{"compare-runs", baseline, candidate, "--policy", policyPath, "--output", format}, &out, &stderr)
		if code != 3 || stderr.Len() != 0 {
			t.Fatalf("format=%s exit=%d stdout=%s stderr=%s", format, code, &out, &stderr)
		}
		if format == "text" {
			if !strings.Contains(out.String(), "Policy comparison verdict: INCONCLUSIVE") {
				t.Fatalf("output=%s", &out)
			}
			continue
		}
		var doc regression.Document
		if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
			t.Fatal(err)
		}
		if doc.Comparison.Policy == nil || doc.Comparison.Policy.BaselineVerdict != "INCONCLUSIVE" || doc.Comparison.Policy.CandidateVerdict != "INCONCLUSIVE" || doc.Comparison.Policy.Verdict != "INCONCLUSIVE" || doc.Comparison.Verdict != "INCONCLUSIVE" {
			t.Fatalf("comparison=%+v", doc.Comparison)
		}
	}
}

func TestCompareRunsImageOnlyTargetDifferenceSuppressesTimingDelta(t *testing.T) {
	baseline, candidate := lifecyclePolicyAuxiliaryRunFixture(t), lifecyclePolicyAuxiliaryRunFixture(t)
	path := filepath.Join(candidate, "cluster-version.jsonl")
	observations, err := upgrade.ReadHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	var data bytes.Buffer
	for _, observation := range observations {
		if observation.ClusterVersion.Status.Desired.Version == "4.20.0" {
			observation.ClusterVersion.Status.Desired.Image = "example.invalid/release@sha256:" + strings.Repeat("c", 64)
			observation.ClusterVersion.Status.History[0].Image = observation.ClusterVersion.Status.Desired.Image
		}
		if err := json.NewEncoder(&data).Encode(observation); err != nil {
			t.Fatal(err)
		}
	}
	writeRunTestFile(t, path, data.Bytes())
	var out, stderr bytes.Buffer
	if code := Run([]string{"compare-runs", baseline, candidate, "--output=json"}, &out, &stderr); code != 0 || stderr.Len() != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, &out, &stderr)
	}
	var doc regression.Document
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	r := doc.Comparison
	if r.Scope.SameFinalTarget || r.Baseline.FinalDesiredVersion != r.Candidate.FinalDesiredVersion || r.Baseline.FinalDesiredImage == r.Candidate.FinalDesiredImage || len(r.Timings) != 4 {
		t.Fatalf("comparison=%+v", r)
	}
	for _, timing := range r.Timings {
		if timing.Baseline == nil || timing.Candidate == nil || timing.Delta != "" {
			t.Fatalf("cross-image timing delta or lost evidence: %+v", timing)
		}
	}
}

func TestCompareRunsTimingTextAcrossTargets(t *testing.T) {
	for _, different := range []bool{false, true} {
		t.Run(strconv.FormatBool(different), func(t *testing.T) {
			baseline, candidate := lifecyclePolicyAuxiliaryRunFixture(t), lifecyclePolicyAuxiliaryRunFixture(t)
			if different {
				for _, file := range []string{"cluster-version.jsonl", "operators/ingress.jsonl"} {
					path := filepath.Join(candidate, file)
					data, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					writeRunTestFile(t, path, bytes.ReplaceAll(data, []byte("4.20.0"), []byte("4.21.0")))
				}
			}
			var out, stderr bytes.Buffer
			if code := Run([]string{"compare-runs", baseline, candidate}, &out, &stderr); code != 0 || stderr.Len() != 0 {
				t.Fatalf("exit=%d stdout=%s stderr=%s", code, &out, &stderr)
			}
			for _, want := range []string{"Final targets match: " + strconv.FormatBool(!different), "baseline=4m0s candidate=4m0s", "Observed timings are measurements between recorded samples; they are not regression verdicts or exact transition durations."} {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("missing %q: %s", want, &out)
				}
			}
			if strings.Contains(out.String(), " delta=") == different || strings.Contains(out.String(), "[REGRESSION]") || strings.Contains(out.String(), "[IMPROVEMENT]") {
				t.Fatalf("unexpected timing interpretation: %s", &out)
			}
		})
	}
}
