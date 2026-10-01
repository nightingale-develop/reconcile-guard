package policy

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"sigs.k8s.io/yaml"
)

const (
	APIVersion = "reconcileguard.io/v1alpha1"
	Kind       = "UpgradePolicy"
)

type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		d.Duration = 0
		return nil
	}
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("duration must be a string: %w", err)
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", raw, err)
	}
	d.Duration = value
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) {
	if d.Duration == 0 {
		return []byte(`"0s"`), nil
	}
	return json.Marshal(d.Duration.String())
}

func (d Duration) Set() bool { return d.Duration != 0 }

type DurationRule struct {
	MaxObservedDuration Duration `json:"maxObservedDuration,omitempty" yaml:"maxObservedDuration,omitempty"`
}

type MachineConfigPoolRule struct {
	PostCompletionGracePeriod Duration `json:"postCompletionGracePeriod,omitempty" yaml:"postCompletionGracePeriod,omitempty"`
}

type NodeRule struct {
	ReadyPostCompletionGracePeriod         Duration `json:"readyPostCompletionGracePeriod,omitempty" yaml:"readyPostCompletionGracePeriod,omitempty"`
	ConfigAlignedPostCompletionGracePeriod Duration `json:"configAlignedPostCompletionGracePeriod,omitempty" yaml:"configAlignedPostCompletionGracePeriod,omitempty"`
}

type OperatorRules struct {
	AvailabilityLoss *DurationRule `json:"availabilityLoss,omitempty" yaml:"availabilityLoss,omitempty"`
	Degraded         *DurationRule `json:"degraded,omitempty" yaml:"degraded,omitempty"`
	Progressing      *DurationRule `json:"progressing,omitempty" yaml:"progressing,omitempty"`
}

type Defaults struct {
	AvailabilityLoss  *DurationRule          `json:"availabilityLoss,omitempty" yaml:"availabilityLoss,omitempty"`
	Degraded          *DurationRule          `json:"degraded,omitempty" yaml:"degraded,omitempty"`
	Progressing       *DurationRule          `json:"progressing,omitempty" yaml:"progressing,omitempty"`
	MachineConfigPool *MachineConfigPoolRule `json:"machineConfigPool,omitempty" yaml:"machineConfigPool,omitempty"`
	Node              *NodeRule              `json:"node,omitempty" yaml:"node,omitempty"`
}

func (d Defaults) OperatorRules() OperatorRules {
	return OperatorRules{
		AvailabilityLoss: d.AvailabilityLoss,
		Degraded:         d.Degraded,
		Progressing:      d.Progressing,
	}
}

type UpgradePolicy struct {
	APIVersion         string                           `json:"apiVersion" yaml:"apiVersion"`
	Kind               string                           `json:"kind" yaml:"kind"`
	TargetVersion      string                           `json:"targetVersion" yaml:"targetVersion"`
	TargetImage        string                           `json:"targetImage,omitempty" yaml:"targetImage,omitempty"`
	Source             string                           `json:"source" yaml:"source"`
	MaxObservationGap  Duration                         `json:"maxObservationGap,omitempty" yaml:"maxObservationGap,omitempty"`
	Defaults           Defaults                         `json:"defaults,omitempty" yaml:"defaults,omitempty"`
	Operators          map[string]OperatorRules         `json:"operators,omitempty" yaml:"operators,omitempty"`
	MachineConfigPools map[string]MachineConfigPoolRule `json:"machineConfigPools,omitempty" yaml:"machineConfigPools,omitempty"`
	Nodes              map[string]NodeRule              `json:"nodes,omitempty" yaml:"nodes,omitempty"`
}

func Parse(data []byte) (UpgradePolicy, error) {
	var result UpgradePolicy
	if err := yaml.UnmarshalStrict(data, &result); err != nil {
		return UpgradePolicy{}, fmt.Errorf("decode policy: %w", err)
	}
	if err := result.Validate(); err != nil {
		return UpgradePolicy{}, err
	}
	return result, nil
}

func (p UpgradePolicy) Validate() error {
	if p.APIVersion != APIVersion {
		return fmt.Errorf("policy apiVersion must be %q", APIVersion)
	}
	if p.Kind != Kind {
		return fmt.Errorf("policy kind must be %q", Kind)
	}
	if strings.TrimSpace(p.TargetVersion) == "" {
		return fmt.Errorf("policy targetVersion is required")
	}
	if strings.TrimSpace(p.Source) == "" {
		return fmt.Errorf("policy source is required")
	}
	if p.MaxObservationGap.Duration < 0 {
		return fmt.Errorf("policy maxObservationGap must be positive when set")
	}
	if err := validateOperatorRules("defaults", p.Defaults.OperatorRules()); err != nil {
		return err
	}
	if p.Defaults.MachineConfigPool != nil {
		if err := validatePoolRule("defaults.machineConfigPool", *p.Defaults.MachineConfigPool); err != nil {
			return err
		}
	}
	if p.Defaults.Node != nil {
		if err := validateNodeRule("defaults.node", *p.Defaults.Node); err != nil {
			return err
		}
	}
	for name, rules := range p.Operators {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("policy operator name must not be empty")
		}
		if err := validateOperatorRules("operators."+name, rules); err != nil {
			return err
		}
	}
	for name, rule := range p.MachineConfigPools {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("policy MachineConfigPool name must not be empty")
		}
		if err := validatePoolRule("machineConfigPools."+name, rule); err != nil {
			return err
		}
	}
	for name, rule := range p.Nodes {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("policy Node name must not be empty")
		}
		if err := validateNodeRule("nodes."+name, rule); err != nil {
			return err
		}
	}

	if p.hasOperatorRules() && p.MaxObservationGap.Duration <= 0 {
		return fmt.Errorf("policy maxObservationGap must be a positive duration when operator duration rules are configured")
	}
	if !p.hasAnyRule() {
		return fmt.Errorf("policy must configure at least one lifecycle rule")
	}
	return nil
}

func validateOperatorRules(prefix string, rules OperatorRules) error {
	for name, rule := range map[string]*DurationRule{
		"availabilityLoss": rules.AvailabilityLoss,
		"degraded":         rules.Degraded,
		"progressing":      rules.Progressing,
	} {
		if rule == nil {
			continue
		}
		if rule.MaxObservedDuration.Duration <= 0 {
			return fmt.Errorf("policy %s.%s.maxObservedDuration must be positive", prefix, name)
		}
	}
	return nil
}

func validatePoolRule(prefix string, rule MachineConfigPoolRule) error {
	if rule.PostCompletionGracePeriod.Duration <= 0 {
		return fmt.Errorf("policy %s.postCompletionGracePeriod must be positive", prefix)
	}
	return nil
}

func validateNodeRule(prefix string, rule NodeRule) error {
	if rule.ReadyPostCompletionGracePeriod.Set() && rule.ReadyPostCompletionGracePeriod.Duration <= 0 {
		return fmt.Errorf("policy %s.readyPostCompletionGracePeriod must be positive", prefix)
	}
	if rule.ConfigAlignedPostCompletionGracePeriod.Set() && rule.ConfigAlignedPostCompletionGracePeriod.Duration <= 0 {
		return fmt.Errorf("policy %s.configAlignedPostCompletionGracePeriod must be positive", prefix)
	}
	if !rule.ReadyPostCompletionGracePeriod.Set() && !rule.ConfigAlignedPostCompletionGracePeriod.Set() {
		return fmt.Errorf("policy %s must configure at least one Node lifecycle rule", prefix)
	}
	return nil
}

func (p UpgradePolicy) hasOperatorRules() bool {
	if hasOperatorRules(p.Defaults.OperatorRules()) {
		return true
	}
	for _, rules := range p.Operators {
		if hasOperatorRules(rules) {
			return true
		}
	}
	return false
}

func (p UpgradePolicy) hasAnyRule() bool {
	if p.hasOperatorRules() || p.Defaults.MachineConfigPool != nil || p.Defaults.Node != nil || len(p.MachineConfigPools) > 0 || len(p.Nodes) > 0 {
		return true
	}
	return false
}

func hasOperatorRules(rules OperatorRules) bool {
	return rules.AvailabilityLoss != nil || rules.Degraded != nil || rules.Progressing != nil
}

func (p UpgradePolicy) OperatorRulesFor(name string) OperatorRules {
	result := p.Defaults.OperatorRules()
	if explicit, ok := p.Operators[name]; ok {
		if explicit.AvailabilityLoss != nil {
			result.AvailabilityLoss = explicit.AvailabilityLoss
		}
		if explicit.Degraded != nil {
			result.Degraded = explicit.Degraded
		}
		if explicit.Progressing != nil {
			result.Progressing = explicit.Progressing
		}
	}
	return result
}

func (p UpgradePolicy) MachineConfigPoolRuleFor(name string) (MachineConfigPoolRule, bool) {
	var result MachineConfigPoolRule
	set := false
	if p.Defaults.MachineConfigPool != nil {
		result = *p.Defaults.MachineConfigPool
		set = true
	}
	if explicit, ok := p.MachineConfigPools[name]; ok {
		result = explicit
		set = true
	}
	return result, set
}

func (p UpgradePolicy) NodeRuleFor(name string) (NodeRule, bool) {
	var result NodeRule
	set := false
	if p.Defaults.Node != nil {
		result = *p.Defaults.Node
		set = true
	}
	if explicit, ok := p.Nodes[name]; ok {
		if explicit.ReadyPostCompletionGracePeriod.Set() {
			result.ReadyPostCompletionGracePeriod = explicit.ReadyPostCompletionGracePeriod
		}
		if explicit.ConfigAlignedPostCompletionGracePeriod.Set() {
			result.ConfigAlignedPostCompletionGracePeriod = explicit.ConfigAlignedPostCompletionGracePeriod
		}
		set = true
	}
	return result, set
}
