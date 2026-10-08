package main

import (
	"encoding/json"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleWebServiceSyncMethodNotAllowed(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/web/sync", nil)
	rec := httptest.NewRecorder()

	sut.handleWebServiceSync(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleWebServiceSyncNoConnectedAgents(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/services/web/sync", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	sut.handleWebServiceSync(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestHandleWebServiceSyncHostNotConnected(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/services/web/sync?host=node-1", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	sut.handleWebServiceSync(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestHandleWebServiceSyncInvalidBody(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/services/web/sync", strings.NewReader(`{"host_asset_id"`))
	rec := httptest.NewRecorder()

	sut.handleWebServiceSync(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleWebServiceManualCRUD(t *testing.T) {
	sut := newTestAPIServer(t)

	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "host-1",
		Type:    "node",
		Name:    "Host 1",
		Source:  "agent",
		Status:  "online",
	})
	if err != nil {
		t.Fatalf("failed to seed host asset: %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/services/web/manual", strings.NewReader(`{
		"host_asset_id":"host-1",
		"name":"My App",
		"category":"Development",
		"url":"http://host-1:9999"
	}`))
	createRec := httptest.NewRecorder()
	sut.handleWebServiceManual(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 on create, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	var createResp struct {
		Service map[string]any `json:"service"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	id, _ := createResp.Service["id"].(string)
	if strings.TrimSpace(id) == "" {
		t.Fatalf("expected created manual service id")
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/services/web/manual?host=host-1", nil)
	listRec := httptest.NewRecorder()
	sut.handleWebServiceManual(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on list, got %d", listRec.Code)
	}
	var listResp struct {
		Services []map[string]any `json:"services"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("failed to decode list response: %v", err)
	}
	if len(listResp.Services) != 1 {
		t.Fatalf("expected 1 manual service, got %d", len(listResp.Services))
	}

	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/services/web/manual/"+id, strings.NewReader(`{
		"name":"My App Renamed",
		"category":"Management"
	}`))
	patchReq.URL.Path = "/api/v1/services/web/manual/" + id
	patchRec := httptest.NewRecorder()
	sut.handleWebServiceManualActions(patchRec, patchReq)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on patch, got %d body=%s", patchRec.Code, patchRec.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/services/web/manual/"+id, nil)
	deleteReq.URL.Path = "/api/v1/services/web/manual/" + id
	deleteRec := httptest.NewRecorder()
	sut.handleWebServiceManualActions(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 on delete, got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}
}

func TestHandleWebServiceManualActionsRejectsExtraPathSegments(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/services/web/manual/manual-1/extra", nil)
	req.URL.Path = "/api/v1/services/web/manual/manual-1/extra"
	rec := httptest.NewRecorder()
	sut.handleWebServiceManualActions(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for manual action path with extra segments, got %d", rec.Code)
	}
}

func TestHandleWebServiceIconLibraryCRUD(t *testing.T) {
	sut := newTestAPIServer(t)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/services/web/icon-library", strings.NewReader(`{
		"name":"My Custom Icon",
		"data_url":"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMBAAII3n8AAAAASUVORK5CYII="
	}`))
	createRec := httptest.NewRecorder()
	sut.handleWebServiceIconLibrary(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 on create, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	var createResp struct {
		Icon webServiceIconLibraryEntry `json:"icon"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	if strings.TrimSpace(createResp.Icon.ID) == "" {
		t.Fatalf("expected created icon id")
	}
	if createResp.Icon.Name != "My Custom Icon" {
		t.Fatalf("icon name = %q, want %q", createResp.Icon.Name, "My Custom Icon")
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/services/web/icon-library", nil)
	getRec := httptest.NewRecorder()
	sut.handleWebServiceIconLibrary(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on get, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	var getResp struct {
		Icons []webServiceIconLibraryEntry `json:"icons"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("failed to decode get response: %v", err)
	}
	if len(getResp.Icons) != 1 {
		t.Fatalf("expected 1 icon in library, got %d", len(getResp.Icons))
	}
	createdID := getResp.Icons[0].ID
	if strings.TrimSpace(createdID) == "" {
		t.Fatalf("expected non-empty icon id in list response")
	}

	renameReq := httptest.NewRequest(http.MethodPatch, "/api/v1/services/web/icon-library?id="+createdID, strings.NewReader(`{
		"name":"Renamed Icon"
	}`))
	renameRec := httptest.NewRecorder()
	sut.handleWebServiceIconLibrary(renameRec, renameReq)
	if renameRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on rename, got %d body=%s", renameRec.Code, renameRec.Body.String())
	}

	var renameResp struct {
		Icon webServiceIconLibraryEntry `json:"icon"`
	}
	if err := json.Unmarshal(renameRec.Body.Bytes(), &renameResp); err != nil {
		t.Fatalf("failed to decode rename response: %v", err)
	}
	if renameResp.Icon.Name != "Renamed Icon" {
		t.Fatalf("renamed icon name = %q, want %q", renameResp.Icon.Name, "Renamed Icon")
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/services/web/icon-library?id="+createResp.Icon.ID, nil)
	deleteRec := httptest.NewRecorder()
	sut.handleWebServiceIconLibrary(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 on delete, got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}

	getAgainReq := httptest.NewRequest(http.MethodGet, "/api/v1/services/web/icon-library", nil)
	getAgainRec := httptest.NewRecorder()
	sut.handleWebServiceIconLibrary(getAgainRec, getAgainReq)
	if getAgainRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on second get, got %d body=%s", getAgainRec.Code, getAgainRec.Body.String())
	}
	var getAgainResp struct {
		Icons []webServiceIconLibraryEntry `json:"icons"`
	}
	if err := json.Unmarshal(getAgainRec.Body.Bytes(), &getAgainResp); err != nil {
		t.Fatalf("failed to decode second get response: %v", err)
	}
	if len(getAgainResp.Icons) != 0 {
		t.Fatalf("expected empty icon library after delete, got %d", len(getAgainResp.Icons))
	}
}

func TestHandleWebServiceIconLibraryRejectsInvalidDataURL(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/services/web/icon-library", strings.NewReader(`{
		"name":"Bad Icon",
		"data_url":"https://example.com/icon.png"
	}`))
	rec := httptest.NewRecorder()
	sut.handleWebServiceIconLibrary(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid data_url, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleWebServiceCompat(t *testing.T) {
	sut := newTestAPIServer(t)

	report := agentmgr.WebServiceReportData{
		HostAssetID: "host-1",
		Services: []agentmgr.DiscoveredWebService{
			{
				ID:          "svc-ha",
				Name:        "Home Assistant",
				Category:    "Home Automation",
				URL:         "http://host-1:8123",
				Status:      "up",
				Source:      "scan",
				HostAssetID: "host-1",
				Metadata: map[string]string{
					"compat_connector":  "homeassistant",
					"compat_confidence": "0.97",
					"compat_auth_hint":  "token",
					"compat_profile":    "homeassistant.api.root",
					"compat_evidence":   "api running message",
				},
			},
			{
				ID:          "svc-portainer",
				Name:        "Portainer",
				Category:    "Management",
				URL:         "https://host-1:9443",
				Status:      "up",
				Source:      "docker",
				HostAssetID: "host-1",
				Metadata: map[string]string{
					"compat_connector":  "portainer",
					"compat_confidence": "0.83",
					"hidden":            "true",
				},
			},
			{
				ID:          "svc-other",
				Name:        "Other",
				Category:    "Other",
				URL:         "http://host-1:8080",
				Status:      "up",
				Source:      "scan",
				HostAssetID: "host-1",
			},
		},
	}
	raw, _ := json.Marshal(report)
	sut.webServiceCoordinator.HandleReport("host-1", agentmgr.Message{Type: agentmgr.MsgWebServiceReport, Data: raw})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/web/compat", nil)
	rec := httptest.NewRecorder()
	sut.handleWebServiceCompat(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Compatible []map[string]any `json:"compatible"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode compat response: %v", err)
	}
	if len(payload.Compatible) != 1 {
		t.Fatalf("expected 1 visible compatible service, got %d", len(payload.Compatible))
	}
	if got := payload.Compatible[0]["connector_id"]; got != "home-assistant" {
		t.Fatalf("connector_id = %#v, want %q", got, "home-assistant")
	}
	if got := payload.Compatible[0]["service_id"]; got != "svc-ha" {
		t.Fatalf("service_id = %#v, want %q", got, "svc-ha")
	}

	includeHiddenReq := httptest.NewRequest(http.MethodGet, "/api/v1/services/web/compat?include_hidden=true&connector=portainer&min_confidence=0.8", nil)
	includeHiddenRec := httptest.NewRecorder()
	sut.handleWebServiceCompat(includeHiddenRec, includeHiddenReq)
	if includeHiddenRec.Code != http.StatusOK {
		t.Fatalf("expected 200 include_hidden response, got %d body=%s", includeHiddenRec.Code, includeHiddenRec.Body.String())
	}

	var includeHiddenPayload struct {
		Compatible []map[string]any `json:"compatible"`
	}
	if err := json.Unmarshal(includeHiddenRec.Body.Bytes(), &includeHiddenPayload); err != nil {
		t.Fatalf("failed to decode include_hidden compat response: %v", err)
	}
	if len(includeHiddenPayload.Compatible) != 1 {
		t.Fatalf("expected 1 hidden compatible service, got %d", len(includeHiddenPayload.Compatible))
	}
	if got := includeHiddenPayload.Compatible[0]["connector_id"]; got != "portainer" {
		t.Fatalf("connector_id = %#v, want %q", got, "portainer")
	}
	if got := includeHiddenPayload.Compatible[0]["service_id"]; got != "svc-portainer" {
		t.Fatalf("service_id = %#v, want %q", got, "svc-portainer")
	}
}

func TestHandleWebServiceCompatInvalidMinConfidence(t *testing.T) {
	sut := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/web/compat?min_confidence=abc", nil)
	rec := httptest.NewRecorder()

	sut.handleWebServiceCompat(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func containsCSVValue(raw, want string) bool {
	for _, part := range strings.Split(raw, ",") {
		if strings.EqualFold(strings.TrimSpace(part), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}
