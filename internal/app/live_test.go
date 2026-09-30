package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/recording"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
)

func liveTestConfig(t *testing.T, beforeVersionList ...func()) string {
	t.Helper()
	return liveTestConfigWithHook(t, nil, beforeVersionList...)
}

func liveTestConfigWithHook(t *testing.T, hook func(http.ResponseWriter, *http.Request) bool, beforeVersionList ...func()) string {
	t.Helper()
	version := map[string]any{
		"apiVersion": "config.openshift.io/v1", "kind": "ClusterVersion",
		"metadata": map[string]any{"name": "version", "resourceVersion": "10"},
		"status":   map[string]any{"desired": map[string]any{"version": "4.20.0"}},
	}
	operatorObject := map[string]any{
		"apiVersion": "config.openshift.io/v1", "kind": "ClusterOperator",
		"metadata": map[string]any{"name": "ingress", "resourceVersion": "10"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected API mutation: %s", r.Method)
			http.Error(w, "read only", 405)
			return
		}
		if hook != nil && hook(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("watch") == "true" {
			if r.URL.Query().Get("resourceVersion") != "10" {
				t.Errorf("WATCH resourceVersion = %q", r.URL.Query().Get("resourceVersion"))
			}
			if r.URL.Query().Get("sendInitialEvents") == "true" {
				t.Error("expected initial LIST, not streaming list")
			}
			if strings.HasSuffix(r.URL.Path, "clusterversions") && r.URL.Query().Get("fieldSelector") != "metadata.name=version" {
				t.Error("missing ClusterVersion selector")
			}
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		var object any
		switch r.URL.Path {
		case "/apis/config.openshift.io/v1/clusterversions/version":
			object = version
		case "/apis/config.openshift.io/v1/clusterversions":
			for _, before := range beforeVersionList {
				before()
			}
			object = map[string]any{"apiVersion": "config.openshift.io/v1", "kind": "ClusterVersionList", "metadata": map[string]any{"resourceVersion": "10"}, "items": []any{version}}
		case "/apis/config.openshift.io/v1/clusteroperators":
			object = map[string]any{"apiVersion": "config.openshift.io/v1", "kind": "ClusterOperatorList", "metadata": map[string]any{"resourceVersion": "10"}, "items": []any{operatorObject}}
		default:
			t.Errorf("unexpected API request: %s", r.URL)
			http.NotFound(w, r)
			return
		}
		if err := json.NewEncoder(w).Encode(object); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	path := filepath.Join(t.TempDir(), "config")
	data := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: local
  cluster:
    server: %s
contexts:
- name: local
  context:
    cluster: local
    user: local
current-context: local
users:
- name: local
  user: {}
`, server.URL)
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCaptureLiveAppendAndDefaultConfig(t *testing.T) {
	config := liveTestConfig(t)
	t.Setenv("KUBECONFIG", config)
	dir := t.TempDir()
	for _, args := range [][]string{{"capture-live", dir}, {"capture-live", dir, "--kubeconfig", config}} {
		var out, stderr bytes.Buffer
		if code := Run(args, &out, &stderr); code != 0 || stderr.Len() != 0 {
			t.Fatalf("exit=%d stderr=%s", code, &stderr)
		}
		if !strings.Contains(out.String(), "ClusterOperators: 1") {
			t.Fatalf("stdout=%s", &out)
		}
	}
	assertLiveHistories(t, dir, 2)
}

func assertLiveHistories(t *testing.T, dir string, count int) {
	t.Helper()
	versions, err := upgrade.ReadHistory(filepath.Join(dir, "cluster-version.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != count {
		t.Fatalf("versions=%d", len(versions))
	}
	if _, err := upgrade.AnalyzeHistory(versions); err != nil {
		t.Fatal(err)
	}
	operators, err := operator.ReadHistory(filepath.Join(dir, "operators", "ingress.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(operators) != count {
		t.Fatalf("operators=%d", len(operators))
	}
	if _, err := operator.AnalyzeHistory(operators); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"replay-version", filepath.Join(dir, "cluster-version.jsonl")}, {"replay", filepath.Join(dir, "operators", "ingress.jsonl")}} {
		var out, stderr bytes.Buffer
		if code := Run(args, &out, &stderr); code != 0 || stderr.Len() != 0 {
			t.Fatalf("replay exit=%d stderr=%s", code, &stderr)
		}
	}
}

func TestLiveCommandProcess(t *testing.T) {
	if os.Getenv("RECONCILE_LIVE_TEST_PROCESS") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Exit(Run(os.Args[i+1:], os.Stdout, os.Stderr))
		}
	}
	os.Exit(99)
}

func TestRecordLiveSignalsAndWriteError(t *testing.T) {
	for _, tc := range []struct {
		name       string
		signal     os.Signal
		writeError bool
		finalError string
	}{
		{"interrupt", os.Interrupt, false, ""}, {"terminate", syscall.SIGTERM, false, ""}, {"write error", nil, true, ""},
		{"final GET error", os.Interrupt, false, "get"}, {"final LIST error", syscall.SIGTERM, false, "list"},
		{"final closing GET error", os.Interrupt, false, "closing"}, {"final write error", os.Interrupt, false, "write"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			var final atomic.Bool
			var gets atomic.Int32
			config := liveTestConfigWithHook(t, func(w http.ResponseWriter, r *http.Request) bool {
				if !final.Load() || r.URL.Query().Get("watch") == "true" {
					return false
				}
				isGet := strings.HasSuffix(r.URL.Path, "clusterversions/version")
				if isGet {
					gets.Add(1)
				}
				if (tc.finalError == "get" && isGet) || (tc.finalError == "list" && strings.HasSuffix(r.URL.Path, "clusteroperators")) || (tc.finalError == "closing" && isGet && gets.Load() == 2) {
					http.Error(w, "final API unavailable", http.StatusServiceUnavailable)
					return true
				}
				if tc.finalError == "write" && isGet && gets.Load() == 1 {
					manifests, _ := filepath.Glob(filepath.Join(dir, "*", "run.json"))
					if len(manifests) != 1 {
						t.Error("missing run")
						return false
					}
					path := filepath.Join(filepath.Dir(manifests[0]), "operators", "ingress.jsonl")
					if err := os.Remove(path); err != nil {
						t.Error(err)
					}
					if err := os.Mkdir(path, 0755); err != nil {
						t.Error(err)
					}
				}
				return false
			}, func() {
				if !tc.writeError {
					return
				}
				manifests, err := filepath.Glob(filepath.Join(dir, "*", "run.json"))
				if err != nil || len(manifests) != 1 {
					t.Errorf("expected one new run, got %v (%v)", manifests, err)
					return
				}
				if err := os.Mkdir(filepath.Join(filepath.Dir(manifests[0]), "cluster-version.jsonl"), 0755); err != nil {
					t.Error(err)
				}
			})
			runDir := ""

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLiveCommandProcess$", "--", "record-live", dir, "--kubeconfig", config)
			cmd.Env = append(os.Environ(), "RECONCILE_LIVE_TEST_PROCESS=1")
			var out, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			if !tc.writeError {
				ready := false
				for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
					manifests, _ := filepath.Glob(filepath.Join(dir, "*", "run.json"))
					if len(manifests) != 1 {
						time.Sleep(10 * time.Millisecond)
						continue
					}
					runDir = filepath.Dir(manifests[0])
					v, ve := upgrade.ReadHistory(filepath.Join(runDir, "cluster-version.jsonl"))
					o, oe := operator.ReadHistory(filepath.Join(runDir, "operators", "ingress.jsonl"))
					if ve == nil && oe == nil && len(v) == 1 && len(o) == 1 {
						ready = true
						break
					}
					time.Sleep(10 * time.Millisecond)
				}
				if !ready {
					cancel()
					_ = cmd.Wait()
					t.Fatalf("initial observations missing: stdout=%s stderr=%s", &out, &stderr)
				}
				final.Store(true)
				if err := cmd.Process.Signal(tc.signal); err != nil {
					cancel()
					_ = cmd.Wait()
					t.Fatal(err)
				}
			}
			err := cmd.Wait()
			if ctx.Err() != nil {
				t.Fatal("record-live hung")
			}
			manifests, _ := filepath.Glob(filepath.Join(dir, "*", "run.json"))
			if len(manifests) != 1 {
				t.Fatalf("expected one run manifest, got %v", manifests)
			}
			manifest, readErr := recording.ReadRunManifest(filepath.Dir(manifests[0]))
			if readErr != nil {
				t.Fatal(readErr)
			}
			wantStatus := recording.RunStatusStopped
			if tc.writeError || tc.finalError != "" {
				wantStatus = recording.RunStatusFailed
			}
			if manifest.Status != wantStatus || manifest.EndedAt == nil || manifest.EndedAt.Before(manifest.StartedAt) {
				t.Fatalf("incorrect final manifest: %+v", manifest)
			}
			if (tc.writeError || tc.finalError != "") && manifest.Error == "" {
				t.Fatal("failed run lost error")
			}
			if !tc.writeError && (len(manifest.Operators) != 1 || manifest.Operators[0] != "ingress") {
				t.Fatalf("operator inventory=%v", manifest.Operators)
			}
			if tc.finalError != "" {
				if err == nil || cmd.ProcessState.ExitCode() != 1 || !strings.Contains(stderr.String(), "final snapshot") || strings.Contains(out.String(), "Recording stopped") {
					t.Fatalf("final failure lost: exit=%v out=%s err=%s", err, &out, &stderr)
				}
			} else if tc.writeError {
				if err == nil || cmd.ProcessState.ExitCode() != 1 || !strings.Contains(stderr.String(), "write ClusterVersion observation") {
					t.Fatalf("exit=%v stdout=%s stderr=%s", err, &out, &stderr)
				}
				if strings.Contains(out.String(), "Recording stopped") {
					t.Fatal("reported clean shutdown after write error")
				}
			} else {
				if err != nil || stderr.Len() != 0 || !strings.Contains(out.String(), "Recording stopped") {
					t.Fatalf("exit=%v stdout=%s stderr=%s", err, &out, &stderr)
				}
				v, ve := upgrade.ReadHistory(filepath.Join(runDir, "cluster-version.jsonl"))
				o, oe := operator.ReadHistory(filepath.Join(runDir, "operators", "ingress.jsonl"))
				if ve != nil || oe != nil || len(v) != 3 || len(o) != 2 {
					t.Fatalf("final capture missing: CV=%d CO=%d errors=%v %v", len(v), len(o), ve, oe)
				}
				if _, err := upgrade.AnalyzeHistory(v); err != nil {
					t.Fatal(err)
				}
				if _, err := operator.AnalyzeHistory(o); err != nil {
					t.Fatal(err)
				}
				if !o[1].ObservedAt.After(v[1].ObservedAt) || !v[2].ObservedAt.After(o[1].ObservedAt) {
					t.Fatal("final LIST lacks real CV bracket")
				}
				if o[0].Operator.ResourceVersion != o[1].Operator.ResourceVersion {
					t.Fatal("test must retain unchanged resourceVersion")
				}
			}
		})
	}
}
