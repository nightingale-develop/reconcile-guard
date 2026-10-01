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
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/machineconfig"
	nodehistory "github.com/nightingale-develop/reconcile-guard/internal/node"
	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/recording"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
)

func liveTestConfig(t *testing.T, beforeVersionList ...func()) string {
	t.Helper()
	return liveTestConfigWithHook(t, nil, beforeVersionList...)
}

func liveTestConfigWithHook(
	t *testing.T,
	hook func(http.ResponseWriter, *http.Request) bool,
	beforeVersionList ...func(),
) string {
	t.Helper()

	version := map[string]any{
		"apiVersion": "config.openshift.io/v1",
		"kind":       "ClusterVersion",
		"metadata": map[string]any{
			"name":            "version",
			"resourceVersion": "10",
		},
		"status": map[string]any{
			"desired": map[string]any{
				"version": "4.20.0",
			},
		},
	}

	operatorObject := map[string]any{
		"apiVersion": "config.openshift.io/v1",
		"kind":       "ClusterOperator",
		"metadata": map[string]any{
			"name":            "ingress",
			"resourceVersion": "10",
		},
	}

	poolObject := map[string]any{
		"apiVersion": "machineconfiguration.openshift.io/v1",
		"kind":       "MachineConfigPool",
		"metadata": map[string]any{
			"name":            "master",
			"resourceVersion": "10",
		},
		"status": map[string]any{
			"configuration": map[string]any{
				"name": "rendered-master-test",
			},
			"machineCount":            1,
			"updatedMachineCount":     1,
			"readyMachineCount":       1,
			"unavailableMachineCount": 0,
			"degradedMachineCount":    0,
		},
	}

	nodeObject := map[string]any{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata": map[string]any{
			"name":            "node-0",
			"resourceVersion": "10",
			"annotations": map[string]any{
				"machineconfiguration.openshift.io/currentConfig": "rendered-master-test",
				"machineconfiguration.openshift.io/desiredConfig": "rendered-master-test",
			},
		},
		"status": map[string]any{
			"nodeInfo": map[string]any{
				"kubeletVersion":          "v1.33.0",
				"osImage":                 "Fedora CoreOS",
				"kernelVersion":           "6.0.0",
				"containerRuntimeVersion": "cri-o://1.33.0",
				"architecture":            "amd64",
				"operatingSystem":         "linux",
			},
			"conditions": []any{
				map[string]any{
					"type":   "Ready",
					"status": "True",
				},
			},
		},
	}

	server := httptest.NewServer(
		http.HandlerFunc(func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			if r.Method != http.MethodGet {
				t.Errorf(
					"unexpected API mutation: %s",
					r.Method,
				)
				http.Error(
					w,
					"read only",
					http.StatusMethodNotAllowed,
				)
				return
			}

			if hook != nil && hook(w, r) {
				return
			}

			w.Header().Set(
				"Content-Type",
				"application/json",
			)

			if r.URL.Query().Get("watch") == "true" {
				if r.URL.Query().Get(
					"resourceVersion",
				) != "10" {
					t.Errorf(
						"WATCH resourceVersion = %q",
						r.URL.Query().Get(
							"resourceVersion",
						),
					)
				}

				if r.URL.Query().Get(
					"sendInitialEvents",
				) == "true" {
					t.Error(
						"expected initial LIST, not streaming list",
					)
				}

				if strings.HasSuffix(
					r.URL.Path,
					"clusterversions",
				) &&
					r.URL.Query().Get(
						"fieldSelector",
					) != "metadata.name=version" {
					t.Error(
						"missing ClusterVersion selector",
					)
				}

				w.WriteHeader(http.StatusOK)
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

				object = map[string]any{
					"apiVersion": "config.openshift.io/v1",
					"kind":       "ClusterVersionList",
					"metadata": map[string]any{
						"resourceVersion": "10",
					},
					"items": []any{
						version,
					},
				}

			case "/apis/config.openshift.io/v1/clusteroperators":
				object = map[string]any{
					"apiVersion": "config.openshift.io/v1",
					"kind":       "ClusterOperatorList",
					"metadata": map[string]any{
						"resourceVersion": "10",
					},
					"items": []any{
						operatorObject,
					},
				}

			case "/apis/machineconfiguration.openshift.io/v1/machineconfigpools":
				object = map[string]any{
					"apiVersion": "machineconfiguration.openshift.io/v1",
					"kind":       "MachineConfigPoolList",
					"metadata": map[string]any{
						"resourceVersion": "10",
					},
					"items": []any{
						poolObject,
					},
				}

			case "/api/v1/nodes":
				object = map[string]any{
					"apiVersion": "v1",
					"kind":       "NodeList",
					"metadata": map[string]any{
						"resourceVersion": "10",
					},
					"items": []any{
						nodeObject,
					},
				}

			default:
				t.Errorf(
					"unexpected API request: %s",
					r.URL,
				)
				http.NotFound(w, r)
				return
			}

			if err := json.NewEncoder(w).Encode(
				object,
			); err != nil {
				t.Error(err)
			}
		}),
	)

	t.Cleanup(server.Close)

	path := filepath.Join(
		t.TempDir(),
		"config",
	)

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

	if err := os.WriteFile(
		path,
		[]byte(data),
		0600,
	); err != nil {
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

func assertLiveHistories(
	t *testing.T,
	dir string,
	count int,
) {
	t.Helper()

	versions, err := upgrade.ReadHistory(
		filepath.Join(
			dir,
			"cluster-version.jsonl",
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(versions) != count {
		t.Fatalf(
			"versions=%d",
			len(versions),
		)
	}

	if _, err := upgrade.AnalyzeHistory(
		versions,
	); err != nil {
		t.Fatal(err)
	}

	operators, err := operator.ReadHistory(
		filepath.Join(
			dir,
			"operators",
			"ingress.jsonl",
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(operators) != count {
		t.Fatalf(
			"operators=%d",
			len(operators),
		)
	}

	if _, err := operator.AnalyzeHistory(
		operators,
	); err != nil {
		t.Fatal(err)
	}

	pools, err := machineconfig.ReadHistory(
		filepath.Join(
			dir,
			"machine-config-pools",
			"master.jsonl",
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(pools) != count {
		t.Fatalf(
			"machineConfigPools=%d",
			len(pools),
		)
	}

	if _, err := machineconfig.AnalyzeHistory(
		pools,
	); err != nil {
		t.Fatal(err)
	}

	nodes, err := nodehistory.ReadHistory(
		filepath.Join(
			dir,
			"nodes",
			"node-0.jsonl",
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(nodes) != count {
		t.Fatalf(
			"nodes=%d",
			len(nodes),
		)
	}

	if _, err := nodehistory.AnalyzeHistory(
		nodes,
	); err != nil {
		t.Fatal(err)
	}
	if pools[0].Pool.Status.Configuration.Name != "rendered-master-test" ||
		nodes[0].Node.Status.NodeInfo.KubeletVersion != "v1.33.0" ||
		nodehistory.CurrentMachineConfig(nodes[0].Node) != "rendered-master-test" ||
		nodehistory.ReadyStatus(nodes[0].Node) != "True" {
		t.Fatal("MCP/Node capture lost status or annotations")
	}

	for _, args := range [][]string{
		{
			"replay-version",
			filepath.Join(
				dir,
				"cluster-version.jsonl",
			),
		},
		{
			"replay",
			filepath.Join(
				dir,
				"operators",
				"ingress.jsonl",
			),
		},
	} {
		var out, stderr bytes.Buffer

		if code := Run(
			args,
			&out,
			&stderr,
		); code != 0 ||
			stderr.Len() != 0 {
			t.Fatalf(
				"replay exit=%d stderr=%s",
				code,
				&stderr,
			)
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
		{"final MCP LIST error", os.Interrupt, false, "mcp-list"}, {"final Node LIST error", syscall.SIGTERM, false, "node-list"},
		{"final MCP write error", os.Interrupt, false, "mcp-write"}, {"final Node write error", syscall.SIGTERM, false, "node-write"},
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
				if (tc.finalError == "get" && isGet) || (tc.finalError == "list" && strings.HasSuffix(r.URL.Path, "clusteroperators")) || (tc.finalError == "closing" && isGet && gets.Load() == 2) ||
					(tc.finalError == "mcp-list" && strings.HasSuffix(r.URL.Path, "machineconfigpools")) || (tc.finalError == "node-list" && r.URL.Path == "/api/v1/nodes") {
					http.Error(w, "final API unavailable", http.StatusServiceUnavailable)
					return true
				}
				if (tc.finalError == "write" || tc.finalError == "mcp-write" || tc.finalError == "node-write") && isGet && gets.Load() == 1 {
					manifests, _ := filepath.Glob(filepath.Join(dir, "*", "run.json"))
					if len(manifests) != 1 {
						t.Error("missing run")
						return false
					}
					path := filepath.Join(filepath.Dir(manifests[0]), "operators", "ingress.jsonl")
					if tc.finalError == "mcp-write" {
						path = filepath.Join(filepath.Dir(manifests[0]), "machine-config-pools", "master.jsonl")
					} else if tc.finalError == "node-write" {
						path = filepath.Join(filepath.Dir(manifests[0]), "nodes", "node-0.jsonl")
					}
					if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
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
				assertFinalAuxiliaryHistories(t, runDir, v[len(v)-2].ObservedAt, v[len(v)-1].ObservedAt)
			}
		})
	}
}

func assertFinalAuxiliaryHistories(
	t *testing.T,
	directory string,
	from, to time.Time,
) {
	t.Helper()

	pools, err := machineconfig.ReadHistory(
		filepath.Join(
			directory,
			"machine-config-pools",
			"master.jsonl",
		),
	)
	if err != nil || len(pools) != 2 {
		t.Fatalf(
			"final MCP history: count=%d error=%v",
			len(pools),
			err,
		)
	}

	if !pools[1].ObservedAt.After(
		pools[0].ObservedAt,
	) {
		t.Fatal(
			"final MCP observation is not newer than initial observation",
		)
	}

	if pools[0].Pool.ResourceVersion !=
		pools[1].Pool.ResourceVersion {
		t.Fatal(
			"test must retain unchanged MCP resourceVersion",
		)
	}

	nodes, err := nodehistory.ReadHistory(
		filepath.Join(
			directory,
			"nodes",
			"node-0.jsonl",
		),
	)
	if err != nil || len(nodes) != 2 {
		t.Fatalf(
			"final Node history: count=%d error=%v",
			len(nodes),
			err,
		)
	}

	if !nodes[1].ObservedAt.After(
		nodes[0].ObservedAt,
	) {
		t.Fatal(
			"final Node observation is not newer than initial observation",
		)
	}

	if nodes[0].Node.ResourceVersion !=
		nodes[1].Node.ResourceVersion {
		t.Fatal(
			"test must retain unchanged Node resourceVersion",
		)
	}

	finalPool := pools[len(pools)-1]
	finalNode := nodes[len(nodes)-1]

	if !finalPool.ObservedAt.After(from) ||
		!finalNode.ObservedAt.After(
			finalPool.ObservedAt,
		) ||
		!to.After(finalNode.ObservedAt) {
		t.Fatal(
			"MCP/Node snapshots not inside final CV bracket in LIST order",
		)
	}

	manifest, err :=
		recording.ReadRunManifest(directory)
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(
		manifest.MachineConfigPools,
		[]string{"master"},
	) ||
		!slices.Equal(
			manifest.Nodes,
			[]string{"node-0"},
		) {
		t.Fatalf(
			"final MCP/Node inventory: %+v",
			manifest,
		)
	}
}
