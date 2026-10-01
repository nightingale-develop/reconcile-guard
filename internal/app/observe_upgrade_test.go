package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/recording"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
)

func observerVersion(rv, phase, version, image string) configv1.ClusterVersion {
	start := metav1.NewTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if version == "4.19.0" {
		start = metav1.NewTime(start.Add(-24 * time.Hour))
	}
	v := configv1.ClusterVersion{
		TypeMeta:   metav1.TypeMeta{APIVersion: "config.openshift.io/v1", Kind: "ClusterVersion"},
		ObjectMeta: metav1.ObjectMeta{Name: "version", ResourceVersion: rv, Generation: 1},
		Spec:       configv1.ClusterVersionSpec{ClusterID: "11111111-1111-4111-8111-111111111111"},
		Status:     configv1.ClusterVersionStatus{ObservedGeneration: 1, Desired: configv1.Release{Version: version, Image: image}},
	}
	progressing := configv1.ConditionTrue
	history := configv1.UpdateHistory{Version: version, Image: image, State: configv1.PartialUpdate, StartedTime: start}
	if phase == "stable" {
		progressing = configv1.ConditionFalse
		end := metav1.NewTime(start.Add(time.Hour))
		history.State, history.CompletionTime = configv1.CompletedUpdate, &end
	}
	v.Status.History = []configv1.UpdateHistory{history}
	v.Status.Conditions = []configv1.ClusterOperatorStatusCondition{{Type: configv1.OperatorProgressing, Status: progressing}, {Type: configv1.OperatorAvailable, Status: configv1.ConditionTrue}}
	if phase == "unknown" {
		v.Status.ObservedGeneration = 0
	}
	return v
}

type observerProcess struct {
	t              *testing.T
	dir            string
	cmd            *exec.Cmd
	ctx            context.Context
	cancel         context.CancelFunc
	out, stderr    bytes.Buffer
	events         chan configv1.ClusterVersion
	operatorsReady chan struct{}
	mu             sync.Mutex
	current        configv1.ClusterVersion
	watchStatus    atomic.Int32
}

func startObserverProcess(t *testing.T, initial configv1.ClusterVersion, operatorVersion string, finalError bool) *observerProcess {
	t.Helper()
	p := &observerProcess{t: t, dir: t.TempDir(), events: make(chan configv1.ClusterVersion), operatorsReady: make(chan struct{}), current: initial}
	config := liveTestConfigWithHook(t, func(w http.ResponseWriter, r *http.Request) bool {
		w.Header().Set("Content-Type", "application/json")
		isVersion := strings.Contains(r.URL.Path, "clusterversions")
		if r.URL.Query().Get("watch") == "true" {
			if isVersion {
				if status := p.watchStatus.Swap(0); status != 0 {
					http.Error(w, http.StatusText(int(status)), int(status))
					return true
				}
			}
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			if !isVersion {
				<-r.Context().Done()
				return true
			}
			for {
				select {
				case <-r.Context().Done():
					return true
				case v := <-p.events:
					if v.ResourceVersion == "restart" {
						return true
					}
					p.mu.Lock()
					p.current = v
					p.mu.Unlock()
					_ = json.NewEncoder(w).Encode(map[string]any{"type": "MODIFIED", "object": v})
					w.(http.Flusher).Flush()
				}
			}
		}
		if isVersion {
			p.mu.Lock()
			v := p.current
			p.mu.Unlock()
			if strings.HasSuffix(r.URL.Path, "/version") {
				if finalError {
					http.Error(w, "final snapshot unavailable", http.StatusForbidden)
				} else {
					_ = json.NewEncoder(w).Encode(v)
				}
			} else {
				_ = json.NewEncoder(w).Encode(map[string]any{"apiVersion": "config.openshift.io/v1", "kind": "ClusterVersionList", "metadata": map[string]string{"resourceVersion": v.ResourceVersion}, "items": []any{v}})
			}
			return true
		}
		select {
		case <-p.operatorsReady:
		case <-r.Context().Done():
			return true
		}
		o := configv1.ClusterOperator{TypeMeta: metav1.TypeMeta{APIVersion: "config.openshift.io/v1", Kind: "ClusterOperator"}, ObjectMeta: metav1.ObjectMeta{Name: "ingress", ResourceVersion: "10"}, Status: configv1.ClusterOperatorStatus{Conditions: []configv1.ClusterOperatorStatusCondition{{Type: configv1.OperatorAvailable, Status: configv1.ConditionTrue}, {Type: configv1.OperatorDegraded, Status: configv1.ConditionFalse}}}}
		if operatorVersion != "" {
			o.Status.Versions = []configv1.OperandVersion{{Name: "operator", Version: operatorVersion}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"apiVersion": "config.openshift.io/v1", "kind": "ClusterOperatorList", "metadata": map[string]string{"resourceVersion": "10"}, "items": []any{o}})
		return true
	})
	p.ctx, p.cancel = context.WithTimeout(context.Background(), 15*time.Second)
	p.cmd = exec.CommandContext(p.ctx, os.Args[0], "-test.run=^TestLiveCommandProcess$", "--", "observe-upgrade", p.dir, "--kubeconfig", config)
	p.cmd.Env = append(os.Environ(), "RECONCILE_LIVE_TEST_PROCESS=1")
	p.cmd.Stdout, p.cmd.Stderr = &p.out, &p.stderr
	if err := p.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		p.cancel()
		if p.cmd.ProcessState == nil {
			_ = p.cmd.Wait()
		}
	})
	p.waitCounts(1, 0)
	close(p.operatorsReady)
	p.waitCounts(1, 1)
	return p
}

func (p *observerProcess) runDir() string {
	paths, _ := filepath.Glob(filepath.Join(p.dir, "*", "run.json"))
	if len(paths) != 1 {
		return ""
	}
	return filepath.Dir(paths[0])
}

func (p *observerProcess) waitCounts(cv, co int) {
	p.t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		dir := p.runDir()
		v, ve := upgrade.ReadHistory(filepath.Join(dir, "cluster-version.jsonl"))
		o, oe := operator.ReadHistory(filepath.Join(dir, "operators", "ingress.jsonl"))
		if ve == nil && len(v) >= cv && (co == 0 || oe == nil && len(o) >= co) {
			return
		}
		select {
		case <-deadline.C:
			p.t.Fatalf("observations never reached CV=%d CO=%d", cv, co)
		case <-tick.C:
		}
	}
}

func (p *observerProcess) send(v configv1.ClusterVersion, count int) {
	p.t.Helper()
	select {
	case p.events <- v:
	case <-p.ctx.Done():
		p.t.Fatal("observer stopped receiving events")
	}
	p.waitCounts(count, 1)
}

func (p *observerProcess) finish(wantCode int, completed bool) {
	p.t.Helper()
	_ = p.cmd.Wait()
	if p.ctx.Err() != nil {
		p.t.Fatalf("observer hung: %s %s", &p.out, &p.stderr)
	}
	if got := p.cmd.ProcessState.ExitCode(); got != wantCode {
		p.t.Fatalf("exit=%d want=%d stdout=%s stderr=%s", got, wantCode, &p.out, &p.stderr)
	}
	m, err := recording.ReadRunManifest(p.runDir())
	if err != nil {
		p.t.Fatal(err)
	}
	wantStatus := recording.RunStatusStopped
	if wantCode == 1 {
		wantStatus = recording.RunStatusFailed
	}
	if m.Status != wantStatus || m.EndedAt == nil {
		p.t.Fatalf("manifest=%+v", m)
	}
	if wantCode == 1 {
		if m.Error == "" || !strings.Contains(p.stderr.String(), "final snapshot") {
			p.t.Fatalf("missing final error: %+v stderr=%s", m, &p.stderr)
		}
		if strings.Contains(p.out.String(), "Upgrade completion observed") || strings.Contains(p.out.String(), "Final snapshot: captured") {
			p.t.Fatal("final snapshot failure reported success")
		}
		return
	}
	var out, stderr bytes.Buffer
	if code := Run([]string{"verify-run", p.runDir()}, &out, &stderr); code != wantCode {
		p.t.Fatalf("verify-run exit=%d observer=%d: %s %s", code, wantCode, &out, &stderr)
	}
	v, err := upgrade.ReadHistory(filepath.Join(p.runDir(), "cluster-version.jsonl"))
	if err != nil {
		p.t.Fatal(err)
	}
	o, err := operator.ReadHistory(filepath.Join(p.runDir(), "operators", "ingress.jsonl"))
	if err != nil {
		p.t.Fatal(err)
	}
	if len(o) != 2 || o[0].Operator.ResourceVersion != o[1].Operator.ResourceVersion {
		p.t.Fatalf("unchanged-RV final operator missing: %+v", o)
	}
	if !o[1].ObservedAt.After(v[len(v)-2].ObservedAt) || !v[len(v)-1].ObservedAt.After(o[1].ObservedAt) {
		p.t.Fatal("final snapshot lacks closing CV bracket")
	}
	for _, want := range []string{fmt.Sprintf("Completion observed: %t", completed), "Final snapshot: captured", "Operators: 1", "Cluster ID: 11111111-1111-4111-8111-111111111111"} {
		if !strings.Contains(p.out.String(), want) {
			p.t.Fatalf("missing %q in stdout=%s", want, &p.out)
		}
	}
	if !completed && strings.Contains(p.out.String(), "Upgrade completion observed") {
		p.t.Fatal("manual stop claimed completion")
	}
	if m.Command != "observe-upgrade" {
		p.t.Fatalf("manifest command=%q", m.Command)
	}
}

func TestObserveUpgradeLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name, initial, operatorVersion string
		want                           int
	}{
		{"stable start with ambiguous operator sample", "stable", "4.20.0", 3},
		{"mid upgrade pass", "updating", "4.20.0", 0},
		{"version failure", "updating", "4.19.0", 2},
		{"missing version", "updating", "", 3},
		{"unknown initial", "unknown", "4.20.0", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			initialVersion, initialImage := "4.20.0", "image-a"
			if tc.initial != "updating" {
				initialVersion, initialImage = "4.19.0", "image-old"
			}
			p := startObserverProcess(t, observerVersion("10", tc.initial, initialVersion, initialImage), tc.operatorVersion, false)
			n := 1
			if tc.initial == "unknown" {
				n++
				p.send(observerVersion(fmt.Sprint(10+n), "stable", initialVersion, initialImage), n)
			}
			n++
			p.send(observerVersion(fmt.Sprint(10+n), "updating", "4.20.0", "image-a"), n)
			n++
			p.send(observerVersion(fmt.Sprint(10+n), "stable", "4.20.0", "image-a"), n)
			p.finish(tc.want, true)
			for _, want := range []string{"Observed target version: \"4.20.0\"", "Observed target image: \"image-a\""} {
				if !strings.Contains(p.out.String(), want) {
					t.Fatalf("missing %q: %s", want, &p.out)
				}
			}
			if tc.initial == "updating" {
				if !strings.Contains(p.out.String(), "pre-upgrade evidence may be incomplete") {
					t.Fatalf("missing mid-upgrade warning: %s", &p.out)
				}
			} else if !strings.Contains(p.out.String(), "Waiting for upgrade") {
				t.Fatalf("missing waiting message: %s", &p.out)
			}
		})
	}
}

func TestObserveUpgradeManualStop(t *testing.T) {
	for _, tc := range []struct {
		name   string
		signal os.Signal
		fail   bool
		want   int
	}{{"interrupt", os.Interrupt, false, 3}, {"terminate", syscall.SIGTERM, false, 3}, {"final failure", os.Interrupt, true, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			p := startObserverProcess(t, observerVersion("10", "stable", "4.20.0", "image-a"), "4.20.0", tc.fail)
			if err := p.cmd.Process.Signal(tc.signal); err != nil {
				t.Fatal(err)
			}
			p.finish(tc.want, false)
		})
	}
}

func TestObserveUpgradeTargetChange(t *testing.T) {
	for _, target := range []struct{ version, image string }{{"4.21.0", "image-b"}, {"4.20.0", "image-b"}} {
		t.Run(target.version+target.image, func(t *testing.T) {
			p := startObserverProcess(t, observerVersion("10", "updating", "4.20.0", "image-a"), target.version, false)
			p.send(observerVersion("11", "stable", target.version, target.image), 2)

			p.send(observerVersion("12", "updating", target.version, target.image), 3)
			p.send(observerVersion("13", "stable", target.version, target.image), 4)
			p.finish(3, true)
		})
	}
}

func TestObserveUpgradeWatchAuthorization(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			p := startObserverProcess(t, observerVersion("10", "updating", "4.20.0", "image-a"), "4.20.0", false)
			p.watchStatus.Store(int32(status))
			select {
			case p.events <- configv1.ClusterVersion{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "restart"}}:
			case <-p.ctx.Done():
				t.Fatal("watch unavailable")
			}
			if status == http.StatusUnauthorized {
				p.send(observerVersion("11", "updating", "4.20.0", "image-a"), 2)
				p.send(observerVersion("12", "stable", "4.20.0", "image-a"), 3)
				p.finish(0, true)
			} else {
				_ = p.cmd.Wait()
				if p.ctx.Err() != nil || p.cmd.ProcessState.ExitCode() != 1 {
					t.Fatalf("forbidden exit=%v stdout=%s stderr=%s", p.cmd.ProcessState, &p.out, &p.stderr)
				}
				m, err := recording.ReadRunManifest(p.runDir())
				if err != nil {
					t.Fatal(err)
				}
				if m.Status != recording.RunStatusFailed || !strings.Contains(strings.ToLower(m.Error), "forbidden") {
					t.Fatalf("manifest=%+v", m)
				}
				if strings.Contains(p.out.String(), "Final snapshot: captured") {
					t.Fatal("fatal watch error reported success")
				}
			}
		})
	}
}

func TestObserveUpgradeUsage(t *testing.T) {
	for _, tc := range []struct {
		args     []string
		code     int
		fragment string
	}{
		{[]string{"help"}, 0, "observe-upgrade <runs-directory>"},
		{[]string{"observe-upgrade"}, 1, "Error:"},
		{[]string{"observe-upgrade", "runs", "--output", "json"}, 1, "Error:"},
	} {
		var out, stderr bytes.Buffer
		code := Run(tc.args, &out, &stderr)
		if code != tc.code || !strings.Contains(out.String()+stderr.String(), tc.fragment) {
			t.Fatalf("args=%v exit=%d stdout=%s stderr=%s", tc.args, code, &out, &stderr)
		}
	}
}

func TestObserveUpgradeCompletionFinalSnapshotFailure(t *testing.T) {
	p := startObserverProcess(t, observerVersion("10", "updating", "4.20.0", "image-a"), "4.20.0", true)
	p.send(observerVersion("11", "stable", "4.20.0", "image-a"), 2)
	p.finish(1, true)
}

func TestObserveUpgradeManualStopFinalSnapshotCannotClaimCompletion(t *testing.T) {
	p := startObserverProcess(t, observerVersion("10", "updating", "4.20.0", "image-a"), "4.20.0", false)
	p.mu.Lock()
	p.current = observerVersion("11", "stable", "4.20.0", "image-a")
	p.mu.Unlock()
	if err := p.cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	p.finish(3, false)
	history, err := upgrade.ReadHistory(filepath.Join(p.runDir(), "cluster-version.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	states, err := upgrade.AnalyzePhases(history)
	if err != nil {
		t.Fatal(err)
	}
	if states[1].Phase != upgrade.UpgradePhaseCompleted {
		t.Fatalf("final snapshot did not exercise completion: %+v", states)
	}
}
