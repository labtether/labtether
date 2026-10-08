package main

import (
	"encoding/json"
	"github.com/labtether/labtether/internal/agentmgr"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

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
