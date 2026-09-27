package app

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nightingale-develop/reconcile-guard/internal/result"
)

func TestClusterUpgradeJSONOutput(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(
		[]string{
			"verify-cluster-upgrade",
			"../../examples/cluster-version-history.jsonl",
			"../../examples/ingress-version-history.jsonl",
			"../../examples/network-version-history.jsonl",
			"--output",
			"json",
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

	var document result.Document

	if err := json.Unmarshal(
		stdout.Bytes(),
		&document,
	); err != nil {
		t.Fatal(err)
	}

	if document.SchemaVersion != result.SchemaVersion {
		t.Fatalf(
			"schema version = %q, want %q",
			document.SchemaVersion,
			result.SchemaVersion,
		)
	}

	if document.Command != "verify-cluster-upgrade" {
		t.Fatalf(
			"command = %q",
			document.Command,
		)
	}

	if document.Result.Verdict != result.VerdictPass {
		t.Fatalf(
			"verdict = %s",
			document.Result.Verdict,
		)
	}

	if len(document.Result.Operators) != 2 {
		t.Fatalf(
			"operators = %d, want 2",
			len(document.Result.Operators),
		)
	}

	for _, operatorResult := range document.Result.Operators {
		if len(operatorResult.Contracts) != 2 {
			t.Fatalf(
				"operator %q contracts = %d, want 2",
				operatorResult.Name,
				len(operatorResult.Contracts),
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

func TestUpgradeJSONOutput(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(
		[]string{
			"verify-upgrade",
			"../../examples/cluster-version-history.jsonl",
			"../../examples/ingress-upgrade-history.jsonl",
			"--output=json",
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

	var document result.Document

	if err := json.Unmarshal(
		stdout.Bytes(),
		&document,
	); err != nil {
		t.Fatal(err)
	}

	if len(document.Result.Operators) != 1 {
		t.Fatalf(
			"operators = %d, want 1",
			len(document.Result.Operators),
		)
	}

	operatorResult :=
		document.Result.Operators[0]

	if operatorResult.Name != "ingress" {
		t.Fatalf(
			"operator = %q",
			operatorResult.Name,
		)
	}

	if len(operatorResult.Contracts) != 1 {
		t.Fatalf(
			"contracts = %d, want 1",
			len(operatorResult.Contracts),
		)
	}
}

func TestOutputTextIsDefault(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(
		[]string{
			"verify-upgrade",
			"../../examples/cluster-version-history.jsonl",
			"../../examples/ingress-upgrade-history.jsonl",
		},
		&stdout,
		&stderr,
	)

	if code != 0 {
		t.Fatalf(
			"exit=%d stderr=%q",
			code,
			stderr.String(),
		)
	}

	if !strings.Contains(
		stdout.String(),
		"Contract:",
	) {
		t.Fatalf(
			"unexpected stdout: %s",
			&stdout,
		)
	}
}

func TestInvalidOutputFormat(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(
		[]string{
			"verify-upgrade",
			"--output",
			"yaml",
		},
		&stdout,
		&stderr,
	)

	if code != 1 {
		t.Fatalf(
			"exit=%d, want 1",
			code,
		)
	}

	if !strings.Contains(
		stderr.String(),
		"unsupported output format",
	) {
		t.Fatalf(
			"unexpected stderr: %s",
			&stderr,
		)
	}
}

func TestOutputRejectedForNonVerifyCommand(
	t *testing.T,
) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(
		[]string{
			"replay",
			"../../examples/ingress-history.jsonl",
			"--output",
			"json",
		},
		&stdout,
		&stderr,
	)

	if code != 1 {
		t.Fatalf(
			"exit=%d, want 1",
			code,
		)
	}

	if !strings.Contains(
		stderr.String(),
		"supported only by verify commands",
	) {
		t.Fatalf(
			"unexpected stderr: %s",
			&stderr,
		)
	}
}

func TestDuplicateOutputOption(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(
		[]string{
			"verify-upgrade",
			"--output",
			"json",
			"--output=text",
		},
		&stdout,
		&stderr,
	)

	if code != 1 {
		t.Fatalf(
			"exit=%d, want 1",
			code,
		)
	}

	if !strings.Contains(
		stderr.String(),
		"may only be specified once",
	) {
		t.Fatalf(
			"unexpected stderr: %s",
			&stderr,
		)
	}
}
