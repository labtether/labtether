package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/logs"
)

func mustCreateGroupWithParent(t *testing.T, sut *apiServer, name string, parentID string) groups.Group {
	t.Helper()
	body := map[string]any{"name": name}
	if parentID != "" {
		body["parent_group_id"] = parentID
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/groups", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	sut.handleGroups(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating group %q, got %d body=%s", name, rec.Code, rec.Body.String())
	}
	var resp struct {
		Group groups.Group `json:"group"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode group response: %v", err)
	}
	return resp.Group
}

func TestHandleGroupReliabilityCollectionUsesGroupKey(t *testing.T) {
	sut := newTestAPIServer(t)
	group := mustCreateGroupWithParent(t, sut, "Reliability Group", "")

	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  "group-reliability-node",
		Type:     "host",
		Name:     "Group Reliability Node",
		Source:   "agent",
		GroupID:  group.ID,
		Status:   "online",
		Platform: "linux",
	}); err != nil {
		t.Fatalf("failed to create asset heartbeat: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/groups/reliability", nil)
	rec := httptest.NewRecorder()
	sut.handleGroupActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode payload: %v", err)
	}

	if _, ok := payload["groupmaintenance"]; ok {
		t.Fatalf("expected groups payload without legacy groupmaintenance key: %s", rec.Body.String())
	}
	rows, ok := payload["groups"].([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("expected one group reliability row, got %#v", payload["groups"])
	}
	row, ok := rows[0].(map[string]any)
	if !ok {
		t.Fatalf("expected group reliability row object, got %#v", rows[0])
	}
	groupObj, ok := row["group"].(map[string]any)
	if !ok || groupObj["id"] != group.ID {
		t.Fatalf("expected reliability row for group %q, got %#v", group.ID, row["group"])
	}
}

func TestHandleGroupTimelineUsesGroupKey(t *testing.T) {
	sut := newTestAPIServer(t)
	group := mustCreateGroupWithParent(t, sut, "Timeline Group", "")

	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  "group-timeline-node",
		Type:     "host",
		Name:     "Group Timeline Node",
		Source:   "agent",
		GroupID:  group.ID,
		Status:   "online",
		Platform: "linux",
	}); err != nil {
		t.Fatalf("failed to create asset heartbeat: %v", err)
	}

	if err := sut.logStore.AppendEvent(logs.Event{
		ID:        "group-timeline-log",
		AssetID:   "group-timeline-node",
		Source:    "agent",
		Level:     "warn",
		Message:   "group timeline event",
		Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("failed to append log event: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/groups/"+group.ID+"/timeline", nil)
	rec := httptest.NewRecorder()
	sut.handleGroupActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode payload: %v", err)
	}

	groupObj, ok := payload["group"].(map[string]any)
	if !ok || groupObj["id"] != group.ID {
		t.Fatalf("expected group timeline payload for %q, got %#v", group.ID, payload["group"])
	}
	if _, ok := payload["events"].([]any); !ok {
		t.Fatalf("expected events array, got %#v", payload["events"])
	}
}
