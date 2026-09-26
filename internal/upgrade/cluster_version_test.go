package upgrade

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
)

func TestAnalyzeClusterVersion(t *testing.T) {
	version, err := ReadClusterVersion("../../examples/cluster-version.json")
	if err != nil {
		t.Fatal(err)
	}

	report, err := AnalyzeClusterVersion(version)
	if err != nil {
		t.Fatal(err)
	}

	if report.Name != "version" ||
		report.Desired.Version != "4.20.0" ||
		!reflect.DeepEqual(report.Conditions, version.Status.Conditions) ||
		!reflect.DeepEqual(report.History, version.Status.History) ||
		report.History[0].CompletionTime != nil {
		t.Fatalf("unexpected report: %+v", report)
	}

	cases := []struct {
		name   string
		change func(*configv1.ClusterVersion)
		want   string
	}{
		{
			"kind",
			func(v *configv1.ClusterVersion) { v.Kind = "ClusterOperator" },
			"unsupported resource",
		},
		{
			"api version",
			func(v *configv1.ClusterVersion) { v.APIVersion = "v1" },
			"unsupported resource",
		},
		{
			"name",
			func(v *configv1.ClusterVersion) { v.Name = "" },
			"name is missing",
		},
		{
			"duplicate",
			func(v *configv1.ClusterVersion) {
				v.Status.Conditions = append(
					v.Status.Conditions,
					v.Status.Conditions[0],
				)
			},
			"duplicate Available",
		},
		{
			"invalid status",
			func(v *configv1.ClusterVersion) {
				v.Status.Conditions[0].Status = configv1.ConditionStatus("broken")
			},
			"invalid Available status",
		},
		{
			"empty type",
			func(v *configv1.ClusterVersion) { v.Status.Conditions[0].Type = "" },
			"type is missing",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := version.DeepCopy()
			tc.change(v)

			if _, err := AnalyzeClusterVersion(*v); err == nil ||
				!strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}

	for _, status := range []configv1.ConditionStatus{
		configv1.ConditionTrue,
		configv1.ConditionFalse,
		configv1.ConditionUnknown,
	} {
		v := version.DeepCopy()
		v.Status.Conditions[0].Status = status

		got, err := AnalyzeClusterVersion(*v)
		if err != nil || got.Conditions[0].Status != status {
			t.Fatalf("status %s: %+v, %v", status, got, err)
		}
	}

	version.Status = configv1.ClusterVersionStatus{}
	report, err = AnalyzeClusterVersion(version)
	if err != nil ||
		report.Desired.Version != "" ||
		len(report.Conditions) != 0 ||
		len(report.History) != 0 {
		t.Fatalf("missing status must not be invented: %+v, %v", report, err)
	}
}

func TestReadClusterVersionInvalidInput(t *testing.T) {
	for _, input := range []string{
		"{invalid",
		`[]`,
		`{"status":{"history":[{"startedTime":"yesterday"}]}}`,
	} {
		path := filepath.Join(t.TempDir(), "input.json")

		if err := os.WriteFile(path, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}

		if _, err := ReadClusterVersion(path); err == nil ||
			!strings.Contains(err.Error(), "decode JSON") {
			t.Fatalf("input %q: %v", input, err)
		}
	}

	if _, err := ReadClusterVersion(
		filepath.Join(t.TempDir(), "missing"),
	); err == nil {
		t.Fatal("expected read error")
	}
}
