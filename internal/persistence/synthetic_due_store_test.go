package persistence

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/synthetic"
)

func TestMemorySyntheticStoreListDueSyntheticChecksOrdersOldestFirst(t *testing.T) {
	store := NewMemorySyntheticStore()
	now := time.Now().UTC()

	first, err := store.CreateSyntheticCheck(synthetic.CreateCheckRequest{
		Name:            "Never Run",
		CheckType:       synthetic.CheckTypeHTTP,
		Target:          "https://example.com",
		IntervalSeconds: 60,
	})
	if err != nil {
		t.Fatalf("create first check: %v", err)
	}
	second, err := store.CreateSyntheticCheck(synthetic.CreateCheckRequest{
		Name:            "Old Due",
		CheckType:       synthetic.CheckTypeHTTP,
		Target:          "https://example.org",
		IntervalSeconds: 60,
	})
	if err != nil {
		t.Fatalf("create second check: %v", err)
	}
	third, err := store.CreateSyntheticCheck(synthetic.CreateCheckRequest{
		Name:            "Not Due Yet",
		CheckType:       synthetic.CheckTypeHTTP,
		Target:          "https://example.net",
		IntervalSeconds: 300,
	})
	if err != nil {
		t.Fatalf("create third check: %v", err)
	}

	oldRun := now.Add(-5 * time.Minute)
	recentRun := now.Add(-30 * time.Second)
	if err := store.UpdateSyntheticCheckStatus(second.ID, synthetic.ResultStatusOK, oldRun); err != nil {
		t.Fatalf("update second status: %v", err)
	}
	if err := store.UpdateSyntheticCheckStatus(third.ID, synthetic.ResultStatusOK, recentRun); err != nil {
		t.Fatalf("update third status: %v", err)
	}

	due, err := store.ListDueSyntheticChecks(context.Background(), now, 10)
	if err != nil {
		t.Fatalf("list due checks: %v", err)
	}
	if len(due) != 2 {
		t.Fatalf("expected 2 due checks, got %d", len(due))
	}
	if due[0].ID != first.ID || due[1].ID != second.ID {
		t.Fatalf("unexpected due order: got [%s %s], want [%s %s]", due[0].ID, due[1].ID, first.ID, second.ID)
	}
}

func TestMemorySyntheticStoreRejectsOutOfRangeIntervals(t *testing.T) {
	store := NewMemorySyntheticStore()

	_, err := store.CreateSyntheticCheck(synthetic.CreateCheckRequest{
		Name:            "Too Large",
		CheckType:       synthetic.CheckTypeHTTP,
		Target:          "https://example.com",
		IntervalSeconds: synthetic.MaxIntervalSeconds + 1,
	})
	if !errors.Is(err, synthetic.ErrInvalidInterval) {
		t.Fatalf("create error = %v, want ErrInvalidInterval", err)
	}

	check, err := store.CreateSyntheticCheck(synthetic.CreateCheckRequest{
		Name:      "Homepage",
		CheckType: synthetic.CheckTypeHTTP,
		Target:    "https://example.com",
	})
	if err != nil {
		t.Fatalf("create check: %v", err)
	}

	tooLarge := synthetic.MaxIntervalSeconds + 1
	_, err = store.UpdateSyntheticCheck(check.ID, synthetic.UpdateCheckRequest{IntervalSeconds: &tooLarge})
	if !errors.Is(err, synthetic.ErrInvalidInterval) {
		t.Fatalf("update error = %v, want ErrInvalidInterval", err)
	}
}

func TestMemorySyntheticStoreListDueSyntheticChecksSkipsOutOfRangeIntervals(t *testing.T) {
	store := NewMemorySyntheticStore()
	now := time.Now().UTC()
	lastRunAt := now.Add(-time.Hour)

	store.mu.Lock()
	store.checks["overflow"] = synthetic.Check{
		ID:              "overflow",
		Name:            "Overflow",
		CheckType:       synthetic.CheckTypeHTTP,
		Target:          "https://example.com",
		IntervalSeconds: synthetic.MaxIntervalSeconds + 1,
		Enabled:         true,
		LastRunAt:       &lastRunAt,
		CreatedAt:       now.Add(-time.Hour),
		UpdatedAt:       now.Add(-time.Hour),
	}
	store.mu.Unlock()

	due, err := store.ListDueSyntheticChecks(context.Background(), now, 10)
	if err != nil {
		t.Fatalf("list due checks: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("expected no due checks, got %d", len(due))
	}
}
