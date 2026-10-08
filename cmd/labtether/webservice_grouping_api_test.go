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
