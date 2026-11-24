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

## Release version check

`internal/version.Current` is the application version source for both the CLI
`version` command and new run manifests' `toolVersion`. Keep the constant in sync
with the intended `v<version>` release tag. JSON schema and policy API versions
are independent; older recordings retain their original tool version.

There is no repository CI or automated release workflow. Before creating a tag
or publishing artifacts, set `RELEASE_TAG` to the intended tag and run this gate
from the source revision being released:

```sh
go run ./internal/releasecheck "$RELEASE_TAG" &&
  go build -o reconcile-guard ./cmd/reconcile-guard
```

The check exits `1` for a mismatch, an empty tag, or invalid arguments. It accepts
only the exact `v<version>` tag, not a `git describe` value with a dirty suffix.
It does not create or change tags. Any future release automation must run this
gate with the release tag before building or publishing artifacts.

Ordinary local builds and tests require no release tag or Git metadata. The check
uses the source constant directly and needs no network or cluster access. To
validate an existing tag's source, run it from that tagged revision when the
checker is available; checking a modified working tree does not validate the
unchanged source behind an existing tag.

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
| `internal/regression/` | Compare verification/policy verdicts and descriptive lifecycle timing. |
| `internal/result/` | Shared verification output model. |
| `internal/timeline/` | Deterministic merged lifecycle timeline model and builder. |
| `examples/` | Synthetic histories and example policy. |

Pipeline: observations → validated timelines → phases → correlation → checks →
evidence report. Domain packages do not depend on the CLI.
The live commands share run creation/shutdown; the observer reuses AnalyzePhases
and verify-run's loader/checks. Collector owns API reads, not contract decisions.
Tests cover lifecycle analysis, MCP/Node evidence correlation, explicit policy parsing/evaluation, signals, final snapshots, exits and HTTP 401/403 on a fake API.

## Limits

OpenShift integration has limited validation; kind is not an OpenShift
substitute. There is no root-cause diagnosis or provenance proof. Sampling gaps,
watch failures and clock differences remain material. Explicit lifecycle policies
have no built-in thresholds. Timeline and Markdown reporting are presentation
layers over recorded evidence and existing verdicts; they do not infer missing
state. Run-to-run comparison includes operator contracts, evidence-only MCP/Node
differences, descriptive observed timing, and optional explicit-policy
regressions. Real OKD SNO validation includes automatic observer completion;
see [real upgrade validation](real-upgrade-validation.md) for the tested scope.
Expired-resourceVersion recovery has not been independently validated on a real API.
