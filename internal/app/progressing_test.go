package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunVerifyProgressing(t *testing.T) {
	for _, tc := range []struct {
		name, statuses, limit string
		code                  int
		verdict, evidence     string
	}{
		{"bounded pass", "FTF", "2m", 0, "PASS", "maximum-span=2m0s"},
		{"observed failure", "TTT", "1m", 2, "FAIL", "observed-span=2m0s"},
		{"right censored", "FT", "10m", 3, "INCONCLUSIVE", "preceding-False=2026-09-26T00:00:00Z"},
		{"unknown gap", "TUT", "1m", 3, "INCONCLUSIVE", "Missing/Unknown Progressing: 1"},
		{"no progressing", "FF", "1m", 0, "PASS", "Observations: 2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var data strings.Builder
			for i, status := range tc.statuses {
				value := "False"
				if status == 'T' {
					value = "True"
				} else if status == 'U' {
					value = "Unknown"
				}
				fmt.Fprintf(&data, `{"observedAt":"2026-09-26T00:%02d:00Z","operator":{"apiVersion":"config.openshift.io/v1","kind":"ClusterOperator","metadata":{"name":"ingress"},"status":{"conditions":[{"type":"Progressing","status":"%s"}]}}}`+"\n", i, value)
			}
			path := filepath.Join(t.TempDir(), "history.jsonl")
			if err := os.WriteFile(path, []byte(data.String()), 0600); err != nil {
				t.Fatal(err)
			}
			var out, stderr bytes.Buffer
			code := Run([]string{"verify-progressing", path, tc.limit}, &out, &stderr)
			if code != tc.code || stderr.Len() != 0 {
				t.Fatalf("exit=%d stderr=%q", code, stderr.String())
			}
			limit, err := time.ParseDuration(tc.limit)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"Maximum duration (user policy): " + limit.String() + "\n", "Contract: observed-progressing-duration", "Operator: ingress", "Verdict: " + tc.verdict + "\n", tc.evidence, "continuous state between samples is not proven"} {
				if !strings.Contains(out.String(), want) {
					t.Errorf("stdout lacks %q: %s", want, &out)
				}
			}
		})
	}
	var out, stderr bytes.Buffer
	if code := Run([]string{"help"}, &out, &stderr); code != 0 || stderr.Len() != 0 || !strings.Contains(out.String(), "verify-progressing <operator-history.jsonl> <max-duration>") {
		t.Fatalf("help exit=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
	}
}

func TestRunVerifyProgressingErrors(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.jsonl")
	empty := filepath.Join(dir, "empty.jsonl")
	invalid := filepath.Join(dir, "invalid.jsonl")
	for path, data := range map[string]string{bad: "\n{broken", empty: "\n", invalid: `{"operator":{"apiVersion":"config.openshift.io/v1","kind":"ClusterOperator","metadata":{"name":"ingress"}}}`} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"missing args", nil, "Usage:"},
		{"missing limit", []string{bad}, "Usage:"},
		{"extra arg", []string{bad, "1m", "extra"}, "Usage:"},
		{"invalid limit", []string{bad, "forever"}, "positive Go duration"},
		{"unit required", []string{bad, "30"}, "positive Go duration"},
		{"zero limit", []string{bad, "0s"}, "positive Go duration"},
		{"negative limit", []string{bad, "-1m"}, "positive Go duration"},
		{"overflow", []string{bad, "999999999999999999999h"}, "positive Go duration"},
		{"missing file", []string{filepath.Join(dir, "absent"), "1m"}, "open history"},
		{"malformed", []string{bad, "1m"}, "line 2: decode JSON"},
		{"empty", []string{empty, "1m"}, "no observations"},
		{"invalid observation", []string{invalid, "1m"}, "observedAt is missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, stderr bytes.Buffer
			code := Run(append([]string{"verify-progressing"}, tc.args...), &out, &stderr)
			if code != 1 || out.Len() != 0 || !strings.Contains(stderr.String(), tc.want) {
				t.Fatalf("exit=%d stdout=%q stderr=%q, want %q", code, out.String(), stderr.String(), tc.want)
			}
		})
	}
}
