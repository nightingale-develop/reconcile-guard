# ReconcileGuard

An offline Go CLI for reproducible lifecycle analysis of OpenShift platform operators. ReconcileGuard reads saved ClusterOperator and ClusterVersion observations, reconstructs upgrade phases and checks explicit behavior rules against the supplied evidence.

**Early prototype:** all included fixtures are synthetic; Live read-only capture has been validated against a real OpenShift cluster. Upgrade contract behavior has not yet been validated during a real cluster upgrade.

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

# Emit a machine-readable report instead of text
./reconcile-guard verify-upgrade \
  examples/cluster-version-history.jsonl \
  examples/ingress-upgrade-history.jsonl \
  --output json
```

The cluster example returns `Aggregate verdict: PASS` for two operators; the JSON example reports PASS for ingress. Both synthetic examples exit with code `0`. This is the offline MVP; real OpenShift validation remains future work.

## Interpreting results

Verification commands return `0` for PASS, `2` for FAIL, `3` for INCONCLUSIVE and `1` for input or usage errors. PASS applies only to the selected checks and supplied observations; it does not certify the whole upgrade or every operator in the cluster. The aggregate report includes condition and version checks; Progressing duration is evaluated separately.

Snapshot and replay commands have [their own exit-code semantics](docs/cli.md#exit-codes). In particular, `Degraded=False` does not establish overall health, and successful replay only means the data was processed.

## Documentation

- [CLI reference](docs/cli.md) — all commands, input format, examples and exit codes.
- [JSON output](docs/json-output.md) — schema, evidence fields and machine-readable reports.
- [Contracts and interpretation](docs/contracts.md) — phases, correlation, evidence requirements and verdict rules.
- [Development](docs/development.md) — package layout, tests, limitations and next steps.
- [Example Progressing policy](examples/progressing-policy.md) — illustrative limits and sampling assumptions.

## Current limits

There is no live collector, watch/reconnect or automatic root-cause diagnosis. Verification commands can emit a versioned machine-readable JSON report, but inputs must come from the same cluster and a comparable run; the CLI cannot establish that provenance. Gaps between snapshots limit what can be concluded.

## License

[Apache License 2.0](LICENSE).
