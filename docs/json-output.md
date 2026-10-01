# JSON output

[README](../README.md) · [CLI](cli.md) · [Contracts](contracts.md)

Verification commands, `verify-run`, `verify-lifecycle-policy`, and `compare-runs` accept `--output json`
or `--output=json`; text is the default. Snapshot/replay commands and
`observe-upgrade` are text-only. JSON is written to stdout and errors to stderr.

Verification documents use schema version `"1"`:

```json
{"schemaVersion":"1","command":"verify-upgrade","result":{"verdict":"PASS","operators":[]}}
```

Contracts may include counts, values, flags and evidence. `verify-run` may also
include optional `machineConfigPools` and `nodes` resource arrays alongside
`operators`; this is an additive schema-version-1 extension and older runs remain
valid. Evidence can carry
observations, intervals, expected/actual values, correlation, reasons, messages,
policy sources and contract-specific attributes. Empty optional fields are
omitted. MCP/Node entries emitted by `verify-run` contain evidence-only lifecycle contracts;
their verdicts do not change the top-level aggregate operator verdict.

`verify-lifecycle-policy` reuses the same `result` envelope but its top-level
verdict aggregates only the explicit policy contracts. Operator entries contain
condition-duration contracts; MCP and Node entries contain post-completion policy
contracts. Contract `details.values` include the declared threshold source and
durations, while evidence records the sampled episode or post-completion state.
A policy FAIL therefore means a supplied policy threshold was directly violated,
not that an OpenShift product guarantee was violated.

JSON summarizes the selected checks; it does not embed input snapshots.

`compare-runs` uses a `comparison` envelope with baseline/candidate summaries,
scope, contract changes and its aggregate verdict. It compares only condition and
version contracts. PASS means no detected regression, not candidate health.
Exit codes are `0` PASS, `2` FAIL, `3` INCONCLUSIVE and `1` for errors.
