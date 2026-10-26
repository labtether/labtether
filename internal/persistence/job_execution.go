package persistence

import (
	"context"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/jobqueue"
	"github.com/labtether/labtether/internal/terminal"
	"github.com/labtether/labtether/internal/updates"
)

const InterruptedJobOutcome = "previous execution attempt was interrupted; outcome unknown; work was not replayed"

// JobExecutionStore owns the queue claim and its business records together.
// Begin commits a durable dispatch marker before any external effect. A running
// redelivery is finalized as unknown rather than replayed. Finish commits the
// result and queue completion atomically. Successful completion sets Job.Status;
// only the handler goroutine reads/writes that local completion acknowledgement.
type JobExecutionStore interface {
	BeginJobExecution(context.Context, *jobqueue.Job) (bool, error)
	FinishJobExecution(context.Context, *jobqueue.Job, JobExecutionResult) error
	StartScheduledAction(context.Context, *jobqueue.Job, string) (actions.Job, error)
	FinishScheduledAction(context.Context, *jobqueue.Job, actions.Job, actions.Result) error
}

type JobExecutionResult struct {
	Terminal *terminal.CommandResult
	Action   *actions.Result
	Update   *updates.Result
	Schedule *ScheduleExecutionResult
}

type ScheduleExecutionResult struct{ Status, Error string }
