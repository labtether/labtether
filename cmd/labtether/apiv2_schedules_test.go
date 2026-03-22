package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/schedules"
)

// scheduleCreateResponse is the envelope around a single scheduled task.
type scheduleCreateResponse struct {
	Data schedules.ScheduledTask `json:"data"`
}

// scheduleListResponse is the envelope for a list of scheduled tasks.
type scheduleListResponse struct {
	Data []schedules.ScheduledTask `json:"data"`
	Meta struct {
		Total int `json:"total"`
	} `json:"meta"`
}

func TestHandleV2Schedules_Create(t *testing.T) {
	s := newTestAPIServer(t)

	body := `{"name":"nightly-cleanup","cron_expr":"0 2 * * *","command":"rm -rf /tmp/cache","targets":["srv1","srv2"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/schedules", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2Schedules(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp scheduleCreateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	task := resp.Data
	if task.ID == "" {
		t.Error("expected non-empty ID")
	}
	if task.Name != "nightly-cleanup" {
		t.Errorf("expected name 'nightly-cleanup', got %q", task.Name)
	}
	if task.CronExpr != "0 2 * * *" {
		t.Errorf("expected cron_expr '0 2 * * *', got %q", task.CronExpr)
	}
	if !task.Enabled {
		t.Error("expected task to be enabled by default")
	}
	if len(task.Targets) != 2 {
		t.Errorf("expected 2 targets, got %d", len(task.Targets))
	}
}

func TestHandleV2Schedules_Create_ValidationErrors(t *testing.T) {
	s := newTestAPIServer(t)

	cases := []struct {
		name string
		body string
	}{
		{"missing name", `{"cron_expr":"0 2 * * *","command":"uptime"}`},
		{"missing cron_expr", `{"name":"task","command":"uptime"}`},
		{"missing command", `{"name":"task","cron_expr":"0 2 * * *"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v2/schedules", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			ctx := contextWithPrincipal(req.Context(), "admin", "admin")
			req = req.WithContext(ctx)

			rec := httptest.NewRecorder()
			s.handleV2Schedules(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandleV2Schedules_Create_ScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)

	body := `{"name":"task","cron_expr":"0 2 * * *","command":"uptime"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/schedules", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"schedules:read"}) // no write scope
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2Schedules(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleV2Schedules_List(t *testing.T) {
	s := newTestAPIServer(t)

	// Create two tasks first.
	for _, name := range []string{"task-a", "task-b"} {
		body := `{"name":"` + name + `","cron_expr":"@daily","command":"echo ` + name + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v2/schedules", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		ctx := contextWithPrincipal(req.Context(), "admin", "admin")
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		s.handleV2Schedules(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("failed to create %s: %d %s", name, rec.Code, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/schedules", nil)
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2Schedules(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp scheduleListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Meta.Total != 2 {
		t.Errorf("expected meta.total=2, got %d", resp.Meta.Total)
	}
	if len(resp.Data) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(resp.Data))
	}
}

func TestHandleV2ScheduleActions_Delete(t *testing.T) {
	s := newTestAPIServer(t)

	// Create a task.
	body := `{"name":"to-delete","cron_expr":"@hourly","command":"cleanup"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/schedules", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2Schedules(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("failed to create task: %d %s", rec.Code, rec.Body.String())
	}

	var created scheduleCreateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	taskID := created.Data.ID

	// Delete it.
	delPath := "/api/v2/schedules/" + taskID
	delReq := httptest.NewRequest(http.MethodDelete, delPath, nil)
	ctx2 := contextWithPrincipal(delReq.Context(), "admin", "admin")
	delReq = delReq.WithContext(ctx2)
	delRec := httptest.NewRecorder()
	s.handleV2ScheduleActions(delRec, delReq)

	if delRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", delRec.Code, delRec.Body.String())
	}

	// Verify gone via GET.
	getReq := httptest.NewRequest(http.MethodGet, delPath, nil)
	ctx3 := contextWithPrincipal(getReq.Context(), "admin", "admin")
	getReq = getReq.WithContext(ctx3)
	getRec := httptest.NewRecorder()
	s.handleV2ScheduleActions(getRec, getReq)

	if getRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", getRec.Code)
	}
}

func TestHandleV2ScheduleActions_Patch(t *testing.T) {
	s := newTestAPIServer(t)

	// Create a task.
	body := `{"name":"patch-me","cron_expr":"@hourly","command":"echo before"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/schedules", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2Schedules(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("failed to create task: %d %s", rec.Code, rec.Body.String())
	}

	var created scheduleCreateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	taskID := created.Data.ID

	// Patch enabled to false and update command.
	patchBody := `{"enabled":false,"command":"echo after"}`
	patchPath := "/api/v2/schedules/" + taskID
	patchReq := httptest.NewRequest(http.MethodPatch, patchPath, strings.NewReader(patchBody))
	patchReq.Header.Set("Content-Type", "application/json")
	ctx2 := contextWithPrincipal(patchReq.Context(), "admin", "admin")
	patchReq = patchReq.WithContext(ctx2)
	patchRec := httptest.NewRecorder()
	s.handleV2ScheduleActions(patchRec, patchReq)

	if patchRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", patchRec.Code, patchRec.Body.String())
	}

	var updatedResp scheduleCreateResponse
	if err := json.Unmarshal(patchRec.Body.Bytes(), &updatedResp); err != nil {
		t.Fatalf("failed to decode patch response: %v", err)
	}
	updated := updatedResp.Data
	if updated.Enabled {
		t.Error("expected task to be disabled after patch")
	}
	if updated.Command != "echo after" {
		t.Errorf("expected command 'echo after', got %q", updated.Command)
	}
}

func TestHandleV2ScheduleActions_MethodNotAllowed(t *testing.T) {
	s := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodPut, "/api/v2/schedules/some-id", nil)
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2ScheduleActions(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}
