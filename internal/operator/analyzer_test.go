package operator

import (
	"strings"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
)

func testOperator(
	conditions ...configv1.ClusterOperatorStatusCondition,
) configv1.ClusterOperator {
	var operator configv1.ClusterOperator

	operator.APIVersion = "config.openshift.io/v1"
	operator.Kind = "ClusterOperator"
	operator.Name = "ingress"
	operator.Status.Conditions = conditions

	return operator
}

func TestAnalyzeOperator(t *testing.T) {
	tests := []struct {
		name       string
		conditions []configv1.ClusterOperatorStatusCondition
		wantResult string
		wantCode   int
		wantError  string
	}{
		{
			name: "reported not degraded",
			conditions: []configv1.ClusterOperatorStatusCondition{
				{
					Type:   configv1.OperatorAvailable,
					Status: configv1.ConditionTrue,
				},
				{
					Type:   configv1.OperatorDegraded,
					Status: configv1.ConditionFalse,
				},
			},
			wantResult: "NOT DEGRADED (reported)",
			wantCode:   0,
		},
		{
			name: "degraded operator",
			conditions: []configv1.ClusterOperatorStatusCondition{
				{
					Type:   configv1.OperatorDegraded,
					Status: configv1.ConditionTrue,
				},
			},
			wantResult: "DEGRADED",
			wantCode:   2,
		},
		{
			name: "unknown status",
			conditions: []configv1.ClusterOperatorStatusCondition{
				{
					Type:   configv1.OperatorDegraded,
					Status: configv1.ConditionUnknown,
				},
			},
			wantResult: "UNKNOWN",
			wantCode:   3,
		},
		{
			name: "missing condition",
			conditions: []configv1.ClusterOperatorStatusCondition{
				{
					Type:   configv1.OperatorAvailable,
					Status: configv1.ConditionTrue,
				},
			},
			wantResult: "UNKNOWN (Degraded condition missing)",
			wantCode:   3,
		},
		{
			name: "duplicate condition",
			conditions: []configv1.ClusterOperatorStatusCondition{
				{
					Type:   configv1.OperatorDegraded,
					Status: configv1.ConditionTrue,
				},
				{
					Type:   configv1.OperatorDegraded,
					Status: configv1.ConditionFalse,
				},
			},
			wantError: "duplicate Degraded condition",
		},
		{
			name: "invalid status",
			conditions: []configv1.ClusterOperatorStatusCondition{
				{
					Type:   configv1.OperatorDegraded,
					Status: configv1.ConditionStatus("Broken"),
				},
			},
			wantError: "invalid Degraded status",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			operator := testOperator(tc.conditions...)

			got, err := Analyze(operator)

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

func TestAnalyzeRejectsInvalidResource(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*configv1.ClusterOperator)
	}{
		{"kind", func(o *configv1.ClusterOperator) { o.Kind = "Pod" }},
		{"apiVersion", func(o *configv1.ClusterOperator) { o.APIVersion = "v1" }},
		{"name", func(o *configv1.ClusterOperator) { o.Name = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := testOperator()
			tc.change(&o)
			if _, err := Analyze(o); err == nil {
				t.Fatal("accepted invalid resource")
			}
		})
	}
}
