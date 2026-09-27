package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyVersionUpgrade(t *testing.T) {
	var out, stderr bytes.Buffer

	code := Run(
		[]string{
			"verify-version-upgrade",
			"../../examples/cluster-version-history.jsonl",
			"../../examples/ingress-version-history.jsonl",
		},
		&out,
		&stderr,
	)

	if code != 0 {
		t.Fatalf(
			"exit=%d stdout=%q stderr=%q",
			code,
			out.String(),
			stderr.String(),
		)
	}

	if !strings.Contains(
		out.String(),
		"Verdict: PASS",
	) {
		t.Fatalf("unexpected stdout: %s", &out)
	}

	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", &stderr)
	}
}

func TestVerifyVersionUpgradeFail(t *testing.T) {
	data, err := os.ReadFile(
		"../../examples/ingress-version-history.jsonl",
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
		"operator.jsonl",
	)

	if err := os.WriteFile(
		path,
		data,
		0600,
	); err != nil {
		t.Fatal(err)
	}

	var out, stderr bytes.Buffer

	code := Run(
		[]string{
			"verify-version-upgrade",
			"../../examples/cluster-version-history.jsonl",
			path,
		},
		&out,
		&stderr,
	)

	if code != 2 {
		t.Fatalf(
			"exit=%d stdout=%q stderr=%q",
			code,
			out.String(),
			stderr.String(),
		)
	}

	if !strings.Contains(
		out.String(),
		"Verdict: FAIL",
	) {
		t.Fatalf("unexpected stdout: %s", &out)
	}
}

func TestVerifyVersionUpgradeUsage(t *testing.T) {
	var out, stderr bytes.Buffer

	code := Run(
		[]string{
			"verify-version-upgrade",
		},
		&out,
		&stderr,
	)

	if code != 1 ||
		out.Len() != 0 ||
		stderr.Len() == 0 {
		t.Fatalf(
			"exit=%d stdout=%q stderr=%q",
			code,
			out.String(),
			stderr.String(),
		)
	}
}
