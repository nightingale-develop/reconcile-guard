# Real upgrade validation

This procedure requires an existing OKD/OpenShift test cluster, a read-only
account, and the matching `oc` client. ReconcileGuard does not start the
upgrade. CRC/OpenShift Local can exercise recording and reconnect behavior, but
does not support upgrading its OpenShift version.

Previous OKD runs exercised recording, final capture and operator-version checks;
CRC stop/start exercised reconnect/relist, including transient Unauthorized recovery.
Real expired-resourceVersion recovery was not independently demonstrated.
These runs did not validate automatic
`observe-upgrade` completion. A real observer run is required before claiming
that behavior is validated. These observations do not prove lossless delivery,
complete coverage, causality, or platform compatibility.

## Before the upgrade

```sh
oc whoami
oc config current-context
oc get clusterversion version
oc get clusteroperators
oc get nodes
oc get machineconfigpools
oc adm upgrade
oc auth can-i get clusterversions.config.openshift.io/version
oc auth can-i list clusterversions.config.openshift.io
oc auth can-i watch clusterversions.config.openshift.io
oc auth can-i list clusteroperators.config.openshift.io
oc auth can-i watch clusteroperators.config.openshift.io
```

Review Available, Progressing, Degraded and Upgradeable conditions, nodes and
machine config pools. Do not force an unsupported target for this validation.

## Observe and start the upgrade

In one terminal, start the observer:

```sh
./reconcile-guard observe-upgrade ./runs --kubeconfig "$HOME/.kube/config"
echo $?  # 0 PASS, 1 error, 2 FAIL, 3 INCONCLUSIVE
```

In another terminal, check that the printed run directory contains initial JSONL
files, then start the upgrade as administrator through the platform's documented
procedure (select a recommended target from `oc adm upgrade`):

```sh
TARGET_VERSION='REPLACE_WITH_RECOMMENDED_TARGET'
oc adm upgrade --to="$TARGET_VERSION"
oc get clusterversion version -w
# Inspect detailed reported state separately:
oc get clusterversion version -o json
oc get clusteroperators -o json
```

The observer records immediately and stops only when its phase
analyzer observes COMPLETED for the active target version and image. Target
changes, unknown gaps, API errors, or missed transitions can require Ctrl+C. A
manual stop still performs final capture and verifies saved evidence, but reports
that live completion was not observed.

## Preserve and inspect the run

```sh
jq '{runId,status,source,operators,error}' ./runs/REPLACE_WITH_RUN_ID/run.json
./reconcile-guard replay-version ./runs/REPLACE_WITH_RUN_ID/cluster-version.jsonl
./reconcile-guard verify-run ./runs/REPLACE_WITH_RUN_ID --output text
./reconcile-guard verify-run ./runs/REPLACE_WITH_RUN_ID --output json > verification.json
# Optional: compare against a previous stopped recording
./reconcile-guard compare-runs ./baseline/REPLACE_WITH_RUN_ID ./runs/REPLACE_WITH_RUN_ID
```

The run must be `stopped`. `record-live` exit0 only confirms clean persistence;
`observe-upgrade` returns the verification verdict after finalization. Preserve
the manifest, JSONL files, reports, tool
revision, source/target releases and any errors.

## Interpretation

Phase names are analytical. `observedAt` is local capture time and differs
from OpenShift `lastTransitionTime`. Independent watches, stale generations,
target changes, clock skew, and missing operator samples can leave a completed
upgrade INCONCLUSIVE. Final GET ClusterVersion → LIST ClusterOperator → GET
ClusterVersion reads provide a temporal bracket; they do not recreate missed
events or prove full-cluster health. Do not edit timestamps or append guessed
state.

For release-specific prerequisites, follow the official documentation for the
installed platform and release. Treat this procedure as an observation aid,
not an upgrade acceptance test.
