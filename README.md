# ReconcileGuard

ReconcileGuard is an early Go CLI prototype for diagnosing Red Hat OpenShift operators from a saved `ClusterOperator` JSON object.

The project's long-term goal is to help investigate operators that fail to reach a stable state, reconcile unnecessarily, generate excessive Kubernetes API traffic, or exhibit anomalous state transitions during normal operation and cluster upgrades. The current prototype only evaluates the reported `Degraded` condition in a single JSON snapshot.

## Current capabilities

- Commands: `version`, `help`, and `check <file>`.
- Read one local JSON object with `apiVersion: config.openshift.io/v1` and `kind: ClusterOperator`.
- Require a non-empty operator name.
- Evaluate `Degraded=True`, `False`, or `Unknown`, and report a missing `Degraded` condition.
- Reject duplicate `Degraded` conditions and invalid `Degraded` status values.
- Print the reason and message when the operator reports `Degraded=True`.
- Return diagnostic exit codes suitable for shell scripts.

## Build and run

The module currently requires Go 1.27.1 or later. It uses only the Go standard library.

```sh
git clone https://github.com/nightingale-develop/reconcile-guard.git
cd reconcile-guard
go build -o reconcile-guard .
./reconcile-guard version
./reconcile-guard help
./reconcile-guard check examples/ingress-ok.json
```

The version command currently prints `ReconcileGuard v0.1.0-dev`.

## Example diagnosis

The files in `examples/` are sample inputs, not evidence of a live cluster test. To inspect the degraded example:

```sh
./reconcile-guard check examples/ingress.json
echo $?
```

Output:

```text
Operator: ingress
Degraded: True
Reason: RouterDeploymentUnavailable
Message: One router replica is unavailable
Result: DEGRADED
2
```

The non-degraded example, `examples/ingress-ok.json`, prints `Result: NOT DEGRADED (reported)` and exits with code `0`.

### Exit codes

| Code | Meaning |
| --- | --- |
| `0` | `check`: `Degraded=False` (reported). Also returned by `help`, `version`, and invocation without arguments. |
| `1` | Command usage error, file read error, malformed JSON, unsupported resource, missing name, duplicate `Degraded`, or invalid `Degraded` status. |
| `2` | `Degraded=True`. |
| `3` | `Degraded=Unknown` or no `Degraded` condition. |

Diagnostics are written to standard output; errors are written to standard error. A reported non-degraded condition does not establish overall operator health or stability.

## Development and tests

```sh
go fmt ./...
go vet ./...
go test -count=1 -v ./...
go test -race -count=1 ./...
go build -o reconcile-guard .
```

The race detector requires a supported platform and a working C compiler with cgo enabled. Unit tests cover all three supported `Degraded` statuses, a missing condition, a duplicate condition, an invalid status, and malformed JSON (`TestCheckFileRejectsInvalidJSON`).

## Limitations

- No OpenShift API connection, authentication, discovery, or live monitoring.
- No reconciliation loop detection, API operation counting, or analysis over time.
- No validation of `Available`, `Progressing`, or other condition types; duplicate detection applies only to `Degraded`.
- No full Kubernetes schema validation. Unknown JSON fields are ignored.
- Input must be one `ClusterOperator` object, not a resource list.
- No assessment of cluster upgrade safety or compatibility certification against OpenShift versions.

## Roadmap

The following capabilities are planned, not implemented:

- Connect to the OpenShift API and collect operator state.
- Observe state transitions over time and define stability criteria.
- Investigate repeated reconciliation and excessive Kubernetes API operations using appropriate telemetry.
- Compare behavior before, during, and after cluster upgrades.
- Produce richer diagnostic reports and support CI workflows.

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE).
