package jobqueue

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// DeadLetterFinalizer closes business records in the same transaction as the
// terminal queue state. Set once before workers start. The callback must retain
// queue→business lock order and perform no remote effect or nested transaction.
type DeadLetterFinalizer func(context.Context, pgx.Tx, *Job) error

func (q *Queue) SetDeadLetterFinalizer(finalize DeadLetterFinalizer) { q.finalizeDeadLetter = finalize }
func rollbackQueueTransaction(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}

// RecoverStaleClaims handles a bounded batch under queue row locks. A failed
// business finalizer rolls back recovery so the next pass can safely retry it.
func (q *Queue) RecoverStaleClaims(ctx context.Context, now time.Time) ([]*Job, error) {
	if q == nil || q.pool == nil || q.staleClaimAt <= 0 {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer rollbackQueueTransaction(tx)
	cutoff := now.Add(-q.staleClaimAt)
	rows, err := tx.Query(ctx,
		`WITH expired AS (
 SELECT id FROM job_queue WHERE status=$3 AND locked_at IS NOT NULL AND locked_at<$5::timestamptz
 ORDER BY locked_at,id LIMIT 100 FOR UPDATE SKIP LOCKED
 ) UPDATE job_queue
		 SET status = CASE WHEN attempts >= max_attempts THEN $1 ELSE $2 END,
		     locked_at = NULL,
		     lock_token = NULL,
		     updated_at = $4::timestamptz,
		     available_at = CASE
		       WHEN attempts >= max_attempts THEN available_at
		       ELSE $4::timestamptz + make_interval(secs => LEAST(30, CAST(power(2, GREATEST(attempts - 1, 0)) AS int)))
		     END,
		     completed_at = CASE WHEN attempts >= max_attempts THEN $4::timestamptz ELSE NULL END,
		     error = CASE
		       WHEN attempts >= max_attempts AND COALESCE(NULLIF(error, ''), '') = '' THEN 'job claim timed out after max attempts'
		       WHEN attempts < max_attempts AND COALESCE(NULLIF(error, ''), '') = '' THEN 'job claim timed out; retrying'
		       ELSE error
		     END
		 WHERE id IN (SELECT id FROM expired)
		 RETURNING id, kind, status, payload, attempts, max_attempts, error, created_at, updated_at, available_at, locked_at, completed_at, lock_token`,
		string(StatusDeadLettered),
		string(StatusQueued),
		string(StatusProcessing),
		now,
		cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	deadLettered := make([]*Job, 0)
	for rows.Next() {
		job, scanErr := scanJob(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		if job.Status == StatusDeadLettered {
			deadLettered = append(deadLettered, job)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	for _, job := range deadLettered {
		if q.finalizeDeadLetter != nil {
			if err = q.finalizeDeadLetter(ctx, tx, job); err != nil {
				return nil, err
			}
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return deadLettered, nil
}

// Fail retries a live claim or atomically closes its terminal business state.
func (q *Queue) Fail(ctx context.Context, jobID, lockToken string, errMsg string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollbackQueueTransaction(tx)
	now := time.Now().UTC()

	row := tx.QueryRow(ctx,
		`UPDATE job_queue SET
				status = CASE WHEN attempts >= max_attempts THEN $1 ELSE $2 END,
				error = $3,
				updated_at = $4::timestamptz,
				available_at = CASE
					WHEN attempts >= max_attempts THEN available_at
					ELSE $4::timestamptz + make_interval(secs => LEAST(30, CAST(power(2, GREATEST(attempts - 1, 0)) AS int)))
				END,
				locked_at = NULL,
				lock_token = NULL,
				completed_at = CASE WHEN attempts >= max_attempts THEN $4::timestamptz ELSE completed_at END
			 WHERE id = $5 AND status = $6 AND lock_token = $7
 RETURNING id,kind,status,payload,attempts,max_attempts,error,created_at,updated_at,available_at,locked_at,completed_at,lock_token`,
		string(StatusDeadLettered), string(StatusQueued), errMsg, now, jobID, string(StatusProcessing), lockToken,
	)
	job, err := scanJob(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrClaimLost
	}
	if err != nil {
		return err
	}
	if job.Status == StatusDeadLettered && q.finalizeDeadLetter != nil {
		if err = q.finalizeDeadLetter(ctx, tx, job); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
