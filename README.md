# ReconcileGuard

ReconcileGuard is an open-source Go CLI for analyzing the lifecycle of OpenShift platform operators.

The long-term goal is to verify how OpenShift operators behave before, during, and after platform upgrades by reconstructing their state timelines and checking documented lifecycle contracts.

The project is currently an early offline prototype. It works with saved `ClusterOperator` data and does not require a running OpenShift cluster.

## Features

ReconcileGuard currently supports:

- Inspecting a saved OpenShift `ClusterOperator` JSON object.
- Evaluating the reported `Degraded` condition.
- Replaying a sequence of operator observations from JSONL.
- Detecting changes in `Available`, `Progressing`, and `Degraded`.
- Detecting missing conditions that prevent reliable comparison.
- Validating chronological order of observations.
- Reporting transition intervals without inventing an exact transition time.

It does **not** currently determine whether an operator behaved correctly during an OpenShift upgrade.

## Build

Requires Go 1.27.1 or later.

```sh
git clone https://github.com/nightingale-develop/reconcile-guard.git
cd reconcile-guard

go build -o reconcile-guard .
```

Check the CLI:

```sh
./reconcile-guard help
./reconcile-guard version
```

## Inspect a ClusterOperator

The files under `examples/` contain synthetic OpenShift data.

```sh
./reconcile-guard check examples/ingress.json
```

Example output:

```text
Operator: ingress
Degraded: True
Reason: RouterDeploymentUnavailable
Message: One router replica is unavailable
Result: DEGRADED
```

A reported `Degraded=False` condition only means that the operator is not reporting itself as degraded. It does not prove overall operator health.

## Inspect a ClusterVersion

```sh
./reconcile-guard check-version examples/cluster-version.json
```

This offline command reads an official `configv1.ClusterVersion` JSON snapshot and reports `status.desired` (not `spec.desiredUpdate`), conditions, and update history in supplied order. The example is synthetic. Missing fields remain unreported; no upgrade phase or correctness is inferred. Condition types must be non-empty and unique, with `True`, `False`, or `Unknown` statuses. This is not full API-schema or update-history validation.

Exit `0` means successful processing, not PASS; input/usage errors return `1`. Output ends with `Verdict: NOT EVALUATED (snapshot report only)`.

A timestamped ClusterVersion history can also be replayed from JSONL:

```sh
./reconcile-guard replay-version examples/cluster-version-history.jsonl
```

Example output:

```text
ClusterVersion: version
Observations: 3
Desired version: "4.20.0"
Verdict: NOT EVALUATED (ClusterVersion summary only)
```

`replay-version` validates the observation order and reports the desired version from the latest snapshot. It does not infer upgrade phases yet.

## Replay operator history

A JSONL history contains one observation per line.

```sh
./reconcile-guard replay examples/ingress-history.jsonl
```

Example:

```text
Operator: ingress
Observations: 3
Observed transitions: 2
  Progressing: False -> True (between 2026-09-19T10:00:00Z and 2026-09-19T10:05:00Z)
  Progressing: True -> False (between 2026-09-19T10:05:00Z and 2026-09-19T10:10:00Z)
Uncompared adjacent condition pairs: 0
Verdict: NOT EVALUATED (transition report only)
```

A transition means that two adjacent observations reported different values.

For example:

```text
10:00  Progressing=False
10:05  Progressing=True
```

ReconcileGuard knows that the reported status changed sometime between those observations. It does not claim to know the exact moment when the change occurred.

`observedAt` is the time when the snapshot was captured. It is intentionally separate from OpenShift's `lastTransitionTime`.

## Exit codes

For `check`:

| Code | Meaning |
| --- | --- |
| `0` | `Degraded=False` |
| `1` | Invalid input or command error |
| `2` | `Degraded=True` |
| `3` | `Degraded=Unknown` or missing |

For `replay` and `replay-version`, exit code `0` currently means that the history was successfully processed. It is **not** a health or upgrade-contract verdict.

## Development

```sh
go fmt ./...
go vet ./...
go test -count=1 -v ./...
go test -race -count=1 ./...
go build -o reconcile-guard .
```

Tests cover ClusterOperator and ClusterVersion snapshots, JSONL history parsing, transition detection, chronological validation, malformed input, duplicate conditions, and invalid statuses.

## Current limitations

ReconcileGuard currently:

- Works only with offline data.
- Does not connect to an OpenShift cluster.
- Can validate and summarize ClusterVersion observation timelines, but does not reconstruct upgrade phases yet.
- Does not evaluate lifecycle contracts.
- Does not perform root-cause analysis.
- Has not yet been validated against a real OpenShift environment.

ClusterOperator and ClusterVersion use official `openshift/api/config/v1` Go types.

## Roadmap

Next steps:

1. Reconstruct upgrade phases from ClusterVersion observations.
2. Correlate operator timelines with OpenShift upgrade phases.
3. Introduce evidence-based lifecycle contract checks.
4. Add `PASS`, `FAIL`, and `INCONCLUSIVE` results.
5. Validate the tool against a real OpenShift environment.

The focus is OpenShift operator lifecycle analysis rather than generic Kubernetes chaos testing or reconciliation-loop detection.

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE).
