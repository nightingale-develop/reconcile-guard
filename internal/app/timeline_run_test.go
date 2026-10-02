package app

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nightingale-develop/reconcile-guard/internal/timeline"
)

func TestTimelineRunOutputFailure(t *testing.T) {
	dir := verifyRunFixture(t)
	for _, format := range []string{"text", "json"} {
		var stderr bytes.Buffer
		if code := Run([]string{"timeline-run", dir, "--output", format}, comparisonErrorWriter{}, &stderr); code != 1 || stderr.Len() == 0 {
			t.Fatalf("format=%s exit=%d stderr=%s", format, code, &stderr)
		}
	}
}

func TestTimelineRunTextAndJSON(t *testing.T) {
	dir := verifyRunFixture(t)

	var textOut, textErr bytes.Buffer
	if code := Run([]string{"timeline-run", dir}, &textOut, &textErr); code != 0 || textErr.Len() != 0 {
		t.Fatalf("text exit=%d stdout=%s stderr=%s", code, &textOut, &textErr)
	}
	if !strings.Contains(textOut.String(), "Timeline events:") ||
		!strings.Contains(textOut.String(), "ClusterVersion/version") ||
		!strings.Contains(textOut.String(), "observation bounds") {
		t.Fatalf("text output=%s", &textOut)
	}

	var jsonOut, jsonErr bytes.Buffer
	if code := Run([]string{"timeline-run", dir, "--output", "json"}, &jsonOut, &jsonErr); code != 0 || jsonErr.Len() != 0 {
		t.Fatalf("json exit=%d stdout=%s stderr=%s", code, &jsonOut, &jsonErr)
	}
	var doc timeline.Document
	if err := json.Unmarshal(jsonOut.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.SchemaVersion != "1" || doc.Command != "timeline-run" || doc.RunID == "" || len(doc.Timeline.Events) == 0 {
		t.Fatalf("document=%+v", doc)
	}
}

func TestTimelineRunUsage(t *testing.T) {
	for _, args := range [][]string{{"timeline-run"}, {"timeline-run", "one", "two"}} {
		var out, stderr bytes.Buffer
		if code := Run(args, &out, &stderr); code != 1 || out.Len() != 0 || stderr.Len() == 0 {
			t.Fatalf("args=%v exit=%d stdout=%s stderr=%s", args, code, &out, &stderr)
		}
	}
}
