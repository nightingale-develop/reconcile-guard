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
| `docs/` | CLI reference, contract semantics and development guide. |
| `examples/` | Synthetic input fixtures and an illustrative Progressing policy. |

Tests live alongside the code in `*_test.go` files. Keep command handling in `app` and reusable analysis in the domain packages. `contracts` uses `operator` and `upgrade`; those packages do not depend on the CLI. Shared runnable examples stay in `examples/`; future fixtures used only by one package's tests belong in that package's `testdata/` directory.

Pipeline: data → observations → validated timelines → upgrade phases → correlation → contract checks → evidence report. Computation accepts typed observations and is independent of JSONL readers or a future collector.

## Limitations and next steps

There is no live collector, watch/reconnect, automatic root-cause analysis or real OpenShift integration validation. Implemented contracts cover normal-upgrade conditions, policy-defined Progressing duration and post-completion operator version consistency. Verification commands provide text output by default and an opt-in schema-versioned JSON report with contract details and evidence. Multi-operator reports aggregate conditions and version checks for the supplied histories only. Data must come from the same cluster and a comparable run; resource names alone cannot prove this. Sampling gaps and clock differences limit inference.

Next: stronger evidence, read-only collection, watch/reconnect, real OpenShift/OKD validation and comparable-run regression analysis. kind can test Kubernetes client/watch mechanics; it does not substitute for OpenShift.
