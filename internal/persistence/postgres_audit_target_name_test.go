package persistence

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/audit"
)

// Audit rows named the changed object only by ID ("Deleted a webhook
// wh_1793…"). A row keeps the name its writer gave, a row about an object
// that still exists gets that object's name from the store, and the name
// is served as details.target_name, which the console reads.
func TestPostgresAuditRowsKeepTheChangedObjectsName(t *testing.T) {
	s := newTestPostgresStore(t)
	suffix := time.Now().UTC().UnixNano()
	deviceID := fmt.Sprintf("audit-name-device-%d", suffix)
	if _, err := s.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: deviceID, Name: "nas01", Source: "agent", Type: "host", Status: "online"}); err != nil {
		t.Fatalf("seed device: %v", err)
	}
	ids := map[string]string{
		"given":   fmt.Sprintf("audit-name-given-%d", suffix),
		"lookup":  fmt.Sprintf("audit-name-lookup-%d", suffix),
		"unknown": fmt.Sprintf("audit-name-unknown-%d", suffix),
	}
	t.Cleanup(func() {
		_, _ = s.pool.Exec(context.Background(), `DELETE FROM audit_events WHERE id = ANY($1::text[])`, []string{ids["given"], ids["lookup"], ids["unknown"]})
		_ = s.DeleteAsset(deviceID)
	})

	now := time.Now().UTC()
	events := []audit.Event{
		{ID: ids["given"], Type: "webhook.deleted", Target: "wh-gone", TargetName: "nightly", Details: map[string]any{"events": "alert.fired"}, Timestamp: now},
		{ID: ids["lookup"], Type: "terminal.session.created", Target: deviceID, Timestamp: now},
		{ID: ids["unknown"], Type: "saved_action.deleted", Target: "act-private", Timestamp: now},
	}
	for _, event := range events {
		if err := s.Append(event); err != nil {
			t.Fatalf("append %s: %v", event.ID, err)
		}
	}
	listed, err := s.List(500, 0)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	got := map[string]audit.Event{}
	for _, event := range listed {
		got[event.ID] = event
	}
	if e := got[ids["given"]]; e.TargetName != "nightly" || e.Details["target_name"] != "nightly" || e.Details["events"] != "alert.fired" {
		t.Fatalf("named row = name %q details %v, want nightly with details intact", e.TargetName, e.Details)
	}
	if e := got[ids["lookup"]]; e.TargetName != "nas01" || e.Details["target_name"] != "nas01" {
		t.Fatalf("row about an existing device has name %q, want nas01", e.TargetName)
	}
	if e := got[ids["unknown"]]; e.TargetName != "" {
		t.Fatalf("row about an unknown object has name %q, want none", e.TargetName)
	}
}
