package jobqueue

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type stubProcessQueue struct {
	job         *Job
	claimErr    error
	failErr     error
	completeErr error

	failCalls     int
	failJobID     string
	failErrMsg    string
	completeCalls int
	completeJobID string
}

func (s *stubProcessQueue) Claim(context.Context) (*Job, error) {
	if s.claimErr != nil {
		return nil, s.claimErr
	}
	job := s.job
	s.job = nil
	return job, nil
}

func (s *stubProcessQueue) Fail(_ context.Context, jobID string, errMsg string) error {
	s.failCalls++
	s.failJobID = jobID
	s.failErrMsg = errMsg
	return s.failErr
}

func (s *stubProcessQueue) Complete(_ context.Context, jobID string) error {
	s.completeCalls++
	s.completeJobID = jobID
	return s.completeErr
}

func TestProcessOneWithQueueNoHandlerDeadLetterCallbackOnFinalAttempt(t *testing.T) {
	w := NewWorker(nil)
	q := &stubProcessQueue{
		job: &Job{
			ID:          "job-final-no-handler",
			Kind:        JobKind("missing-handler"),
			Attempts:    3,
			MaxAttempts: 3,
		},
	}

	cbCalls := 0
	var cbErr error
	w.OnDeadLetter(func(_ context.Context, _ *Job, jobErr error) {
		cbCalls++
		cbErr = jobErr
	})

	processed := w.processOneWithQueue(context.Background(), q)
	if !processed {
		t.Fatalf("expected processOneWithQueue to process one job")
	}
	if q.failCalls != 1 {
		t.Fatalf("expected fail to be called once, got %d", q.failCalls)
	}
	if q.failJobID != "job-final-no-handler" {
		t.Fatalf("expected fail job id job-final-no-handler, got %s", q.failJobID)
	}
	if cbCalls != 1 {
		t.Fatalf("expected dead-letter callback once, got %d", cbCalls)
	}
	if cbErr == nil || !strings.Contains(cbErr.Error(), "no handler registered") {
		t.Fatalf("expected no-handler callback error, got %v", cbErr)
	}
}

func TestProcessOneWithQueueNoHandlerDoesNotDeadLetterBeforeMaxAttempts(t *testing.T) {
	w := NewWorker(nil)
	q := &stubProcessQueue{
		job: &Job{
			ID:          "job-retry-no-handler",
			Kind:        JobKind("missing-handler"),
			Attempts:    1,
			MaxAttempts: 3,
		},
	}

	cbCalls := 0
	w.OnDeadLetter(func(_ context.Context, _ *Job, _ error) {
		cbCalls++
	})

	processed := w.processOneWithQueue(context.Background(), q)
	if !processed {
		t.Fatalf("expected processOneWithQueue to process one job")
	}
	if q.failCalls != 1 {
		t.Fatalf("expected fail to be called once, got %d", q.failCalls)
	}
	if cbCalls != 0 {
		t.Fatalf("expected no dead-letter callback before max attempts, got %d", cbCalls)
	}
}

func TestProcessOneWithQueueNoHandlerSkipsCallbackWhenFailPersistFails(t *testing.T) {
	w := NewWorker(nil)
	q := &stubProcessQueue{
		job: &Job{
			ID:          "job-no-handler-fail-error",
			Kind:        JobKind("missing-handler"),
			Attempts:    3,
			MaxAttempts: 3,
		},
		failErr: errors.New("db unavailable"),
	}

	cbCalls := 0
	w.OnDeadLetter(func(_ context.Context, _ *Job, _ error) {
		cbCalls++
	})

	processed := w.processOneWithQueue(context.Background(), q)
	if !processed {
		t.Fatalf("expected processOneWithQueue to process one job")
	}
	if q.failCalls != 1 {
		t.Fatalf("expected fail to be called once, got %d", q.failCalls)
	}
	if cbCalls != 0 {
		t.Fatalf("expected no dead-letter callback when fail persistence fails, got %d", cbCalls)
	}
}

func TestProcessOneWithQueueHandlerFailureDeadLettersOnFinalAttempt(t *testing.T) {
	w := NewWorker(nil)
	q := &stubProcessQueue{
		job: &Job{
			ID:          "job-final-with-handler",
			Kind:        KindActionRun,
			Attempts:    2,
			MaxAttempts: 2,
		},
	}

	handlerErr := errors.New("handler exploded")
	w.Register(KindActionRun, func(interface{ Deadline() (time.Time, bool) }, *Job) error {
		return handlerErr
	})

	cbCalls := 0
	var cbErr error
	w.OnDeadLetter(func(_ context.Context, _ *Job, jobErr error) {
		cbCalls++
		cbErr = jobErr
	})

	processed := w.processOneWithQueue(context.Background(), q)
	if !processed {
		t.Fatalf("expected processOneWithQueue to process one job")
	}
	if q.failCalls != 1 {
		t.Fatalf("expected fail to be called once, got %d", q.failCalls)
	}
	if q.completeCalls != 0 {
		t.Fatalf("expected complete not to be called on handler failure, got %d", q.completeCalls)
	}
	if cbCalls != 1 {
		t.Fatalf("expected dead-letter callback once, got %d", cbCalls)
	}
	if !errors.Is(cbErr, handlerErr) {
		t.Fatalf("expected callback error to match handler error, got %v", cbErr)
	}
}

func TestProcessOneWithQueueHandlerPanicIsRecoveredAndFailed(t *testing.T) {
	w := NewWorker(nil)
	q := &stubProcessQueue{
		job: &Job{
			ID:          "job-handler-panic",
			Kind:        KindTerminalCommand,
			Attempts:    1,
			MaxAttempts: 3,
		},
	}

	w.Register(KindTerminalCommand, func(interface{ Deadline() (time.Time, bool) }, *Job) error {
		panic("boom")
	})

	processed := w.processOneWithQueue(context.Background(), q)
	if !processed {
		t.Fatalf("expected processOneWithQueue to process one job")
	}
	if q.failCalls != 1 {
		t.Fatalf("expected fail to be called once after panic, got %d", q.failCalls)
	}
	if !strings.Contains(q.failErrMsg, "panic") {
		t.Fatalf("expected panic-derived failure message, got %q", q.failErrMsg)
	}
	if q.completeCalls != 0 {
		t.Fatalf("expected complete not to be called on panic, got %d", q.completeCalls)
	}
}

func TestProcessOneWithQueueDeadLetterCallbackPanicIsRecovered(t *testing.T) {
	w := NewWorker(nil)
	q := &stubProcessQueue{
		job: &Job{
			ID:          "job-deadletter-callback-panic",
			Kind:        JobKind("missing-handler"),
			Attempts:    2,
			MaxAttempts: 2,
		},
	}
	w.OnDeadLetter(func(context.Context, *Job, error) {
		panic("callback panic")
	})

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("expected callback panic to be recovered, got %v", recovered)
		}
	}()

	processed := w.processOneWithQueue(context.Background(), q)
	if !processed {
		t.Fatalf("expected processOneWithQueue to process one job")
	}
	if q.failCalls != 1 {
		t.Fatalf("expected fail to be called once, got %d", q.failCalls)
	}
}
