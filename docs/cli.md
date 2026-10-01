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
| `verify-progressing-upgrade <version.jsonl> <operator.jsonl> <policy.json>` | Apply a policy during an upgrade. |
| `capture-live <dir> [--kubeconfig <path>]` | Capture one ClusterVersion and operator LIST. |
| `record-live <dir> [--kubeconfig <path>]` | Record both resources through LIST/WATCH until stopped. |
| `observe-upgrade <dir> [--kubeconfig <path>]` | Record, stop on observed completion, finalize and verify. |
| `verify-run <run-dir>` | Verify one stopped recording. |
| `compare-runs <baseline-dir> <candidate-dir>` | Compare condition/version verdicts between stopped runs. |

Use the current kubeconfig/context unless `--kubeconfig` is supplied. Live
commands are read-only. Recording needs LIST/WATCH on ClusterVersion and
ClusterOperator; final capture also needs GET on ClusterVersion. Generated runs
contain `run.json`, a ClusterVersion JSONL stream, and per-operator streams.
`capture-live` needs only GET ClusterVersion and LIST ClusterOperators.
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
→ GET ClusterVersion bracket is captured before the run is marked stopped. The
final API context lasts 30 seconds. Ctrl+C/SIGTERM preserves a stopped partial run when
final capture succeeds; the summary says whether live completion was observed,
even if final reads show completion later.
Final reads append unchanged resourceVersions with fresh per-resource UTC
timestamps; WATCH still deduplicates them. Final API/write errors mark the run
failed and exit `1`. After success, the summary includes run/cluster IDs, target
version/image, completion flag, final snapshot status and operator verdict counts.
Both automatic and manual stopping return the saved-run verification exit code.

## Runs and comparison

`verify-run` accepts only a locally consistent stopped run and checks conditions
and operator-version consistency. It does not include Progressing duration.
`compare-runs` allows different clusters and targets after local validation and
compares only condition/version contracts. PASS means no detected regression;
FAIL→FAIL is unchanged; either INCONCLUSIVE/missing side is inconclusive. Different
operator sets are INCONCLUSIVE unless a proven regression takes precedence.

## Input and output

JSONL records contain `observedAt` and either `operator` or `clusterVersion`.
Timestamps must be strictly increasing and nonzero. Blank lines are ignored;
malformed JSON and oversized lines report their physical line. Unknown fields
are ignored. `observedAt` is capture time; OpenShift `lastTransitionTime` is a
reported field and is not substituted for capture time.

Verification, `verify-run`, and `compare-runs` accept `--output text|json` (or
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
