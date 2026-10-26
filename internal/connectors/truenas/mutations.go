package truenas

import (
	"context"
	"fmt"
	"strings"
)

// CallMutation sends once. A middleware error is not evidence that a write
// had no effect, so callers must not retry or fall back after this call.
func (c *Client) CallMutation(ctx context.Context, method string, params []any, dest any) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("mutation not dispatched: %w", err)
	}
	if err := c.Call(ctx, method, params, dest); err != nil {
		return fmt.Errorf("%w (not retried; mutation outcome may be unknown)", err)
	}
	return nil
}

// StartPoolScrub requests the manual job, not the threshold-based cron check.
// The returned ID proves acceptance only, never completion or current state.
func (c *Client) StartPoolScrub(ctx context.Context, name string) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, fmt.Errorf("pool name is required")
	}
	return c.CallJob(ctx, "pool.scrub.scrub", []any{name, "START"})
}

// CallJob keeps the provider acceptance identity without claiming job state.
func (c *Client) CallJob(ctx context.Context, method string, params []any) (int64, error) {
	var jobID int64
	if err := c.CallMutation(ctx, method, params, &jobID); err != nil {
		return 0, err
	}
	if jobID <= 0 {
		return 0, fmt.Errorf("invalid provider job ID; request was not retried and outcome is unknown")
	}
	return jobID, nil
}
