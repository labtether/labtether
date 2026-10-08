package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groupmaintenance"
	"github.com/labtether/labtether/internal/persistence"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type maintenanceOnlyGroupMaintenanceStore struct {
	windows map[string][]groupmaintenance.MaintenanceWindow
}

func (m *maintenanceOnlyGroupMaintenanceStore) CreateGroupMaintenanceWindow(groupID string, req groupmaintenance.CreateMaintenanceWindowRequest) (groupmaintenance.MaintenanceWindow, error) {
	window := groupmaintenance.MaintenanceWindow{
		ID:             "mw-" + groupID,
		GroupID:        groupID,
		Name:           req.Name,
		StartAt:        req.StartAt.UTC(),
		EndAt:          req.EndAt.UTC(),
		SuppressAlerts: req.SuppressAlerts,
		BlockActions:   req.BlockActions,
		BlockUpdates:   req.BlockUpdates,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	if m.windows == nil {
		m.windows = make(map[string][]groupmaintenance.MaintenanceWindow)
	}
	m.windows[groupID] = append(m.windows[groupID], window)
	return window, nil
}

func (m *maintenanceOnlyGroupMaintenanceStore) GetGroupMaintenanceWindow(groupID, windowID string) (groupmaintenance.MaintenanceWindow, bool, error) {
	for _, window := range m.windows[groupID] {
		if window.ID == windowID {
			return window, true, nil
		}
	}
	return groupmaintenance.MaintenanceWindow{}, false, nil
}

func (m *maintenanceOnlyGroupMaintenanceStore) ListGroupMaintenanceWindows(groupID string, activeAt *time.Time, limit int) ([]groupmaintenance.MaintenanceWindow, error) {
	windows := append([]groupmaintenance.MaintenanceWindow(nil), m.windows[groupID]...)
	if activeAt == nil {
		return windows, nil
	}
	filtered := make([]groupmaintenance.MaintenanceWindow, 0, len(windows))
	for _, window := range windows {
		if !window.StartAt.After(activeAt.UTC()) && !window.EndAt.Before(activeAt.UTC()) {
			filtered = append(filtered, window)
		}
	}
	return filtered, nil
}

func (m *maintenanceOnlyGroupMaintenanceStore) UpdateGroupMaintenanceWindow(groupID, windowID string, req groupmaintenance.UpdateMaintenanceWindowRequest) (groupmaintenance.MaintenanceWindow, error) {
	return groupmaintenance.MaintenanceWindow{}, errors.New("not implemented")
}

func (m *maintenanceOnlyGroupMaintenanceStore) DeleteGroupMaintenanceWindow(groupID, windowID string) error {
	return errors.New("not implemented")
}

func TestActionRunQueueUnavailable(t *testing.T) {
	sut := newTestAPIServer(t)

	payload := []byte(`{"type":"command","actor_id":"owner","target":"lab-host-01","command":"uptime"}`)
	req := httptest.NewRequest(http.MethodPost, "/actions/execute", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	sut.handleActionExecute(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/actions/runs?limit=10", nil)
	listRec := httptest.NewRecorder()
	sut.handleActionRuns(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", listRec.Code)
	}

	auditEvents, err := sut.auditStore.List(20, 0)
	if err != nil {
		t.Fatalf("failed to list audit events: %v", err)
	}

	foundPolicyCheck := false
	foundQueueFailure := false
	for _, event := range auditEvents {
		switch event.Type {
		case "actions.run.policy_checked":
			foundPolicyCheck = true
		case "actions.run.queued":
			if event.Decision == "failed" && strings.Contains(strings.ToLower(event.Reason), "queue unavailable") {
				foundQueueFailure = true
			}
		}
	}
	if !foundPolicyCheck {
		t.Fatalf("expected actions.run.policy_checked audit event")
	}
	if !foundQueueFailure {
		t.Fatalf("expected actions.run.queued failed audit event when queue is unavailable")
	}
}

func TestActionPolicyAuditDoesNotPersistRawCommand(t *testing.T) {
	sut := newTestAPIServer(t)
	secretCommand := "printf LTQA_ACTION_SECRET_b92d"
	payload, err := json.Marshal(map[string]any{
		"type":    "command",
		"target":  "lab-host-01",
		"command": secretCommand,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/actions/execute", bytes.NewReader(payload))
	req = req.WithContext(contextWithPrincipal(req.Context(), "audit-redaction-user", "operator"))
	rec := httptest.NewRecorder()
	sut.handleActionExecute(rec, req)

	events, err := sut.auditStore.List(100, 0)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secretCommand) || strings.Contains(string(encoded), "LTQA_ACTION_SECRET_b92d") {
		t.Fatalf("audit events persisted raw action command: %s", encoded)
	}
	foundMetadata := false
	for _, event := range events {
		if event.Type == "actions.run.policy_checked" && event.Details["command_bytes"] == len([]byte(secretCommand)) {
			foundMetadata = true
		}
	}
	if !foundMetadata {
		t.Fatal("expected non-sensitive command length metadata in action policy audit")
	}
}

func TestActionRunGetByID(t *testing.T) {
	sut := newTestAPIServer(t)
	actionStore := sut.actionStore.(*persistence.MemoryActionStore)

	run, err := actionStore.CreateActionRun(actions.ExecuteRequest{
		Type:    actions.RunTypeCommand,
		Target:  "lab-host-01",
		Command: "uptime",
	})
	if err != nil {
		t.Fatalf("failed to seed action run: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/actions/runs/"+run.ID, nil)
	rec := httptest.NewRecorder()
	sut.handleActionRunActions(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestActionRunDeleteByID(t *testing.T) {
	sut := newTestAPIServer(t)
	actionStore := sut.actionStore.(*persistence.MemoryActionStore)

	run, err := actionStore.CreateActionRun(actions.ExecuteRequest{
		Type:    actions.RunTypeCommand,
		Target:  "lab-host-01",
		Command: "uptime",
	})
	if err != nil {
		t.Fatalf("failed to seed action run: %v", err)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/actions/runs/"+run.ID, nil)
	deleteRec := httptest.NewRecorder()
	sut.handleActionRunActions(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected 200 deleting action run, got %d", deleteRec.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/actions/runs/"+run.ID, nil)
	getRec := httptest.NewRecorder()
	sut.handleActionRunActions(getRec, getReq)
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after deleting action run, got %d", getRec.Code)
	}
}

func TestActionExecuteUsesAuthenticatedActor(t *testing.T) {
	sut := newTestAPIServer(t)

	payload := []byte(`{"type":"command","actor_id":"spoofed","target":"lab-host-01","command":"uptime"}`)
	req := httptest.NewRequest(http.MethodPost, "/actions/execute", bytes.NewReader(payload))
	req = req.WithContext(contextWithUserID(req.Context(), "usr-action-01"))
	rec := httptest.NewRecorder()
	sut.handleActionExecute(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when queue is unavailable, got %d", rec.Code)
	}

	runs, err := sut.actionStore.ListActionRuns(10, 0, "", "")
	if err != nil {
		t.Fatalf("failed to list action runs: %v", err)
	}
	if len(runs) == 0 {
		t.Fatal("expected at least one action run")
	}
	if runs[0].ActorID != "usr-action-01" {
		t.Fatalf("expected action run actor_id to be context user, got %q", runs[0].ActorID)
	}
}

func TestActionExecuteRejectsDuplicateNormalizedParamKeys(t *testing.T) {
	sut := newTestAPIServer(t)

	payload := []byte(`{"type":"command","target":"lab-host-01","command":"uptime","params":{"group_id":"one"," group_id ":"two"}}`)
	req := httptest.NewRequest(http.MethodPost, "/actions/execute", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	sut.handleActionExecute(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "duplicate action param key") {
		t.Fatalf("expected duplicate key error, got %s", rec.Body.String())
	}
}

func TestActionExecuteNormalizesActionParamKeysBeforeQueueing(t *testing.T) {
	sut := newTestAPIServer(t)

	payload := []byte(`{"type":"command","target":"lab-host-01","command":"uptime","params":{" timeout ":" 15 "}}`)
	req := httptest.NewRequest(http.MethodPost, "/actions/execute", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	sut.handleActionExecute(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when queue is unavailable, got %d", rec.Code)
	}

	runs, err := sut.actionStore.ListActionRuns(10, 0, "", "")
	if err != nil {
		t.Fatalf("failed to list action runs: %v", err)
	}
	if len(runs) == 0 {
		t.Fatal("expected at least one action run")
	}
	if got := runs[0].Params["timeout"]; got != "15" {
		t.Fatalf("expected trimmed timeout param, got %q", got)
	}
	if _, exists := runs[0].Params[" timeout "]; exists {
		t.Fatalf("expected raw param key to be removed, got %#v", runs[0].Params)
	}
}

func TestActionExecuteReturnsLockedWhenGroupMaintenanceBlocksActions(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.groupMaintenanceStore = &maintenanceOnlyGroupMaintenanceStore{}

	groupID := mustCreateGroup(t, sut, "Maintenance Group", "maintenance-group")
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  "lab-host-01",
		Type:     "host",
		Name:     "Lab Host 01",
		Source:   "agent",
		GroupID:  groupID,
		Status:   "online",
		Platform: "linux",
	}); err != nil {
		t.Fatalf("seed asset heartbeat: %v", err)
	}

	_, err := sut.groupMaintenanceStore.CreateGroupMaintenanceWindow(groupID, groupmaintenance.CreateMaintenanceWindowRequest{
		Name:           "Block Actions",
		StartAt:        time.Now().UTC().Add(-time.Hour),
		EndAt:          time.Now().UTC().Add(time.Hour),
		SuppressAlerts: true,
		BlockActions:   true,
	})
	if err != nil {
		t.Fatalf("CreateGroupMaintenanceWindow failed: %v", err)
	}

	payload := []byte(`{"type":"command","target":"lab-host-01","command":"uptime"}`)
	req := httptest.NewRequest(http.MethodPost, "/actions/execute", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	sut.handleActionExecute(rec, req)

	if rec.Code != http.StatusLocked {
		t.Fatalf("expected 423, got %d", rec.Code)
	}
}

func TestActionAndUpdateGroupFiltersReturnServiceUnavailableWithoutGroupStore(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.groupStore = nil

	actionReq := httptest.NewRequest(http.MethodGet, "/actions/runs?group_id=group-1", nil)
	actionRec := httptest.NewRecorder()
	sut.handleActionRuns(actionRec, actionReq)
	if actionRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected action runs group filter to return 503 without group store, got %d", actionRec.Code)
	}
	if !strings.Contains(actionRec.Body.String(), "An internal error occurred.") {
		t.Fatalf("expected sanitized error message, got %s", actionRec.Body.String())
	}

	updateReq := httptest.NewRequest(http.MethodGet, "/updates/runs?group_id=group-1", nil)
	updateRec := httptest.NewRecorder()
	sut.handleUpdateRuns(updateRec, updateReq)
	if updateRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected update runs group filter to return 503 without group store, got %d", updateRec.Code)
	}
	if !strings.Contains(updateRec.Body.String(), "An internal error occurred.") {
		t.Fatalf("expected sanitized error message, got %s", updateRec.Body.String())
	}
}
