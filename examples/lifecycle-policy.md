# Example lifecycle policy

This is a synthetic project policy for demonstrating `verify-lifecycle-policy`.
The durations are examples, not OpenShift product guarantees or recommended
production thresholds.

The matching YAML is [lifecycle-policy.yaml](lifecycle-policy.yaml).

## Synthetic example

- `targetVersion` scopes the policy to one OpenShift release target. `targetImage`
  may also be supplied when an exact release image must be selected.
- `source` identifies the document or decision that owns the thresholds.
- `maxObservationGap` is required when an operator duration rule is configured.
  A larger gap breaks an observed episode and makes that part of the result
  inconclusive instead of silently bridging missing samples.
- `defaults.availabilityLoss`, `defaults.degraded`, and `defaults.progressing`
  apply to recorded ClusterOperators. Per-operator entries override only the
  rules they name.
- `defaults.machineConfigPool.postCompletionGracePeriod` starts sampled MCP
  enforcement after an observed ClusterVersion completion. A confidently
  correlated `UPDATING` or `DEGRADED` MCP sample after the grace period is a
  policy violation; `UNKNOWN` remains inconclusive.
- Node Ready and MachineConfig alignment use independent post-completion grace
  periods. Missing/Unknown state remains inconclusive rather than becoming a
  synthetic failure.

There are no built-in lifecycle thresholds. A rule is evaluated only when it is
present in the supplied policy.
