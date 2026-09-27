package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
	configv1 "github.com/openshift/api/config/v1"
)

const validUpgradePolicy = `{"maxDuration":"2m","maxObservationGap":"5m","operator":"ingress","targetVersion":"4.20.0","source":"team policy v1"}`

func writeUpgradeTestFile(t *testing.T, name, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadProgressingUpgradePolicy(t *testing.T) {
	got, err := readProgressingPolicy(writeUpgradeTestFile(t, "policy.json", validUpgradePolicy))
	if err != nil {
		t.Fatal(err)
	}
	if got.Limit != 2*time.Minute || got.MaxObservationGap != 5*time.Minute || got.Operator != "ingress" || got.TargetVersion != "4.20.0" || got.Source != "team policy v1" {
		t.Fatalf("policy: %+v", got)
	}
	for _, tc := range []struct{ name, data, want string }{
		{"malformed", "{", "decode policy"},
		{"unknown field", strings.Replace(validUpgradePolicy, `"source":`, `"surprise":`, 1), "unknown field"},
		{"second object", validUpgradePolicy + " {}", "one JSON object"},
		{"trailing junk", validUpgradePolicy + " junk", "one JSON object"},
		{"wrong type", strings.Replace(validUpgradePolicy, `"2m"`, `2`, 1), "decode policy"},
		{"missing duration", `{}`, "maxDuration"},
		{"zero duration", strings.Replace(validUpgradePolicy, "2m", "0s", 1), "maxDuration"},
		{"negative duration", strings.Replace(validUpgradePolicy, "2m", "-2m", 1), "maxDuration"},
		{"invalid duration", strings.Replace(validUpgradePolicy, "2m", "forever", 1), "maxDuration"},
		{"overflow duration", strings.Replace(validUpgradePolicy, "2m", "999999999999999999999h", 1), "maxDuration"},
		{"zero gap", strings.Replace(validUpgradePolicy, "5m", "0s", 1), "maxObservationGap"},
		{"negative gap", strings.Replace(validUpgradePolicy, "5m", "-1s", 1), "maxObservationGap"},
		{"invalid gap", strings.Replace(validUpgradePolicy, "5m", "forever", 1), "maxObservationGap"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := readProgressingPolicy(writeUpgradeTestFile(t, "policy.json", tc.data))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %s", err, tc.want)
			}
		})
	}
	if _, err := readProgressingPolicy(filepath.Join(t.TempDir(), "absent")); err == nil || !strings.Contains(err.Error(), "read policy") {
		t.Fatalf("missing file: %v", err)
	}
}

func TestRunProgressingUpgrade(t *testing.T) {
	base, err := upgrade.ReadHistory("../../examples/cluster-version-history.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	var cv bytes.Buffer
	enc := json.NewEncoder(&cv)
	for i := 0; i < 3; i++ {
		v := base[1]
		v.ObservedAt = v.ObservedAt.Add(time.Duration(i) * time.Minute)
		if err := enc.Encode(v); err != nil {
			t.Fatal(err)
		}
	}
	cvPath := writeUpgradeTestFile(t, "cv.jsonl", cv.String())
	for _, tc := range []struct {
		name, statuses, policy string
		code                   int
		verdict                string
	}{
		{"pass", "FTF", validUpgradePolicy, 0, "PASS"},
		{"fail", "TTT", strings.Replace(validUpgradePolicy, "2m", "1m", 1), 2, "FAIL"},
		{"censored", "TTT", validUpgradePolicy, 3, "INCONCLUSIVE"},
		{"missing source", "TTT", strings.Replace(validUpgradePolicy, "team policy v1", "", 1), 3, "INCONCLUSIVE"},
		{"unknown gap", "TUT", validUpgradePolicy, 3, "INCONCLUSIVE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var op bytes.Buffer
			encoder := json.NewEncoder(&op)
			for i, status := range tc.statuses {
				value := map[rune]configv1.ConditionStatus{'F': configv1.ConditionFalse, 'T': configv1.ConditionTrue, 'U': configv1.ConditionUnknown}[status]
				var observation operator.Observation
				observation.ObservedAt = base[1].ObservedAt.Add(time.Duration(i) * time.Minute)
				observation.Operator.APIVersion = "config.openshift.io/v1"
				observation.Operator.Kind = "ClusterOperator"
				observation.Operator.Name = "ingress"
				observation.Operator.Status.Conditions = []configv1.ClusterOperatorStatusCondition{{Type: configv1.OperatorProgressing, Status: value}}
				if err := encoder.Encode(observation); err != nil {
					t.Fatal(err)
				}
			}
			opPath := writeUpgradeTestFile(t, "op.jsonl", op.String())
			policyPath := writeUpgradeTestFile(t, "policy.json", tc.policy)
			var out, stderr bytes.Buffer
			code := Run([]string{"verify-progressing-upgrade", cvPath, opPath, policyPath}, &out, &stderr)
			if code != tc.code || stderr.Len() != 0 {
				t.Fatalf("exit=%d stdout=%s stderr=%s", code, &out, &stderr)
			}
			for _, want := range []string{"Contract: operator-progressing-duration", "Policy: user-supplied project policy (not an OpenShift guarantee)", "Target version: 4.20.0", "Maximum observation gap: 5m0s", "Evaluated UPDATING samples: 3", "Verdict: " + tc.verdict + "\n", "correlation=EXACT", "version-interval=["} {
				if !strings.Contains(out.String(), want) {
					t.Errorf("missing %q: %s", want, &out)
				}
			}
			if tc.name == "unknown gap" && !strings.Contains(out.String(), "Missing/Unknown Progressing: 1") {
				t.Errorf("missing gap count: %s", &out)
			}
		})
	}
	policy := writeUpgradeTestFile(t, "policy.json", validUpgradePolicy)
	bad := writeUpgradeTestFile(t, "bad.jsonl", "{bad")
	empty := writeUpgradeTestFile(t, "empty.jsonl", "")
	missing := filepath.Join(t.TempDir(), "absent")
	for _, args := range [][]string{
		{}, {cvPath}, {cvPath, bad}, {cvPath, bad, policy, "extra"},
		{cvPath, bad, missing}, {missing, bad, policy}, {bad, bad, policy}, {cvPath, missing, policy}, {cvPath, bad, policy}, {cvPath, empty, policy},
	} {
		var out, stderr bytes.Buffer
		if code := Run(append([]string{"verify-progressing-upgrade"}, args...), &out, &stderr); code != 1 || out.Len() != 0 || stderr.Len() == 0 {
			t.Fatalf("args=%v exit=%d stdout=%s stderr=%s", args, code, &out, &stderr)
		}
	}
}
