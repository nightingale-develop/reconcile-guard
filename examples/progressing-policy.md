# Example project policy

## Synthetic example

This policy is illustrative, not an OpenShift or Red Hat requirement, a recommended production timeout, or a validated operator SLO. Replace it with an approved policy for your environment before evaluating real data.

For the synthetic ingress operator and target version 4.20.0, this example sets a maximum observed Progressing run of 10 minutes and a maximum adjacent observation gap of 2 minutes in **both** timelines. These values are example project choices, not defaults built into the analyzer.

Within confidently correlated UPDATING intervals for the same target version and image, consecutive known Progressing samples no more than 2 minutes apart may be treated as a sampled run for this policy. This assumption does not establish physical continuity between snapshots. Missing/Unknown conditions, excessive gaps, ambiguous phases, or target changes break the run.

FAIL requires an observed True-to-True span strictly greater than the limit within such a run. PASS for a True episode requires preceding and following False observations in the same eligible segment and their total span no greater than the limit. Otherwise the episode is INCONCLUSIVE. No eligible samples is INCONCLUSIVE; all eligible False samples may pass only for the supplied observations. Supported FAIL overrides incomplete evidence elsewhere.

The source reference and applicability are user declarations; ReconcileGuard does not fetch or authenticate the policy document. Keep the policy and input histories with the report.
