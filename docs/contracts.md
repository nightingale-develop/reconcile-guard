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

Operator samples are correlated to the ClusterVersion timeline as EXACT,
BRACKETED, AMBIGUOUS, or OUTSIDE. BRACKETED is an inference from matching
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

`verify-run` applies the aggregate checks to one stopped recording.
`compare-runs` compares only those condition/version verdicts: regression is
PASS→FAIL; FAIL→PASS is improvement; equal definite verdicts are unchanged.
Missing or inconclusive contracts remain INCONCLUSIVE.

## Evidence limits

Reports describe supplied snapshots and intervals. They do not infer unsampled
state, prove uninterrupted availability, diagnose causes, or establish cluster
provenance. Preserve the original JSONL files with reports.
