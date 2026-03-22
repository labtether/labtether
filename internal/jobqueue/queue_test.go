package jobqueue

import (
	"testing"
	"time"
)

func TestJobKindConstants(t *testing.T) {
	kinds := []JobKind{KindTerminalCommand, KindActionRun, KindUpdateRun}
	for _, k := range kinds {
		if k == "" {
			t.Error("job kind must not be empty")
		}
	}
}

func TestJobStatusConstants(t *testing.T) {
	statuses := []JobStatus{StatusQueued, StatusProcessing, StatusCompleted, StatusFailed, StatusDeadLettered}
	for _, s := range statuses {
		if s == "" {
			t.Error("job status must not be empty")
		}
	}
}

func TestNewQueueDefaults(t *testing.T) {
	q := New(nil, 0, 0)
	if q.pollInterval != 500*time.Millisecond {
		t.Errorf("expected 500ms default poll interval, got %s", q.pollInterval)
	}
	if q.MaxAttempts() != 5 {
		t.Errorf("expected 5 default max attempts, got %d", q.MaxAttempts())
	}
}

func TestNewQueueCustom(t *testing.T) {
	q := New(nil, 2*time.Second, 3)
	if q.pollInterval != 2*time.Second {
		t.Errorf("expected 2s poll interval, got %s", q.pollInterval)
	}
	if q.MaxAttempts() != 3 {
		t.Errorf("expected 3 max attempts, got %d", q.MaxAttempts())
	}
}

func TestSetMaxAttempts(t *testing.T) {
	q := New(nil, time.Second, 2)
	if got := q.MaxAttempts(); got != 2 {
		t.Fatalf("expected initial max attempts 2, got %d", got)
	}

	q.SetMaxAttempts(7)
	if got := q.MaxAttempts(); got != 7 {
		t.Fatalf("expected updated max attempts 7, got %d", got)
	}

	q.SetMaxAttempts(0)
	if got := q.MaxAttempts(); got != 7 {
		t.Fatalf("expected invalid update to be ignored, got %d", got)
	}
}

func TestNewDeadLetterEvent(t *testing.T) {
	evt := NewDeadLetterEvent("worker.test", "test_subject", 5, []byte("hello"), nil)
	if evt.Component != "worker.test" {
		t.Errorf("expected component worker.test, got %s", evt.Component)
	}
	if evt.Deliveries != 5 {
		t.Errorf("expected 5 deliveries, got %d", evt.Deliveries)
	}
	if evt.Error != "processing failed" {
		t.Errorf("expected default error message, got %s", evt.Error)
	}
	if evt.PayloadB64 == "" {
		t.Error("expected base64 payload")
	}
}

func TestNewDeadLetterEventWithError(t *testing.T) {
	evt := NewDeadLetterEvent("worker.test", "test_subject", 3, nil, &testError{msg: "custom error"})
	if evt.Error != "custom error" {
		t.Errorf("expected custom error, got %s", evt.Error)
	}
	if evt.PayloadB64 != "" {
		t.Errorf("expected empty payload, got %s", evt.PayloadB64)
	}
}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }
