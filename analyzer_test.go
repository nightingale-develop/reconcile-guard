package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testOperator(conditions ...Condition) ClusterOperator {
	operator := ClusterOperator{
		APIVersion: "config.openshift.io/v1",
		Kind:       "ClusterOperator",
	}

	operator.Metadata.Name = "ingress"
	operator.Status.Conditions = conditions

	return operator
}

func TestAnalyzeOperator(t *testing.T) {
	tests := []struct {
		name       string
		conditions []Condition
		wantResult string
		wantCode   int
		wantError  string
	}{
		{
			name: "healthy operator",
			conditions: []Condition{
				{Type: "Available", Status: "True"},
				{Type: "Degraded", Status: "False"},
			},
			wantResult: "NOT DEGRADED (reported)",
			wantCode:   0,
		},
		{
			name: "degraded operator",
			conditions: []Condition{
				{Type: "Degraded", Status: "True"},
			},
			wantResult: "DEGRADED",
			wantCode:   2,
		},
		{
			name: "unknown status",
			conditions: []Condition{
				{Type: "Degraded", Status: "Unknown"},
			},
			wantResult: "UNKNOWN",
			wantCode:   3,
		},
		{
			name: "missing condition",
			conditions: []Condition{
				{Type: "Available", Status: "True"},
			},
			wantResult: "UNKNOWN (Degraded condition missing)",
			wantCode:   3,
		},
		{
			name: "duplicate condition",
			conditions: []Condition{
				{Type: "Degraded", Status: "True"},
				{Type: "Degraded", Status: "False"},
			},
			wantError: "duplicate Degraded condition",
		},
		{
			name: "invalid status",
			conditions: []Condition{
				{Type: "Degraded", Status: "Broken"},
			},
			wantError: "invalid Degraded status",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			operator := testOperator(tc.conditions...)

			got, err := analyzeOperator(operator)

			if tc.wantError != "" {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}

				if !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("unexpected error: %v", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got.Result != tc.wantResult {
				t.Errorf(
					"result = %q, want %q",
					got.Result,
					tc.wantResult,
				)
			}

			if got.ExitCode != tc.wantCode {
				t.Errorf(
					"exit code = %d, want %d",
					got.ExitCode,
					tc.wantCode,
				)
			}
		})
	}
}

func TestCheckFileRejectsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.json")

	err := os.WriteFile(path, []byte("{invalid"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	_, err = checkFile(path)

	if err == nil {
		t.Fatal("expected JSON decoding error")
	}

	if !strings.Contains(err.Error(), "decode JSON") {
		t.Fatalf("unexpected error: %v", err)
	}
}
