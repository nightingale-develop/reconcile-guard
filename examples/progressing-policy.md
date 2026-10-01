# Example Progressing policy

## Synthetic example

This synthetic policy is a project example, not an OpenShift requirement or a
production SLO. It sets a 10-minute maximum observed Progressing span and a
2-minute maximum adjacent gap in both timelines for ingress targeting 4.20.0.

Only confidently correlated UPDATING samples for the same target version and
image qualify. Missing/unknown conditions, larger gaps, ambiguous phases and
target changes break the segment. An observed True span over 10 minutes is FAIL;
a True episode needs surrounding False samples within the limit to PASS;
otherwise it is INCONCLUSIVE. No eligible samples is INCONCLUSIVE.

The policy source and applicability are user declarations. ReconcileGuard does
not fetch or authenticate them. Keep the policy and input histories with the
report.
