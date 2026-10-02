package policy

import (
	"testing"
	"time"
)

func TestParsePolicy(t *testing.T) {
	input := []byte(`
apiVersion: reconcileguard.io/v1alpha1
kind: UpgradePolicy
targetVersion: 4.22.0
source: test-policy
maxObservationGap: 2m
defaults:
  availabilityLoss:
    maxObservedDuration: 1m
  machineConfigPool:
    postCompletionGracePeriod: 10m
  node:
    readyPostCompletionGracePeriod: 5m
operators:
  ingress:
    availabilityLoss:
      maxObservedDuration: 30s
nodes:
  worker-0:
    configAlignedPostCompletionGracePeriod: 15m
`)
	p, err := Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	if p.MaxObservationGap.Duration != 2*time.Minute {
		t.Fatalf("gap=%s", p.MaxObservationGap.Duration)
	}
	if got := p.OperatorRulesFor("ingress").AvailabilityLoss.MaxObservedDuration.Duration; got != 30*time.Second {
		t.Fatalf("override=%s", got)
	}
	if got := p.OperatorRulesFor("network").AvailabilityLoss.MaxObservedDuration.Duration; got != time.Minute {
		t.Fatalf("default=%s", got)
	}
	node, ok := p.NodeRuleFor("worker-0")
	if !ok || node.ReadyPostCompletionGracePeriod.Duration != 5*time.Minute || node.ConfigAlignedPostCompletionGracePeriod.Duration != 15*time.Minute {
		t.Fatalf("node rule=%+v ok=%v", node, ok)
	}
}

func TestParseRejectsExplicitZeroNodeOverride(t *testing.T) {
	_, err := Parse([]byte(`apiVersion: reconcileguard.io/v1alpha1
kind: UpgradePolicy
targetVersion: 4.20.0
source: test
defaults:
  node:
    readyPostCompletionGracePeriod: 5m
nodes:
  worker-0:
    readyPostCompletionGracePeriod: 0s
    configAlignedPostCompletionGracePeriod: 1m
`))
	if err == nil {
		t.Fatal("explicit zero override silently inherited default")
	}
}

func TestParsePolicyRejectsUnknownAndMissingRules(t *testing.T) {
	for _, input := range []string{
		`apiVersion: reconcileguard.io/v1alpha1
kind: UpgradePolicy
targetVersion: 4.22.0
source: test
unknown: true
`,
		`apiVersion: reconcileguard.io/v1alpha1
kind: UpgradePolicy
targetVersion: 4.22.0
source: test
`,
		`apiVersion: reconcileguard.io/v1alpha1
kind: UpgradePolicy
targetVersion: 4.22.0
source: test
defaults:
  degraded:
    maxObservedDuration: 1m
`,
	} {
		if _, err := Parse([]byte(input)); err == nil {
			t.Fatalf("expected error for %q", input)
		}
	}
}

func TestParsePolicyRejectsInvalidDurations(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"malformed gap", `{"apiVersion":"reconcileguard.io/v1alpha1","kind":"UpgradePolicy","targetVersion":"4.22.0","source":"test","maxObservationGap":"not-a-duration","defaults":{"degraded":{"maxObservedDuration":"1m"}}}`},
		{"zero rule", `{"apiVersion":"reconcileguard.io/v1alpha1","kind":"UpgradePolicy","targetVersion":"4.22.0","source":"test","maxObservationGap":"1m","defaults":{"degraded":{"maxObservedDuration":"0s"}}}`},
		{"negative rule", `{"apiVersion":"reconcileguard.io/v1alpha1","kind":"UpgradePolicy","targetVersion":"4.22.0","source":"test","maxObservationGap":"1m","defaults":{"degraded":{"maxObservedDuration":"-1s"}}}`},
		{"negative pool grace", `{"apiVersion":"reconcileguard.io/v1alpha1","kind":"UpgradePolicy","targetVersion":"4.22.0","source":"test","defaults":{"machineConfigPool":{"postCompletionGracePeriod":"-1s"}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse([]byte(tc.body)); err == nil {
				t.Fatal("expected invalid duration error")
			}
		})
	}
}
