package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nightingale-develop/reconcile-guard/internal/version"
)

func TestReleaseVersionCheck(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		code int
	}{
		{"matching tag", []string{"v" + version.Current}, 0},
		{"stale tag", []string{"v1.0.0"}, 1},
		{"different tag", []string{"v9.9.9"}, 1},
		{"missing prefix", []string{version.Current}, 1},
		{"dirty description", []string{"v" + version.Current + "-dirty"}, 1},
		{"empty tag", []string{""}, 1},
		{"missing argument", nil, 1},
		{"extra argument", []string{"v" + version.Current, "extra"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(tc.args, &stdout, &stderr); code != tc.code {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, &stdout, &stderr)
			}
			if tc.code == 0 {
				if stderr.Len() != 0 || !strings.Contains(stdout.String(), "matches application version "+version.Current) {
					t.Fatalf("stdout=%q stderr=%q", &stdout, &stderr)
				}
			} else if stdout.Len() != 0 || stderr.Len() == 0 {
				t.Fatalf("stdout=%q stderr=%q", &stdout, &stderr)
			}
		})
	}
}
