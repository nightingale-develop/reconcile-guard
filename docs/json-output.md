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

The current working tree also contains regression coverage for representative
PASS (`verify-upgrade`), FAIL (`verify-version-upgrade`) and INCONCLUSIVE
(`verify-progressing-upgrade`) results. That file is not part of the JSON
implementation commit yet; its schema assertion compares against the current
implementation constant.
