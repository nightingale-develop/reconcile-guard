package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
)

func TestAnalyzeClusterVersion(t *testing.T) {
	version, err := readClusterVersion("examples/cluster-version.json")
	if err != nil {
		t.Fatal(err)
	}

	report, err := analyzeClusterVersion(version)
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

			if _, err := analyzeClusterVersion(*v); err == nil ||
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

		got, err := analyzeClusterVersion(*v)
		if err != nil || got.Conditions[0].Status != status {
			t.Fatalf("status %s: %+v, %v", status, got, err)
		}
	}

	version.Status = configv1.ClusterVersionStatus{}
	report, err = analyzeClusterVersion(version)
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

		if _, err := readClusterVersion(path); err == nil ||
			!strings.Contains(err.Error(), "decode JSON") {
			t.Fatalf("input %q: %v", input, err)
		}
	}

	if _, err := readClusterVersion(
		filepath.Join(t.TempDir(), "missing"),
	); err == nil {
		t.Fatal("expected read error")
	}
}

func TestClusterVersionCLIProcess(t *testing.T) {
	if os.Getenv("RECONCILEGUARD_CV_HELPER") == "1" {
		for i, arg := range os.Args {
			if arg == "--" {
				os.Args = append([]string{"reconcile-guard"}, os.Args[i+1:]...)
				os.Exit(run())
			}
		}
		os.Exit(99)
	}

	invalid := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(invalid, []byte("{invalid"), 0600); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		args []string
		code int
		want string
	}{
		{
			[]string{"check-version", "examples/cluster-version.json"},
			0,
			"Verdict: NOT EVALUATED (snapshot report only)",
		},
		{
			[]string{"replay-version", "examples/cluster-version-history.jsonl"},
			0,
			"Verdict: NOT EVALUATED (phase reconstruction only)",
		},
		{[]string{"check-version"}, 1, "Usage:"},
		{[]string{"replay-version"}, 1, "Usage:"},
		{[]string{"check-version", invalid}, 1, "decode JSON"},
		{
			[]string{"check-version", "examples/ingress.json"},
			1,
			"unsupported resource",
		},
		{[]string{"check", "examples/ingress.json"}, 2, "Result: DEGRADED"},
	} {
		cmd := exec.Command(
			os.Args[0],
			append(
				[]string{"-test.run=^TestClusterVersionCLIProcess$", "--"},
				tc.args...,
			)...,
		)
		cmd.Env = append(os.Environ(), "RECONCILEGUARD_CV_HELPER=1")

		out, err := cmd.CombinedOutput()
		code := 0

		if err != nil {
			if exitError, ok := err.(*exec.ExitError); ok {
				code = exitError.ExitCode()
			} else {
				t.Fatal(err)
			}
		}

		if code != tc.code || !strings.Contains(string(out), tc.want) {
			t.Fatalf("%v: exit %d, output %s", tc.args, code, out)
		}
	}
}
