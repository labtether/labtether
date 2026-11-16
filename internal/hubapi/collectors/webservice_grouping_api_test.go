package collectors_test

import (
	"encoding/json"
	"github.com/labtether/labtether/internal/agentmgr"
	collectorspkg "github.com/labtether/labtether/internal/hubapi/collectors"
	"github.com/labtether/labtether/internal/hubapi/testutil/hubdeps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func seedGroupingConfig(sut *hubdeps.Hub, cfg collectorspkg.WebServiceURLGroupingConfig) {
	sut.CollectorsDeps().WebServiceURLGroupingCfgMu.Lock()
	sut.CollectorsDeps().WebServiceURLGroupingCfg = cfg
	sut.CollectorsDeps().WebServiceURLGroupingCfgAt = time.Now().UTC()
	sut.CollectorsDeps().WebServiceURLGroupingCfgTTL = time.Hour
	sut.CollectorsDeps().WebServiceURLGroupingCfgMu.Unlock()
}

func TestHandleWebServicesAppliesAliasGroupingRules(t *testing.T) {
	sut := hubdeps.New(t)

	seedGroupingConfig(sut, collectorspkg.WebServiceURLGroupingConfig{
		Mode:                collectorspkg.WebServiceURLGroupingModeBalanced,
		DryRun:              false,
		ConfidenceThreshold: 85,
		AliasRules:          collectorspkg.ParseWebServiceAliasRules("*.*.example.com => *.example.com"),
	})

	report := agentmgr.WebServiceReportData{
		HostAssetID: "host-1",
		Services: []agentmgr.DiscoveredWebService{
			{
				ID:          "svc-a",
				ServiceKey:  "grafana",
				Name:        "Grafana",
				Category:    "Monitoring",
				URL:         "https://xyz.tail.example.com",
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
				URL:         "https://xyz.example.com",
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
	sut.WebServiceCoordinator.HandleReport("host-1", agentmgr.Message{Type: agentmgr.MsgWebServiceReport, Data: raw})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/web", nil)
	rec := httptest.NewRecorder()
	sut.CollectorsDeps().HandleWebServices(rec, req)
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
	case "https://xyz.tail.example.com":
		if !containsCSVValue(altURLs, "https://xyz.example.com") {
			t.Fatalf("expected alt_urls to include canonical alias, got %q", altURLs)
		}
	case "https://xyz.example.com":
		if !containsCSVValue(altURLs, "https://xyz.tail.example.com") {
			t.Fatalf("expected alt_urls to include middle-label alias, got %q", altURLs)
		}
	default:
		t.Fatalf("unexpected primary url %q", primary)
	}
}

func TestHandleWebServicesGroupingDryRunSkipsGrouping(t *testing.T) {
	sut := hubdeps.New(t)

	seedGroupingConfig(sut, collectorspkg.WebServiceURLGroupingConfig{
		Mode:                collectorspkg.WebServiceURLGroupingModeBalanced,
		DryRun:              true,
		ConfidenceThreshold: 85,
		AliasRules:          collectorspkg.ParseWebServiceAliasRules("*.*.example.com => *.example.com"),
	})

	report := agentmgr.WebServiceReportData{
		HostAssetID: "host-1",
		Services: []agentmgr.DiscoveredWebService{
			{
				ID:          "svc-a",
				ServiceKey:  "grafana",
				Name:        "Grafana",
				Category:    "Monitoring",
				URL:         "https://xyz.tail.example.com",
				Status:      "up",
				Source:      "proxy",
				HostAssetID: "host-1",
			},
			{
				ID:          "svc-b",
				ServiceKey:  "grafana",
				Name:        "Grafana",
				Category:    "Monitoring",
				URL:         "https://xyz.example.com",
				Status:      "up",
				Source:      "proxy",
				HostAssetID: "host-1",
			},
		},
	}
	raw, _ := json.Marshal(report)
	sut.WebServiceCoordinator.HandleReport("host-1", agentmgr.Message{Type: agentmgr.MsgWebServiceReport, Data: raw})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/web", nil)
	rec := httptest.NewRecorder()
	sut.CollectorsDeps().HandleWebServices(rec, req)
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
	sut := hubdeps.New(t)

	seedGroupingConfig(sut, collectorspkg.WebServiceURLGroupingConfig{
		Mode:                collectorspkg.WebServiceURLGroupingModeBalanced,
		DryRun:              false,
		ConfidenceThreshold: 85,
		AliasRules:          collectorspkg.ParseWebServiceAliasRules("*.*.example.com => *.example.com"),
		NeverGroupRules:     collectorspkg.ParseWebServicePairRules("https://xyz.tail.example.com => https://xyz.example.com"),
	})

	report := agentmgr.WebServiceReportData{
		HostAssetID: "host-1",
		Services: []agentmgr.DiscoveredWebService{
			{
				ID:          "svc-a",
				ServiceKey:  "grafana",
				Name:        "Grafana",
				Category:    "Monitoring",
				URL:         "https://xyz.tail.example.com",
				Status:      "up",
				Source:      "proxy",
				HostAssetID: "host-1",
			},
			{
				ID:          "svc-b",
				ServiceKey:  "grafana",
				Name:        "Grafana",
				Category:    "Monitoring",
				URL:         "https://xyz.example.com",
				Status:      "up",
				Source:      "proxy",
				HostAssetID: "host-1",
			},
		},
	}
	raw, _ := json.Marshal(report)
	sut.WebServiceCoordinator.HandleReport("host-1", agentmgr.Message{Type: agentmgr.MsgWebServiceReport, Data: raw})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/web", nil)
	rec := httptest.NewRecorder()
	sut.CollectorsDeps().HandleWebServices(rec, req)
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
	sut := hubdeps.New(t)

	seedGroupingConfig(sut, collectorspkg.WebServiceURLGroupingConfig{
		Mode:                collectorspkg.WebServiceURLGroupingModeBalanced,
		DryRun:              false,
		ConfidenceThreshold: 85,
		ForceGroupRules:     collectorspkg.ParseWebServicePairRules("https://alpha.internal:8443 => https://beta.internal:8443"),
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
	sut.WebServiceCoordinator.HandleReport("host-1", agentmgr.Message{Type: agentmgr.MsgWebServiceReport, Data: raw})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/web", nil)
	rec := httptest.NewRecorder()
	sut.CollectorsDeps().HandleWebServices(rec, req)
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
	sut := hubdeps.New(t)

	seedGroupingConfig(sut, collectorspkg.WebServiceURLGroupingConfig{
		Mode:                collectorspkg.WebServiceURLGroupingModeBalanced,
		DryRun:              false,
		ConfidenceThreshold: 96,
		AliasRules:          collectorspkg.ParseWebServiceAliasRules("*.*.example.com => *.example.com"),
	})

	report := agentmgr.WebServiceReportData{
		HostAssetID: "host-1",
		Services: []agentmgr.DiscoveredWebService{
			{
				ID:          "svc-a",
				ServiceKey:  "grafana",
				Name:        "Grafana",
				Category:    "Monitoring",
				URL:         "https://xyz.tail.example.com",
				Status:      "up",
				Source:      "proxy",
				HostAssetID: "host-1",
			},
			{
				ID:          "svc-b",
				ServiceKey:  "grafana",
				Name:        "Grafana",
				Category:    "Monitoring",
				URL:         "https://xyz.example.com",
				Status:      "up",
				Source:      "proxy",
				HostAssetID: "host-1",
			},
		},
	}
	raw, _ := json.Marshal(report)
	sut.WebServiceCoordinator.HandleReport("host-1", agentmgr.Message{Type: agentmgr.MsgWebServiceReport, Data: raw})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/web", nil)
	rec := httptest.NewRecorder()
	sut.CollectorsDeps().HandleWebServices(rec, req)
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
	sut := hubdeps.New(t)

	seedGroupingConfig(sut, collectorspkg.WebServiceURLGroupingConfig{
		Mode:                collectorspkg.WebServiceURLGroupingModeBalanced,
		DryRun:              false,
		ConfidenceThreshold: 85,
		ForceGroupRules:     collectorspkg.ParseWebServicePairRules("https://alpha.internal:8443 => https://beta.internal:8443"),
		NeverGroupRules:     collectorspkg.ParseWebServicePairRules("https://alpha.internal:8443 => https://beta.internal:8443"),
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
	sut.WebServiceCoordinator.HandleReport("host-1", agentmgr.Message{Type: agentmgr.MsgWebServiceReport, Data: raw})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/web", nil)
	rec := httptest.NewRecorder()
	sut.CollectorsDeps().HandleWebServices(rec, req)
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
	sut := hubdeps.New(t)
	sut.CollectorsDeps().WebServiceURLGroupingCfgTTL = time.Hour

	// First resolve should produce defaults (no db, no runtime overrides with definitions).
	cfg := sut.CollectorsDeps().ResolveWebServiceURLGroupingConfig()
	if cfg.Mode != collectorspkg.WebServiceURLGroupingModeConservative {
		t.Fatalf("url grouping mode = %q, want %q", cfg.Mode, collectorspkg.WebServiceURLGroupingModeConservative)
	}

	// Directly inject a different config into the cache to verify caching works.
	sut.CollectorsDeps().WebServiceURLGroupingCfgMu.Lock()
	sut.CollectorsDeps().WebServiceURLGroupingCfg = collectorspkg.WebServiceURLGroupingConfig{
		Mode:                collectorspkg.WebServiceURLGroupingModeBalanced,
		ConfidenceThreshold: 85,
	}
	sut.CollectorsDeps().WebServiceURLGroupingCfgMu.Unlock()

	// Second resolve should return cached value (balanced) without re-resolving.
	cfg = sut.CollectorsDeps().ResolveWebServiceURLGroupingConfig()
	if cfg.Mode != collectorspkg.WebServiceURLGroupingModeBalanced {
		t.Fatalf("url grouping mode = %q, want %q (cached)", cfg.Mode, collectorspkg.WebServiceURLGroupingModeBalanced)
	}

	// After invalidation, resolve should recompute and return defaults again.
	sut.CollectorsDeps().InvalidateWebServiceURLGroupingConfigCache()
	cfg = sut.CollectorsDeps().ResolveWebServiceURLGroupingConfig()
	if cfg.Mode != collectorspkg.WebServiceURLGroupingModeConservative {
		t.Fatalf("url grouping mode = %q, want %q (after invalidation)", cfg.Mode, collectorspkg.WebServiceURLGroupingModeConservative)
	}
}
