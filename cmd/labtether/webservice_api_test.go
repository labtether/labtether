package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
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

func TestHandleWebServicesHiddenFilter(t *testing.T) {
	sut := newTestAPIServer(t)

	report := agentmgr.WebServiceReportData{
		HostAssetID: "host-1",
		Services: []agentmgr.DiscoveredWebService{
			{
				ID:          "svc-1",
				Name:        "Grafana",
				Category:    "Monitoring",
				URL:         "http://host-1:3000",
				Status:      "up",
				Source:      "docker",
				HostAssetID: "host-1",
			},
		},
	}
	raw, _ := json.Marshal(report)
	sut.webServiceCoordinator.HandleReport("host-1", agentmgr.Message{Type: agentmgr.MsgWebServiceReport, Data: raw})

	overrideReq := httptest.NewRequest(http.MethodPost, "/api/v1/services/web/overrides", strings.NewReader(`{
		"host_asset_id":"host-1",
		"service_id":"svc-1",
		"hidden":true
	}`))
	overrideRec := httptest.NewRecorder()
	sut.handleWebServiceOverrides(overrideRec, overrideReq)
	if overrideRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on override save, got %d body=%s", overrideRec.Code, overrideRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/services/web", nil)
	listRec := httptest.NewRecorder()
	sut.handleWebServices(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on list, got %d", listRec.Code)
	}
	var listResp struct {
		Services []map[string]any `json:"services"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("failed to decode list response: %v", err)
	}
	if len(listResp.Services) != 0 {
		t.Fatalf("expected hidden service to be filtered from default list, got %d services", len(listResp.Services))
	}

	allReq := httptest.NewRequest(http.MethodGet, "/api/v1/services/web?include_hidden=true", nil)
	allRec := httptest.NewRecorder()
	sut.handleWebServices(allRec, allReq)
	if allRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on include_hidden list, got %d", allRec.Code)
	}
	var allResp struct {
		Services []map[string]any `json:"services"`
	}
	if err := json.Unmarshal(allRec.Body.Bytes(), &allResp); err != nil {
		t.Fatalf("failed to decode include_hidden response: %v", err)
	}
	if len(allResp.Services) != 1 {
		t.Fatalf("expected 1 hidden service with include_hidden=true, got %d", len(allResp.Services))
	}
}

func TestHandleWebServicesIncludesDiscoveryStats(t *testing.T) {
	sut := newTestAPIServer(t)

	report := agentmgr.WebServiceReportData{
		HostAssetID: "host-1",
		Services: []agentmgr.DiscoveredWebService{
			{
				ID:          "svc-1",
				Name:        "Grafana",
				Category:    "Monitoring",
				URL:         "http://host-1:3000",
				Status:      "up",
				Source:      "docker",
				HostAssetID: "host-1",
			},
		},
		Discovery: &agentmgr.WebServiceDiscoveryStats{
			CollectedAt:     time.Now().UTC().Format(time.RFC3339),
			CycleDurationMs: 143,
			TotalServices:   1,
			Sources: map[string]agentmgr.WebServiceDiscoverySourceStat{
				"docker": {
					Enabled:       true,
					DurationMs:    40,
					ServicesFound: 1,
				},
			},
			FinalSourceCount: map[string]int{
				"docker": 1,
			},
		},
	}

	raw, _ := json.Marshal(report)
	sut.webServiceCoordinator.HandleReport("host-1", agentmgr.Message{
		Type: agentmgr.MsgWebServiceReport,
		Data: raw,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/web?host=host-1", nil)
	rec := httptest.NewRecorder()
	sut.handleWebServices(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on list, got %d", rec.Code)
	}

	var payload struct {
		Services       []agentmgr.DiscoveredWebService `json:"services"`
		DiscoveryStats []struct {
			HostAssetID string                            `json:"host_asset_id"`
			LastSeen    string                            `json:"last_seen"`
			Discovery   agentmgr.WebServiceDiscoveryStats `json:"discovery"`
		} `json:"discovery_stats"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response payload: %v", err)
	}
	if len(payload.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(payload.Services))
	}
	if len(payload.DiscoveryStats) != 1 {
		t.Fatalf("expected 1 discovery stats entry, got %d", len(payload.DiscoveryStats))
	}
	if payload.DiscoveryStats[0].HostAssetID != "host-1" {
		t.Fatalf("discovery host id = %q, want %q", payload.DiscoveryStats[0].HostAssetID, "host-1")
	}
	if payload.DiscoveryStats[0].Discovery.CycleDurationMs != 143 {
		t.Fatalf("discovery cycle duration = %d, want %d", payload.DiscoveryStats[0].Discovery.CycleDurationMs, 143)
	}
	if payload.DiscoveryStats[0].Discovery.Sources["docker"].ServicesFound != 1 {
		t.Fatalf("docker services found = %d, want %d", payload.DiscoveryStats[0].Discovery.Sources["docker"].ServicesFound, 1)
	}
	if payload.DiscoveryStats[0].Discovery.FinalSourceCount["docker"] != 1 {
		t.Fatalf("final source count docker = %d, want %d", payload.DiscoveryStats[0].Discovery.FinalSourceCount["docker"], 1)
	}
}

func TestHandleWebServicesIncludesHealthSummary(t *testing.T) {
	sut := newTestAPIServer(t)

	reportUp := agentmgr.WebServiceReportData{
		HostAssetID: "host-1",
		Services: []agentmgr.DiscoveredWebService{
			{
				ID:          "svc-1",
				Name:        "Grafana",
				Category:    "Monitoring",
				URL:         "http://host-1:3000",
				Status:      "up",
				ResponseMs:  80,
				Source:      "docker",
				HostAssetID: "host-1",
			},
		},
	}
	rawUp, _ := json.Marshal(reportUp)
	sut.webServiceCoordinator.HandleReport("host-1", agentmgr.Message{Type: agentmgr.MsgWebServiceReport, Data: rawUp})

	reportDown := reportUp
	reportDown.Services = append([]agentmgr.DiscoveredWebService(nil), reportUp.Services...)
	reportDown.Services[0].Status = "down"
	reportDown.Services[0].ResponseMs = 0
	rawDown, _ := json.Marshal(reportDown)
	sut.webServiceCoordinator.HandleReport("host-1", agentmgr.Message{Type: agentmgr.MsgWebServiceReport, Data: rawDown})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/web?host=host-1", nil)
	rec := httptest.NewRecorder()
	sut.handleWebServices(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on list, got %d", rec.Code)
	}

	var payload struct {
		Services []agentmgr.DiscoveredWebService `json:"services"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response payload: %v", err)
	}
	if len(payload.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(payload.Services))
	}
	health := payload.Services[0].Health
	if health == nil {
		t.Fatalf("expected health summary on service payload")
	}
	if health.Window != "24h" {
		t.Fatalf("window=%q want=%q", health.Window, "24h")
	}
	if health.Checks != 2 {
		t.Fatalf("checks=%d want=%d", health.Checks, 2)
	}
	if health.UpChecks != 1 {
		t.Fatalf("up_checks=%d want=%d", health.UpChecks, 1)
	}
	if health.UptimePercent < 49.9 || health.UptimePercent > 50.1 {
		t.Fatalf("uptime_percent=%f want around 50", health.UptimePercent)
	}
	if len(health.Recent) != 2 {
		t.Fatalf("recent=%d want=%d", len(health.Recent), 2)
	}
}

func TestHandleWebServicesCompactDetailOmitsExpandedFields(t *testing.T) {
	sut := newTestAPIServer(t)

	reportUp := agentmgr.WebServiceReportData{
		HostAssetID: "host-1",
		Services: []agentmgr.DiscoveredWebService{
			{
				ID:          "svc-1",
				Name:        "Grafana",
				Category:    "Monitoring",
				URL:         "http://host-1:3000",
				Status:      "up",
				ResponseMs:  80,
				Source:      "docker",
				HostAssetID: "host-1",
			},
			{
				ID:          "svc-2",
				Name:        "Loki",
				Category:    "Monitoring",
				URL:         "http://host-1:3100",
				Status:      "up",
				ResponseMs:  95,
				Source:      "docker",
				HostAssetID: "host-1",
			},
		},
	}
	rawUp, _ := json.Marshal(reportUp)
	sut.webServiceCoordinator.HandleReport("host-1", agentmgr.Message{Type: agentmgr.MsgWebServiceReport, Data: rawUp})

	reportDown := reportUp
	reportDown.Services = append([]agentmgr.DiscoveredWebService(nil), reportUp.Services...)
	reportDown.Services[0].Status = "down"
	reportDown.Services[0].ResponseMs = 0
	rawDown, _ := json.Marshal(reportDown)
	sut.webServiceCoordinator.HandleReport("host-1", agentmgr.Message{Type: agentmgr.MsgWebServiceReport, Data: rawDown})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/web?host=host-1&service_id=svc-1&detail=compact", nil)
	rec := httptest.NewRecorder()
	sut.handleWebServices(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on compact list, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Services []agentmgr.DiscoveredWebService `json:"services"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode compact response payload: %v", err)
	}
	if len(payload.Services) != 1 {
		t.Fatalf("expected 1 compact service, got %d", len(payload.Services))
	}
	if payload.Services[0].ID != "svc-1" {
		t.Fatalf("service id=%q want=%q", payload.Services[0].ID, "svc-1")
	}
	if payload.Services[0].Health == nil {
		t.Fatal("expected compact response to retain health summary")
	}
	if len(payload.Services[0].Health.Recent) != 0 {
		t.Fatalf("expected compact response to omit recent history, got %d points", len(payload.Services[0].Health.Recent))
	}
}

func TestHandleWebServicesCompactDetailSkipsExpandedAltURLArrays(t *testing.T) {
	sut := newTestAPIServer(t)

	report := agentmgr.WebServiceReportData{
		HostAssetID: "host-1",
		Services: []agentmgr.DiscoveredWebService{
			{
				ID:          "svc-1",
				Name:        "Grafana",
				Category:    "Monitoring",
				URL:         "http://host-1:3000",
				Status:      "up",
				ResponseMs:  80,
				Source:      "docker",
				HostAssetID: "host-1",
				Metadata: map[string]string{
					"alt_urls": "https://grafana.home.lab",
				},
			},
		},
	}
	raw, _ := json.Marshal(report)
	sut.webServiceCoordinator.HandleReport("host-1", agentmgr.Message{Type: agentmgr.MsgWebServiceReport, Data: raw})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/web?host=host-1&detail=compact", nil)
	rec := httptest.NewRecorder()
	sut.handleWebServices(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on compact list, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Services []map[string]any `json:"services"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode compact response payload: %v", err)
	}
	if len(payload.Services) != 1 {
		t.Fatalf("expected 1 compact service, got %d", len(payload.Services))
	}
	if _, ok := payload.Services[0]["alt_urls"]; ok {
		t.Fatalf("expected compact response to omit expanded alt_urls array, got %#v", payload.Services[0]["alt_urls"])
	}
	metadata, _ := payload.Services[0]["metadata"].(map[string]any)
	if metadata["alt_urls"] != "https://grafana.home.lab" {
		t.Fatalf("compact metadata alt_urls=%v want=%q", metadata["alt_urls"], "https://grafana.home.lab")
	}
}

func TestHandleWebServiceAltURLsSynthesizesAutoAliasesWithoutPersistence(t *testing.T) {
	sut := newTestAPIServer(t)

	report := agentmgr.WebServiceReportData{
		HostAssetID: "host-1",
		Services: []agentmgr.DiscoveredWebService{
			{
				ID:          "svc-1",
				Name:        "Grafana",
				Category:    "Monitoring",
				URL:         "http://host-1:3000",
				Status:      "up",
				ResponseMs:  80,
				Source:      "docker",
				HostAssetID: "host-1",
				Metadata: map[string]string{
					"alt_urls": "https://grafana.auto.lab, https://grafana.persisted.lab",
				},
			},
		},
	}
	raw, _ := json.Marshal(report)
	sut.webServiceCoordinator.HandleReport("host-1", agentmgr.Message{Type: agentmgr.MsgWebServiceReport, Data: raw})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/web/alt-urls?web_service_id=http://host-1:3000", nil)
	rec := httptest.NewRecorder()
	sut.handleWebServiceAltURLs(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on alt url list, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		AltURLs []struct {
			URL    string `json:"url"`
			Source string `json:"source"`
		} `json:"alt_urls"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode alt url payload: %v", err)
	}
	if len(payload.AltURLs) != 2 {
		t.Fatalf("expected 2 synthesized alt urls, got %d", len(payload.AltURLs))
	}
	if payload.AltURLs[0].URL != "https://grafana.auto.lab" {
		t.Fatalf("first alt url=%q want=%q", payload.AltURLs[0].URL, "https://grafana.auto.lab")
	}
	if payload.AltURLs[1].URL != "https://grafana.persisted.lab" {
		t.Fatalf("second alt url=%q want=%q", payload.AltURLs[1].URL, "https://grafana.persisted.lab")
	}
	if payload.AltURLs[0].Source != "auto" || payload.AltURLs[1].Source != "auto" {
		t.Fatalf("expected synthesized alt urls to be marked auto, got %+v", payload.AltURLs)
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

func seedGroupingConfig(sut *apiServer, cfg webServiceURLGroupingConfig) {
	sut.ensureCollectorsDeps().WebServiceURLGroupingCfgMu.Lock()
	sut.ensureCollectorsDeps().WebServiceURLGroupingCfg = cfg
	sut.ensureCollectorsDeps().WebServiceURLGroupingCfgAt = time.Now().UTC()
	sut.ensureCollectorsDeps().WebServiceURLGroupingCfgTTL = time.Hour
	sut.ensureCollectorsDeps().WebServiceURLGroupingCfgMu.Unlock()
}

func TestHandleWebServicesAppliesAliasGroupingRules(t *testing.T) {
	sut := newTestAPIServer(t)

	seedGroupingConfig(sut, webServiceURLGroupingConfig{
		Mode:                webServiceURLGroupingModeBalanced,
		DryRun:              false,
		ConfidenceThreshold: 85,
		AliasRules:          parseWebServiceAliasRules("*.*.simbaslabs.com => *.simbaslabs.com"),
	})

	report := agentmgr.WebServiceReportData{
		HostAssetID: "host-1",
		Services: []agentmgr.DiscoveredWebService{
			{
				ID:          "svc-a",
				ServiceKey:  "grafana",
				Name:        "Grafana",
				Category:    "Monitoring",
				URL:         "https://xyz.tail.simbaslabs.com",
				Status:      "up",
				Source:      "proxy",
				HostAssetID: "host-1",
				Metadata: map[string]string{
					"proxy_provider": "traefik",
				},
			},
			{
				ID:          "svc-b",
				ServiceKey:  "grafana",
				Name:        "Grafana",
				Category:    "Monitoring",
				URL:         "https://xyz.simbaslabs.com",
				Status:      "up",
				Source:      "proxy",
				HostAssetID: "host-1",
				Metadata: map[string]string{
					"proxy_provider": "traefik",
				},
			},
		},
	}
	raw, _ := json.Marshal(report)
	sut.webServiceCoordinator.HandleReport("host-1", agentmgr.Message{Type: agentmgr.MsgWebServiceReport, Data: raw})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/web", nil)
	rec := httptest.NewRecorder()
	sut.handleWebServices(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Services []agentmgr.DiscoveredWebService `json:"services"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode services payload: %v", err)
	}
	if len(payload.Services) != 1 {
		t.Fatalf("expected grouped services length 1, got %d", len(payload.Services))
	}

	grouped := payload.Services[0]
	if grouped.Metadata == nil {
		t.Fatalf("expected grouped metadata")
	}
	altURLs := grouped.Metadata["alt_urls"]
	primary := strings.TrimSpace(grouped.URL)

	switch primary {
	case "https://xyz.tail.simbaslabs.com":
		if !containsCSVValue(altURLs, "https://xyz.simbaslabs.com") {
			t.Fatalf("expected alt_urls to include canonical alias, got %q", altURLs)
		}
	case "https://xyz.simbaslabs.com":
		if !containsCSVValue(altURLs, "https://xyz.tail.simbaslabs.com") {
			t.Fatalf("expected alt_urls to include middle-label alias, got %q", altURLs)
		}
	default:
		t.Fatalf("unexpected primary url %q", primary)
	}
}

func TestHandleWebServicesGroupingDryRunSkipsGrouping(t *testing.T) {
	sut := newTestAPIServer(t)

	seedGroupingConfig(sut, webServiceURLGroupingConfig{
		Mode:                webServiceURLGroupingModeBalanced,
		DryRun:              true,
		ConfidenceThreshold: 85,
		AliasRules:          parseWebServiceAliasRules("*.*.simbaslabs.com => *.simbaslabs.com"),
	})

	report := agentmgr.WebServiceReportData{
		HostAssetID: "host-1",
		Services: []agentmgr.DiscoveredWebService{
			{
				ID:          "svc-a",
				ServiceKey:  "grafana",
				Name:        "Grafana",
				Category:    "Monitoring",
				URL:         "https://xyz.tail.simbaslabs.com",
				Status:      "up",
				Source:      "proxy",
				HostAssetID: "host-1",
			},
			{
				ID:          "svc-b",
				ServiceKey:  "grafana",
				Name:        "Grafana",
				Category:    "Monitoring",
				URL:         "https://xyz.simbaslabs.com",
				Status:      "up",
				Source:      "proxy",
				HostAssetID: "host-1",
			},
		},
	}
	raw, _ := json.Marshal(report)
	sut.webServiceCoordinator.HandleReport("host-1", agentmgr.Message{Type: agentmgr.MsgWebServiceReport, Data: raw})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/web", nil)
	rec := httptest.NewRecorder()
	sut.handleWebServices(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Services []agentmgr.DiscoveredWebService `json:"services"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode services payload: %v", err)
	}
	if len(payload.Services) != 2 {
		t.Fatalf("expected dry-run mode to skip grouping and keep 2 services, got %d", len(payload.Services))
	}
}

func TestHandleWebServicesNeverGroupRuleSkipsAliasGrouping(t *testing.T) {
	sut := newTestAPIServer(t)

	seedGroupingConfig(sut, webServiceURLGroupingConfig{
		Mode:                webServiceURLGroupingModeBalanced,
		DryRun:              false,
		ConfidenceThreshold: 85,
		AliasRules:          parseWebServiceAliasRules("*.*.simbaslabs.com => *.simbaslabs.com"),
		NeverGroupRules:     parseWebServicePairRules("https://xyz.tail.simbaslabs.com => https://xyz.simbaslabs.com"),
	})

	report := agentmgr.WebServiceReportData{
		HostAssetID: "host-1",
		Services: []agentmgr.DiscoveredWebService{
			{
				ID:          "svc-a",
				ServiceKey:  "grafana",
				Name:        "Grafana",
				Category:    "Monitoring",
				URL:         "https://xyz.tail.simbaslabs.com",
				Status:      "up",
				Source:      "proxy",
				HostAssetID: "host-1",
			},
			{
				ID:          "svc-b",
				ServiceKey:  "grafana",
				Name:        "Grafana",
				Category:    "Monitoring",
				URL:         "https://xyz.simbaslabs.com",
				Status:      "up",
				Source:      "proxy",
				HostAssetID: "host-1",
			},
		},
	}
	raw, _ := json.Marshal(report)
	sut.webServiceCoordinator.HandleReport("host-1", agentmgr.Message{Type: agentmgr.MsgWebServiceReport, Data: raw})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/web", nil)
	rec := httptest.NewRecorder()
	sut.handleWebServices(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Services []agentmgr.DiscoveredWebService `json:"services"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode services payload: %v", err)
	}
	if len(payload.Services) != 2 {
		t.Fatalf("expected never-group rule to keep 2 services, got %d", len(payload.Services))
	}
}

func TestHandleWebServicesForceGroupRuleGroupsServices(t *testing.T) {
	sut := newTestAPIServer(t)

	seedGroupingConfig(sut, webServiceURLGroupingConfig{
		Mode:                webServiceURLGroupingModeBalanced,
		DryRun:              false,
		ConfidenceThreshold: 85,
		ForceGroupRules:     parseWebServicePairRules("https://alpha.internal:8443 => https://beta.internal:8443"),
	})

	report := agentmgr.WebServiceReportData{
		HostAssetID: "host-1",
		Services: []agentmgr.DiscoveredWebService{
			{
				ID:          "svc-a",
				ServiceKey:  "alpha",
				Name:        "Alpha",
				Category:    "Other",
				URL:         "https://alpha.internal:8443",
				Status:      "up",
				Source:      "proxy",
				HostAssetID: "host-1",
			},
			{
				ID:          "svc-b",
				ServiceKey:  "beta",
				Name:        "Beta",
				Category:    "Other",
				URL:         "https://beta.internal:8443",
				Status:      "up",
				Source:      "proxy",
				HostAssetID: "host-1",
			},
		},
	}
	raw, _ := json.Marshal(report)
	sut.webServiceCoordinator.HandleReport("host-1", agentmgr.Message{Type: agentmgr.MsgWebServiceReport, Data: raw})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/web", nil)
	rec := httptest.NewRecorder()
	sut.handleWebServices(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Services []agentmgr.DiscoveredWebService `json:"services"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode services payload: %v", err)
	}
	if len(payload.Services) != 1 {
		t.Fatalf("expected force-group rule to produce 1 service, got %d", len(payload.Services))
	}
}

func TestHandleWebServicesGroupingThresholdBlocksAliasGrouping(t *testing.T) {
	sut := newTestAPIServer(t)

	seedGroupingConfig(sut, webServiceURLGroupingConfig{
		Mode:                webServiceURLGroupingModeBalanced,
		DryRun:              false,
		ConfidenceThreshold: 96,
		AliasRules:          parseWebServiceAliasRules("*.*.simbaslabs.com => *.simbaslabs.com"),
	})

	report := agentmgr.WebServiceReportData{
		HostAssetID: "host-1",
		Services: []agentmgr.DiscoveredWebService{
			{
				ID:          "svc-a",
				ServiceKey:  "grafana",
				Name:        "Grafana",
				Category:    "Monitoring",
				URL:         "https://xyz.tail.simbaslabs.com",
				Status:      "up",
				Source:      "proxy",
				HostAssetID: "host-1",
			},
			{
				ID:          "svc-b",
				ServiceKey:  "grafana",
				Name:        "Grafana",
				Category:    "Monitoring",
				URL:         "https://xyz.simbaslabs.com",
				Status:      "up",
				Source:      "proxy",
				HostAssetID: "host-1",
			},
		},
	}
	raw, _ := json.Marshal(report)
	sut.webServiceCoordinator.HandleReport("host-1", agentmgr.Message{Type: agentmgr.MsgWebServiceReport, Data: raw})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/web", nil)
	rec := httptest.NewRecorder()
	sut.handleWebServices(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Services []agentmgr.DiscoveredWebService `json:"services"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode services payload: %v", err)
	}
	if len(payload.Services) != 2 {
		t.Fatalf("expected high threshold to keep 2 services, got %d", len(payload.Services))
	}
}

func TestHandleWebServicesNeverGroupRuleOverridesForceGroupRule(t *testing.T) {
	sut := newTestAPIServer(t)

	seedGroupingConfig(sut, webServiceURLGroupingConfig{
		Mode:                webServiceURLGroupingModeBalanced,
		DryRun:              false,
		ConfidenceThreshold: 85,
		ForceGroupRules:     parseWebServicePairRules("https://alpha.internal:8443 => https://beta.internal:8443"),
		NeverGroupRules:     parseWebServicePairRules("https://alpha.internal:8443 => https://beta.internal:8443"),
	})

	report := agentmgr.WebServiceReportData{
		HostAssetID: "host-1",
		Services: []agentmgr.DiscoveredWebService{
			{
				ID:          "svc-a",
				ServiceKey:  "alpha",
				Name:        "Alpha",
				Category:    "Other",
				URL:         "https://alpha.internal:8443",
				Status:      "up",
				Source:      "proxy",
				HostAssetID: "host-1",
			},
			{
				ID:          "svc-b",
				ServiceKey:  "beta",
				Name:        "Beta",
				Category:    "Other",
				URL:         "https://beta.internal:8443",
				Status:      "up",
				Source:      "proxy",
				HostAssetID: "host-1",
			},
		},
	}
	raw, _ := json.Marshal(report)
	sut.webServiceCoordinator.HandleReport("host-1", agentmgr.Message{Type: agentmgr.MsgWebServiceReport, Data: raw})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/web", nil)
	rec := httptest.NewRecorder()
	sut.handleWebServices(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Services []agentmgr.DiscoveredWebService `json:"services"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode services payload: %v", err)
	}
	if len(payload.Services) != 2 {
		t.Fatalf("expected never-group rule to override force-group and keep 2 services, got %d", len(payload.Services))
	}
}

func TestResolveWebServiceURLGroupingConfigCachesBetweenCalls(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.ensureCollectorsDeps().WebServiceURLGroupingCfgTTL = time.Hour

	// First resolve should produce defaults (no db, no runtime overrides with definitions).
	cfg := sut.resolveWebServiceURLGroupingConfig()
	if cfg.Mode != webServiceURLGroupingModeConservative {
		t.Fatalf("url grouping mode = %q, want %q", cfg.Mode, webServiceURLGroupingModeConservative)
	}

	// Directly inject a different config into the cache to verify caching works.
	sut.ensureCollectorsDeps().WebServiceURLGroupingCfgMu.Lock()
	sut.ensureCollectorsDeps().WebServiceURLGroupingCfg = webServiceURLGroupingConfig{
		Mode:                webServiceURLGroupingModeBalanced,
		ConfidenceThreshold: 85,
	}
	sut.ensureCollectorsDeps().WebServiceURLGroupingCfgMu.Unlock()

	// Second resolve should return cached value (balanced) without re-resolving.
	cfg = sut.resolveWebServiceURLGroupingConfig()
	if cfg.Mode != webServiceURLGroupingModeBalanced {
		t.Fatalf("url grouping mode = %q, want %q (cached)", cfg.Mode, webServiceURLGroupingModeBalanced)
	}

	// After invalidation, resolve should recompute and return defaults again.
	sut.invalidateWebServiceURLGroupingConfigCache()
	cfg = sut.resolveWebServiceURLGroupingConfig()
	if cfg.Mode != webServiceURLGroupingModeConservative {
		t.Fatalf("url grouping mode = %q, want %q (after invalidation)", cfg.Mode, webServiceURLGroupingModeConservative)
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
