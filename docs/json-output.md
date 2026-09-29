# JSON output

[README](../README.md) · [CLI](cli.md) · [Contracts](contracts.md)

Every `verify-*` command accepts `--output json` (or `--output=json`). The
default remains human-readable text, and the option is rejected by snapshot and
replay commands. JSON is written to stdout; errors remain on stderr.

The top-level document has schema version `"1"` (a string). This abbreviated
example omits contract details:

```json
{
  "schemaVersion": "1",
  "command": "verify-upgrade",
  "result": {
    "verdict": "PASS",
    "operators": [
      {
        "name": "ingress",
        "verdict": "PASS",
        "contracts": [
          {
            "name": "normal-upgrade-operator-conditions",
            "verdict": "PASS"
          }
        ]
      }
    ]
  }
}
```

The `result` object contains the aggregate verdict and operator results. Each
contract can include `details` with counts, values and flags, and `evidence`
with observations, intervals, expected/actual values, correlation, reasons,
messages, policy sources and contract-specific attributes. Empty optional fields
are omitted; envelope fields, verdicts, operator names and contract names remain.

JSON preserves the selected contract result; it does not collect new data or
embed the original input snapshots. Keep the source JSONL files with a report
when the evidence needs to be traced back to its observations. Exit codes stay
the same as text mode: `0` for PASS, `2` for FAIL, `3` for INCONCLUSIVE and `1`
for input, usage or output-encoding errors.

Regression coverage includes representative
PASS (`verify-upgrade`), FAIL (`verify-version-upgrade`) and INCONCLUSIVE
(`verify-progressing-upgrade`) results; its schema assertion compares against
the current implementation constant.

`verify-run --output json` uses the same envelope with `schemaVersion: "1"` and
`command: "verify-run"`; its result is the condition/version aggregate and does
not include Progressing duration. A recording that is `recording`, `failed`,
invalid, or inconsistent is an input error: diagnostics go to stderr, exit
code is `1`, and no verdict JSON is emitted.

`compare-runs --output json` uses `schemaVersion: "1"`, `command:
"compare-runs"` and a separate `comparison` envelope. It reports baseline and
candidate summaries, scope, sorted per-operator contract changes and the
aggregate verdict. It compares only conditions and version contracts;
Progressing, durations, samples and provenance are excluded.

The envelope has `comparison` instead of `result`. Each `baseline`/`candidate`
summary contains `runId`, `clusterId`, `finalDesiredVersion` and
`verificationVerdict`. The final desired version comes from the last
ClusterVersion observation; it does not certify completion.
`scope` contains `commonOperators` (a count), `baselineOnlyOperators` and
`candidateOnlyOperators` (sorted arrays). Common operators are sorted by name;
each includes its comparison `verdict`, `baselineVerdict`, `candidateVerdict`
and `contracts`. Contracts contain `name`, both source verdicts and `change`:
`UNCHANGED`, `REGRESSION`, `IMPROVEMENT` or `INCONCLUSIVE`. Conditions precede
version consistency. Missing contract verdicts are omitted, never reported as
PASS; empty scope arrays are `[]`.

Comparison PASS means no detected regression, not candidate verification PASS.
FAIL→FAIL is `UNCHANGED`; any INCONCLUSIVE side prevents claiming regression
for that contract. Exit codes remain `0`/`2`/`3` for comparison PASS/FAIL/INCONCLUSIVE
and `1` for errors. CLI regression tests assert the literal schema version `"1"`
and preserve both verification verdicts independently of the comparison.
