# CLI reference

[README](../README.md) · [Contracts](contracts.md) · [JSON output](json-output.md)

| Command | Purpose |
| --- | --- |
| `help` / `version` | Show usage / application version. |
| `check <file>` | Inspect one operator snapshot's Degraded condition. |
| `replay <file.jsonl>` | Report adjacent Available, Progressing and Degraded changes. |
| `check-version <file.json>` | Inspect one ClusterVersion snapshot. |
| `replay-version <file.jsonl>` | Validate history and reconstruct phases. |
| `verify-upgrade <version.jsonl> <operator.jsonl>` | Check normal-upgrade conditions for one operator. |
| `verify-version-upgrade <version.jsonl> <operator.jsonl>` | Check post-completion operator version. |
| `verify-cluster-upgrade <version.jsonl> <operator.jsonl>...` | Check several operators and aggregate results. |
| `verify-progressing <operator.jsonl> <duration>` | Apply the operator-only sampled duration check. |
| `verify-progressing-upgrade <version.jsonl> <operator.jsonl> <policy.json>` | Apply a Progressing duration policy during an upgrade. |
| `verify-lifecycle-policy <run-dir> <policy.yaml>` | Apply explicit operator/MCP/Node lifecycle thresholds to one stopped run. |
| `capture-live <dir> [--kubeconfig <path>]` | Capture one ClusterVersion plus ClusterOperator, MachineConfigPool, and Node LISTs. |
| `record-live <dir> [--kubeconfig <path>]` | Record ClusterVersion, ClusterOperator, MachineConfigPool, and Node resources through LIST/WATCH until stopped. |
| `observe-upgrade <dir> [--kubeconfig <path>]` | Record, stop on observed completion, finalize and verify. |
| `verify-run <run-dir>` | Verify one stopped recording. |
| `timeline-run <run-dir>` | Merge ClusterVersion phase changes and recorded operator/MCP/Node transitions into one ordered timeline. |
| `report-run <run-dir> [--policy <policy.yaml>] [--file <report.md>]` | Render a Markdown report from one stopped run. |
| `compare-runs <baseline-dir> <candidate-dir>` | Compare condition/version verdicts between stopped runs. |

Use the current kubeconfig/context unless `--kubeconfig` is supplied. Live
commands are read-only. Recording needs LIST/WATCH on ClusterVersion,
ClusterOperator, MachineConfigPool, and Node resources; final capture also needs
GET on ClusterVersion. Generated runs contain `run.json`, a ClusterVersion JSONL
stream, per-operator streams, per-MCP streams, and per-Node streams.
`capture-live` needs GET ClusterVersion and LIST access to ClusterOperators,
MachineConfigPools, and Nodes.
Client-go manages reconnect/relist. Initial Unauthorized, Forbidden, missing
ClusterVersion and sink errors are fatal; Unauthorized after sync is retried.
ClusterOperator deletion is ignored.

## Observe one upgrade

```sh
./reconcile-guard observe-upgrade ./runs --kubeconfig "$HOME/.kube/config"
```

The administrator starts the upgrade separately. Recording begins immediately;
STABLE waits, an initial UPDATING state is accepted with an incomplete-baseline
warning, and UNKNOWN is preserved without guessing. The observer tracks desired
version and image together. A target change resets the active target and
requires new UPDATING evidence. Automatic stop requires the phase analyzer to
observe COMPLETED for that same target. Missed or ambiguous transitions may
require Ctrl+C.

On stop, WATCH writes drain and a fresh GET ClusterVersion → LIST all operators
→ LIST MachineConfigPools → LIST Nodes → GET ClusterVersion bracket is captured
before the run is marked stopped. The final API context lasts 30 seconds. Ctrl+C/SIGTERM preserves a stopped partial run when
final capture succeeds; the summary says whether live completion was observed,
even if final reads show completion later.
Final reads append unchanged resourceVersions with fresh per-resource UTC
timestamps; WATCH still deduplicates them. Final API/write errors mark the run
failed and exit `1`. After success, the summary includes run/cluster IDs, target
version/image, completion flag, final snapshot status and operator verdict counts.
Both automatic and manual stopping return the saved-run verification exit code.

## Explicit lifecycle policy

```sh
./reconcile-guard verify-lifecycle-policy \
  ./runs/REPLACE_WITH_RUN_ID \
  examples/lifecycle-policy.yaml --output text
```

`verify-lifecycle-policy` first performs the same local run validation as
`verify-run`, then evaluates only rules explicitly present in the supplied
`UpgradePolicy`. YAML and JSON are accepted. Unknown policy fields are rejected.
The policy must declare `apiVersion: reconcileguard.io/v1alpha1`,
`kind: UpgradePolicy`, `targetVersion`, and a nonempty `source`. `targetImage` is
optional and can disambiguate multiple release images for one version.

Operator rules apply only to confidently correlated `UPDATING` samples for the
policy target. `availabilityLoss` evaluates `Available=False`, `degraded`
evaluates `Degraded=True`, and `progressing` evaluates `Progressing=True`. Each
rule declares `maxObservedDuration`. `maxObservationGap` is required when any
operator duration rule is configured; larger gaps break episodes and produce
INCONCLUSIVE evidence instead of bridging missing samples. A directly observed
adverse span longer than the threshold is FAIL. A shorter episode is PASS only
when its good-state bounds establish that the whole sampled episode fits inside
the threshold; otherwise it remains INCONCLUSIVE.

MachineConfigPool and Node rules are evaluated only after an observed
ClusterVersion COMPLETED transition for the matching target. Their
`postCompletionGracePeriod` values define when sampled enforcement begins. A
confident MCP `UPDATING`/`DEGRADED` sample after the deadline is FAIL; MCP
`UNKNOWN` is INCONCLUSIVE. Node Ready and MachineConfig alignment have independent
grace periods. `Ready=False` or directly observed config divergence after the
respective deadline is FAIL; missing/Unknown data is INCONCLUSIVE.

Defaults apply to recorded resources of that kind. Named resource entries
override the rules they specify. A resource explicitly named by policy but absent
from the run is INCONCLUSIVE. There are no built-in duration defaults. Policy
thresholds are user-supplied project rules, not OpenShift guarantees. Aggregate
policy precedence is FAIL, then INCONCLUSIVE, then PASS.

## Timeline and Markdown report

```sh
./reconcile-guard timeline-run ./runs/REPLACE_WITH_RUN_ID
./reconcile-guard timeline-run ./runs/REPLACE_WITH_RUN_ID --output json > timeline.json

./reconcile-guard report-run ./runs/REPLACE_WITH_RUN_ID --file report.md
./reconcile-guard report-run ./runs/REPLACE_WITH_RUN_ID \
  --policy examples/lifecycle-policy.yaml --file report-with-policy.md
```

`timeline-run` uses the same stopped-run validation as `verify-run`. It emits a
deterministically ordered view of ClusterVersion phase/target changes,
ClusterOperator condition transitions, MachineConfigPool lifecycle/configuration
transitions, and Node Ready/MachineConfig/kubelet transitions. Exact observations
retain `observedAt`; transitions retain their `[from,to]` observation interval.
The command does not replace an interval with an inferred exact event timestamp.
Text and JSON output are available, and successful rendering exits `0`.

`report-run` renders Markdown to stdout by default. `--file` writes the report to
the supplied path. `--policy` optionally evaluates an explicit `UpgradePolicy`
and includes its existing policy verdicts in the report. The report contains run
metadata, aggregate verification results, per-resource summaries, the merged
timeline, and scope/interpretation notes. Report generation is a presentation
operation and exits `0` when rendering succeeds even when the embedded
verification or policy verdict is FAIL or INCONCLUSIVE. It does not create new
verdict semantics.

## Runs and comparison

`verify-run` accepts only a locally consistent stopped run and checks conditions
and operator-version consistency. When MCP/Node histories are present, it also
reconstructs their lifecycle evidence and correlates post-completion samples with
the ClusterVersion timeline. Those base MCP/Node evidence contracts do not change
the aggregate operator verdict and do not create FAIL. Explicit policy FAILs are
reported only by `verify-lifecycle-policy`. `verify-run` does not include
Progressing duration.
`compare-runs` allows different clusters and targets after local validation and
compares only condition/version contracts. PASS means no detected regression;
FAIL→FAIL is unchanged; either INCONCLUSIVE/missing side is inconclusive. Different
operator sets are INCONCLUSIVE unless a proven regression takes precedence.

## Input and output

JSONL records contain `observedAt` and one of `operator`, `clusterVersion`,
`machineConfigPool`, or `node`.
Timestamps must be strictly increasing and nonzero. Blank lines are ignored;
malformed JSON and oversized lines report their physical line. Unknown fields
are ignored. `observedAt` is capture time; OpenShift `lastTransitionTime` is a
reported field and is not substituted for capture time.

Verification commands, `verify-run`, `verify-lifecycle-policy`, `timeline-run`, and `compare-runs` accept `--output text|json` (or
the equals form); text is the default. See [JSON output](json-output.md).
`observe-upgrade` is text-only. `check-version` and replay commands return `0`
for successful processing and `1` for errors, without a contract verdict.
`check` returns `0` for Degraded=False, `2` for True, `3` for Unknown/missing,
and `1` for errors. `record-live` returns `0` on clean shutdown, without verification.

| Exit | Meaning |
| --- | --- |
| `0` | PASS, or successful non-verification processing |
| `1` | Input, usage, API, finalization, or output error |
| `2` | FAIL or detected comparison regression |
| `3` | INCONCLUSIVE |
