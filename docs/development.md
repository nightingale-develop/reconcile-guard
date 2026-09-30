# Development

[README](../README.md) · [CLI](cli.md) · [Contracts](contracts.md) · [Development](development.md)

```sh
gofmt -w .
go mod tidy
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go build -o reconcile-guard ./cmd/reconcile-guard
```

The race detector requires a supported platform and working C toolchain.

## Project layout

| Path | Responsibility |
| --- | --- |
| `cmd/reconcile-guard/` | Executable entry point; delegates to `internal/app`. |
| `internal/app/` | CLI commands, input loading, report formatting and exit codes. |
| `internal/operator/` | ClusterOperator snapshot and JSONL readers, observation validation and condition transitions. |
| `internal/upgrade/` | ClusterVersion snapshot and JSONL readers, timeline validation and upgrade phase reconstruction. |
| `internal/contracts/` | Timeline correlation, contract evaluation and aggregate verdicts. |
| `internal/collector/` | Read-only API capture and client-go LIST/WATCH. |
| `internal/recording/` | JSONL streams and run manifests. |
| `internal/regression/` | Run comparison of computed contract verdicts. |
| `internal/result/` | Shared verification result model. |
| `docs/` | CLI reference, contract semantics and development guide. |
| `examples/` | Synthetic input fixtures and an illustrative Progressing policy. |

Tests live alongside the code in `*_test.go` files. Keep command handling in `app` and reusable analysis in the domain packages. `contracts` uses `operator` and `upgrade`; those packages do not depend on the CLI. Shared runnable examples stay in `examples/`; future fixtures used only by one package's tests belong in that package's `testdata/` directory.

Pipeline: data → observations → validated timelines → upgrade phases → correlation → contract checks → evidence report. Computation accepts typed observations and is independent of JSONL readers and live collection.

## Limitations and next steps

Live recording uses two shared informers with an initial LIST then WATCH, resync disabled, per-resource UTC timestamps and JSONL sinks. Client-go handles ordinary renewal and resourceVersion recovery; real reconnect/relist, including transient Unauthorized recovery, was exercised with CRC stop/start. Expired-resourceVersion recovery has synthetic coverage, not a separate real-API validation. Run manifests support local validation with `verify-run`; `compare-runs` compares only conditions/version verdicts between two validated runs. Verification commands provide text output by default and an opt-in schema-versioned JSON report. Supplied histories must remain traceable to a comparable source run; the CLI cannot establish cluster provenance. Sampling gaps and clock differences limit inference.

Post-v0.1.0: broaden real disconnect coverage, independently validate expired resourceVersion on a real API, and consider explicit recovery/duration policies. kind can test client/watch mechanics; it does not substitute for OpenShift.

The manual [real-upgrade validation procedure](real-upgrade-validation.md) is
records both user-executed OKD upgrades. The 4.21 → 4.22 run validated final
capture on the real API and version consistency for all 34 recorded operators. Asynchronous watch streams can
leave a successful upgrade INCONCLUSIVE; the tool does not extrapolate missing
observations. It performs an explicit final GET/LIST/GET snapshot bracket on
graceful shutdown, including unchanged objects, before marking a run stopped. CRC supports recording but does not
support upgrading its OpenShift version.

Run loading and validation are shared by `verify-run` and `compare-runs` in
`internal/app/run_input.go`; `internal/regression` compares computed contract
reports without file or CLI access. Tests cover the full verdict matrix,
missing contracts, scope and precedence, ordering, input immutability,
persistent FAIL, different clusters/versions, text/JSON exits and output errors.

Shutdown tests cover SIGINT/SIGTERM, final GET/LIST/closing-GET and write errors,
unchanged resourceVersions, stream monotonicity, sorted operators and completed
version evaluation. Condition tests retain transient/recovered and unresolved
adverse evidence as INCONCLUSIVE; no duration threshold is inferred.
