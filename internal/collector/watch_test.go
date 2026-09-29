package collector

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"

	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
)

type memorySink struct {
	mu        sync.Mutex
	versions  []upgrade.ClusterVersionObservation
	operators []operator.Observation
}

func (s *memorySink) AppendClusterVersion(
	observation upgrade.ClusterVersionObservation,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.versions = append(
		s.versions,
		observation,
	)

	return nil
}

func (s *memorySink) AppendOperator(
	observation operator.Observation,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.operators = append(
		s.operators,
		observation,
	)

	return nil
}

func (s *memorySink) counts() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.versions), len(s.operators)
}

func TestLiveRecorderRecordsInitialStateAndUpdate(
	t *testing.T,
) {
	scheme := runtime.NewScheme()

	if err := configv1.Install(scheme); err != nil {
		t.Fatal(err)
	}

	version := &configv1.ClusterVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "version",
			ResourceVersion: "1",
		},
	}

	version.TypeMeta = metav1.TypeMeta{
		APIVersion: "config.openshift.io/v1",
		Kind:       "ClusterVersion",
	}

	version.Status.Desired.Version = "4.20.0"

	ingress := &configv1.ClusterOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "ingress",
			ResourceVersion: "1",
		},
	}

	ingress.TypeMeta = metav1.TypeMeta{
		APIVersion: "config.openshift.io/v1",
		Kind:       "ClusterOperator",
	}

	client := fake.NewSimpleDynamicClient(
		scheme,
		version,
		ingress,
	)

	sink := &memorySink{}

	recorder := newLiveRecorder(
		client,
		sink,
		time.Now,
	)

	ctx, cancel :=
		context.WithCancel(context.Background())
	t.Cleanup(cancel)

	done := make(chan error, 1)

	go func() {
		done <- recorder.Run(ctx)
	}()

	waitForCounts(
		t,
		sink,
		1,
		1,
	)

	updated := ingress.DeepCopy()
	updated.ResourceVersion = "2"

	updated.Status.Conditions =
		[]configv1.ClusterOperatorStatusCondition{
			{
				Type:   configv1.OperatorProgressing,
				Status: configv1.ConditionTrue,
			},
		}

	data, err :=
		runtime.DefaultUnstructuredConverter.
			ToUnstructured(updated)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.
		Resource(clusterOperatorResource).
		Update(
			context.Background(),
			&unstructured.Unstructured{
				Object: data,
			},
			metav1.UpdateOptions{},
		)
	if err != nil {
		t.Fatal(err)
	}

	waitForCounts(
		t,
		sink,
		1,
		2,
	)

	sink.mu.Lock()
	observations := append([]operator.Observation(nil), sink.operators...)
	sink.mu.Unlock()
	last := observations[len(observations)-1]
	if last.Operator.ResourceVersion != "2" ||
		len(last.Operator.Status.Conditions) != 1 ||
		last.Operator.Status.Conditions[0].Status != configv1.ConditionTrue {
		t.Fatalf("updated observation was not recorded: %+v", last.Operator)
	}
	if !last.ObservedAt.After(observations[0].ObservedAt) {
		t.Fatal("updated observation timestamp must follow initial observation")
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}

	case <-time.After(5 * time.Second):
		t.Fatal("recorder did not stop")
	}
}

func TestSameResourceVersion(t *testing.T) {
	for _, tc := range []struct {
		name       string
		oldVersion string
		newVersion string
		want       bool
	}{
		{"unchanged", "1", "1", true},
		{"updated", "1", "2", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldObject := &unstructured.Unstructured{}
			oldObject.SetResourceVersion(tc.oldVersion)
			newObject := &unstructured.Unstructured{}
			newObject.SetResourceVersion(tc.newVersion)
			if got := sameResourceVersion(oldObject, newObject); got != tc.want {
				t.Fatalf("sameResourceVersion = %v, want %v", got, tc.want)
			}
		})
	}
}

func waitForCounts(
	t *testing.T,
	sink *memorySink,
	versions int,
	operators int,
) {
	t.Helper()

	deadline :=
		time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		gotVersions, gotOperators :=
			sink.counts()

		if gotVersions >= versions &&
			gotOperators >= operators {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	gotVersions, gotOperators :=
		sink.counts()

	t.Fatalf(
		"counts = (%d, %d), want at least (%d, %d)",
		gotVersions,
		gotOperators,
		versions,
		operators,
	)
}

func TestLiveRecorderPreservesUpgradeFields(t *testing.T) {
	versions, err := upgrade.ReadHistory("../../testdata/upgrade/cluster-version.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	operators, err := operator.ReadHistory("../../testdata/upgrade/ingress.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	object := func(value any) *unstructured.Unstructured {
		t.Helper()
		data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(value)
		if err != nil {
			t.Fatal(err)
		}
		return &unstructured.Unstructured{Object: data}
	}
	client := liveClient(t, object(&versions[0].ClusterVersion), object(&operators[0].Operator))
	vwatches, owatches := watchStream(client, "clusterversions"), watchStream(client, "clusteroperators")
	sink := &memorySink{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	recorder := newLiveRecorder(client, sink, func() time.Time { return now })
	done := make(chan error, 1)
	go func() { done <- recorder.Run(ctx) }()
	vw, ow := nextWatch(t, vwatches), nextWatch(t, owatches)
	waitForCounts(t, sink, 1, 1)
	for i := 1; i < len(versions); i++ {
		vw.Modify(object(&versions[i].ClusterVersion))
		waitForCounts(t, sink, i+1, 1)
	}
	for i := 1; i < len(operators); i++ {
		ow.Modify(object(&operators[i].Operator))
		waitForCounts(t, sink, len(versions), i+1)
	}
	cancel()
	if err := recordingResult(t, done); err != nil {
		t.Fatal(err)
	}
	if len(sink.versions) != len(versions) || len(sink.operators) != len(operators) {
		t.Fatal("lost or duplicated observations")
	}
	for i, want := range versions {
		got := sink.versions[i]
		if !reflect.DeepEqual(got.ClusterVersion, want.ClusterVersion) {
			t.Fatalf("ClusterVersion fields changed at %d", i)
		}
		if !got.ObservedAt.Equal(now.Add(time.Duration(i)*time.Nanosecond)) || got.ObservedAt.Location() != time.UTC {
			t.Fatalf("wrong capture timestamp: %v", got.ObservedAt)
		}
	}
	for i, want := range operators {
		got := sink.operators[i]
		if !reflect.DeepEqual(got.Operator, want.Operator) {
			t.Fatalf("ClusterOperator fields changed at %d", i)
		}
		if !got.ObservedAt.Equal(now.Add(time.Duration(i)*time.Nanosecond)) || got.ObservedAt.Location() != time.UTC {
			t.Fatalf("wrong operator capture timestamp: %v", got.ObservedAt)
		}
	}
}
