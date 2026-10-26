package persistence

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/incidents"
)

func TestMemoryIncidentScopeReadsCompleteAndCancellable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store, assetStore := NewMemoryIncidentStore(), NewMemoryAssetStore()
	inc, err := store.CreateIncident(incidents.CreateIncidentRequest{Title: "scope-owner"})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := store.CreateIncident(incidents.CreateIncidentRequest{Title: "other-owner"})
	if err != nil {
		t.Fatal(err)
	}
	wantLinks := map[string]incidents.AlertLink{}
	wantAssets := make([]string, 0, 513)
	for i := 0; i < 513; i++ {
		link, err := store.LinkIncidentAlert(inc.ID, incidents.LinkAlertRequest{
			AlertRuleID: fmt.Sprintf("rule-%04d", i), AlertInstanceID: fmt.Sprintf("instance-%04d", i),
			AlertFingerprint: fmt.Sprintf("fingerprint-%04d", i), LinkType: incidents.LinkTypeRelated})
		if err != nil {
			t.Fatal(err)
		}
		wantLinks[link.ID] = link
		id := fmt.Sprintf("asset-%04d", i)
		if i == 0 {
			id = "asset-Ä"
		}
		if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: id, GroupID: "owned-group"}); err != nil {
			t.Fatal(err)
		}
		wantAssets = append(wantAssets, id)
	}
	if _, err := store.LinkIncidentAlert(foreign.ID, incidents.LinkAlertRequest{AlertInstanceID: "foreign", LinkType: incidents.LinkTypeRelated}); err != nil {
		t.Fatal(err)
	}
	if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: "foreign-asset", GroupID: "other-group"}); err != nil {
		t.Fatal(err)
	}
	old, err := store.ListIncidentAlertLinks(inc.ID, 1000)
	if err != nil || len(old) != 500 {
		t.Fatalf("legacy truncation precondition: %d/%v", len(old), err)
	}
	gotLinks, previous := map[string]incidents.AlertLink{}, ""
	if err := store.ReadIncidentAlertLinksContext(ctx, inc.ID, func(row incidents.AlertLink) error {
		if row.ID <= previous {
			return fmt.Errorf("scope order regressed")
		}
		previous = row.ID
		gotLinks[row.ID] = row
		return nil
	}); err != nil || !reflect.DeepEqual(gotLinks, wantLinks) {
		t.Fatalf("complete exact conjunctive links: count=%d err=%v", len(gotLinks), err)
	}
	var gotAssets []string
	err = assetStore.ReadGroupAssetIDsContext(ctx, "owned-group", func(id string) error { gotAssets = append(gotAssets, id); return nil })
	sort.Strings(wantAssets)
	if err != nil || !reflect.DeepEqual(gotAssets, wantAssets) {
		t.Fatalf("exact current group: %d/%v", len(gotAssets), err)
	}
	// Cancellation after a full page must not deliver a later row or leave a
	// read lock that blocks the next mutation. No store is reentered by a visitor.
	canceled, stop := context.WithCancel(ctx)
	visits := 0
	err = store.ReadIncidentAlertLinksContext(canceled, inc.ID, func(incidents.AlertLink) error {
		visits++
		if visits == 129 {
			stop()
		}
		return nil
	})
	stop()
	if !errors.Is(err, context.Canceled) || visits != 129 {
		t.Fatalf("canceled partial scope acknowledged: %d/%v", visits, err)
	}
	if !store.mu.TryLock() {
		t.Fatal("canceled scope retained its lock")
	}
	store.mu.Unlock()
	// A writer can hold the lock longer than the caller's budget. The reader
	// must return to that caller, not leak a blocked lock-wait goroutine.
	assetStore.mu.Lock()
	short, end := context.WithTimeout(ctx, 10*time.Millisecond)
	err = assetStore.ReadGroupAssetIDsContext(short, "owned-group", func(string) error { t.Error("visited under writer"); return nil })
	end()
	assetStore.mu.Unlock()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lock wait ignored cancellation: %v", err)
	}
	// A corrupt legacy discriminator must not be broadened into a rule-only
	// match, even though other rows in the same scope are valid.
	store.mu.Lock()
	store.links[inc.ID][0].AlertInstanceID = " malformed "
	store.mu.Unlock()
	if err := store.ReadIncidentAlertLinksContext(ctx, inc.ID, func(incidents.AlertLink) error { return nil }); err == nil {
		t.Fatal("malformed scope accepted")
	}
}
