# Contracts and interpretation

[README](../README.md) · [CLI](cli.md) · [Contracts](contracts.md) · [Development](development.md)

## Upgrade phases and correlation

STABLE, UPDATING, COMPLETED and UNKNOWN are ReconcileGuard analytical states, not OpenShift condition names. Reconstruction uses Progressing, Available, status.desired and the newest update-history entry. Missing or contradictory evidence produces UNKNOWN. COMPLETED marks an observed UPDATING-to-STABLE transition for the same target release; a later stable observation is STABLE again.

Operator observations are correlated with the validated ClusterVersion timeline:

| Kind | Meaning |
| --- | --- |
| EXACT | Same observedAt as a ClusterVersion state; its phase may still be UNKNOWN. |
| BRACKETED | Between two observations with the same known phase and target version/image. |
| AMBIGUOUS | Between different phases, target versions/images, or an UNKNOWN endpoint. No phase is guessed. |
| OUTSIDE | Before or after the known timeline, or no states available. |

BRACKETED is an inference from matching endpoints, not proof that no unobserved change occurred between samples.

## Normal-upgrade conditions

`normal-upgrade-operator-conditions` is a conservative sampled-condition assessment,
not a guarantee of uninterrupted availability during an upgrade. It inspects
Available and Degraded only in confidently correlated UPDATING observations.
The historical contract identifier is retained for artifact compatibility.

- **PASS**: at least one UPDATING sample was evaluated, every evaluated sample
  reports Available=True and Degraded=False, and there are no missing conditions
  or ambiguous/outside/unknown-phase samples. This means no adverse condition
  was observed in the assessed samples, not that the upgrade is healthy.
- **INCONCLUSIVE**: an evaluated sample reports Available=False or Degraded=True,
  or the evidence requirements above are not met. Later recovery does not turn
  an adverse observation into PASS. Repeated or long-lived adverse snapshots
  also remain INCONCLUSIVE without an applicable duration/recovery policy.
- **FAIL**: this condition assessment currently cannot establish one from these
  snapshots alone. No condition-duration policy is implemented here. Other
  contracts still return FAIL for demonstrated violations, such as a reported
  operator-version mismatch inside a completed-target window.

The first user-run OKD 4.20 → 4.21 recording completed successfully but contained
Available=False/Degraded=True observations during UPDATING. Treating each such
sample as a lifecycle violation was too strong. The [OpenShift condition
API definitions](https://github.com/openshift/api/blob/9abfa327cff2/config/v1/types_cluster_operator.go)
describe the conditions and normal-upgrade expectations; they do not supply a
universal duration allowance or identify the cause of an observed transient.
This assessment does not infer cause, persistence between samples, or causality.

Adverse-condition evidence retains observation time, status, reason/message,
correlation and CV interval endpoints. Its JSON `verdict` is INCONCLUSIVE, not
FAIL. Missing or ambiguous observations remain conservative. Preserve both input
histories; reports are not self-contained evidence archives. Compare old and new
runs using the same tool revision: unchanged JSON schema does not make the former
strict condition policy equivalent to this revised assessment.

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

A window starts at a reconstructed COMPLETED observation and extends through consecutive STABLE observations with the same desired target version and image. A target-version or target-image change closes the window; its endpoints are inclusive and samples outside these windows are ignored. No completed target, no evaluated versions, a missing target/version or a window without operator observations makes an otherwise passing result INCONCLUSIVE. A mismatch takes precedence and yields FAIL. Duplicate `operator` version entries in an evaluated sample are input errors. Comparison uses exact version strings.

Graceful `record-live` shutdown supplies fresh operator observations between two
fresh ClusterVersion reads. An unchanged operator resourceVersion is valid new
temporal evidence when re-read after completion. Ordinary WATCH events remain
deduplicated. Without post-completion evidence, old artifacts stay INCONCLUSIVE;
no synthetic timestamp or last-value carry-forward is added by analysis.

This is the implemented rule in [VerifyOperatorVersionConsistency](../internal/contracts/version_consistency.go). The window extension checks phase, desired version and image; it has no maximum sampling-gap policy. It does not prove state outside the supplied observations or physical continuity between them.

With asynchronous observation streams, even a successful real upgrade may remain
INCONCLUSIVE when target/image boundaries, generations or operator observations
cannot be aligned. The tool does not extrapolate beyond recorded windows; graceful shutdown
now performs a fresh final snapshot bracket; these limitations remain even though two real OKD upgrades have now been
exercised (see the [validation record](real-upgrade-validation.md)).

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
