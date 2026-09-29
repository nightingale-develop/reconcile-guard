# Synthetic upgrade observations

These are invented snapshots, not a real OpenShift capture. ClusterVersion
contains stable snapshots, two Partial observations, Completed history, then
stable snapshots. Generation/observedGeneration, images, conditions and history
are retained. The operator stream has independent timestamps and an old operator
version during rollout. Its new version is first observed after cluster
completion; this models delayed observation, not a guarantee that operators
update after the cluster. Additional operand versions are not release versions.
