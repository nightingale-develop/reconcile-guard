# JSON output

[README](../README.md) · [CLI](cli.md) · [Contracts](contracts.md)

Verification commands, `verify-run`, and `compare-runs` accept `--output json`
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
omitted. MCP/Node entries contain evidence-only lifecycle contracts; their verdicts do not
change the top-level aggregate operator verdict. JSON summarizes the selected
checks; it does not embed input snapshots.

`compare-runs` uses a `comparison` envelope with baseline/candidate summaries,
scope, contract changes and its aggregate verdict. It compares only condition and
version contracts. PASS means no detected regression, not candidate health.
Exit codes are `0` PASS, `2` FAIL, `3` INCONCLUSIVE and `1` for errors.
