package collector

import (
	"context"
	"fmt"
)

func (r *LiveRecorder) CaptureFinal(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("final snapshot: %w", err)
	}
	c := newLiveCollector(r.client, r.clock.now)
	capture, err := c.Capture(ctx)
	if err != nil {
		return fmt.Errorf("final snapshot: %w", err)
	}

	closing, err := c.captureVersion(ctx)
	if err != nil {
		return fmt.Errorf("final snapshot closing ClusterVersion: %w", err)
	}
	capture.ClusterVersion.ObservedAt = r.clock.Observe("clusterversion/version", capture.ClusterVersion.ObservedAt)
	if err := r.sink.AppendClusterVersion(capture.ClusterVersion); err != nil {
		return fmt.Errorf("final snapshot: %w", err)
	}
	for _, observation := range capture.Operators {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("final snapshot: %w", err)
		}
		observation.ObservedAt = r.clock.Observe("clusteroperator/"+observation.Operator.Name, observation.ObservedAt)
		if err := r.sink.AppendOperator(observation); err != nil {
			return fmt.Errorf("final snapshot: %w", err)
		}
	}
	closing.ObservedAt = r.clock.Observe("clusterversion/version", closing.ObservedAt)
	if err := r.sink.AppendClusterVersion(closing); err != nil {
		return fmt.Errorf("final snapshot: %w", err)
	}
	return nil
}
