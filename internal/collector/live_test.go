package collector

import (
	"context"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
)

func TestLiveCollectorCapture(t *testing.T) {
	scheme := runtime.NewScheme()

	if err := configv1.Install(scheme); err != nil {
		t.Fatal(err)
	}

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

	client := fake.NewSimpleDynamicClient(
		scheme,
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
