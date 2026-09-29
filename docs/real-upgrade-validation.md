# Real upgrade validation

Status: **pending a user-executed real upgrade**. Synthetic tests and fake
LIST/WATCH tests do not establish real OpenShift/OKD upgrade or reconnect
validation. ReconcileGuard only reads the API; the administrator starts upgrades.

Use an existing OKD/OpenShift test cluster with a recommended update path and
the matching `oc` client. Follow the prerequisites for your installed release:
[OpenShift 4.20](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/updating_clusters/performing-a-cluster-update)
or [OKD 4.20](https://docs.okd.io/4.20/updating/updating_a_cluster/updating-cluster-cli.html).
CRC can exercise recording, but [CRC does not support upgrading its OpenShift
version](https://crc.dev/docs/introducing/); use OKD/OpenShift for this validation.

## 1. Inspect health and upgrade eligibility

```sh
oc whoami
oc config current-context
oc get clusterversion version
oc get clusteroperators
oc get nodes
oc get machineconfigpools
oc adm upgrade
oc get clusterversion version -o json | jq '{generation:.metadata.generation, observedGeneration:.status.observedGeneration, desired:.status.desired, conditions:.status.conditions, history:.status.history}'
oc get clusteroperators -o json | jq '.items[] | {name:.metadata.name, conditions:.status.conditions, versions:.status.versions}'
```

Investigate unhealthy nodes/operators, incomplete rollouts and blocked updates
before proceeding. Review Available/Progressing/Degraded and Upgradeable
conditions, plus ClusterVersion failure messages. A single condition is not a
complete health check. Select a recommended target from `oc adm upgrade`; do
not force an unsupported update for this test.

The recorder identity needs read-only LIST/WATCH access:

```sh
oc auth can-i list clusterversions.config.openshift.io
oc auth can-i watch clusterversions.config.openshift.io
oc auth can-i list clusteroperators.config.openshift.io
oc auth can-i watch clusteroperators.config.openshift.io
```

## 2. Record before starting the upgrade

Terminal A, from the repository root:

```sh
./reconcile-guard record-live ./runs --kubeconfig "$HOME/.kube/config"
```

Copy the printed `Output:` path. In terminal B set it explicitly and confirm
initial observations exist before requesting the update:

```sh
RUN_DIR='./runs/REPLACE_WITH_PRINTED_RUN_ID'
find "$RUN_DIR" -maxdepth 2 -type f | sort
wc -l "$RUN_DIR/cluster-version.jsonl" "$RUN_DIR"/operators/*.jsonl
head -n 1 "$RUN_DIR/cluster-version.jsonl" | jq '.clusterVersion.status'
oc get clusteroperators -o name
```

Check the initial operator files against the current operator list. The startup
message alone does not confirm both asynchronous initial LISTs have completed.
Keep terminal A running, with reliable connectivity, disk space and local clock.

## 3. Start and monitor the real upgrade manually

In terminal B, using the same cluster/context and an administrator identity:

```sh
TARGET_VERSION='REPLACE_WITH_RECOMMENDED_VERSION'
oc adm upgrade --to="$TARGET_VERSION"
oc get clusterversion version -w
```

Ctrl+C in terminal B stops only this `oc` watch. Inspect repeatedly:

```sh
oc get clusteroperators
oc get nodes
oc get machineconfigpools
oc get clusterversion version -o json | jq --arg target "$TARGET_VERSION" '{target:$target, desired:.status.desired, generation:.metadata.generation, observedGeneration:.status.observedGeneration, latestHistory:.status.history[0], conditions:.status.conditions}'
```

Confirm the requested target is reflected in `status.desired` and the newest
history entry, its state is `Completed` with a non-null `completionTime`,
generation is observed, and Available=True/Progressing=False. Recheck operator
versions/conditions and cluster health. Progressing=False alone can describe
the old stable state before the request is processed. Follow release-specific
completion checks for nodes and machine config pools as well.

Keep recording through post-completion observations, then Ctrl+C in terminal A:

```sh
echo $?  # immediately after record-live: expect 0
```

## 4. Validate and preserve the run

In terminal B:

```sh
jq '{runId,status,source,operators,error}' "$RUN_DIR/run.json"
./reconcile-guard replay-version "$RUN_DIR/cluster-version.jsonl"
./reconcile-guard verify-run "$RUN_DIR" --output text
echo $?  # 0 PASS, 1 input error, 2 FAIL, 3 INCONCLUSIVE
./reconcile-guard verify-run "$RUN_DIR" --output json > "$RUN_DIR/verification.json"
echo $?
# Optional: a previous stopped upgrade recording, not just a pre-upgrade snapshot
BASELINE='./runs/REPLACE_WITH_BASELINE_RUN_ID'
./reconcile-guard compare-runs "$BASELINE" "$RUN_DIR" --output json > "$RUN_DIR/comparison.json"
echo $?
```

`run.json` must be `stopped`; recorder exit 0 only confirms clean shutdown.
Preserve the manifest, all JSONL files, reports, tool revision, actual source and
target releases, and any recorder errors. Comparison PASS means no detected
regression; persistent FAIL is unchanged and candidate verification may fail.

## Interpretation and limits

The [ClusterVersion API](https://github.com/openshift/api/blob/release-4.20/config/v1/types_cluster_version.go)
defines newest-first history and generation freshness. Partial is not success;
older Partial entries can have completion timestamps when superseded. The
analyzer needs consistent desired/history evidence and observed generation.
STABLE → UPDATING → COMPLETED → STABLE are analytical phases, not API conditions.
Missing or contradictory evidence remains UNKNOWN/INCONCLUSIVE.

The [ClusterOperator API](https://github.com/openshift/api/blob/release-4.20/config/v1/types_cluster_operator.go)
allows Progressing during rollout. An evaluated Degraded=True violates the
normal-upgrade contract even if it later clears. `versions[name=operator]`
tracks operand rollout; it may change before global completion. A first recorded
new version after completion can be delivery timing, not actual update order.

Independent WATCH streams have different local `observedAt` times. Phase/target
boundaries, stale generations and observations outside the CV timeline can
make a successful real upgrade INCONCLUSIVE. No last-value extrapolation is
performed. Recording longer does not force new snapshots: unchanged objects
are deduplicated. A post-completion version check needs operator evidence within
the recorded completion/stable window. Do not edit timestamps, inject duplicate
snapshots or weaken contracts to manufacture PASS.

This exercise does not by itself validate disconnect, expired-resourceVersion
recovery or lossless event delivery. Client-go handles LIST/WATCH renewal and
resourceVersion recovery; real fault scenarios remain a separate validation.
