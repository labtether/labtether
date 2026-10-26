package worker

import (
	"context"
	"errors"

	"github.com/labtether/labtether/internal/jobqueue"
)

func (d *Deps) beginExecution(ctx context.Context, job *jobqueue.Job) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if d.JobExecutionStore == nil {
		return false, errors.New("claim-fenced execution store is unavailable")
	}
	return d.JobExecutionStore.BeginJobExecution(ctx, job)
}
