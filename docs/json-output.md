# JSON output

[README](../README.md) · [CLI](cli.md) · [Contracts](contracts.md)

Verification commands, `verify-run`, `verify-lifecycle-policy`, `timeline-run`, and `compare-runs` accept `--output json`
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

## Timeline JSON

`timeline-run` uses a separate schema-version-`"1"` document because it reports
recorded lifecycle ordering rather than a verification verdict:

```json
{
  "schemaVersion": "1",
  "command": "timeline-run",
  "runId": "20261001T120907.175149978Z",
  "clusterId": "example-cluster-id",
  "timeline": {
    "events": [
      {
        "kind": "cluster-version-state",
        "resourceKind": "ClusterVersion",
        "resourceName": "version",
        "observedAt": "2026-10-01T12:09:07Z",
        "summary": "phase UPDATING, desired version 4.21.0"
      },
      {
        "kind": "operator-condition-transition",
        "resourceKind": "ClusterOperator",
        "resourceName": "ingress",
        "from": "2026-10-01T12:09:10Z",
        "to": "2026-10-01T12:09:20Z",
        "summary": "Available True -> False"
      }
    ]
  }
}
```

An event has either an exact `observedAt` or observation bounds (`from`/`to`).
The latter are not converted into an inferred exact transition timestamp.
`attributes` contains event-specific machine-readable fields. JSON events are
sorted deterministically by their observed or upper-bound time and then resource
identity.

`report-run` produces Markdown rather than a JSON envelope. Use
`timeline-run --output json`, `verify-run --output json`, and
`verify-lifecycle-policy --output json` when machine-readable output is required.
