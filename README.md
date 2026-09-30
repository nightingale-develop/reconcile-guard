# ReconcileGuard

An offline Go CLI for reproducible lifecycle analysis of OpenShift platform operators, with read-only live capture and recording. ReconcileGuard reads saved ClusterOperator and ClusterVersion observations, reconstructs upgrade phases and checks explicit behavior rules against supplied evidence.

**v0.1.0:** two user-executed OKD upgrades (4.20 → 4.21 and 4.21 → 4.22) exercised the recording and analysis pipeline. The second validated graceful final capture on the real API and operator-version consistency for all 34 recorded operators. CRC/OpenShift Local stop/start also validated reconnect/relist, including recovery from transient Unauthorized after startup. Included fixtures remain synthetic; these runs do not prove lossless delivery or exhaustive platform compatibility. See the [validation record](docs/real-upgrade-validation.md).

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

# Record live JSONL observations (LIST/WATCH plus GET ClusterVersion access)
./reconcile-guard record-live ./runs

# After stopping record-live with Ctrl+C, verify one finalized recording run.
RUN_DIR='./runs/REPLACE_WITH_RUN_ID'
./reconcile-guard verify-run "$RUN_DIR" --output text

# Compare two finalized runs
./reconcile-guard compare-runs ./baseline-runs/REPLACE_WITH_RUN_ID ./candidate-runs/REPLACE_WITH_RUN_ID --output text
```

The cluster example returns `Aggregate verdict: PASS` for two operators; the JSON example reports PASS for ingress. Both synthetic examples exit with code `0`. Live recording creates a timestamped run directory with `run.json`, `cluster-version.jsonl` and per-operator JSONL files; replay those generated paths or use `verify-run`. The run manifest validates local consistency, not cluster provenance or full-cluster coverage. `compare-runs` compares existing conditions/version contract verdicts; PASS means no detected regression, not candidate health. Persistent FAIL is unchanged, and an INCONCLUSIVE baseline cannot establish a regression. See the [CLI reference](docs/cli.md) for scope differences, output and exit codes.

## Interpreting results

Verification commands return `0` for PASS, `2` for FAIL, `3` for INCONCLUSIVE and `1` for input or usage errors. PASS applies only to the selected checks and supplied observations; it does not certify the whole upgrade or every operator in the cluster. The aggregate report includes condition and version checks; Progressing duration is evaluated separately.

Snapshot and replay commands have [their own exit-code semantics](docs/cli.md#exit-codes). In particular, `Degraded=False` does not establish overall health, and successful replay only means the data was processed.

## Documentation

- [CLI reference](docs/cli.md) — all commands, input format, examples and exit codes.
- [JSON output](docs/json-output.md) — schema, evidence fields and machine-readable reports.
- [Contracts and interpretation](docs/contracts.md) — phases, correlation, evidence requirements and verdict rules.
- [Development](docs/development.md) — package layout, tests, limitations and next steps.
- [Real upgrade validation](docs/real-upgrade-validation.md) — manual OpenShift/OKD procedure, first-run findings and limits.
- [Example Progressing policy](examples/progressing-policy.md) — illustrative limits and sampling assumptions.

## Current limits

Live recording now writes read-only ClusterVersion and ClusterOperator JSONL streams. Verification remains offline, and there is no automatic root-cause diagnosis. Client-go handles ordinary LIST/WATCH renewal and resourceVersion recovery; real reconnect/relist was exercised through CRC stop/start. Expired-resourceVersion recovery has separate synthetic coverage; it has not been independently demonstrated on a real API. Graceful shutdown makes fresh final reads before marking the run stopped. Adverse conditions during UPDATING are evidence for INCONCLUSIVE, not automatic violations. Gaps between snapshots limit what can be concluded.

Within each verification run, inputs must come from the same cluster; comparison permits different clusters and release targets. The CLI cannot establish that provenance. Reports cover only the supplied histories.

## License

[Apache License 2.0](LICENSE).
