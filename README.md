# ReconcileGuard

ReconcileGuard is an offline Go CLI for reproducible lifecycle analysis of OpenShift platform operators. It reads saved ClusterOperator and ClusterVersion data using official OpenShift API types, reconstructs upgrade phases, correlates observations, and checks one normal-upgrade condition contract.

Real OpenShift compatibility has **not** been validated. All included fixtures are synthetic.

## Build and use

Requires Go 1.27.1 or later.

```sh
go build -o reconcile-guard ./cmd/reconcile-guard
./reconcile-guard help
./reconcile-guard version
./reconcile-guard check examples/ingress-ok.json
./reconcile-guard replay examples/ingress-history.jsonl
./reconcile-guard check-version examples/cluster-version.json
./reconcile-guard replay-version examples/cluster-version-history.jsonl
./reconcile-guard verify-upgrade examples/cluster-version-history.jsonl examples/ingress-upgrade-history.jsonl
```

| Command | Result |
| --- | --- |
| `check <file>` | Report the snapshot's Degraded condition. False does not establish overall health. |
| `replay <file.jsonl>` | Report changes in Available, Progressing and Degraded between adjacent observations. Missing conditions are counted as uncompared pairs, never bridged. |
| `check-version <file.json>` | Inspect ClusterVersion status.desired, conditions and update history. |
| `replay-version <file.jsonl>` | Validate the history and reconstruct analytical upgrade phases. |
| `verify-upgrade <version.jsonl> <operator.jsonl>` | Correlate both timelines and evaluate the normal-upgrade condition contract for one operator. |

JSONL records contain `observedAt` and either `operator` or `clusterVersion`. Each file must describe one resource with strictly increasing, nonzero timestamps. Blank lines are ignored; malformed JSON and oversized lines report their physical line number. Readers allow lines smaller than 4 MiB. Unknown JSON fields are ignored; this is not full API-schema validation.

`observedAt` is the snapshot capture time. OpenShift's `lastTransitionTime` is a separate reported timestamp. Transitions retain the interval between adjacent observations, not an invented exact event time.

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

## First contract and verdicts

`normal-upgrade-operator-conditions` checks Available=True and Degraded=False only for samples correlated to UPDATING. The underlying normal-upgrade expectation is documented in the pinned [OpenShift condition definitions](https://github.com/openshift/api/blob/9abfa327cff2/config/v1/types_cluster_operator.go). Phase reconstruction also uses the pinned [ClusterVersion definitions](https://github.com/openshift/api/blob/9abfa327cff2/config/v1/types_cluster_version.go).

- **FAIL**: an evaluated observation reports Available=False or Degraded=True. A concrete failure takes precedence over incomplete evidence elsewhere.
- **INCONCLUSIVE**: no UPDATING samples, missing/Unknown required conditions, or ambiguous/outside/unknown-phase operator samples prevent a complete assessment of the supplied observations.
- **PASS**: at least one UPDATING sample was evaluated, all required conditions satisfy the contract, and no evidence gaps above remain.

PASS applies to this rule and these samples only. It does not certify the entire upgrade or unsampled intervals. The tool cannot establish that the environment met the assumptions of a normal upgrade, or determine whether an infrastructure incident caused a failure.

Failure evidence includes the operator observation time, condition/status, reason/message, correlation kind and ClusterVersion interval endpoints. Preserve both input files to trace findings back to the original snapshots. The tool does not yet produce a self-contained evidence archive.

## Exit codes

| Code | `check` | `verify-upgrade` |
| --- | --- | --- |
| 0 | Degraded=False | PASS |
| 1 | Input/usage error | Input/usage error |
| 2 | Degraded=True | FAIL |
| 3 | Degraded=Unknown or missing | INCONCLUSIVE |

`check-version`, `replay` and `replay-version` return 0 for successful processing and 1 for errors; they do not return contract verdicts. Diagnostics go to stdout, errors to stderr.

## Development

```sh
gofmt -w .
go mod tidy
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go build -o reconcile-guard ./cmd/reconcile-guard
```

The race detector requires a supported platform and working C toolchain. Domain code lives in `internal/operator`, `internal/upgrade` and `internal/contracts`; `internal/app` handles arguments, output and exit codes. The entry point is `cmd/reconcile-guard`.

Pipeline: data → observations → validated timelines → upgrade phases → correlation → contract checks → evidence report. Computation accepts typed observations and is independent of JSONL readers or a future collector.

## Limitations and next steps

There is no live collector, watch/reconnect, multi-operator analysis, automatic root-cause analysis or real OpenShift integration validation. Only one contract is implemented. Data must come from the same cluster and a comparable run; resource names alone cannot prove this. Sampling gaps and clock differences limit inference.

Next: Progressing duration and operator version consistency contracts, multi-operator reports, stronger evidence and machine-readable output, read-only collection, watch/reconnect, real OpenShift/OKD validation and comparable-run regression analysis. kind can test Kubernetes client/watch mechanics; it does not substitute for OpenShift.

## License

[Apache License 2.0](LICENSE).
