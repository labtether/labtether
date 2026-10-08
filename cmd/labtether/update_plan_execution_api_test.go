package main

import (
	"bytes"
	"encoding/json"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groupmaintenance"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/updates"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUpdatePlanCreateAndExecuteQueueUnavailable(t *testing.T) {
	sut := newTestAPIServer(t)

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

func TestUpdatePlanCreateRejectsUnsupportedScope(t *testing.T) {
	sut := newTestAPIServer(t)
	payload := []byte(`{"name":"Unsupported Updates","targets":["lab-host-01"],"scopes":["docker_images"]}`)
	req := httptest.NewRequest(http.MethodPost, "/updates/plans", bytes.NewReader(payload))
	rec := httptest.NewRecorder()

	sut.handleUpdatePlans(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "not supported") {
		t.Fatalf("expected unsupported-scope error, got %s", rec.Body.String())
	}
}

func TestUpdatePlanCreateRejectsMissingOrBlankTargets(t *testing.T) {
	for _, payload := range []string{
		`{"name":"Missing Target","scopes":["os_packages"]}`,
		`{"name":"Blank Target","targets":[" "],"scopes":["os_packages"]}`,
	} {
		t.Run(payload, func(t *testing.T) {
			sut := newTestAPIServer(t)
			req := httptest.NewRequest(http.MethodPost, "/updates/plans", strings.NewReader(payload))
			rec := httptest.NewRecorder()

			sut.handleUpdatePlans(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(strings.ToLower(rec.Body.String()), "target") {
				t.Fatalf("expected target validation error, got %s", rec.Body.String())
			}
		})
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
