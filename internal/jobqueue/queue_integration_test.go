//go:build integration

package jobqueue

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func setupTestQueue(t *testing.T) (*Queue, func()) {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	// Ensure job_queue table exists
	_, err = pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS job_queue (
		id TEXT PRIMARY KEY,
		kind TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'queued',
		payload BYTEA,
		attempts INT NOT NULL DEFAULT 0,
		max_attempts INT NOT NULL DEFAULT 5,
		error TEXT DEFAULT '',
		created_at TIMESTAMPTZ DEFAULT now(),
		updated_at TIMESTAMPTZ DEFAULT now(),
		locked_at TIMESTAMPTZ,
		completed_at TIMESTAMPTZ
	)`)
	if err != nil {
		pool.Close()
		t.Fatalf("failed to create table: %v", err)
	}

	q := New(pool, 100*time.Millisecond, 2) // maxAttempts=2 for testing
	return q, func() {
		// Clean up test data
		_, _ = pool.Exec(ctx, `DELETE FROM job_queue WHERE kind LIKE 'test_%'`)
		pool.Close()
	}
}

func TestFailAndRetry(t *testing.T) {
	q, cleanup := setupTestQueue(t)
	defer cleanup()

	ctx := context.Background()

	// Enqueue a test job
	jobID, err := q.Enqueue(ctx, JobKind("test_dlq_retry"), []byte(`{"test": true}`))
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	// Claim the job (attempt 1)
	job, err := q.Claim(ctx)
	if err != nil {
		t.Fatalf("Claim failed: %v", err)
	}
	if job == nil || job.ID != jobID {
		t.Fatalf("expected job %s, got %v", jobID, job)
	}
	if job.Attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", job.Attempts)
	}

	// Fail the job (should retry since attempts < maxAttempts)
	err = q.Fail(ctx, jobID, "transient error")
	if err != nil {
		t.Fatalf("Fail failed: %v", err)
	}

	// Claim again (attempt 2)
	job, err = q.Claim(ctx)
	if err != nil {
		t.Fatalf("Claim (attempt 2) failed: %v", err)
	}
	if job == nil || job.ID != jobID {
		t.Fatalf("expected job %s on retry, got %v", jobID, job)
	}
	if job.Attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", job.Attempts)
	}

	// Fail again (should dead-letter since attempts >= maxAttempts)
	err = q.Fail(ctx, jobID, "permanent error")
	if err != nil {
		t.Fatalf("Fail (attempt 2) failed: %v", err)
	}

	// Verify it's dead-lettered: no more jobs to claim
	job, err = q.Claim(ctx)
	if err != nil {
		t.Fatalf("Claim after DLQ failed: %v", err)
	}
	if job != nil {
		t.Fatalf("expected no claimable jobs after dead-letter, got %v", job.ID)
	}

	// Verify ListDeadLettered returns the entry
	now := time.Now().UTC()
	from := now.Add(-1 * time.Hour)
	deadJobs, err := q.ListDeadLettered(ctx, from, now, 10)
	if err != nil {
		t.Fatalf("ListDeadLettered failed: %v", err)
	}

	found := false
	for _, dj := range deadJobs {
		if dj.ID == jobID {
			found = true
			if dj.Status != StatusDeadLettered {
				t.Errorf("expected status dead_lettered, got %s", dj.Status)
			}
			if dj.Error != "permanent error" {
				t.Errorf("expected error 'permanent error', got %q", dj.Error)
			}
			if dj.Attempts != 2 {
				t.Errorf("expected 2 attempts, got %d", dj.Attempts)
			}
		}
	}
	if !found {
		t.Errorf("dead-lettered job %s not found in ListDeadLettered results", jobID)
	}
}

func TestCompleteSuccess(t *testing.T) {
	q, cleanup := setupTestQueue(t)
	defer cleanup()

	ctx := context.Background()

	jobID, err := q.Enqueue(ctx, JobKind("test_dlq_success"), []byte(`{"test": true}`))
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	job, err := q.Claim(ctx)
	if err != nil {
		t.Fatalf("Claim failed: %v", err)
	}
	if job == nil || job.ID != jobID {
		t.Fatalf("expected job %s, got %v", jobID, job)
	}

	// Complete successfully
	err = q.Complete(ctx, jobID)
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	// Should not be claimable
	job, err = q.Claim(ctx)
	if err != nil {
		t.Fatalf("Claim after complete failed: %v", err)
	}
	if job != nil && job.ID == jobID {
		t.Fatalf("completed job should not be claimable")
	}

	// Should not appear in dead letter list
	now := time.Now().UTC()
	deadJobs, err := q.ListDeadLettered(ctx, now.Add(-1*time.Hour), now, 10)
	if err != nil {
		t.Fatalf("ListDeadLettered failed: %v", err)
	}
	for _, dj := range deadJobs {
		if dj.ID == jobID {
			t.Errorf("completed job should not be in dead letter list")
		}
	}
}
