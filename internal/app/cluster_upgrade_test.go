package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyClusterUpgrade(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(
		[]string{
			"verify-cluster-upgrade",
			"../../examples/cluster-version-history.jsonl",
			"../../examples/ingress-version-history.jsonl",
			"../../examples/network-version-history.jsonl",
		},
		&stdout,
		&stderr,
	)

	if code != 0 {
		t.Fatalf(
			"exit=%d stdout=%q stderr=%q",
			code,
			stdout.String(),
			stderr.String(),
		)
	}

	output := stdout.String()

	expected := []string{
		"Aggregate verdict: PASS",
		"Operators: 2",
		"Passed: 2",
		"Operator: ingress",
		"Operator: network",
		"normal-upgrade-operator-conditions: PASS",
		"operator-version-consistency: PASS",
	}

	for _, value := range expected {
		if !strings.Contains(output, value) {
			t.Fatalf(
				"stdout missing %q:\n%s",
				value,
				output,
			)
		}
	}

	if stderr.Len() != 0 {
		t.Fatalf(
			"unexpected stderr: %s",
			&stderr,
		)
	}
}

func TestVerifyClusterUpgradeFail(t *testing.T) {
	data, err := os.ReadFile(
		"../../examples/network-version-history.jsonl",
	)
	if err != nil {
		t.Fatal(err)
	}

	data = bytes.Replace(
		data,
		[]byte(
			`"name":"operator","version":"4.20.0"`,
		),
		[]byte(
			`"name":"operator","version":"4.19.0"`,
		),
		1,
	)

	path := filepath.Join(
		t.TempDir(),
		"network.jsonl",
	)

	if err := os.WriteFile(
		path,
		data,
		0600,
	); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(
		[]string{
			"verify-cluster-upgrade",
			"../../examples/cluster-version-history.jsonl",
			"../../examples/ingress-version-history.jsonl",
			path,
		},
		&stdout,
		&stderr,
	)

	if code != 2 {
		t.Fatalf(
			"exit=%d stdout=%q stderr=%q",
			code,
			stdout.String(),
			stderr.String(),
		)
	}

	if !strings.Contains(
		stdout.String(),
		"Aggregate verdict: FAIL",
	) {
		t.Fatalf(
			"unexpected stdout:\n%s",
			&stdout,
		)
	}
}

func TestVerifyClusterUpgradeUsage(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(
		[]string{
			"verify-cluster-upgrade",
		},
		&stdout,
		&stderr,
	)

	if code != 1 {
		t.Fatalf("exit=%d, want 1", code)
	}

	if stdout.Len() != 0 {
		t.Fatalf(
			"unexpected stdout: %s",
			&stdout,
		)
	}

	if stderr.Len() == 0 {
		t.Fatal("expected usage on stderr")
	}
}
