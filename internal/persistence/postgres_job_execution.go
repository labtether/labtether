package persistence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labtether/labtether/internal/jobqueue"
)

const jobExecutionTimeout = 5 * time.Second

func rollbackJobExecution(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), jobExecutionTimeout)
	defer cancel()
	_ = tx.Rollback(ctx)
}

// Lock order is always queue row, then business row(s). Holding this row lock
// prevents recovery/reclaim from changing the token between validation and
// business writes. Payload equality also prevents redirecting a valid claim.
func lockJobClaim(ctx context.Context, tx pgx.Tx, job *jobqueue.Job, dead bool) error {
	if job == nil || job.ID == "" || (!dead && job.LockToken == "") {
		return jobqueue.ErrClaimLost
	}
	state := string(jobqueue.StatusProcessing)
	if dead {
		state = string(jobqueue.StatusDeadLettered)
	}
	var id string
	err := tx.QueryRow(ctx, `SELECT id FROM job_queue WHERE id=$1 AND status=$2
 AND ($3 OR lock_token=$4) AND kind=$5 AND payload=$6::jsonb FOR UPDATE`,
		job.ID, state, dead, job.LockToken, string(job.Kind), string(job.Payload)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return jobqueue.ErrClaimLost
	}
	return err
}

func completeLockedJob(ctx context.Context, tx pgx.Tx, job *jobqueue.Job) error {
	_, err := tx.Exec(ctx, `UPDATE job_queue SET status='completed',completed_at=NOW(),
 updated_at=NOW(),locked_at=NULL,lock_token=NULL WHERE id=$1`, job.ID)
	return err
}

func commitJobExecution(ctx context.Context, tx pgx.Tx, job *jobqueue.Job, completed bool) error {
	if completed {
		if err := completeLockedJob(ctx, tx, job); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	// This is authoritative even if a concurrent renewal now returns ErrClaimLost.
	if completed {
		job.Status = jobqueue.StatusCompleted
	}
	return nil
}

func (s *PostgresStore) BeginJobExecution(parent context.Context, job *jobqueue.Job) (bool, error) {
	ctx, cancel := context.WithTimeout(parent, jobExecutionTimeout)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer rollbackJobExecution(tx)
	if err = lockJobClaim(ctx, tx, job, false); err != nil {
		return false, err
	}
	owner, err := lockJobBusiness(ctx, tx, job)
	if err != nil {
		return false, err
	}
	if owner.status == "superseded" {
		if err = closeInterruptedScheduleChildren(ctx, tx, owner); err != nil {
			return false, err
		}
	}
	dispatch := owner.status == "queued"
	if dispatch {
		err = owner.markRunning(ctx, tx)
	} else if owner.status == "running" {
		err = owner.writeResult(ctx, tx, owner.interruptedResult())
	} else if owner.status == "disabled" {
		err = owner.writeResult(ctx, tx, JobExecutionResult{Schedule: &ScheduleExecutionResult{Status: "skipped", Error: "schedule disabled before dispatch"}})
	}
	if err != nil {
		return false, err
	}
	if err = commitJobExecution(ctx, tx, job, !dispatch); err != nil {
		return false, err
	}
	return dispatch, nil
}

func (s *PostgresStore) FinishJobExecution(parent context.Context, job *jobqueue.Job, result JobExecutionResult) error {
	ctx, cancel := context.WithTimeout(parent, jobExecutionTimeout)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollbackJobExecution(tx)
	if err = lockJobClaim(ctx, tx, job, false); err != nil {
		return err
	}
	owner, err := lockJobBusiness(ctx, tx, job)
	if err != nil {
		return err
	}
	// A removed/replaced schedule occurrence must not overwrite its successor.
	if owner.status == "superseded" {
		if err = closeInterruptedScheduleChildren(ctx, tx, owner); err != nil {
			return err
		}
	} else {
		if owner.status != "running" {
			return fmt.Errorf("job business record is not running: %s", owner.status)
		}
		if err = owner.writeResult(ctx, tx, result); err != nil {
			return err
		}
	}
	return commitJobExecution(ctx, tx, job, true)
}

// FinalizeDeadJobInTransaction runs only inside Queue.Fail/stale recovery's
// transaction, after it has locked and changed the exact row to dead-lettered.
func (s *PostgresStore) FinalizeDeadJobInTransaction(ctx context.Context, tx pgx.Tx, job *jobqueue.Job) error {
	if err := lockJobClaim(ctx, tx, job, true); err != nil {
		return err
	}
	owner, err := lockJobBusiness(ctx, tx, job)
	// Invalid/deleted business identity is not authority to change another row.
	// The invalid queue entry can still be dead-lettered; database errors retry.
	if errors.Is(err, errJobBusinessIdentity) {
		return nil
	}
	if err != nil {
		return err
	}
	if owner.status == "superseded" {
		return closeInterruptedScheduleChildren(ctx, tx, owner)
	}
	if owner.status == "disabled" {
		return owner.writeResult(ctx, tx, JobExecutionResult{Schedule: &ScheduleExecutionResult{Status: "skipped", Error: "schedule disabled before dispatch"}})
	}
	if owner.status == "running" || owner.status == "queued" {
		result := owner.interruptedResult()
		if owner.status == "queued" {
			result = owner.failedResult("queue retries exhausted before dispatch; work was not executed")
		}
		return owner.writeResult(ctx, tx, result)
	}
	return nil
}
