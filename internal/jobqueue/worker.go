package jobqueue

import (
	"context"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

// DeadLetterCallback is called when a job is dead-lettered.
type DeadLetterCallback func(ctx context.Context, job *Job, jobErr error)

// Worker polls the job queue and dispatches to registered handlers.
type Worker struct {
	queue        *Queue
	mu           sync.RWMutex
	handlers     map[JobKind]HandlerFunc
	deadLetterCB DeadLetterCallback
}

type processQueue interface {
	Claim(ctx context.Context) (*Job, error)
	Fail(ctx context.Context, jobID, lockToken, errMsg string) error
	Complete(ctx context.Context, jobID, lockToken string) error
}

// NewWorker creates a Worker for the given Queue.
func NewWorker(queue *Queue) *Worker {
	return &Worker{
		queue:    queue,
		handlers: make(map[JobKind]HandlerFunc, 4),
	}
}

// Register adds a handler for the given job kind.
// Safe to call concurrently with other Register calls or before Run.
func (w *Worker) Register(kind JobKind, handler HandlerFunc) {
	w.mu.Lock()
	w.handlers[kind] = handler
	w.mu.Unlock()
}

// OnDeadLetter sets a callback invoked when a job exceeds max attempts.
func (w *Worker) OnDeadLetter(cb DeadLetterCallback) {
	w.mu.Lock()
	w.deadLetterCB = cb
	w.mu.Unlock()
}

// Run starts the poll loop. It blocks until ctx is cancelled.
// Uses LISTEN/NOTIFY for low-latency wake-up with poll interval as fallback.
func (w *Worker) Run(ctx context.Context) {
	// Acquire a dedicated connection for LISTEN.
	conn, err := w.queue.pool.Acquire(ctx)
	if err != nil {
		log.Printf("jobqueue worker: failed to acquire listen connection: %v", err)
		w.runPollOnly(ctx)
		return
	}
	defer conn.Release()

	_, err = conn.Exec(ctx, "LISTEN new_job")
	if err != nil {
		log.Printf("jobqueue worker: LISTEN failed, falling back to poll-only: %v", err)
		w.runPollOnly(ctx)
		return
	}

	log.Printf("jobqueue worker: started (poll=%s, listen=new_job)", w.queue.pollInterval)

	// Periodic stale-claim recovery (moved from per-Claim to reduce DB load).
	staleRecovery := time.NewTicker(30 * time.Second)
	go w.runStaleRecoveryLoop(ctx, staleRecovery)

	for {
		// Process all available jobs before waiting.
		for {
			if ctx.Err() != nil {
				return
			}
			if !w.processOne(ctx) {
				break
			}
		}

		// Wait for NOTIFY or poll timeout.
		waitCtx, cancel := context.WithTimeout(ctx, w.queue.pollInterval)
		_, _ = conn.Conn().WaitForNotification(waitCtx)
		cancel()

		if ctx.Err() != nil {
			log.Printf("jobqueue worker: stopped")
			return
		}
	}
}

// runPollOnly is the fallback when LISTEN is unavailable.
func (w *Worker) runPollOnly(ctx context.Context) {
	log.Printf("jobqueue worker: running in poll-only mode (interval=%s)", w.queue.pollInterval)
	ticker := time.NewTicker(w.queue.pollInterval)
	defer ticker.Stop()

	// Periodic stale-claim recovery (moved from per-Claim to reduce DB load).
	staleRecovery := time.NewTicker(30 * time.Second)
	go w.runStaleRecoveryLoop(ctx, staleRecovery)

	for {
		select {
		case <-ctx.Done():
			log.Printf("jobqueue worker: stopped")
			return
		case <-ticker.C:
			for w.processOne(ctx) {
				if ctx.Err() != nil {
					return
				}
			}
		}
	}
}

// processOne claims and processes a single job. Returns true if a job was processed.
func (w *Worker) processOne(ctx context.Context) bool {
	return w.processOneWithQueue(ctx, w.queue)
}

func (w *Worker) processOneWithQueue(ctx context.Context, q processQueue) bool {
	if q == nil {
		return false
	}

	job, err := q.Claim(ctx)
	if err != nil {
		log.Printf("jobqueue worker: claim error: %v", err)
		return false
	}
	if job == nil {
		return false
	}

	w.mu.RLock()
	handler, ok := w.handlers[job.Kind]
	deadLetterCB := w.deadLetterCB
	w.mu.RUnlock()

	if !ok {
		log.Printf("jobqueue worker: no handler for kind %q, dead-lettering job %s", job.Kind, job.ID)
		jobErr := errors.New("no handler registered for job kind")
		failErr := q.Fail(ctx, job.ID, job.LockToken, jobErr.Error())
		if failErr != nil {
			log.Printf("jobqueue worker: failed to mark job %s as failed: %v", job.ID, failErr)
			return true
		}
		if job.Attempts >= job.MaxAttempts && deadLetterCB != nil {
			w.invokeDeadLetterCB(deadLetterCB, ctx, job, jobErr)
		}
		return true
	}

	handlerCtx, cancel := context.WithCancel(ctx)
	stopHeartbeat := w.startClaimHeartbeat(handlerCtx, q, job)
	err = w.invokeHandler(handler, handlerCtx, job)
	cancel()
	if stopHeartbeat != nil {
		stopHeartbeat()
	}
	if err != nil {
		log.Printf("jobqueue worker: job %s (%s) failed: %v", job.ID, job.Kind, err)
		failErr := q.Fail(ctx, job.ID, job.LockToken, err.Error())
		if failErr != nil {
			log.Printf("jobqueue worker: failed to mark job %s as failed: %v", job.ID, failErr)
		}

		// Check if this failure caused dead-lettering.
		if failErr == nil && job.Attempts >= job.MaxAttempts && deadLetterCB != nil {
			w.invokeDeadLetterCB(deadLetterCB, ctx, job, err)
		}
		return true
	}

	if err := q.Complete(ctx, job.ID, job.LockToken); err != nil {
		log.Printf("jobqueue worker: failed to complete job %s: %v", job.ID, err)
	}
	return true
}

func (w *Worker) invokeHandler(handler HandlerFunc, ctx context.Context, job *Job) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("job handler panic: %v", recovered)
			log.Printf("jobqueue worker: recovered panic in job %s (%s): %v\n%s",
				job.ID, job.Kind, recovered, string(debug.Stack()))
		}
	}()
	return handler(ctx, job)
}

func (w *Worker) invokeDeadLetterCB(cb DeadLetterCallback, ctx context.Context, job *Job, jobErr error) {
	if cb == nil {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("jobqueue worker: recovered panic in dead-letter callback for job %s (%s): %v\n%s",
				job.ID, job.Kind, recovered, string(debug.Stack()))
		}
	}()
	cb(ctx, job, jobErr)
}

func (w *Worker) startClaimHeartbeat(ctx context.Context, q processQueue, job *Job) func() {
	if job == nil || job.ID == "" || job.LockToken == "" {
		return nil
	}
	renewable, ok := q.(interface {
		RenewClaim(ctx context.Context, jobID, lockToken string) error
		leaseHeartbeatInterval() time.Duration
	})
	if !ok {
		return nil
	}

	ticker := time.NewTicker(renewable.leaseHeartbeatInterval())
	done := make(chan struct{})
	go func() {
		defer ticker.Stop()
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := renewable.RenewClaim(ctx, job.ID, job.LockToken); err != nil {
					if !errors.Is(err, ErrClaimLost) && ctx.Err() == nil {
						log.Printf("jobqueue worker: failed to renew claim for job %s: %v", job.ID, err)
					}
					return
				}
			}
		}
	}()
	return func() {
		<-done
	}
}

func (w *Worker) runStaleRecoveryLoop(ctx context.Context, ticker *time.Ticker) {
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			deadJobs, err := w.queue.RecoverStaleClaims(ctx, time.Now().UTC())
			if err != nil {
				log.Printf("jobqueue worker: failed to recover stale claims: %v", err)
				continue
			}
			w.handleRecoveredDeadJobs(ctx, deadJobs)
		}
	}
}

func (w *Worker) handleRecoveredDeadJobs(ctx context.Context, jobs []*Job) {
	w.mu.RLock()
	deadLetterCB := w.deadLetterCB
	w.mu.RUnlock()
	for _, job := range jobs {
		if deadLetterCB == nil || job == nil {
			continue
		}
		jobErr := errors.New(strings.TrimSpace(job.Error))
		if strings.TrimSpace(jobErr.Error()) == "" {
			jobErr = errors.New("job claim timed out after max attempts")
		}
		w.invokeDeadLetterCB(deadLetterCB, ctx, job, jobErr)
	}
}
