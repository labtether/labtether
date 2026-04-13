package main

import (
	"context"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/synthetic"
)

type syntheticRunnerStore struct {
	listChecks         []synthetic.Check
	listCalls          int
	recordedCheckIDs   []string
	updatedCheckStatus []string
}

func (s *syntheticRunnerStore) CreateSyntheticCheck(req synthetic.CreateCheckRequest) (synthetic.Check, error) {
	return synthetic.Check{}, nil
}

func (s *syntheticRunnerStore) GetSyntheticCheck(id string) (synthetic.Check, bool, error) {
	for _, check := range s.listChecks {
		if check.ID == id {
			return check, true, nil
		}
	}
	return synthetic.Check{}, false, nil
}

func (s *syntheticRunnerStore) GetSyntheticCheckByServiceID(context.Context, string) (*synthetic.Check, error) {
	return nil, nil
}

func (s *syntheticRunnerStore) ListSyntheticChecks(limit int, enabledOnly bool) ([]synthetic.Check, error) {
	s.listCalls++
	return append([]synthetic.Check(nil), s.listChecks...), nil
}

func (s *syntheticRunnerStore) UpdateSyntheticCheck(id string, req synthetic.UpdateCheckRequest) (synthetic.Check, error) {
	return synthetic.Check{}, nil
}

func (s *syntheticRunnerStore) DeleteSyntheticCheck(id string) error { return nil }

func (s *syntheticRunnerStore) RecordSyntheticResult(checkID string, result synthetic.Result) (synthetic.Result, error) {
	s.recordedCheckIDs = append(s.recordedCheckIDs, checkID)
	result.CheckID = checkID
	if result.CheckedAt.IsZero() {
		result.CheckedAt = time.Now().UTC()
	}
	return result, nil
}

func (s *syntheticRunnerStore) ListSyntheticResults(checkID string, limit int) ([]synthetic.Result, error) {
	return nil, nil
}

func (s *syntheticRunnerStore) UpdateSyntheticCheckStatus(id string, status string, runAt time.Time) error {
	s.updatedCheckStatus = append(s.updatedCheckStatus, id+":"+status)
	return nil
}

type syntheticRunnerFallbackStore struct {
	syntheticRunnerStore
}

type syntheticRunnerDueStore struct {
	syntheticRunnerStore
	dueChecks []synthetic.Check
	dueCalls  int
}

func (s *syntheticRunnerDueStore) ListDueSyntheticChecks(_ context.Context, _ time.Time, limit int) ([]synthetic.Check, error) {
	s.dueCalls++
	return append([]synthetic.Check(nil), s.dueChecks...), nil
}

func (s *syntheticRunnerDueStore) GetSyntheticCheck(id string) (synthetic.Check, bool, error) {
	for _, check := range s.dueChecks {
		if check.ID == id {
			return check, true, nil
		}
	}
	return s.syntheticRunnerStore.GetSyntheticCheck(id)
}

func TestRunPendingSyntheticChecksPrefersDueStore(t *testing.T) {
	store := &syntheticRunnerDueStore{
		dueChecks: []synthetic.Check{{
			ID:              "check-1",
			Name:            "Unsupported",
			CheckType:       "unsupported",
			Target:          "unused",
			Enabled:         true,
			IntervalSeconds: 60,
		}},
	}
	sut := newTestAPIServer(t)
	sut.syntheticStore = store

	sut.runPendingSyntheticChecks(context.Background())

	if store.dueCalls != 1 {
		t.Fatalf("expected due store to be called once, got %d", store.dueCalls)
	}
	if store.listCalls != 0 {
		t.Fatalf("expected generic list not to be called, got %d", store.listCalls)
	}
	if len(store.recordedCheckIDs) != 1 || store.recordedCheckIDs[0] != "check-1" {
		t.Fatalf("unexpected recorded checks: %v", store.recordedCheckIDs)
	}
}

func TestRunPendingSyntheticChecksSkipsChecksAlreadyRunning(t *testing.T) {
	store := &syntheticRunnerDueStore{
		dueChecks: []synthetic.Check{{
			ID:              "check-1",
			Name:            "Unsupported",
			CheckType:       "unsupported",
			Target:          "unused",
			Enabled:         true,
			IntervalSeconds: 60,
		}},
	}
	sut := newTestAPIServer(t)
	sut.syntheticStore = store
	sut.syntheticCheckRunState.Store("check-1", struct{}{})

	sut.runPendingSyntheticChecks(context.Background())

	if len(store.recordedCheckIDs) != 0 {
		t.Fatalf("expected no recorded checks, got %v", store.recordedCheckIDs)
	}
}

func TestRunPendingSyntheticChecksFallbackFiltersNonPositiveAndNotDueIntervals(t *testing.T) {
	now := time.Now().UTC()
	store := &syntheticRunnerFallbackStore{
		syntheticRunnerStore: syntheticRunnerStore{
			listChecks: []synthetic.Check{
				{
					ID:              "due",
					Name:            "Due",
					CheckType:       "unsupported",
					Target:          "unused",
					Enabled:         true,
					IntervalSeconds: 60,
					LastRunAt:       timePtr(now.Add(-2 * time.Minute)),
				},
				{
					ID:              "not-due",
					Name:            "Not Due",
					CheckType:       "unsupported",
					Target:          "unused",
					Enabled:         true,
					IntervalSeconds: 300,
					LastRunAt:       timePtr(now.Add(-30 * time.Second)),
				},
				{
					ID:              "invalid-interval",
					Name:            "Invalid",
					CheckType:       "unsupported",
					Target:          "unused",
					Enabled:         true,
					IntervalSeconds: 0,
				},
			},
		},
	}
	sut := newTestAPIServer(t)
	sut.syntheticStore = store

	sut.runPendingSyntheticChecks(context.Background())

	if store.listCalls != 1 {
		t.Fatalf("expected fallback list to be called once, got %d", store.listCalls)
	}
	if len(store.recordedCheckIDs) != 1 || store.recordedCheckIDs[0] != "due" {
		t.Fatalf("unexpected recorded checks: %v", store.recordedCheckIDs)
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}
