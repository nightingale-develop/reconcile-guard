# Contracts and interpretation

[README](../README.md) · [CLI](cli.md) · [Contracts](contracts.md) · [Development](development.md)

## Upgrade phases and correlation

STABLE, UPDATING, COMPLETED and UNKNOWN are ReconcileGuard analytical states, not OpenShift condition names. Reconstruction uses Progressing, Available, status.desired and the newest update-history entry. Missing or contradictory evidence produces UNKNOWN. COMPLETED marks an observed UPDATING-to-STABLE transition for the same target release; a later stable observation is STABLE again.

Operator observations are correlated with the validated ClusterVersion timeline:

| Kind | Meaning |
| --- | --- |
| EXACT | Same observedAt as a ClusterVersion state; its phase may still be UNKNOWN. |
| BRACKETED | Between two observations with the same known phase. |
| AMBIGUOUS | Between different phases or an UNKNOWN endpoint. No phase is guessed. |
| OUTSIDE | Before or after the known timeline, or no states available. |

BRACKETED is an inference from matching endpoints, not proof that no unobserved change occurred between samples.

## Normal-upgrade conditions

`normal-upgrade-operator-conditions` checks Available=True and Degraded=False only for samples correlated to UPDATING. The underlying normal-upgrade expectation is documented in the pinned [OpenShift condition definitions](https://github.com/openshift/api/blob/9abfa327cff2/config/v1/types_cluster_operator.go). Phase reconstruction also uses the pinned [ClusterVersion definitions](https://github.com/openshift/api/blob/9abfa327cff2/config/v1/types_cluster_version.go).

- **FAIL**: an evaluated observation reports Available=False or Degraded=True. A concrete failure takes precedence over incomplete evidence elsewhere.
- **INCONCLUSIVE**: no UPDATING samples, missing/Unknown required conditions, or ambiguous/outside/unknown-phase operator samples prevent a complete assessment of the supplied observations.
- **PASS**: at least one UPDATING sample was evaluated, all required conditions satisfy the contract, and no evidence gaps above remain.

PASS applies to this rule and these samples only. It does not certify the entire upgrade or unsampled intervals. The tool cannot establish that the environment met the assumptions of a normal upgrade, or determine whether an infrastructure incident caused a failure.

Failure evidence includes the operator observation time, condition/status, reason/message, correlation kind and ClusterVersion interval endpoints. Preserve both input files to trace findings back to the original snapshots. The tool does not yet produce a self-contained evidence archive.

## Progressing duration during upgrades

```sh
./reconcile-guard verify-progressing-upgrade examples/cluster-version-history.jsonl examples/ingress-upgrade-history.jsonl examples/progressing-policy.json
```

`operator-progressing-duration` evaluates only confidently correlated UPDATING samples. Its policy file declares `maxDuration`, `maxObservationGap` (positive Go durations), `operator`, `targetVersion`, and a `source` reference documenting both the limit and sampling assumptions. See the [illustrative project policy](../examples/progressing-policy.md). No universal OpenShift threshold is assumed, and source authenticity/applicability is not independently verified.

Gaps in either timeline, intervening phases, or changes of target version/image break continuity. Missing/Unknown Progressing and ambiguous correlations make an otherwise successful result INCONCLUSIVE. Missing source/operator/target applicability also prevents a verdict other than INCONCLUSIVE. Invalid input or durations are errors.

Within a policy-supported run, an observed True span exceeding the limit yields FAIL. A True episode can PASS only when surrounding False samples bound its maximum span within the limit; censored/uncertain episodes are INCONCLUSIVE. A concrete FAIL overrides other evidence gaps. Evidence includes start/end observations, interval, correlation endpoints, threshold and source. This is a sampled project-policy result, not proof of uninterrupted physical state.

The existing three-observation fixtures have insufficient UPDATING evidence for this duration check and return INCONCLUSIVE (exit 3). Use denser observations and your documented policy for a substantive evaluation. Exit codes are 0/2/3 for PASS/FAIL/INCONCLUSIVE and 1 for errors.

The earlier `verify-progressing <operator.jsonl> <max-duration>` remains a separate operator-only sampling check; it does not perform upgrade correlation or establish a documented threshold.

## Operator version consistency

`operator-version-consistency` checks `status.versions[name=operator]` only after a target release has been observed as completed.

During an upgrade, an operator may continue reporting its previous version while old operands are still present. ReconcileGuard therefore does not treat a version mismatch during UPDATING as a failure.

After upgrade completion, the reported operator version must match the completed ClusterVersion target. Missing version evidence or missing post-completion observations produce INCONCLUSIVE. A concrete mismatch produces FAIL.

A window starts at a reconstructed COMPLETED observation and extends through consecutive STABLE observations with the same desired version. Its endpoints are inclusive; samples outside these windows are ignored. No completed target, no evaluated versions, a missing target/version or a window without operator observations makes an otherwise passing result INCONCLUSIVE. A mismatch takes precedence and yields FAIL. Duplicate `operator` version entries in an evaluated sample are input errors. Comparison uses exact version strings.

This is the implemented rule in [VerifyOperatorVersionConsistency](../internal/contracts/version_consistency.go). The window extension checks phase and desired version; it has no maximum sampling-gap policy. It does not prove state outside the supplied observations or physical continuity between them.

## Multi-operator upgrade report

`verify-cluster-upgrade` evaluates multiple saved ClusterOperator histories against the same ClusterVersion timeline.

For each supplied operator it currently evaluates:

- `normal-upgrade-operator-conditions`
- `operator-version-consistency`

The aggregate verdict uses fail-first precedence:

- `FAIL` if at least one supplied operator fails;
- otherwise `INCONCLUSIVE` if at least one supplied operator is inconclusive;
- otherwise `PASS`.

The report covers only the operator histories supplied to the command. A PASS does not prove that every ClusterOperator in the cluster was evaluated.

The policy-based Progressing duration contract is not included because its threshold and applicability are supplied separately per operator.

The verification commands support `--output json` for machine-readable reports.
The output uses schema version `1` and includes the command name, aggregate
verdict, per-operator contract results, counts/values/flags and evidence fields
when the contract produces them. The JSON report represents the same selected
checks as text output; it does not add observations or establish evidence that
the input snapshots do not contain.


At least one operator history is required; duplicate operator names are input errors. The CLI prints counts and per-operator verdicts in text mode, and exposes the corresponding contract details and evidence in JSON mode. The JSON document is not a self-contained evidence archive: preserve the ClusterVersion history and all supplied operator histories to trace findings back to the original snapshots.

`compare-runs` compares two independently validated stopped run directories.
It compares only conditions and operator-version verdicts for matching
operators: PASS→FAIL is a regression, FAIL→PASS an improvement, and PASS→PASS or FAIL→FAIL unchanged. Missing or INCONCLUSIVE contracts are inconclusive;
aggregate precedence is FAIL, then INCONCLUSIVE, then PASS. Different cluster
IDs or target versions are allowed after within-run validation. PASS means no
detected regression among compared contracts; it does not certify the candidate
run or either run's completeness.

Persistent FAIL is not a new regression. Any INCONCLUSIVE side, including
INCONCLUSIVE→FAIL, leaves that contract comparison inconclusive. A missing
operator on either side makes the aggregate INCONCLUSIVE unless a comparable
contract regresses. Progressing duration requires a separate policy and is not
included in v1 comparison.
