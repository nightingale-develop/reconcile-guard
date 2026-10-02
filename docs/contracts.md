# Contracts and interpretation

[README](../README.md) · [CLI](cli.md) · [JSON output](json-output.md)

## Phases and correlation

STABLE, UPDATING, COMPLETED and UNKNOWN are analytical states. They are inferred
from ClusterVersion Progressing, Available, desired release and update history.
Missing or contradictory evidence yields UNKNOWN. COMPLETED means an observed
UPDATING-to-STABLE transition for the same target version and image.
Generation must be observed; history must match desired and have valid times.
UPDATING requires Progressing=True and Partial history without completionTime;
STABLE requires Available=True, Progressing=False and Completed history with
completionTime. Progressing=False alone is insufficient.

Operator, MachineConfigPool, and Node samples can be correlated to the
ClusterVersion timeline as EXACT, BRACKETED, AMBIGUOUS, or OUTSIDE. BRACKETED is an inference from matching
known phase and version/image endpoints, not proof of uninterrupted state.

## Implemented checks

`normal-upgrade-operator-conditions` assesses Available=True and Degraded=False
only in confidently correlated UPDATING samples. Missing conditions, ambiguous
or outside samples, and adverse observations make the result INCONCLUSIVE.
PASS means the selected samples met the rule; it does not certify the upgrade.

`observed-progressing-duration` evaluates consecutive True samples: their span
over the user limit is FAIL, even without surrounding False samples. PASS requires
a False-to-False bound within the limit, or only known False samples. Otherwise
the result is INCONCLUSIVE. This is a sampling policy, not an OpenShift timeout.
`operator-progressing-duration` additionally restricts evaluation to correlated
UPDATING, with operator, targetVersion, source and maximum gaps in both timelines.
Unknowns, gaps and target changes break segments; no eligible samples is INCONCLUSIVE.

`operator-version-consistency` checks `status.versions[name=operator]` only in
the window beginning at observed completion and continuing through matching
STABLE samples. A mismatch is FAIL; missing target, version, or coverage is
INCONCLUSIVE. A mismatch during UPDATING is not a failure.

`verify-cluster-upgrade` runs condition and version checks for each supplied
operator. Aggregate precedence is FAIL, then INCONCLUSIVE, then PASS. It does
not evaluate every ClusterOperator unless every history is supplied.

`verify-run` applies the aggregate operator checks to one stopped recording and,
when present, adds the evidence-only MCP and Node checks described below.
`compare-runs` uses condition/version verdicts as its base regression signal:
PASS→FAIL is regression, FAIL→PASS is improvement, and equal definite verdicts
are unchanged. Missing or inconclusive contracts remain INCONCLUSIVE. Recorded
MCP/Node evidence is compared separately but stays evidence-only and cannot
create a regression verdict without an explicit lifecycle policy. Observed
upgrade/convergence timing is reported as descriptive sample-to-sample duration
and does not affect the verdict.

With `--policy`, the same supplied `UpgradePolicy` is evaluated against both
runs and its resulting operator/MCP/Node contract verdicts are compared with the
same matrix. Operator policy comparisons also expose the largest recorded
`observedSpan` carried by policy episode evidence and its candidate-minus-baseline
delta when both sides have one; the delta is descriptive and does not itself
change a verdict. A policy PASS→FAIL is a regression because its threshold comes
from explicit user input. ReconcileGuard does not retarget or relax the policy
for either run.


### MachineConfigPool lifecycle evidence

`machine-config-pool-lifecycle-evidence` classifies each recorded pool sample as
STABLE, UPDATING, DEGRADED, or UNKNOWN. DEGRADED is based on a reported
Degraded=True condition or a nonzero degradedMachineCount. UPDATING requires
Updating=True. STABLE requires Updated=True, Updating=False, Degraded=False and
consistent updated/ready/unavailable/degraded counters. Missing, Unknown, or
contradictory evidence remains UNKNOWN.

The evidence contract only evaluates confidently correlated samples at or after
an observed ClusterVersion COMPLETED transition for the same target. At least one
STABLE applicable sample yields PASS. Otherwise the result is INCONCLUSIVE. The
base evidence contract does not emit FAIL and does not affect the aggregate
operator verdict; explicit threshold-based FAILs belong to `verify-lifecycle-policy`.

### Node lifecycle evidence

`node-lifecycle-evidence` records Node Ready status, current/desired MachineConfig
annotations, kubelet version, and observed transitions. A post-completion sample
is considered converged only when Ready=True and both MachineConfig annotations
are present and equal. At least one such applicable sample yields PASS; otherwise
the result is INCONCLUSIVE. A NotReady or non-converged sample is retained as
evidence, not promoted to FAIL without an explicit lifecycle policy.

## Evidence limits

Reports describe supplied snapshots and intervals. They do not infer unsampled
state, prove uninterrupted availability, diagnose causes, or establish cluster
provenance. Preserve the original JSONL files with reports.

## Explicit lifecycle policies

`verify-lifecycle-policy` turns selected lifecycle evidence into policy verdicts
only when the user supplies thresholds. The policy is versioned as
`reconcileguard.io/v1alpha1` / `UpgradePolicy` and must identify its threshold
`source`. ReconcileGuard does not ship default lifecycle timeouts.

### Operator duration rules

During confidently correlated ClusterVersion `UPDATING` intervals, policies may
bound observed episodes of:

- `availabilityLoss`: `Available=False`;
- `degraded`: `Degraded=True`;
- `progressing`: `Progressing=True`.

Each rule has `maxObservedDuration`. A directly observed adverse span beyond the
threshold is FAIL. A shorter adverse episode is PASS only when known good samples
before and after bound the sampled episode inside the threshold. An open-ended
short episode is INCONCLUSIVE. Missing/Unknown conditions, ambiguous correlation,
target version/image changes, UNKNOWN ClusterVersion states, oversized brackets,
and gaps larger than `maxObservationGap` break the episode and preserve
uncertainty instead of bridging it. A zero duration is invalid policy input.

These are project policies, not claims that OpenShift requires a ClusterOperator
to recover within the configured duration.

### MachineConfigPool post-completion rule

`postCompletionGracePeriod` delays policy enforcement until that interval has
elapsed after an observed ClusterVersion COMPLETED transition for the policy
target. UNKNOWN, target changes, and other phase boundaries end the current
window. Only a newly observed COMPLETED transition for the target starts a new
grace window; earlier valid findings are retained separately. Applicable-window
evidence includes its completion and deadline. After the deadline, confidently
correlated MCP samples are
evaluated as sampled state:

- STABLE is compliant;
- UPDATING or DEGRADED is a direct policy violation and yields FAIL;
- UNKNOWN remains INCONCLUSIVE.

A PASS means the applicable recorded samples after the grace period were STABLE.
It does not prove the exact instant when the pool converged between samples.

### Node post-completion rules

Nodes can configure independent `readyPostCompletionGracePeriod` and
`configAlignedPostCompletionGracePeriod` thresholds. After the corresponding
deadline, a confidently correlated `Ready=False` sample or directly observed
current/desired MachineConfig divergence is FAIL. Ready=Unknown or missing
MachineConfig annotations are INCONCLUSIVE. PASS describes sampled compliance
only and does not infer unsampled continuity. Node and MCP records with invalid
condition statuses or duplicate conditions are invalid input; negative MCP
counts are invalid as well.

Policy results are separate from the evidence-only MCP/Node contracts emitted by
`verify-run`. `verify-lifecycle-policy` aggregates only policy contracts with
FAIL > INCONCLUSIVE > PASS precedence.
