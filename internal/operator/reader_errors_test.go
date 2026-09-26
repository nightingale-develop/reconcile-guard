package operator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadHistoryReportsOversizedLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized.jsonl")
	if err := os.WriteFile(path, []byte("\n"+strings.Repeat("x", 4*1024*1024)), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadHistory(path)
	if err == nil || !strings.Contains(err.Error(), "line 2") || len(got) != 0 {
		t.Fatalf("partial data or missing line: %d %v", len(got), err)
	}
}
