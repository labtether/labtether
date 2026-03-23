package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groupmaintenance"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/updates"
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

func TestUpdatePlanCreateAndExecuteQueueUnavailable(t *testing.T) {
	sut := newTestAPIServer(t)

	createPayload := []byte(`{"name":"Weekly Updates","targets":["lab-host-01"],"scopes":["os_packages","docker_images"],"default_dry_run":true}`)
	createReq := httptest.NewRequest(http.MethodPost, "/updates/plans", bytes.NewReader(createPayload))
	createRec := httptest.NewRecorder()
	sut.handleUpdatePlans(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createRec.Code)
	}

	var createResponse struct {
		Plan struct {
			ID string `json:"id"`
		} `json:"plan"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResponse); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	if createResponse.Plan.ID == "" {
		t.Fatalf("expected plan id")
	}

	execReq := httptest.NewRequest(http.MethodPost, "/updates/plans/"+createResponse.Plan.ID+"/execute", bytes.NewReader([]byte(`{"actor_id":"owner"}`)))
	execRec := httptest.NewRecorder()
	sut.handleUpdatePlanActions(execRec, execReq)
	if execRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", execRec.Code)
	}

	runReq := httptest.NewRequest(http.MethodGet, "/updates/runs?limit=10", nil)
	runRec := httptest.NewRecorder()
	sut.handleUpdateRuns(runRec, runReq)
	if runRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", runRec.Code)
	}

	auditEvents, err := sut.auditStore.List(20, 0)
	if err != nil {
		t.Fatalf("failed to list audit events: %v", err)
	}

	foundQueueFailure := false
	for _, event := range auditEvents {
		if event.Type == "updates.run.queued" &&
			event.Decision == "failed" &&
			strings.Contains(strings.ToLower(event.Reason), "queue unavailable") {
			foundQueueFailure = true
			break
		}
	}
	if !foundQueueFailure {
		t.Fatalf("expected updates.run.queued failed audit event when queue is unavailable")
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

func TestUpdateRunGetByID(t *testing.T) {
	sut := newTestAPIServer(t)
	updateStore := sut.updateStore.(*persistence.MemoryUpdateStore)

	plan, err := updateStore.CreateUpdatePlan(updates.CreatePlanRequest{
		Name:    "Test Plan",
		Targets: []string{"lab-host-01"},
	})
	if err != nil {
		t.Fatalf("failed to seed update plan: %v", err)
	}

	run, err := updateStore.CreateUpdateRun(plan, updates.ExecutePlanRequest{ActorID: "owner"})
	if err != nil {
		t.Fatalf("failed to seed update run: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/updates/runs/"+run.ID, nil)
	rec := httptest.NewRecorder()
	sut.handleUpdateRunActions(rec, req)
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

func TestUpdateRunDeleteByID(t *testing.T) {
	sut := newTestAPIServer(t)
	updateStore := sut.updateStore.(*persistence.MemoryUpdateStore)

	plan, err := updateStore.CreateUpdatePlan(updates.CreatePlanRequest{
		Name:    "Delete Run Plan",
		Targets: []string{"lab-host-01"},
	})
	if err != nil {
		t.Fatalf("failed to seed update plan: %v", err)
	}

	run, err := updateStore.CreateUpdateRun(plan, updates.ExecutePlanRequest{ActorID: "owner"})
	if err != nil {
		t.Fatalf("failed to seed update run: %v", err)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/updates/runs/"+run.ID, nil)
	deleteRec := httptest.NewRecorder()
	sut.handleUpdateRunActions(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected 200 deleting update run, got %d", deleteRec.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/updates/runs/"+run.ID, nil)
	getRec := httptest.NewRecorder()
	sut.handleUpdateRunActions(getRec, getReq)
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after deleting update run, got %d", getRec.Code)
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

func TestUpdateExecuteUsesAuthenticatedActor(t *testing.T) {
	sut := newTestAPIServer(t)

	createPayload := []byte(`{"name":"Nightly Updates","targets":["lab-host-01"],"scopes":["os_packages"]}`)
	createReq := httptest.NewRequest(http.MethodPost, "/updates/plans", bytes.NewReader(createPayload))
	createRec := httptest.NewRecorder()
	sut.handleUpdatePlans(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createRec.Code)
	}

	var createResponse struct {
		Plan struct {
			ID string `json:"id"`
		} `json:"plan"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResponse); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	if createResponse.Plan.ID == "" {
		t.Fatalf("expected plan id")
	}

	execReq := httptest.NewRequest(http.MethodPost, "/updates/plans/"+createResponse.Plan.ID+"/execute", bytes.NewReader([]byte(`{"actor_id":"spoofed-update-user"}`)))
	execReq = execReq.WithContext(contextWithUserID(execReq.Context(), "usr-update-01"))
	execRec := httptest.NewRecorder()
	sut.handleUpdatePlanActions(execRec, execReq)
	if execRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when queue is unavailable, got %d", execRec.Code)
	}

	runs, err := sut.updateStore.ListUpdateRuns(10, "")
	if err != nil {
		t.Fatalf("failed to list update runs: %v", err)
	}
	if len(runs) == 0 {
		t.Fatal("expected at least one update run")
	}
	if runs[0].ActorID != "usr-update-01" {
		t.Fatalf("expected update run actor_id to be context user, got %q", runs[0].ActorID)
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

func TestUpdatePlanExecuteReturnsLockedWhenGroupMaintenanceBlocksUpdates(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.groupMaintenanceStore = &maintenanceOnlyGroupMaintenanceStore{}

	groupID := mustCreateGroup(t, sut, "Maintenance Update Group", "maintenance-update-group")
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
		Name:           "Block Updates",
		StartAt:        time.Now().UTC().Add(-time.Hour),
		EndAt:          time.Now().UTC().Add(time.Hour),
		SuppressAlerts: true,
		BlockUpdates:   true,
	})
	if err != nil {
		t.Fatalf("CreateGroupMaintenanceWindow failed: %v", err)
	}

	createPayload := []byte(`{"name":"Weekly Updates","targets":["lab-host-01"],"scopes":["os_packages"],"default_dry_run":true}`)
	createReq := httptest.NewRequest(http.MethodPost, "/updates/plans", bytes.NewReader(createPayload))
	createRec := httptest.NewRecorder()
	sut.handleUpdatePlans(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createRec.Code)
	}

	var createResponse struct {
		Plan struct {
			ID string `json:"id"`
		} `json:"plan"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResponse); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	execReq := httptest.NewRequest(http.MethodPost, "/updates/plans/"+createResponse.Plan.ID+"/execute", bytes.NewReader([]byte(`{}`)))
	execRec := httptest.NewRecorder()
	sut.handleUpdatePlanActions(execRec, execReq)
	if execRec.Code != http.StatusLocked {
		t.Fatalf("expected 423, got %d", execRec.Code)
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
