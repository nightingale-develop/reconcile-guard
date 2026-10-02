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

func writeLifecyclePolicy(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "policy.yaml")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestVerifyLifecyclePolicy(t *testing.T) {
	dir := verifyRunFixture(t)
	policyPath := writeLifecyclePolicy(t, `
apiVersion: reconcileguard.io/v1alpha1
kind: UpgradePolicy
targetVersion: 4.20.0
source: test-policy
maxObservationGap: 10m
defaults:
  availabilityLoss:
    maxObservedDuration: 1m
  degraded:
    maxObservedDuration: 1m
`)

	for _, format := range []string{"text", "json"} {
		t.Run(format, func(t *testing.T) {
			var out, stderr bytes.Buffer
			code := Run([]string{"verify-lifecycle-policy", dir, policyPath, "--output", format}, &out, &stderr)
			if code != 0 || stderr.Len() != 0 {
				t.Fatalf("exit=%d stdout=%s stderr=%s", code, &out, &stderr)
			}
			if format == "text" {
				for _, want := range []string{"explicit user-supplied lifecycle policy", "Verdict: PASS", "operator-availability-loss-policy", "operator-degraded-policy"} {
					if !strings.Contains(out.String(), want) {
						t.Fatalf("missing %q in %s", want, &out)
					}
				}
				return
			}
			var doc result.Document
			if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
				t.Fatal(err)
			}
			if doc.Command != "verify-lifecycle-policy" || doc.Result.Verdict != result.VerdictPass || len(doc.Result.Operators) != 2 {
				t.Fatalf("document=%+v", doc)
			}
		})
	}
}

func TestVerifyLifecyclePolicyMissingExplicitResourceIsInconclusive(t *testing.T) {
	dir := verifyRunFixture(t)
	policyPath := writeLifecyclePolicy(t, `
apiVersion: reconcileguard.io/v1alpha1
kind: UpgradePolicy
targetVersion: 4.20.0
source: test-policy
maxObservationGap: 10m
operators:
  does-not-exist:
    degraded:
      maxObservedDuration: 1m
`)
	var out, stderr bytes.Buffer
	code := Run([]string{"verify-lifecycle-policy", dir, policyPath, "--output", "json"}, &out, &stderr)
	if code != 3 || stderr.Len() != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, &out, &stderr)
	}
	var doc result.Document
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Result.Verdict != result.VerdictInconclusive || len(doc.Result.Operators) != 1 || doc.Result.Operators[0].Name != "does-not-exist" {
		t.Fatalf("document=%+v", doc)
	}
}

func TestVerifyLifecyclePolicyRejectsInvalidPolicy(t *testing.T) {
	dir := verifyRunFixture(t)
	path := writeLifecyclePolicy(t, `apiVersion: reconcileguard.io/v1alpha1
kind: UpgradePolicy
targetVersion: 4.20.0
source: test-policy
`)
	var out, stderr bytes.Buffer
	if code := Run([]string{"verify-lifecycle-policy", dir, path}, &out, &stderr); code != 1 || out.Len() != 0 || stderr.Len() == 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, &out, &stderr)
	}
}

func TestVerifyLifecyclePolicyMissingAuxiliaryResources(t *testing.T) {
	dir := verifyRunFixture(t)
	path := writeLifecyclePolicy(t, `apiVersion: reconcileguard.io/v1alpha1
kind: UpgradePolicy
targetVersion: 4.20.0
source: test-policy
machineConfigPools:
  absent-pool:
    postCompletionGracePeriod: 1m
nodes:
  absent-node:
    readyPostCompletionGracePeriod: 1m
`)
	var out, stderr bytes.Buffer
	if code := Run([]string{"verify-lifecycle-policy", dir, path, "--output=json"}, &out, &stderr); code != 3 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, &out, &stderr)
	}
	var doc result.Document
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Result.Verdict != result.VerdictInconclusive || len(doc.Result.MachineConfigPools) != 1 || len(doc.Result.Nodes) != 1 || doc.Result.MachineConfigPools[0].Verdict != result.VerdictInconclusive || doc.Result.Nodes[0].Verdict != result.VerdictInconclusive {
		t.Fatalf("missing policy scope silently passed: %+v", doc.Result)
	}
}

func lifecyclePolicyAuxiliaryRunFixture(t *testing.T) string {
	t.Helper()
	start := time.Date(2026, 9, 30, 9, 59, 0, 0, time.UTC)
	run, err := recording.StartRun(t.TempDir(), "test", "https://api.example", start)
	if err != nil {
		t.Fatal(err)
	}
	dir := run.Directory()
	for _, name := range []string{"operators", "machine-config-pools", "nodes"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []struct{ src, dst string }{
		{"../../testdata/upgrade/cluster-version.jsonl", "cluster-version.jsonl"},
		{"../../testdata/upgrade/ingress.jsonl", "operators/ingress.jsonl"},
	} {
		data, err := os.ReadFile(file.src)
		if err != nil {
			t.Fatal(err)
		}
		writeRunTestFile(t, filepath.Join(dir, file.dst), data)
	}
	pool := `{"observedAt":"2026-09-30T10:08:00Z","machineConfigPool":{"apiVersion":"machineconfiguration.openshift.io/v1","kind":"MachineConfigPool","metadata":{"name":"master","generation":1},"spec":{"configuration":{"name":"rendered-b"}},"status":{"observedGeneration":1,"configuration":{"name":"rendered-b"},"machineCount":1,"updatedMachineCount":1,"readyMachineCount":1,"unavailableMachineCount":0,"degradedMachineCount":0,"conditions":[{"type":"Updated","status":"True"},{"type":"Updating","status":"False"},{"type":"Degraded","status":"False"}]}}}` + "\n"
	node := `{"observedAt":"2026-09-30T10:08:00Z","node":{"apiVersion":"v1","kind":"Node","metadata":{"name":"node-0","annotations":{"machineconfiguration.openshift.io/currentConfig":"rendered-b","machineconfiguration.openshift.io/desiredConfig":"rendered-b"}},"status":{"conditions":[{"type":"Ready","status":"True"}],"nodeInfo":{"kubeletVersion":"v1.34.4"}}}}` + "\n"
	writeRunTestFile(t, filepath.Join(dir, "machine-config-pools", "master.jsonl"), []byte(pool))
	writeRunTestFile(t, filepath.Join(dir, "nodes", "node-0.jsonl"), []byte(node))
	if err := run.Finish(recording.RunStatusStopped, start.Add(20*time.Minute), recording.RunSnapshot{
		ClusterID:          "12345678-1234-4234-8234-123456789abc",
		Operators:          []string{"ingress"},
		MachineConfigPools: []string{"master"},
		Nodes:              []string{"node-0"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestVerifyLifecyclePolicyMachineConfigPoolAndNode(t *testing.T) {
	dir := lifecyclePolicyAuxiliaryRunFixture(t)
	policyPath := writeLifecyclePolicy(t, `
apiVersion: reconcileguard.io/v1alpha1
kind: UpgradePolicy
targetVersion: 4.20.0
source: test-policy
defaults:
  machineConfigPool:
    postCompletionGracePeriod: 1m
  node:
    readyPostCompletionGracePeriod: 1m
    configAlignedPostCompletionGracePeriod: 1m
`)
	var out, stderr bytes.Buffer
	code := Run([]string{"verify-lifecycle-policy", dir, policyPath, "--output", "json"}, &out, &stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, &out, &stderr)
	}
	var doc result.Document
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Result.Verdict != result.VerdictPass || len(doc.Result.MachineConfigPools) != 1 || len(doc.Result.Nodes) != 1 {
		t.Fatalf("document=%+v", doc)
	}
	if doc.Result.MachineConfigPools[0].Verdict != result.VerdictPass || doc.Result.Nodes[0].Verdict != result.VerdictPass {
		t.Fatalf("resources=%+v %+v", doc.Result.MachineConfigPools, doc.Result.Nodes)
	}
}
