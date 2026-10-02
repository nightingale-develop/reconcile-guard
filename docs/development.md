# Development

[README](../README.md) · [CLI](cli.md) · [Contracts](contracts.md)

```sh
gofmt -w .
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go build -o reconcile-guard ./cmd/reconcile-guard
git diff --check
```

The race detector needs a supported platform and C toolchain.

## Layout

| Path | Role |
| --- | --- |
| `cmd/reconcile-guard/` | Executable entry point. |
| `internal/app/` | Commands, live orchestration, verification and formatting. |
| `internal/collector/` | Read-only LIST/WATCH collection. |
| `internal/operator/` | ClusterOperator observations and readers. |
| `internal/machineconfig/` | MachineConfigPool history validation and lifecycle analysis. |
| `internal/node/` | Node history validation and lifecycle evidence analysis. |
| `internal/upgrade/` | ClusterVersion readers and phase reconstruction. |
| `internal/contracts/` | Correlation and contract evaluation. |
| `internal/policy/` | Versioned explicit lifecycle policy schema, parsing, validation and inheritance. |
| `internal/recording/` | Run manifests and JSONL persistence. |
| `internal/regression/` | Compare computed contract verdicts. |
| `internal/result/` | Shared verification output model. |
| `internal/timeline/` | Deterministic merged lifecycle timeline model and builder. |
| `examples/` | Synthetic histories and example policy. |

Pipeline: observations → validated timelines → phases → correlation → checks →
evidence report. Domain packages do not depend on the CLI.
The live commands share run creation/shutdown; the observer reuses AnalyzePhases
and verify-run's loader/checks. Collector owns API reads, not contract decisions.
Tests cover lifecycle analysis, MCP/Node evidence correlation, explicit policy parsing/evaluation, signals, final snapshots, exits and HTTP 401/403 on a fake API.

## Limits and next work

OpenShift integration has limited validation; kind is not an OpenShift
substitute. There is no root-cause diagnosis or provenance proof. Sampling gaps,
watch failures and clock differences remain material. Explicit lifecycle policies
have no built-in thresholds. Timeline and Markdown reporting are presentation
layers over recorded evidence and existing verdicts; they do not infer missing
state. Future work includes richer run-to-run regression analysis, real automatic
observer completion validation, additional expired-resourceVersion validation,
and release/compatibility automation. Run manifests, MCP/Node evidence analysis,
and comparable-run analysis are already implemented.
