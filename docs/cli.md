# CLI reference

[README](../README.md) · [CLI](cli.md) · [Contracts](contracts.md) · [Development](development.md)

| Command | Result |
| --- | --- |
| `help` / `version` | Show usage / the development version string. |
| `check <file>` | Report the snapshot's Degraded condition. False does not establish overall health. |
| `replay <file.jsonl>` | Report changes in Available, Progressing and Degraded between adjacent observations. Missing conditions are counted as uncompared pairs, never bridged. |
| `check-version <file.json>` | Inspect ClusterVersion status.desired, conditions and update history. |
| `replay-version <file.jsonl>` | Validate the history and reconstruct analytical upgrade phases. |
| `verify-upgrade <version.jsonl> <operator.jsonl>` | Correlate both timelines and evaluate the normal-upgrade condition contract for one operator. |
| `verify-version-upgrade <version.jsonl> <operator.jsonl>` | Verify that the ClusterOperator reports the completed OpenShift target version after upgrade completion. |
| `verify-cluster-upgrade <version.jsonl> <operator.jsonl>...` | Run upgrade contracts for multiple supplied ClusterOperators and produce an aggregate report. |
| `verify-run <runs-directory/run-id>` | Verify a stopped recording run's condition and version histories; supports `--output text|json`. |
| `verify-progressing <operator.jsonl> <max-duration>` | Operator-only sampled Progressing check with a positive Go duration. |
| `verify-progressing-upgrade <version.jsonl> <operator.jsonl> <policy.json>` | Upgrade-correlated Progressing check with an explicit project policy. |

`capture-live <directory> [--kubeconfig <path>]` performs one append operation:
it GETs the named ClusterVersion and LISTs ClusterOperators. `record-live
<directory> [--kubeconfig <path>]` keeps both resources under LIST/WATCH until
SIGINT/SIGTERM (exit `0`). Recording requires LIST/WATCH permissions on both
resource types in `config.openshift.io`; capture needs GET on ClusterVersion/version
and LIST on ClusterOperators, without WATCH. Both use the current kubeconfig/context
unless `--kubeconfig <path>` is supplied. Auth, missing-resource and sink errors exit `1`. Ordinary watch
recovery is handled by client-go. ClusterVersion is filtered to
`metadata.name=version`; ClusterOperator deletion is ignored, while missing or
deleted ClusterVersion is fatal.

`record-live <runs-directory>` creates a unique UTC run directory containing
`run.json`, `cluster-version.jsonl` and `operators/<name>.jsonl`. The manifest
is finalized as `stopped` on SIGINT/SIGTERM or `failed` on an error. `verify-run`
accepts only a stopped run, validates its manifest and local histories, and
returns `0` PASS, `2` FAIL, `3` INCONCLUSIVE or `1` for invalid/incomplete input.
The run must be `stopped`; `recording` and `failed` runs are rejected before any
verdict. It checks strict manifest inventory, operator filenames and names,
local/resolved paths, and nonempty clusterID consistency before conditions and
version consistency. Progressing duration is not part of this aggregate. A run
manifest does not provide cryptographic provenance or prove that the full
cluster was recorded. Replay paths are the generated files,
for example `replay-version <runs-root>/<run-id>/cluster-version.jsonl` and
`replay <runs-root>/<run-id>/operators/ingress.jsonl`.

All `verify-*` commands accept the optional `--output text|json` (also
`--output=text|json`). Text is the default. JSON output is a document with
`schemaVersion`, `command` and `result` fields; the result contains the overall
verdict, per-operator contracts, contract details and any available evidence.
See the [JSON output reference](json-output.md) for the schema and evidence
fields. The current schema version is `1`. The option is rejected for `check`,
`replay`, `check-version` and `replay-version`, and it may be supplied only once.

## Input format

JSONL records contain `observedAt` and either `operator` or `clusterVersion`. Each file must describe one resource with strictly increasing, nonzero timestamps. Blank lines are ignored; malformed JSON and oversized lines report their physical line number. Readers allow lines smaller than 4 MiB. Unknown JSON fields are ignored; this is not full API-schema validation.

`observedAt` is the snapshot capture time. OpenShift's `lastTransitionTime` is a separate reported timestamp. Transitions retain the interval between adjacent observations, not an invented exact event time.

## Examples

Run from the repository root after the build in the [quick start](../README.md#quick-start).

```sh
./reconcile-guard check examples/ingress-ok.json
./reconcile-guard replay examples/ingress-history.jsonl
./reconcile-guard check-version examples/cluster-version.json
./reconcile-guard replay-version examples/cluster-version-history.jsonl
./reconcile-guard verify-upgrade examples/cluster-version-history.jsonl examples/ingress-upgrade-history.jsonl
./reconcile-guard verify-version-upgrade examples/cluster-version-history.jsonl examples/ingress-version-history.jsonl
./reconcile-guard verify-cluster-upgrade examples/cluster-version-history.jsonl examples/ingress-version-history.jsonl examples/network-version-history.jsonl
./reconcile-guard verify-progressing examples/ingress-history.jsonl 10m
./reconcile-guard verify-progressing-upgrade examples/cluster-version-history.jsonl examples/ingress-upgrade-history.jsonl examples/progressing-policy.json
```

All fixtures are synthetic. The version and multi-operator examples return PASS (0). The operator-only Progressing example returns PASS (0) at the illustrative `10m` limit. The upgrade-aware Progressing example returns INCONCLUSIVE (3); this is expected for these sparse inputs, not a processing error. The `10m` limit is illustrative, not an OpenShift requirement. See [contract semantics](contracts.md) and the [example policy](../examples/progressing-policy.md).

## Exit codes

| Code | `check` | all `verify-*` commands |
| --- | --- | --- |
| 0 | Degraded=False | PASS |
| 1 | Input/usage error | Input/usage error |
| 2 | Degraded=True | FAIL |
| 3 | Degraded=Unknown or missing | INCONCLUSIVE |

`check-version`, `replay` and `replay-version` return 0 for successful processing and 1 for errors; they do not return contract verdicts. Diagnostics go to stdout, errors to stderr. JSON encoding failures are reported as input/usage-style errors with exit code 1.
