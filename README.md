# ReconcileGuard

Read-only OpenShift lifecycle analysis. The CLI records ClusterVersion,
ClusterOperator, MachineConfigPool, and Node observations, reconstructs upgrade
phases, and verifies saved runs offline. It never starts an upgrade.

Included fixtures are synthetic. Live recording has been exercised on OKD and
OpenShift Local (CRC), including ClusterVersion/ClusterOperator/MachineConfigPool/
Node LIST/WATCH recording and final snapshots. Offline verification includes MCP
and Node lifecycle evidence, conservative upgrade correlation, and optional
user-supplied lifecycle policies. ReconcileGuard ships no hidden lifecycle
thresholds. Unified timelines and Markdown reports can be rendered from stopped
runs without changing verification semantics. These checks do not prove lossless
delivery, complete cluster coverage, or broad platform compatibility. See
[real validation](docs/real-upgrade-validation.md).

## Quick start

Requires Go 1.27.1 or later.

```sh
go build -o reconcile-guard ./cmd/reconcile-guard
./reconcile-guard help
# Start the upgrade separately as administrator after initial files appear.
./reconcile-guard observe-upgrade ./runs
# Offline example:
./reconcile-guard verify-cluster-upgrade \
  examples/cluster-version-history.jsonl \
  examples/ingress-version-history.jsonl \
  examples/network-version-history.jsonl
./reconcile-guard record-live ./runs
# After stopping record-live with Ctrl+C:
./reconcile-guard verify-run ./runs/REPLACE_WITH_RUN_ID --output text
# Apply explicit project thresholds to a stopped run:
./reconcile-guard verify-lifecycle-policy \
  ./runs/REPLACE_WITH_RUN_ID \
  examples/lifecycle-policy.yaml --output text
# Merge recorded lifecycle transitions into one timeline:
./reconcile-guard timeline-run ./runs/REPLACE_WITH_RUN_ID
# Render a Markdown report, optionally including policy results:
./reconcile-guard report-run ./runs/REPLACE_WITH_RUN_ID \
  --policy examples/lifecycle-policy.yaml --file report.md
# Compare two stopped runs; policy comparison is optional:
./reconcile-guard compare-runs \
  ./baseline/REPLACE_WITH_RUN_ID \
  ./runs/REPLACE_WITH_RUN_ID
```

`observe-upgrade` records immediately, stops after the phase analyzer observes
COMPLETED for the active version and image, performs final reads, and verifies
the saved run. Ctrl+C saves a partial run without claiming live completion.
`record-live` only records. `verify-run` provides base operator checks plus
evidence-only MCP/Node checks. `verify-lifecycle-policy` applies explicit user
thresholds; `timeline-run` merges recorded transitions; `report-run` renders
the same evidence as Markdown.

Verification and `observe-upgrade` return `0` for PASS, `2` for FAIL, `3` for
INCONCLUSIVE, and `1` for errors. PASS covers only the selected checks and supplied
observations. `Degraded=False` and successful replay do not establish overall
health.

## Documentation

- [CLI reference](docs/cli.md)
- [Contracts and interpretation](docs/contracts.md)
- [JSON output](docs/json-output.md)
- [Development](docs/development.md)
- [Real upgrade validation](docs/real-upgrade-validation.md)
- [Example lifecycle policy](examples/lifecycle-policy.md)
- [Example Progressing policy](examples/progressing-policy.md)

## Limits

Verification is offline and does not diagnose root causes. Sampling gaps,
ambiguous phases, target changes, and clock differences can produce
INCONCLUSIVE results. A run manifest checks local consistency; it does not prove
cluster provenance or full-cluster coverage. `compare-runs` compares operator
contracts, reports MCP/Node evidence and recorded timing deltas, and can compare
explicit lifecycle-policy verdicts when `--policy` is supplied. Descriptive
timing and evidence differences do not become regressions by themselves.
The base `verify-run` MCP/Node checks remain evidence-only and do not change the
aggregate operator verdict. Policy FAILs require thresholds in the supplied
policy. Timeline and Markdown rendering do not add verdict semantics.

## License

[Apache License 2.0](LICENSE).
