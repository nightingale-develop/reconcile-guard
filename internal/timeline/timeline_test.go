package timeline

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/machineconfig"
	nodehistory "github.com/nightingale-develop/reconcile-guard/internal/node"
	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
	configv1 "github.com/openshift/api/config/v1"
	machineconfigv1 "github.com/openshift/api/machineconfiguration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildShowsImageOnlyTargetChange(t *testing.T) {
	base := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	first := versionObservation(base, "4.20.0", configv1.ConditionTrue, configv1.PartialUpdate, false)
	second := versionObservation(base.Add(time.Minute), "4.20.0", configv1.ConditionTrue, configv1.PartialUpdate, false)
	second.ClusterVersion.Status.Desired.Image = "image:replacement"
	second.ClusterVersion.Status.History[0].Image = "image:replacement"
	report, err := Build([]upgrade.ClusterVersionObservation{first, second}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Events) != 2 || !strings.Contains(report.Events[1].Summary, "image:replacement") || report.Events[0].Summary == report.Events[1].Summary {
		t.Fatalf("image-only target change hidden: %+v", report.Events)
	}
}

func TestBuildDeterministicTiesAndUnchangedSnapshots(t *testing.T) {
	base := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	versions := []upgrade.ClusterVersionObservation{versionObservation(base, "4.20.0", configv1.ConditionFalse, configv1.CompletedUpdate, true)}
	duplicate := versions[0]
	duplicate.ObservedAt = base.Add(time.Hour)
	versions = append(versions, duplicate)
	histories := [][]operator.Observation{}
	for _, name := range []string{"network", "ingress"} {
		histories = append(histories, []operator.Observation{
			operatorObservation(base, name, configv1.ConditionTrue),
			operatorObservation(base.Add(time.Minute), name, configv1.ConditionTrue),
			operatorObservation(base.Add(time.Hour), name, configv1.ConditionFalse),
		})
	}
	first, err := Build(versions, histories, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	histories[0], histories[1] = histories[1], histories[0]
	second, err := Build(versions, histories, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("ordering depends on input history order")
	}
	if len(first.Events) != 3 {
		t.Fatalf("unchanged snapshots generated events: %+v", first)
	}
	for i, name := range []string{"ingress", "network"} {
		event := first.Events[i+1]
		if event.ResourceName != name || event.ObservedAt != nil || event.From == nil || event.To == nil || !event.From.Equal(base.Add(time.Minute)) || !event.To.Equal(base.Add(time.Hour)) {
			t.Fatalf("lost ordering or wide observation bounds: %+v", event)
		}
	}
}

func TestBuildRejectsInvalidHistory(t *testing.T) {
	base := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	version := versionObservation(base, "4.20.0", configv1.ConditionFalse, configv1.CompletedUpdate, true)
	for _, versions := range [][]upgrade.ClusterVersionObservation{nil, {version, version}} {
		if _, err := Build(versions, nil, nil, nil); err == nil {
			t.Fatal("invalid ClusterVersion history accepted")
		}
	}
	observation := operatorObservation(base, "ingress", configv1.ConditionTrue)
	if _, err := Build([]upgrade.ClusterVersionObservation{version}, [][]operator.Observation{{observation, observation}}, nil, nil); err == nil {
		t.Fatal("duplicate operator timestamps accepted")
	}
}

func TestBuildOrdersLifecycleEventsWithoutInventingTransitionTimes(t *testing.T) {
	base := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	versions := []upgrade.ClusterVersionObservation{
		versionObservation(base, "4.20.0", configv1.ConditionTrue, configv1.PartialUpdate, false),
		versionObservation(base.Add(10*time.Minute), "4.20.0", configv1.ConditionFalse, configv1.CompletedUpdate, true),
	}
	operators := [][]operator.Observation{{
		operatorObservation(base.Add(time.Minute), "ingress", configv1.ConditionTrue),
		operatorObservation(base.Add(2*time.Minute), "ingress", configv1.ConditionFalse),
	}}
	pools := [][]machineconfig.Observation{{
		poolObservation(base.Add(3*time.Minute), machineconfigv1.MachineConfigPoolUpdating, corev1.ConditionTrue, "rendered-old", "rendered-new"),
		poolObservation(base.Add(8*time.Minute), machineconfigv1.MachineConfigPoolUpdated, corev1.ConditionTrue, "rendered-new", "rendered-new"),
	}}
	nodes := [][]nodehistory.Observation{{
		nodeObservation(base.Add(4*time.Minute), corev1.ConditionFalse, "rendered-old", "rendered-new"),
		nodeObservation(base.Add(9*time.Minute), corev1.ConditionTrue, "rendered-new", "rendered-new"),
	}}

	report, err := Build(versions, operators, pools, nodes)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Events) != 5 {
		t.Fatalf("events=%d want=5: %+v", len(report.Events), report.Events)
	}
	if report.Events[0].Kind != "cluster-version-state" || report.Events[0].ObservedAt == nil || !report.Events[0].ObservedAt.Equal(base) {
		t.Fatalf("first event=%+v", report.Events[0])
	}
	operatorEvent := report.Events[1]
	if operatorEvent.Kind != "operator-condition-transition" || operatorEvent.ObservedAt != nil || operatorEvent.From == nil || operatorEvent.To == nil {
		t.Fatalf("operator event=%+v", operatorEvent)
	}
	if !operatorEvent.From.Equal(base.Add(time.Minute)) || !operatorEvent.To.Equal(base.Add(2*time.Minute)) {
		t.Fatalf("operator interval=[%v,%v]", operatorEvent.From, operatorEvent.To)
	}
	if report.Events[len(report.Events)-1].Kind != "cluster-version-state" || report.Events[len(report.Events)-1].Attributes["phase"] != string(upgrade.UpgradePhaseCompleted) {
		t.Fatalf("last event=%+v", report.Events[len(report.Events)-1])
	}
}

func versionObservation(at time.Time, version string, progressing configv1.ConditionStatus, state configv1.UpdateState, completed bool) upgrade.ClusterVersionObservation {
	started := metav1.NewTime(at.Add(-time.Minute))
	history := configv1.UpdateHistory{Version: version, Image: "image:" + version, State: state, StartedTime: started}
	if completed {
		completion := metav1.NewTime(at.Add(-time.Second))
		history.CompletionTime = &completion
	}
	return upgrade.ClusterVersionObservation{
		ObservedAt: at,
		ClusterVersion: configv1.ClusterVersion{
			TypeMeta:   metav1.TypeMeta{APIVersion: "config.openshift.io/v1", Kind: "ClusterVersion"},
			ObjectMeta: metav1.ObjectMeta{Name: "version", Generation: 1},
			Status: configv1.ClusterVersionStatus{
				ObservedGeneration: 1,
				Desired:            configv1.Release{Version: version, Image: "image:" + version},
				Conditions: []configv1.ClusterOperatorStatusCondition{
					{Type: configv1.OperatorProgressing, Status: progressing},
					{Type: configv1.OperatorAvailable, Status: configv1.ConditionTrue},
				},
				History: []configv1.UpdateHistory{history},
			},
		},
	}
}

func operatorObservation(at time.Time, name string, available configv1.ConditionStatus) operator.Observation {
	return operator.Observation{
		ObservedAt: at,
		Operator: configv1.ClusterOperator{
			TypeMeta:   metav1.TypeMeta{APIVersion: "config.openshift.io/v1", Kind: "ClusterOperator"},
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Status: configv1.ClusterOperatorStatus{Conditions: []configv1.ClusterOperatorStatusCondition{
				{Type: configv1.OperatorAvailable, Status: available},
				{Type: configv1.OperatorProgressing, Status: configv1.ConditionFalse},
				{Type: configv1.OperatorDegraded, Status: configv1.ConditionFalse},
			}},
		},
	}
}

func poolObservation(at time.Time, conditionType machineconfigv1.MachineConfigPoolConditionType, status corev1.ConditionStatus, current, desired string) machineconfig.Observation {
	conditions := []machineconfigv1.MachineConfigPoolCondition{
		{Type: machineconfigv1.MachineConfigPoolUpdated, Status: corev1.ConditionFalse},
		{Type: machineconfigv1.MachineConfigPoolUpdating, Status: corev1.ConditionFalse},
		{Type: machineconfigv1.MachineConfigPoolDegraded, Status: corev1.ConditionFalse},
	}
	for i := range conditions {
		if conditions[i].Type == conditionType {
			conditions[i].Status = status
		}
	}
	updated, ready := int32(0), int32(0)
	if current == desired && conditionType == machineconfigv1.MachineConfigPoolUpdated && status == corev1.ConditionTrue {
		updated, ready = 1, 1
	}
	pool := machineconfigv1.MachineConfigPool{
		TypeMeta:   metav1.TypeMeta{APIVersion: "machineconfiguration.openshift.io/v1", Kind: "MachineConfigPool"},
		ObjectMeta: metav1.ObjectMeta{Name: "master", Generation: 1},
		Status: machineconfigv1.MachineConfigPoolStatus{
			ObservedGeneration:  1,
			MachineCount:        1,
			UpdatedMachineCount: updated,
			ReadyMachineCount:   ready,
			Conditions:          conditions,
		},
	}
	pool.Spec.Configuration.Name = desired
	pool.Status.Configuration.Name = current
	return machineconfig.Observation{ObservedAt: at, Pool: pool}
}

func nodeObservation(at time.Time, ready corev1.ConditionStatus, current, desired string) nodehistory.Observation {
	return nodehistory.Observation{
		ObservedAt: at,
		Node: corev1.Node{
			TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Node"},
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-0",
				Annotations: map[string]string{
					nodehistory.CurrentMachineConfigAnnotation: current,
					nodehistory.DesiredMachineConfigAnnotation: desired,
				},
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: ready}},
				NodeInfo:   corev1.NodeSystemInfo{KubeletVersion: "v1.34.4"},
			},
		},
	}
}
