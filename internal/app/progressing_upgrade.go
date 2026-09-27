package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/contracts"
	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
)

func (c cli) verifyUpgradeProgressing(args []string) int {
	if len(args) != 4 {
		fmt.Fprintln(c.stderr, "Usage: reconcile-guard verify-progressing-upgrade <cluster-version-history.jsonl> <operator-history.jsonl> <policy.json>")
		return 1
	}
	policy, err := readProgressingPolicy(args[3])
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}
	versions, err := upgrade.ReadHistory(args[1])
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}
	observations, err := operator.ReadHistory(args[2])
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}
	report, err := contracts.VerifyUpgradeProgressing(versions, observations, policy)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}
	c.printUpgradeProgressing(report)
	return contractExitCode(report.Verdict)
}

func readProgressingPolicy(path string) (contracts.ProgressingPolicy, error) {
	var input struct {
		MaxDuration       string `json:"maxDuration"`
		MaxObservationGap string `json:"maxObservationGap"`
		Operator          string `json:"operator"`
		TargetVersion     string `json:"targetVersion"`
		Source            string `json:"source"`
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return contracts.ProgressingPolicy{}, fmt.Errorf("read policy: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return contracts.ProgressingPolicy{}, fmt.Errorf("decode policy: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return contracts.ProgressingPolicy{}, fmt.Errorf("policy must contain one JSON object")
	}
	limit, err := time.ParseDuration(input.MaxDuration)
	if err != nil || limit <= 0 {
		return contracts.ProgressingPolicy{}, fmt.Errorf("policy maxDuration must be a positive Go duration")
	}
	gap, err := time.ParseDuration(input.MaxObservationGap)
	if err != nil || gap <= 0 {
		return contracts.ProgressingPolicy{}, fmt.Errorf("policy maxObservationGap must be a positive Go duration")
	}
	return contracts.ProgressingPolicy{Limit: limit, MaxObservationGap: gap, Operator: input.Operator, TargetVersion: input.TargetVersion, Source: input.Source}, nil
}

func (c cli) printUpgradeProgressing(report contracts.ProgressingUpgradeReport) {
	fmt.Fprintln(c.stdout, "Contract: operator-progressing-duration")
	fmt.Fprintln(c.stdout, "Operator:", report.Operator)
	fmt.Fprintln(c.stdout, "Policy: user-supplied project policy (not an OpenShift guarantee)")
	fmt.Fprintln(c.stdout, "Threshold source:", report.Policy.Source)
	fmt.Fprintln(c.stdout, "Target version:", report.Policy.TargetVersion)
	fmt.Fprintln(c.stdout, "Maximum duration:", report.Policy.Limit)
	fmt.Fprintln(c.stdout, "Maximum observation gap:", report.Policy.MaxObservationGap)
	fmt.Fprintln(c.stdout, "Policy applicable:", report.PolicyApplicable)
	fmt.Fprintln(c.stdout, "Evaluated UPDATING samples:", report.EvaluatedSamples)
	fmt.Fprintln(c.stdout, "Uncertain samples:", report.UncertainSamples)
	fmt.Fprintln(c.stdout, "Missing/Unknown Progressing:", report.MissingConditions)
	fmt.Fprintln(c.stdout, "Discontinuities:", report.Discontinuities)
	fmt.Fprintln(c.stdout, "Verdict:", report.Verdict)
	for _, finding := range report.Evidence {
		episode := finding.Episode
		fmt.Fprintf(c.stdout, "  %s start=%s end=%s observed-interval=%s threshold=%s source=%q\n", episode.Verdict,
			episode.FirstTrue.Format(time.RFC3339Nano), episode.LastTrue.Format(time.RFC3339Nano), episode.ObservedSpan, report.Policy.Limit, report.Policy.Source)
		for _, sample := range []contracts.CorrelatedObservation{finding.Start, finding.End} {
			fmt.Fprintf(c.stdout, "    operator-observation=%s correlation=%s version-interval=[%s,%s]\n", sample.Observation.ObservedAt.Format(time.RFC3339Nano), sample.Kind, sample.FromTime.Format(time.RFC3339Nano), sample.ToTime.Format(time.RFC3339Nano))
		}
		if !episode.BeforeFalse.IsZero() {
			fmt.Fprintln(c.stdout, "    preceding-False:", episode.BeforeFalse.Format(time.RFC3339Nano))
		}
		if !episode.AfterFalse.IsZero() {
			fmt.Fprintln(c.stdout, "    following-False:", episode.AfterFalse.Format(time.RFC3339Nano))
		}
	}
	fmt.Fprintln(c.stdout, "Scope: sampled UPDATING intervals under the declared gap policy; source applicability is supplied by the user, not independently verified.")
}
