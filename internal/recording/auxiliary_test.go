package recording

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/machineconfig"
	nodehistory "github.com/nightingale-develop/reconcile-guard/internal/node"
	machineconfigv1 "github.com/openshift/api/machineconfiguration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAuxiliaryRunRecording(t *testing.T) {
	start := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	run, err := StartRun(t.TempDir(), "test", "server", start)
	if err != nil {
		t.Fatal(err)
	}
	recorder, err := NewRunRecorder(run.Directory())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		directory string
		append    func(string, time.Time) error
		inventory func(RunSnapshot) []string
	}{
		{"machine-config-pools", func(name string, at time.Time) error {
			return recorder.AppendMachineConfigPool(machineconfig.Observation{ObservedAt: at, Pool: machineconfigv1.MachineConfigPool{
				TypeMeta:   metav1.TypeMeta{APIVersion: "machineconfiguration.openshift.io/v1", Kind: "MachineConfigPool"},
				ObjectMeta: metav1.ObjectMeta{Name: name}, Status: machineconfigv1.MachineConfigPoolStatus{MachineCount: 3},
			}})
		}, func(s RunSnapshot) []string { return s.MachineConfigPools }},
		{"nodes", func(name string, at time.Time) error {
			return recorder.AppendNode(nodehistory.Observation{ObservedAt: at, Node: corev1.Node{
				TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Node"},
				ObjectMeta: metav1.ObjectMeta{Name: name, Annotations: map[string]string{nodehistory.CurrentMachineConfigAnnotation: "rendered-test"}},
			}})
		}, func(s RunSnapshot) []string { return s.Nodes }},
	} {
		t.Run(tc.directory, func(t *testing.T) {
			for i, name := range []string{"zeta", "alpha", "alpha"} {
				if err := tc.append(name, start.Add(time.Duration(i)*time.Second)); err != nil {
					t.Fatal(err)
				}
			}
			inventory := tc.inventory(recorder.Snapshot())
			if !slices.Equal(inventory, []string{"alpha", "zeta"}) {
				t.Fatalf("inventory=%v", inventory)
			}
			inventory[0] = "tampered"
			before := recorder.Snapshot()
			if tc.inventory(before)[0] != "alpha" {
				t.Fatal("snapshot aliases inventory")
			}
			for _, name := range []string{"", ".", "..", "../escape"} {
				if err := tc.append(name, start); err == nil {
					t.Fatalf("accepted invalid name %q", name)
				}
			}
			if err := os.Mkdir(filepath.Join(run.Directory(), tc.directory, "blocked.jsonl"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := tc.append("blocked", start); err == nil {
				t.Fatal("lost write error")
			}
			if !reflect.DeepEqual(before, recorder.Snapshot()) {
				t.Fatal("failed writes changed inventory")
			}
		})
	}
	pools, err := machineconfig.ReadHistory(filepath.Join(run.Directory(), "machine-config-pools", "alpha.jsonl"))
	if err != nil || len(pools) != 2 || pools[0].Pool.Status.MachineCount != 3 {
		t.Fatalf("MCP roundtrip=%+v err=%v", pools, err)
	}
	nodes, err := nodehistory.ReadHistory(filepath.Join(run.Directory(), "nodes", "alpha.jsonl"))
	if err != nil || len(nodes) != 2 || nodehistory.CurrentMachineConfig(nodes[0].Node) != "rendered-test" {
		t.Fatalf("Node roundtrip=%+v err=%v", nodes, err)
	}
	if err := run.Finish(RunStatusStopped, start.Add(time.Minute), recorder.Snapshot(), nil); err != nil {
		t.Fatal(err)
	}
	manifest, err := ReadRunManifest(run.Directory())
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != "1" || !slices.Equal(manifest.MachineConfigPools, []string{"alpha", "zeta"}) || !slices.Equal(manifest.Nodes, []string{"alpha", "zeta"}) {
		t.Fatalf("manifest=%+v", manifest)
	}
	copy := run.Manifest()
	copy.MachineConfigPools[0], copy.Nodes[0] = "tampered", "tampered"
	if run.Manifest().MachineConfigPools[0] != "alpha" || run.Manifest().Nodes[0] != "alpha" {
		t.Fatal("Manifest aliases auxiliary inventories")
	}
}

func TestLegacyRunManifestWithoutAuxiliaryFields(t *testing.T) {
	directory := t.TempDir()
	data := `{"schemaVersion":"1","runId":"legacy","status":"stopped","startedAt":"2026-09-30T10:00:00Z","endedAt":"2026-09-30T11:00:00Z","toolVersion":"legacy","command":"record-live","source":{"server":"test"},"files":{"clusterVersion":"cluster-version.jsonl","operatorsDirectory":"operators"},"operators":["ingress"]}`
	if err := os.WriteFile(filepath.Join(directory, "run.json"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	manifest, err := ReadRunManifest(directory)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"machineConfigPoolsDirectory", "nodesDirectory", "machineConfigPools", "nodes"} {
		if strings.Contains(string(encoded), `"`+field+`"`) {
			t.Fatalf("legacy manifest acquired optional field %q", field)
		}
	}
}
