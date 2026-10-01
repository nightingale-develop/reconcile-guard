# ReconcileGuard

Read-only OpenShift lifecycle analysis. The CLI records ClusterVersion,
ClusterOperator, MachineConfigPool, and Node observations, reconstructs upgrade
phases, and verifies saved runs offline. It never starts an upgrade.

Included fixtures are synthetic. Live recording has been exercised on OKD and
OpenShift Local (CRC), including ClusterVersion/ClusterOperator/MachineConfigPool/
Node LIST/WATCH recording and final snapshots. v0.3.0 adds offline MCP and Node
lifecycle analysis and conservative post-completion evidence checks. These checks
do not prove lossless delivery, complete cluster coverage, or broad platform
compatibility. See [real validation](docs/real-upgrade-validation.md).

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
```

`observe-upgrade` records immediately, waits for the phase analyzer to observe
COMPLETED for the active desired version and image, performs final reads, and
verifies the saved run. Ctrl+C saves a partial run without claiming live
completion was observed. `record-live` only records.

Both commands record ClusterVersion, ClusterOperator, MachineConfigPool, and
Node observations through read-only LIST/WATCH access and perform a final
snapshot before run finalization. `verify-run` now analyzes recorded MCP and Node
histories as lifecycle evidence and correlates them conservatively with the
ClusterVersion timeline.

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
- [Example Progressing policy](examples/progressing-policy.md)

## Limits

Verification is offline and does not diagnose root causes. Sampling gaps,
ambiguous phases, target changes, and clock differences can produce
INCONCLUSIVE results. A run manifest checks local consistency; it does not prove
cluster provenance or full-cluster coverage. `compare-runs` reports detected
contract regressions, not candidate health.
MachineConfigPool and Node results are evidence-only in v0.3.0. They can report
PASS when post-completion convergence is directly supported by recorded evidence,
but they do not create FAIL and do not change the aggregate ClusterOperator
verification verdict.

## License

[Apache License 2.0](LICENSE).
