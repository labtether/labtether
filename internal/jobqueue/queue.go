package jobqueue

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/labtether/labtether/internal/idgen"
)

// Queue provides Postgres-backed job queue operations.
type Queue struct {
	pool         *pgxpool.Pool
	pollInterval time.Duration
	maxAttempts  atomic.Int64
	staleClaimAt time.Duration
}

// New creates a Queue backed by the given connection pool.
func New(pool *pgxpool.Pool, pollInterval time.Duration, maxAttempts int) *Queue {
	if pollInterval <= 0 {
		pollInterval = 500 * time.Millisecond
	}
	if maxAttempts <= 0 {
		maxAttempts = 5
	}

	staleClaimAt := pollInterval * 20
	if staleClaimAt < 30*time.Second {
		staleClaimAt = 30 * time.Second
	}

	q := &Queue{
		pool:         pool,
		pollInterval: pollInterval,
		staleClaimAt: staleClaimAt,
	}
	q.maxAttempts.Store(int64(maxAttempts))
	return q
}

// MaxAttempts returns the configured maximum delivery attempts for new jobs.
func (q *Queue) MaxAttempts() int {
	if q == nil {
		return 5
	}
	value := int(q.maxAttempts.Load())
	if value <= 0 {
		return 5
	}
	return value
}

// SetMaxAttempts updates max attempts for newly enqueued jobs.
func (q *Queue) SetMaxAttempts(maxAttempts int) {
	if q == nil || maxAttempts <= 0 {
		return
	}
	q.maxAttempts.Store(int64(maxAttempts))
}

// Enqueue inserts a new job and sends a NOTIFY to wake polling workers.
func (q *Queue) Enqueue(ctx context.Context, kind JobKind, payload []byte) (string, error) {
	if err := ValidateKind(kind); err != nil {
		return "", err
	}
	id := idgen.New("jq")
	now := time.Now().UTC()

	_, err := q.pool.Exec(ctx,
		`INSERT INTO job_queue (id, kind, status, payload, attempts, max_attempts, error, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, 0, $5, '', $6, $6)`,
		id, string(kind), string(StatusQueued), payload, q.MaxAttempts(), now,
	)
	if err != nil {
		return "", err
	}

	// Best-effort NOTIFY; worker also polls as fallback.
	_, _ = q.pool.Exec(ctx, "SELECT pg_notify('new_job', '')")
	return id, nil
}

// Claim atomically picks the oldest queued job and marks it processing.
// Returns nil, nil when no job is available.
func (q *Queue) Claim(ctx context.Context) (*Job, error) {
	if q == nil || q.pool == nil {
		return nil, errors.New("job queue unavailable")
	}

	now := time.Now().UTC()
	row := q.pool.QueryRow(ctx,
		`UPDATE job_queue
		 SET status = $1, locked_at = $2, attempts = attempts + 1, updated_at = $2
		 WHERE id = (
		   SELECT id FROM job_queue
		   WHERE status = $3
		   ORDER BY created_at ASC
		   LIMIT 1
		   FOR UPDATE SKIP LOCKED
		 )
		 RETURNING id, kind, status, payload, attempts, max_attempts, error, created_at, updated_at, locked_at, completed_at`,
		string(StatusProcessing), now, string(StatusQueued),
	)

	job := &Job{}
	var kind, status string
	err := row.Scan(
		&job.ID, &kind, &status, &job.Payload,
		&job.Attempts, &job.MaxAttempts, &job.Error,
		&job.CreatedAt, &job.UpdatedAt, &job.LockedAt, &job.CompletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	job.Kind = JobKind(kind)
	job.Status = JobStatus(status)
	return job, nil
}

// RecoverStaleClaims requeues or dead-letters jobs that have been in the
// processing state past the stale-claim threshold. It is called periodically
// by the Worker rather than on every Claim to reduce per-Claim DB overhead.
func (q *Queue) RecoverStaleClaims(ctx context.Context, now time.Time) error {
	if q == nil || q.pool == nil || q.staleClaimAt <= 0 {
		return nil
	}
	cutoff := now.Add(-q.staleClaimAt)
	_, err := q.pool.Exec(ctx,
		`UPDATE job_queue
		 SET status = CASE WHEN attempts >= max_attempts THEN $1 ELSE $2 END,
		     locked_at = NULL,
		     updated_at = $4::timestamptz,
		     completed_at = CASE WHEN attempts >= max_attempts THEN $4::timestamptz ELSE NULL END,
		     error = CASE
		       WHEN attempts >= max_attempts AND COALESCE(NULLIF(error, ''), '') = '' THEN 'job claim timed out after max attempts'
		       ELSE error
		     END
		 WHERE status = $3
		   AND locked_at IS NOT NULL
		   AND locked_at < $5::timestamptz`,
		string(StatusDeadLettered),
		string(StatusQueued),
		string(StatusProcessing),
		now,
		cutoff,
	)
	return err
}

// Complete marks a job as completed.
func (q *Queue) Complete(ctx context.Context, jobID string) error {
	now := time.Now().UTC()
	_, err := q.pool.Exec(ctx,
		`UPDATE job_queue SET status = $1, completed_at = $2, updated_at = $2 WHERE id = $3`,
		string(StatusCompleted), now, jobID,
	)
	return err
}

// Fail marks a job as failed or dead-lettered if max attempts reached.
// Uses a single atomic UPDATE to avoid race conditions between read and write.
func (q *Queue) Fail(ctx context.Context, jobID string, errMsg string) error {
	now := time.Now().UTC()

	_, err := q.pool.Exec(ctx,
		`UPDATE job_queue SET
			status = CASE WHEN attempts >= max_attempts THEN $1 ELSE $2 END,
			error = $3,
			updated_at = $4,
			locked_at = CASE WHEN attempts >= max_attempts THEN locked_at ELSE NULL END,
			completed_at = CASE WHEN attempts >= max_attempts THEN $4 ELSE completed_at END
		 WHERE id = $5`,
		string(StatusDeadLettered), string(StatusQueued), errMsg, now, jobID,
	)
	return err
}

// PurgeOldJobs deletes completed and dead-lettered jobs older than the given
// cutoff time. This prevents unbounded growth of the job_queue table.
// It returns the number of rows deleted.
func (q *Queue) PurgeOldJobs(ctx context.Context, olderThan time.Time) (int64, error) {
	tag, err := q.pool.Exec(ctx,
		`DELETE FROM job_queue
		 WHERE status IN ($1, $2) AND updated_at < $3`,
		string(StatusCompleted), string(StatusDeadLettered), olderThan,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ListDeadLettered returns dead-lettered jobs within a time range.
func (q *Queue) ListDeadLettered(ctx context.Context, from, to time.Time, limit int) ([]Job, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := q.pool.Query(ctx,
		`SELECT id, kind, status, payload, attempts, max_attempts, error, created_at, updated_at, locked_at, completed_at
		 FROM job_queue
		 WHERE status = $1 AND created_at >= $2 AND created_at <= $3
		 ORDER BY created_at DESC
		 LIMIT $4`,
		string(StatusDeadLettered), from, to, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var j Job
		var kind, status string
		if err := rows.Scan(
			&j.ID, &kind, &status, &j.Payload,
			&j.Attempts, &j.MaxAttempts, &j.Error,
			&j.CreatedAt, &j.UpdatedAt, &j.LockedAt, &j.CompletedAt,
		); err != nil {
			return nil, err
		}
		j.Kind = JobKind(kind)
		j.Status = JobStatus(status)
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}
