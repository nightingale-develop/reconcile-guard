package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nightingale-develop/reconcile-guard/internal/result"
)

func assertJSONVerdict(
	t *testing.T,
	stdout *bytes.Buffer,
	command string,
	verdict result.Verdict,
) {
	t.Helper()

	var document result.Document

	if err := json.Unmarshal(
		stdout.Bytes(),
		&document,
	); err != nil {
		t.Fatalf(
			"decode JSON: %v\noutput:\n%s",
			err,
			stdout.String(),
		)
	}

	if document.SchemaVersion != result.SchemaVersion {
		t.Fatalf(
			"schema version = %q, want %q",
			document.SchemaVersion,
			result.SchemaVersion,
		)
	}

	if document.Command != command {
		t.Fatalf(
			"command = %q, want %q",
			document.Command,
			command,
		)
	}

	if document.Result.Verdict != verdict {
		t.Fatalf(
			"verdict = %s, want %s",
			document.Result.Verdict,
			verdict,
		)
	}
}

func TestJSONOutputPreservesPassExitCode(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(
		[]string{
			"verify-upgrade",
			"../../examples/cluster-version-history.jsonl",
			"../../examples/ingress-upgrade-history.jsonl",
			"--output",
			"json",
		},
		&stdout,
		&stderr,
	)

	if code != 0 {
		t.Fatalf(
			"exit = %d, want 0\nstderr: %s",
			code,
			stderr.String(),
		)
	}

	assertJSONVerdict(
		t,
		&stdout,
		"verify-upgrade",
		result.VerdictPass,
	)
}

func TestJSONOutputPreservesFailExitCode(t *testing.T) {
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

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(
		[]string{
			"verify-version-upgrade",
			"../../examples/cluster-version-history.jsonl",
			path,
			"--output=json",
		},
		&stdout,
		&stderr,
	)

	if code != 2 {
		t.Fatalf(
			"exit = %d, want 2\nstderr: %s",
			code,
			stderr.String(),
		)
	}

	assertJSONVerdict(
		t,
		&stdout,
		"verify-version-upgrade",
		result.VerdictFail,
	)
}

func TestJSONOutputPreservesInconclusiveExitCode(
	t *testing.T,
) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(
		[]string{
			"verify-progressing-upgrade",
			"../../examples/cluster-version-history.jsonl",
			"../../examples/ingress-upgrade-history.jsonl",
			"../../examples/progressing-policy.json",
			"--output",
			"json",
		},
		&stdout,
		&stderr,
	)

	if code != 3 {
		t.Fatalf(
			"exit = %d, want 3\nstderr: %s",
			code,
			stderr.String(),
		)
	}

	assertJSONVerdict(
		t,
		&stdout,
		"verify-progressing-upgrade",
		result.VerdictInconclusive,
	)
}
