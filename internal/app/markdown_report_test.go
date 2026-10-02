package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportRunRendersMarkdownToStdout(t *testing.T) {
	dir := verifyRunFixture(t)
	var out, stderr bytes.Buffer
	if code := Run([]string{"report-run", dir}, &out, &stderr); code != 0 || stderr.Len() != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, &out, &stderr)
	}
	for _, expected := range []string{
		"# ReconcileGuard run report",
		"## Run",
		"## Verification summary",
		"### ClusterOperators",
		"## Timeline",
		"Transition intervals are observation bounds",
	} {
		if !strings.Contains(out.String(), expected) {
			t.Fatalf("missing %q in report:\n%s", expected, &out)
		}
	}
}

func TestReportRunWritesFileAndIncludesPolicy(t *testing.T) {
	dir := verifyRunFixture(t)
	output := filepath.Join(t.TempDir(), "report.md")
	policyPath := filepath.Join("../../examples", "lifecycle-policy.yaml")
	var out, stderr bytes.Buffer
	if code := Run([]string{"report-run", dir, "--policy", policyPath, "--file", output}, &out, &stderr); code != 0 || stderr.Len() != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, &out, &stderr)
	}
	if !strings.Contains(out.String(), "Markdown report: "+output) {
		t.Fatalf("stdout=%s", &out)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "## Lifecycle policy") ||
		!strings.Contains(text, "examples/lifecycle-policy.md#synthetic-example") ||
		!strings.Contains(text, "operator-progressing-policy") {
		t.Fatalf("report=%s", text)
	}
}

func TestReportRunRejectsInvalidOptions(t *testing.T) {
	dir := verifyRunFixture(t)
	for _, args := range [][]string{
		{"report-run"},
		{"report-run", dir, "--unknown"},
		{"report-run", dir, "--policy"},
		{"report-run", dir, "--file"},
		{"report-run", dir, "--policy=a", "--policy=b"},
	} {
		var out, stderr bytes.Buffer
		if code := Run(args, &out, &stderr); code != 1 || out.Len() != 0 || stderr.Len() == 0 {
			t.Fatalf("args=%v exit=%d stdout=%s stderr=%s", args, code, &out, &stderr)
		}
	}
}

func TestMarkdownCellEscapesUntrustedEvidence(t *testing.T) {
	input := "<img src=x onerror=alert(1)> [link](https://example.org) `code` *bold* _em_ | &\r\nnext"
	want := "&lt;img src=x onerror=alert(1)&gt; \\[link\\](https://example.org) \\`code\\` \\*bold\\* \\_em\\_ \\| &amp;  next"
	if got := markdownCell(input); got != want {
		t.Fatalf("escaped cell = %q, want %q", got, want)
	}
	if got := markdownCell("<missing>"); got != "&lt;missing&gt;" {
		t.Fatalf("missing marker would render as HTML: %q", got)
	}
}

func TestReportRunDeterministic(t *testing.T) {
	dir := verifyRunFixture(t)
	var first, second, stderr bytes.Buffer
	args := []string{"report-run", dir, "--policy", "../../examples/lifecycle-policy.yaml"}
	for _, out := range []*bytes.Buffer{&first, &second} {
		if code := Run(args, out, &stderr); code != 0 {
			t.Fatalf("exit=%d stderr=%s", code, &stderr)
		}
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("report changed for identical evidence and policy")
	}
}

func TestReportRunPreservesVerdictAndPresentationExit(t *testing.T) {
	for _, tc := range []struct{ version, verdict string }{
		{"4.19.0", "FAIL"}, {"", "INCONCLUSIVE"},
	} {
		t.Run(tc.verdict, func(t *testing.T) {
			dir := verifyRunFixture(t)
			changeRunHistory(t, dir, `"version":"4.20.0"`, `"version":"`+tc.version+`"`)
			var out, stderr bytes.Buffer
			if code := Run([]string{"report-run", dir}, &out, &stderr); code != 0 || !strings.Contains(out.String(), "| Aggregate verdict | "+tc.verdict+" |") {
				t.Fatalf("exit=%d stdout=%s stderr=%s", code, &out, &stderr)
			}
		})
	}
}

func TestReportRunOutputFailures(t *testing.T) {
	dir := verifyRunFixture(t)
	var stderr, out bytes.Buffer
	if code := Run([]string{"report-run", dir}, comparisonErrorWriter{}, &stderr); code != 1 || stderr.Len() == 0 {
		t.Fatalf("stdout failure: exit=%d stderr=%s", code, &stderr)
	}
	stderr.Reset()
	if code := Run([]string{"report-run", dir, "--file", t.TempDir()}, &out, &stderr); code != 1 || stderr.Len() == 0 {
		t.Fatalf("file failure: exit=%d stderr=%s", code, &stderr)
	}
}
