package app

import (
	"bytes"
	"encoding/json"
	"github.com/nightingale-develop/reconcile-guard/internal/result"
	"testing"
)

func TestRunConditionEvidenceDoesNotClaimFailure(t *testing.T) {
	dir := verifyRunFixture(t)
	changeRunHistory(t, dir, `"type":"Degraded","status":"False"`, `"type":"Degraded","status":"True"`)
	for _, format := range []string{"text", "json"} {
		var out, stderr bytes.Buffer
		if code := Run([]string{"verify-run", dir, "--output=" + format}, &out, &stderr); code != 3 || stderr.Len() != 0 {
			t.Fatalf("%s code=%d out=%s err=%s", format, code, &out, &stderr)
		}
		if format == "json" {
			var doc result.Document
			if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
				t.Fatal(err)
			}
			c := doc.Result.Operators[0].Contracts[0]
			if doc.SchemaVersion != "1" || doc.Result.Verdict != result.VerdictInconclusive || c.Verdict != result.VerdictInconclusive || len(c.Evidence) == 0 {
				t.Fatalf("document=%+v", doc)
			}
			for _, e := range c.Evidence {
				if e.Verdict != result.VerdictInconclusive {
					t.Fatalf("evidence=%+v", e)
				}
			}
		}
	}
}
