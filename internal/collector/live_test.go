package collector

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	machineconfigv1 "github.com/openshift/api/machineconfiguration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clienttesting "k8s.io/client-go/testing"
)

func TestLiveCollectorCapture(t *testing.T) {
	version := &configv1.ClusterVersion{}
	version.APIVersion = "config.openshift.io/v1"
	version.Kind = "ClusterVersion"
	version.Name = "version"
	version.Status.Desired.Version = "4.20.0"

	ingress := &configv1.ClusterOperator{}
	ingress.APIVersion = "config.openshift.io/v1"
	ingress.Kind = "ClusterOperator"
	ingress.Name = "ingress"

	network := &configv1.ClusterOperator{}
	network.APIVersion = "config.openshift.io/v1"
	network.Kind = "ClusterOperator"
	network.Name = "network"

	client := liveClient(
		t,
		version,
		network,
		ingress,
	)

	times := []time.Time{
		time.Date(
			2026,
			time.September,
			28,
			10,
			0,
			0,
			0,
			time.UTC,
		),
		time.Date(
			2026,
			time.September,
			28,
			10,
			0,
			1,
			0,
			time.UTC,
		),
	}

	times = append(times, times[1].Add(time.Second), times[1].Add(2*time.Second))
	index := 0

	collector := newLiveCollector(
		client,
		func() time.Time {
			value := times[index]
			index++
			return value
		},
	)

	capture, err := collector.Capture(
		context.Background(),
	)
	if err != nil {
		t.Fatal(err)
	}

	if capture.ClusterVersion.ClusterVersion.Name !=
		"version" {
		t.Fatalf(
			"ClusterVersion = %q",
			capture.ClusterVersion.ClusterVersion.Name,
		)
	}

	if capture.ClusterVersion.ObservedAt != times[0] {
		t.Fatalf(
			"ClusterVersion observedAt = %s",
			capture.ClusterVersion.ObservedAt,
		)
	}

	if len(capture.Operators) != 2 {
		t.Fatalf(
			"operators = %d, want 2",
			len(capture.Operators),
		)
	}

	if capture.Operators[0].Operator.Name !=
		"ingress" {
		t.Fatalf(
			"operator 0 = %q",
			capture.Operators[0].Operator.Name,
		)
	}

	if capture.Operators[1].Operator.Name !=
		"network" {
		t.Fatalf(
			"operator 1 = %q",
			capture.Operators[1].Operator.Name,
		)
	}

	for _, observation := range capture.Operators {
		if observation.ObservedAt != times[1] {
			t.Fatalf(
				"operator observedAt = %s",
				observation.ObservedAt,
			)
		}
	}
}

func poolNodeObjects() (*machineconfigv1.MachineConfigPool, *corev1.Node) {
	pool := &machineconfigv1.MachineConfigPool{TypeMeta: metav1.TypeMeta{APIVersion: "machineconfiguration.openshift.io/v1", Kind: "MachineConfigPool"}, ObjectMeta: metav1.ObjectMeta{Name: "worker", ResourceVersion: "7", Generation: 2}, Status: machineconfigv1.MachineConfigPoolStatus{ObservedGeneration: 2, MachineCount: 3, UpdatedMachineCount: 2, ReadyMachineCount: 2, UnavailableMachineCount: 1, DegradedMachineCount: 1}}
	pool.Spec.Configuration.Name = "rendered-worker-new"
	pool.Status.Configuration.Name = "rendered-worker-old"
	pool.Status.Conditions = []machineconfigv1.MachineConfigPoolCondition{{Type: machineconfigv1.MachineConfigPoolUpdating, Status: corev1.ConditionTrue, Reason: "RollingOut", Message: "working"}}
	node := &corev1.Node{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Node"}, ObjectMeta: metav1.ObjectMeta{Name: "worker-b", ResourceVersion: "9", Labels: map[string]string{"node-role.kubernetes.io/worker": ""}, Annotations: map[string]string{"machineconfiguration.openshift.io/currentConfig": "old", "machineconfiguration.openshift.io/desiredConfig": "new", "machineconfiguration.openshift.io/state": "Working"}}, Spec: corev1.NodeSpec{Unschedulable: true}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionFalse, Reason: "Updating", Message: "rebooting"}}}}
	return pool, node
}

func TestCaptureMachineConfigPoolsAndNodes(t *testing.T) {
	pool, node := poolNodeObjects()
	otherPool := pool.DeepCopy()
	otherPool.Name = "master"
	otherNode := node.DeepCopy()
	otherNode.Name = "worker-a"
	client := liveClient(t, liveObject("ClusterVersion", "version", "1"), pool, node, otherPool, otherNode)
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.FixedZone("local", 3600))
	calls := 0
	capture, err := newLiveCollector(client, func() time.Time { v := base.Add(time.Duration(calls) * time.Second); calls++; return v }).Capture(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(capture.MachineConfigPools) != 2 || len(capture.Nodes) != 2 {
		t.Fatalf("capture=%+v", capture)
	}
	for i, want := range []*machineconfigv1.MachineConfigPool{otherPool, pool} {
		got := capture.MachineConfigPools[i]
		if !reflect.DeepEqual(got.Pool, *want) || !got.ObservedAt.Equal(base.Add(2*time.Second)) || got.ObservedAt.Location() != time.UTC {
			t.Fatalf("pool=%+v want=%+v", got, want)
		}
	}
	for i, want := range []*corev1.Node{otherNode, node} {
		got := capture.Nodes[i]
		if !reflect.DeepEqual(got.Node, *want) || !got.ObservedAt.Equal(base.Add(3*time.Second)) || got.ObservedAt.Location() != time.UTC {
			t.Fatalf("node=%+v want=%+v", got, want)
		}
	}
	for _, a := range client.Actions() {
		if a.GetVerb() != "get" && a.GetVerb() != "list" {
			t.Fatalf("mutation: %v", a)
		}
	}
}

func TestCaptureAdditionalResourcesEmptyAndErrors(t *testing.T) {
	for _, resource := range []string{"machineconfigpools", "nodes"} {
		for _, mode := range []string{"empty", "list error", "decode error"} {
			t.Run(resource+"/"+mode, func(t *testing.T) {
				client := liveClient(t, liveObject("ClusterVersion", "version", "1"))
				want := errors.New("API unavailable")
				if mode != "empty" {
					client.PrependReactor("list", resource, func(clienttesting.Action) (bool, runtime.Object, error) {
						if mode == "list error" {
							return true, nil, want
						}
						field := "machineCount"
						if resource == "nodes" {
							field = "conditions"
						}
						return true, &unstructured.UnstructuredList{Items: []unstructured.Unstructured{{Object: map[string]any{"metadata": map[string]any{"name": "broken"}, "status": map[string]any{field: "bad-type"}}}}}, nil
					})
				}
				c, err := newLiveCollector(client, time.Now).Capture(context.Background())
				switch mode {
				case "empty":
					if err != nil || len(c.MachineConfigPools) != 0 || len(c.Nodes) != 0 {
						t.Fatalf("capture=%+v error=%v", c, err)
					}
				case "list error":
					if !errors.Is(err, want) {
						t.Fatalf("error=%v", err)
					}
				case "decode error":
					if err == nil || !strings.Contains(err.Error(), "decode") || !strings.Contains(err.Error(), "broken") {
						t.Fatalf("error=%v", err)
					}
				}
			})
		}
	}
}
