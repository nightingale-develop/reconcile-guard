# ReconcileGuard

An offline Go CLI for reproducible lifecycle analysis of OpenShift platform operators. ReconcileGuard reads saved ClusterOperator and ClusterVersion observations, reconstructs upgrade phases and checks explicit behavior rules against the supplied evidence.

**Early prototype:** all included fixtures are synthetic; compatibility with a real OpenShift cluster has not been validated.

## What it does

- Inspects snapshots and replays reported condition changes.
- Checks Available and Degraded conditions during observed upgrades.
- Checks Progressing duration against an explicit project policy.
- Compares reported operator versions with completed upgrade targets.
- Combines condition and version checks into a report for multiple supplied operators.

## Quick start

Requires **Go 1.27.1 or later**. Run from the repository root:

```sh
go build -o reconcile-guard ./cmd/reconcile-guard
./reconcile-guard help
./reconcile-guard verify-cluster-upgrade \
  examples/cluster-version-history.jsonl \
  examples/ingress-version-history.jsonl \
  examples/network-version-history.jsonl
```

This synthetic example returns `Aggregate verdict: PASS` for two operators, with exit code `0`.

## Interpreting results

Verification commands return `0` for PASS, `2` for FAIL, `3` for INCONCLUSIVE and `1` for input or usage errors. PASS applies only to the selected checks and supplied observations; it does not certify the whole upgrade or every operator in the cluster. The aggregate report includes condition and version checks; Progressing duration is evaluated separately.

Snapshot and replay commands have [their own exit-code semantics](docs/cli.md#exit-codes). In particular, `Degraded=False` does not establish overall health, and successful replay only means the data was processed.

## Documentation

- [CLI reference](docs/cli.md) — all commands, input format, examples and exit codes.
- [Contracts and interpretation](docs/contracts.md) — phases, correlation, evidence requirements and verdict rules.
- [Development](docs/development.md) — package layout, tests, limitations and next steps.
- [Example Progressing policy](examples/progressing-policy.md) — illustrative limits and sampling assumptions.

## Current limits

There is no live collector, watch/reconnect, automatic root-cause diagnosis or machine-readable report. Inputs must come from the same cluster and a comparable run; the CLI cannot establish that provenance. Gaps between snapshots limit what can be concluded.

## License

[Apache License 2.0](LICENSE).
