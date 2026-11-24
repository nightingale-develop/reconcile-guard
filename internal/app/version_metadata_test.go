package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nightingale-develop/reconcile-guard/internal/recording"
	"github.com/nightingale-develop/reconcile-guard/internal/result"
	appversion "github.com/nightingale-develop/reconcile-guard/internal/version"
)

func assertRunVersionMatchesCLI(t *testing.T, manifest recording.RunManifest) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run([]string{"version"}, &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 || manifest.ToolVersion != appversion.Current || stdout.String() != "ReconcileGuard v"+manifest.ToolVersion+"\n" {
		t.Fatalf("manifest version=%q application version=%q exit=%d stdout=%q stderr=%q", manifest.ToolVersion, appversion.Current, code, &stdout, &stderr)
	}
}

func TestVerifyRunAcceptsOlderToolVersion(t *testing.T) {
	dir := verifyRunWithAuxiliaryFixture(t)
	changeRunManifest(t, dir, func(m *recording.RunManifest) { m.ToolVersion = "1.0.0" })
	path := filepath.Join(dir, "run.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"text", "json"} {
		var stdout, stderr bytes.Buffer
		if code := Run([]string{"verify-run", dir, "--output", format}, &stdout, &stderr); code != 0 || stderr.Len() != 0 {
			t.Fatalf("%s: exit=%d stdout=%s stderr=%s", format, code, &stdout, &stderr)
		}
		if format == "json" {
			var doc result.Document
			if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
				t.Fatal(err)
			}
			if doc.SchemaVersion != "1" || doc.Result.Verdict != result.VerdictPass {
				t.Fatalf("document=%+v", doc)
			}
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("verification modified the older manifest: %v", err)
	}
}
