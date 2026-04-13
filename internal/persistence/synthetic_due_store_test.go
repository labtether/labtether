package persistence

import (
	"context"
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
