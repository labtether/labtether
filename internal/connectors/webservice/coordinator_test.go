package webservice

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/persistence"
)

type countingWebServiceStore struct {
	*persistence.MemoryWebServiceStore
	manualListCalls   int
	overrideListCalls int
}

type transientManualListErrorStore struct {
	*countingWebServiceStore
	failManualReads int
}

func (c *countingWebServiceStore) ListManualWebServices(hostAssetID string) ([]persistence.WebServiceManual, error) {
	c.manualListCalls++
	return c.MemoryWebServiceStore.ListManualWebServices(hostAssetID)
}

func (c *countingWebServiceStore) ListWebServiceOverrides(hostAssetID string) ([]persistence.WebServiceOverride, error) {
	c.overrideListCalls++
	return c.MemoryWebServiceStore.ListWebServiceOverrides(hostAssetID)
}

func (c *transientManualListErrorStore) ListManualWebServices(hostAssetID string) ([]persistence.WebServiceManual, error) {
	c.manualListCalls++
	if c.failManualReads > 0 {
		c.failManualReads--
		return nil, errors.New("transient manual list failure")
	}
	return c.MemoryWebServiceStore.ListManualWebServices(hostAssetID)
}

func makeReportMsg(data agentmgr.WebServiceReportData) agentmgr.Message {
	raw, _ := json.Marshal(data)
	return agentmgr.Message{Type: agentmgr.MsgWebServiceReport, Data: raw}
}

func TestHandleReport(t *testing.T) {
	coord := NewCoordinator()

	// First report with 2 services.
	coord.HandleReport("agent-01", makeReportMsg(agentmgr.WebServiceReportData{
		HostAssetID: "agent-01",
		Services: []agentmgr.DiscoveredWebService{
			{ID: "svc-1", Name: "Grafana", Category: "monitoring", URL: "http://localhost:3000", Status: "up"},
			{ID: "svc-2", Name: "Prometheus", Category: "monitoring", URL: "http://localhost:9090", Status: "up"},
		},
	}))

	all := coord.ListAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 services after first report, got %d", len(all))
	}

	// Second report replaces first (full replacement).
	coord.HandleReport("agent-01", makeReportMsg(agentmgr.WebServiceReportData{
		HostAssetID: "agent-01",
		Services: []agentmgr.DiscoveredWebService{
			{ID: "svc-3", Name: "Portainer", Category: "management", URL: "http://localhost:9443", Status: "up"},
		},
	}))

	all = coord.ListAll()
	if len(all) != 1 {
		t.Fatalf("expected 1 service after second report (full replacement), got %d", len(all))
	}
	if all[0].Name != "Portainer" {
		t.Errorf("expected Portainer, got %s", all[0].Name)
	}
}

func TestListByHost(t *testing.T) {
	coord := NewCoordinator()

	coord.HandleReport("agent-01", makeReportMsg(agentmgr.WebServiceReportData{
		HostAssetID: "agent-01",
		Services: []agentmgr.DiscoveredWebService{
			{ID: "svc-1", Name: "Grafana", Category: "monitoring", URL: "http://host1:3000", Status: "up"},
		},
	}))
	coord.HandleReport("agent-02", makeReportMsg(agentmgr.WebServiceReportData{
		HostAssetID: "agent-02",
		Services: []agentmgr.DiscoveredWebService{
			{ID: "svc-2", Name: "Portainer", Category: "management", URL: "http://host2:9443", Status: "up"},
			{ID: "svc-3", Name: "Traefik", Category: "networking", URL: "http://host2:8080", Status: "up"},
		},
	}))

	// All services across hosts.
	all := coord.ListAll()
	if len(all) != 3 {
		t.Fatalf("expected 3 total services, got %d", len(all))
	}

	// Filter by host.
	host1 := coord.ListByHost("agent-01")
	if len(host1) != 1 {
		t.Fatalf("expected 1 service for agent-01, got %d", len(host1))
	}
	if host1[0].Name != "Grafana" {
		t.Errorf("expected Grafana for agent-01, got %s", host1[0].Name)
	}

	host2 := coord.ListByHost("agent-02")
	if len(host2) != 2 {
		t.Fatalf("expected 2 services for agent-02, got %d", len(host2))
	}

	// Non-existent host returns nil.
	missing := coord.ListByHost("agent-99")
	if missing != nil {
		t.Errorf("expected nil for non-existent host, got %v", missing)
	}
}

func TestCategories(t *testing.T) {
	coord := NewCoordinator()

	coord.HandleReport("agent-01", makeReportMsg(agentmgr.WebServiceReportData{
		HostAssetID: "agent-01",
		Services: []agentmgr.DiscoveredWebService{
			{ID: "svc-1", Name: "Grafana", Category: "monitoring", Status: "up"},
			{ID: "svc-2", Name: "Portainer", Category: "management", Status: "up"},
			{ID: "svc-3", Name: "Prometheus", Category: "monitoring", Status: "up"},
		},
	}))
	coord.HandleReport("agent-02", makeReportMsg(agentmgr.WebServiceReportData{
		HostAssetID: "agent-02",
		Services: []agentmgr.DiscoveredWebService{
			{ID: "svc-4", Name: "Traefik", Category: "networking", Status: "up"},
		},
	}))

	cats := coord.Categories()
	expected := []string{"management", "monitoring", "networking"}
	if len(cats) != len(expected) {
		t.Fatalf("expected %d categories, got %d: %v", len(expected), len(cats), cats)
	}
	for i, want := range expected {
		if cats[i] != want {
			t.Errorf("categories[%d] = %q, want %q", i, cats[i], want)
		}
	}
}

func TestDiscoveryStatsSnapshots(t *testing.T) {
	coord := NewCoordinator()
	coord.HandleReport("agent-01", makeReportMsg(agentmgr.WebServiceReportData{
		HostAssetID: "agent-01",
		Services: []agentmgr.DiscoveredWebService{
			{ID: "svc-1", Name: "Grafana", Category: "monitoring", URL: "http://host1:3000", Status: "up", Source: "docker"},
		},
		Discovery: &agentmgr.WebServiceDiscoveryStats{
			CollectedAt:     "2026-03-02T12:00:00Z",
			CycleDurationMs: 120,
			TotalServices:   1,
			Sources: map[string]agentmgr.WebServiceDiscoverySourceStat{
				"docker": {
					Enabled:       true,
					DurationMs:    45,
					ServicesFound: 1,
				},
			},
			FinalSourceCount: map[string]int{
				"docker": 1,
			},
		},
	}))

	stats := coord.DiscoveryStats("")
	if len(stats) != 1 {
		t.Fatalf("expected 1 discovery stats snapshot, got %d", len(stats))
	}
	if stats[0].HostAssetID != "agent-01" {
		t.Fatalf("snapshot host = %q, want %q", stats[0].HostAssetID, "agent-01")
	}
	if stats[0].Discovery.CycleDurationMs != 120 {
		t.Fatalf("cycle duration = %d, want %d", stats[0].Discovery.CycleDurationMs, 120)
	}
	if stats[0].Discovery.Sources["docker"].ServicesFound != 1 {
		t.Fatalf("docker services found = %d, want %d", stats[0].Discovery.Sources["docker"].ServicesFound, 1)
	}

	filtered := coord.DiscoveryStats("agent-01")
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered stats snapshot, got %d", len(filtered))
	}
	if got := len(coord.DiscoveryStats("agent-99")); got != 0 {
		t.Fatalf("expected 0 snapshots for missing host, got %d", got)
	}

	// Ensure returned snapshots are cloned and cannot mutate coordinator state.
	stats[0].Discovery.Sources["docker"] = agentmgr.WebServiceDiscoverySourceStat{
		Enabled:       true,
		DurationMs:    1,
		ServicesFound: 999,
	}
	fresh := coord.DiscoveryStats("agent-01")
	if fresh[0].Discovery.Sources["docker"].ServicesFound != 1 {
		t.Fatalf("expected cloned snapshot to preserve original value, got %d", fresh[0].Discovery.Sources["docker"].ServicesFound)
	}
}

func TestHostDisconnectMarkUnknown(t *testing.T) {
	coord := NewCoordinator()

	coord.HandleReport("agent-01", makeReportMsg(agentmgr.WebServiceReportData{
		HostAssetID: "agent-01",
		Services: []agentmgr.DiscoveredWebService{
			{ID: "svc-1", Name: "Grafana", Status: "up"},
			{ID: "svc-2", Name: "Prometheus", Status: "up"},
		},
	}))

	coord.MarkHostDisconnected("agent-01")

	services := coord.ListByHost("agent-01")
	if len(services) != 2 {
		t.Fatalf("expected 2 services after disconnect, got %d", len(services))
	}
	for _, svc := range services {
		if svc.Status != "unknown" {
			t.Errorf("service %s status = %q, want unknown", svc.Name, svc.Status)
		}
	}

	// Disconnecting a non-existent host is a no-op (should not panic).
	coord.MarkHostDisconnected("agent-99")
}

func TestRemoveHost(t *testing.T) {
	coord := NewCoordinator()

	coord.HandleReport("agent-01", makeReportMsg(agentmgr.WebServiceReportData{
		HostAssetID: "agent-01",
		Services: []agentmgr.DiscoveredWebService{
			{ID: "svc-1", Name: "Grafana", Status: "up"},
		},
	}))

	if got := len(coord.ListAll()); got != 1 {
		t.Fatalf("expected 1 service before RemoveHost, got %d", got)
	}

	coord.RemoveHost("agent-01")

	if got := len(coord.ListAll()); got != 0 {
		t.Fatalf("expected 0 services after RemoveHost, got %d", got)
	}

	// Removing an unknown host should be a no-op.
	coord.RemoveHost("missing-host")
}

func TestRemoveHostRemovesPersistedHostEntries(t *testing.T) {
	store := persistence.NewMemoryWebServiceStore()
	coord := NewCoordinator(store)

	if _, err := coord.SaveManualService(persistence.WebServiceManual{
		HostAssetID: "agent-01",
		Name:        "grafana",
		Category:    "Monitoring",
		URL:         "http://agent-01:3000",
	}); err != nil {
		t.Fatalf("SaveManualService(host1) error: %v", err)
	}
	if _, err := coord.SaveManualService(persistence.WebServiceManual{
		HostAssetID: "agent-02",
		Name:        "portainer",
		Category:    "Management",
		URL:         "http://agent-02:9443",
	}); err != nil {
		t.Fatalf("SaveManualService(host2) error: %v", err)
	}
	if _, err := coord.SaveOverride(persistence.WebServiceOverride{
		HostAssetID: "agent-01",
		ServiceID:   "svc-1",
		Hidden:      true,
	}); err != nil {
		t.Fatalf("SaveOverride(host1) error: %v", err)
	}
	if _, err := coord.SaveOverride(persistence.WebServiceOverride{
		HostAssetID: "agent-02",
		ServiceID:   "svc-2",
		Hidden:      true,
	}); err != nil {
		t.Fatalf("SaveOverride(host2) error: %v", err)
	}

	coord.RemoveHost("agent-01")

	manualsHost1, err := coord.ListManualServices("agent-01")
	if err != nil {
		t.Fatalf("ListManualServices(host1) error: %v", err)
	}
	if len(manualsHost1) != 0 {
		t.Fatalf("expected host1 manuals removed, got %d", len(manualsHost1))
	}

	manualsHost2, err := coord.ListManualServices("agent-02")
	if err != nil {
		t.Fatalf("ListManualServices(host2) error: %v", err)
	}
	if len(manualsHost2) != 1 {
		t.Fatalf("expected host2 manuals to remain, got %d", len(manualsHost2))
	}

	overridesHost1, err := coord.ListOverrides("agent-01")
	if err != nil {
		t.Fatalf("ListOverrides(host1) error: %v", err)
	}
	if len(overridesHost1) != 0 {
		t.Fatalf("expected host1 overrides removed, got %d", len(overridesHost1))
	}

	overridesHost2, err := coord.ListOverrides("agent-02")
	if err != nil {
		t.Fatalf("ListOverrides(host2) error: %v", err)
	}
	if len(overridesHost2) != 1 {
		t.Fatalf("expected host2 overrides to remain, got %d", len(overridesHost2))
	}
}

func TestHostExpiry(t *testing.T) {
	coord := NewCoordinator()
	coord.hostTTL = 50 * time.Millisecond // Short TTL for testing.

	coord.HandleReport("agent-01", makeReportMsg(agentmgr.WebServiceReportData{
		HostAssetID: "agent-01",
		Services: []agentmgr.DiscoveredWebService{
			{ID: "svc-1", Name: "Grafana", Status: "up"},
		},
	}))

	// Disconnect the host.
	coord.MarkHostDisconnected("agent-01")

	// Before TTL expires, host should still be present.
	coord.CleanExpired()
	if len(coord.ListAll()) != 1 {
		t.Fatal("expected host to still be present before TTL")
	}

	// Wait for TTL to expire.
	time.Sleep(60 * time.Millisecond)

	coord.CleanExpired()
	if len(coord.ListAll()) != 0 {
		t.Fatal("expected host to be removed after TTL expiry")
	}
}

func TestListAllIncludesManualAndAppliesOverrides(t *testing.T) {
	store := persistence.NewMemoryWebServiceStore()
	coord := NewCoordinator(store)

	coord.HandleReport("agent-01", makeReportMsg(agentmgr.WebServiceReportData{
		HostAssetID: "agent-01",
		Services: []agentmgr.DiscoveredWebService{
			{ID: "svc-1", Name: "Grafana", Category: "monitoring", URL: "http://host:3000", Status: "up", HostAssetID: "agent-01"},
		},
	}))

	_, err := coord.SaveManualService(persistence.WebServiceManual{
		HostAssetID: "agent-01",
		Name:        "Manual App",
		Category:    "Other",
		URL:         "http://host:9999",
	})
	if err != nil {
		t.Fatalf("SaveManualService() error: %v", err)
	}

	_, err = coord.SaveOverride(persistence.WebServiceOverride{
		HostAssetID:      "agent-01",
		ServiceID:        "svc-1",
		NameOverride:     "Grafana Renamed",
		CategoryOverride: "Monitoring",
		Hidden:           true,
	})
	if err != nil {
		t.Fatalf("SaveOverride() error: %v", err)
	}

	all := coord.ListAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 services after manual+override merge, got %d", len(all))
	}

	var hasManual bool
	var hasOverridden bool
	for _, svc := range all {
		if svc.Source == "manual" && svc.Name == "Manual App" {
			hasManual = true
		}
		if svc.ID == "svc-1" && svc.Name == "Grafana Renamed" && svc.Metadata["hidden"] == "true" {
			hasOverridden = true
		}
	}
	if !hasManual {
		t.Fatal("expected manual service in merged list")
	}
	if !hasOverridden {
		t.Fatal("expected override to be applied to discovered service")
	}
}

func TestListAllNormalizesLabTetherConsoleAndAPI(t *testing.T) {
	coord := NewCoordinator()
	coord.HandleReport("agent-01", makeReportMsg(agentmgr.WebServiceReportData{
		HostAssetID: "agent-01",
		Services: []agentmgr.DiscoveredWebService{
			{
				ID:          "svc-console",
				ServiceKey:  "labtether",
				Name:        "LabTether",
				Category:    "Management",
				URL:         "https://host1:3000",
				Status:      "up",
				HostAssetID: "agent-01",
			},
			{
				ID:          "svc-api",
				ServiceKey:  "labtether",
				Name:        "LabTether",
				Category:    "Management",
				URL:         "https://host1:8443",
				Status:      "up",
				HostAssetID: "agent-01",
			},
		},
	}))

	all := coord.ListAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 services, got %d", len(all))
	}

	var consoleSvc *agentmgr.DiscoveredWebService
	var apiSvc *agentmgr.DiscoveredWebService
	for i := range all {
		switch all[i].ID {
		case "svc-console":
			consoleSvc = &all[i]
		case "svc-api":
			apiSvc = &all[i]
		}
	}
	if consoleSvc == nil || apiSvc == nil {
		t.Fatalf("expected both console and api services, got: %+v", all)
	}

	if consoleSvc.Name != "LabTether Console" {
		t.Fatalf("console name = %q, want %q", consoleSvc.Name, "LabTether Console")
	}
	if consoleSvc.Metadata["labtether_component"] != "console" {
		t.Fatalf("console component = %q, want %q", consoleSvc.Metadata["labtether_component"], "console")
	}
	if consoleSvc.Metadata["hidden"] == "true" {
		t.Fatalf("console hidden = %q, want visible", consoleSvc.Metadata["hidden"])
	}

	if apiSvc.Name != "LabTether API" {
		t.Fatalf("api name = %q, want %q", apiSvc.Name, "LabTether API")
	}
	if apiSvc.Metadata["labtether_component"] != "api" {
		t.Fatalf("api component = %q, want %q", apiSvc.Metadata["labtether_component"], "api")
	}
	if apiSvc.Metadata["hidden"] != "true" {
		t.Fatalf("api hidden = %q, want %q", apiSvc.Metadata["hidden"], "true")
	}
}

func TestListAllLabTetherOverrideCanUnhideAPI(t *testing.T) {
	store := persistence.NewMemoryWebServiceStore()
	coord := NewCoordinator(store)
	coord.HandleReport("agent-01", makeReportMsg(agentmgr.WebServiceReportData{
		HostAssetID: "agent-01",
		Services: []agentmgr.DiscoveredWebService{
			{
				ID:          "svc-console",
				ServiceKey:  "labtether",
				Name:        "LabTether",
				Category:    "Management",
				URL:         "https://host1:3000",
				Status:      "up",
				HostAssetID: "agent-01",
			},
			{
				ID:          "svc-api",
				ServiceKey:  "labtether",
				Name:        "LabTether",
				Category:    "Management",
				URL:         "https://host1:8443",
				Status:      "up",
				HostAssetID: "agent-01",
			},
		},
	}))

	if _, err := coord.SaveOverride(persistence.WebServiceOverride{
		HostAssetID: "agent-01",
		ServiceID:   "svc-api",
		Hidden:      false,
	}); err != nil {
		t.Fatalf("SaveOverride() error: %v", err)
	}

	all := coord.ListAll()
	var apiSvc *agentmgr.DiscoveredWebService
	for i := range all {
		if all[i].ID == "svc-api" {
			apiSvc = &all[i]
			break
		}
	}
	if apiSvc == nil {
		t.Fatalf("expected api service in list, got %+v", all)
	}
	if apiSvc.Name != "LabTether API" {
		t.Fatalf("api name = %q, want %q", apiSvc.Name, "LabTether API")
	}
	if apiSvc.Metadata["hidden"] == "true" {
		t.Fatalf("api hidden = %q, want visible due override", apiSvc.Metadata["hidden"])
	}
}

func TestSummaryByHostsCountsVisibleServicesAndRespectsOverrides(t *testing.T) {
	store := persistence.NewMemoryWebServiceStore()
	coord := NewCoordinator(store)

	coord.HandleReport("agent-01", makeReportMsg(agentmgr.WebServiceReportData{
		HostAssetID: "agent-01",
		Services: []agentmgr.DiscoveredWebService{
			{
				ID:          "svc-console",
				ServiceKey:  "labtether",
				Name:        "LabTether",
				URL:         "https://host1:3000",
				Status:      "up",
				HostAssetID: "agent-01",
			},
			{
				ID:          "svc-api",
				ServiceKey:  "labtether",
				Name:        "LabTether",
				URL:         "https://host1:8443",
				Status:      "up",
				HostAssetID: "agent-01",
			},
			{
				ID:          "svc-hidden",
				Name:        "Hidden Service",
				Status:      "down",
				HostAssetID: "agent-01",
				Metadata: map[string]string{
					"hidden": "true",
				},
			},
			{
				ID:          "svc-up",
				Name:        "Visible Service",
				Status:      "up",
				HostAssetID: "agent-01",
			},
		},
	}))

	manual, err := coord.SaveManualService(persistence.WebServiceManual{
		HostAssetID: "agent-01",
		Name:        "Manual Service",
		Category:    "Other",
		URL:         "http://host1:9999",
	})
	if err != nil {
		t.Fatalf("SaveManualService() error: %v", err)
	}

	if _, err := coord.SaveOverride(persistence.WebServiceOverride{
		HostAssetID: "agent-01",
		ServiceID:   "svc-hidden",
		Hidden:      false,
	}); err != nil {
		t.Fatalf("SaveOverride(unhide svc-hidden) error: %v", err)
	}
	if _, err := coord.SaveOverride(persistence.WebServiceOverride{
		HostAssetID: "agent-01",
		ServiceID:   "svc-up",
		Hidden:      true,
	}); err != nil {
		t.Fatalf("SaveOverride(hide svc-up) error: %v", err)
	}
	if _, err := coord.SaveOverride(persistence.WebServiceOverride{
		HostAssetID: "agent-01",
		ServiceID:   manual.ID,
		Hidden:      true,
	}); err != nil {
		t.Fatalf("SaveOverride(hide manual) error: %v", err)
	}

	up, total := coord.SummaryByHosts(map[string]struct{}{"agent-01": {}})
	if total != 2 {
		t.Fatalf("total = %d, want %d", total, 2)
	}
	if up != 1 {
		t.Fatalf("up = %d, want %d", up, 1)
	}
}

func TestSummaryByHostsEmptyFilterIncludesAllHosts(t *testing.T) {
	coord := NewCoordinator()
	coord.HandleReport("agent-01", makeReportMsg(agentmgr.WebServiceReportData{
		HostAssetID: "agent-01",
		Services: []agentmgr.DiscoveredWebService{
			{ID: "svc-1", Status: "up", HostAssetID: "agent-01"},
		},
	}))
	coord.HandleReport("agent-02", makeReportMsg(agentmgr.WebServiceReportData{
		HostAssetID: "agent-02",
		Services: []agentmgr.DiscoveredWebService{
			{ID: "svc-2", Status: "down", HostAssetID: "agent-02"},
		},
	}))

	up, total := coord.SummaryByHosts(map[string]struct{}{})
	if total != 2 {
		t.Fatalf("total = %d, want %d", total, 2)
	}
	if up != 1 {
		t.Fatalf("up = %d, want %d", up, 1)
	}
}

func TestSummaryByHostsCachesStoreSnapshot(t *testing.T) {
	store := &countingWebServiceStore{MemoryWebServiceStore: persistence.NewMemoryWebServiceStore()}
	coord := NewCoordinator(store)

	coord.HandleReport("agent-01", makeReportMsg(agentmgr.WebServiceReportData{
		HostAssetID: "agent-01",
		Services: []agentmgr.DiscoveredWebService{
			{ID: "svc-1", Status: "up", HostAssetID: "agent-01"},
		},
	}))

	if _, err := coord.SaveManualService(persistence.WebServiceManual{
		HostAssetID: "agent-01",
		Name:        "Manual Service",
		URL:         "http://host1:9999",
	}); err != nil {
		t.Fatalf("SaveManualService() error: %v", err)
	}
	if _, err := coord.SaveOverride(persistence.WebServiceOverride{
		HostAssetID: "agent-01",
		ServiceID:   "svc-1",
		Hidden:      false,
	}); err != nil {
		t.Fatalf("SaveOverride() error: %v", err)
	}
	store.manualListCalls = 0
	store.overrideListCalls = 0

	for i := 0; i < 2; i++ {
		up, total := coord.SummaryByHosts(map[string]struct{}{"agent-01": {}})
		if up != 1 || total != 2 {
			t.Fatalf("SummaryByHosts() = (%d,%d), want (1,2)", up, total)
		}
	}
	if store.manualListCalls != 1 {
		t.Fatalf("manual list calls = %d, want 1", store.manualListCalls)
	}
	if store.overrideListCalls != 1 {
		t.Fatalf("override list calls = %d, want 1", store.overrideListCalls)
	}
}

func TestSummaryByHostsInvalidatesCacheOnOverrideSave(t *testing.T) {
	store := &countingWebServiceStore{MemoryWebServiceStore: persistence.NewMemoryWebServiceStore()}
	coord := NewCoordinator(store)

	coord.HandleReport("agent-01", makeReportMsg(agentmgr.WebServiceReportData{
		HostAssetID: "agent-01",
		Services: []agentmgr.DiscoveredWebService{
			{ID: "svc-1", Status: "up", HostAssetID: "agent-01"},
		},
	}))

	up, total := coord.SummaryByHosts(map[string]struct{}{"agent-01": {}})
	if up != 1 || total != 1 {
		t.Fatalf("warm SummaryByHosts() = (%d,%d), want (1,1)", up, total)
	}

	if _, err := coord.SaveOverride(persistence.WebServiceOverride{
		HostAssetID: "agent-01",
		ServiceID:   "svc-1",
		Hidden:      true,
	}); err != nil {
		t.Fatalf("SaveOverride() error: %v", err)
	}

	up, total = coord.SummaryByHosts(map[string]struct{}{"agent-01": {}})
	if up != 0 || total != 0 {
		t.Fatalf("post-invalidation SummaryByHosts() = (%d,%d), want (0,0)", up, total)
	}
	if store.overrideListCalls < 2 {
		t.Fatalf("override list calls = %d, want at least 2 after invalidation", store.overrideListCalls)
	}
}

func TestSummaryByHostsDoesNotCacheFailedSnapshotLoad(t *testing.T) {
	store := &transientManualListErrorStore{
		countingWebServiceStore: &countingWebServiceStore{MemoryWebServiceStore: persistence.NewMemoryWebServiceStore()},
		failManualReads:         1,
	}
	coord := NewCoordinator(store)

	coord.HandleReport("agent-01", makeReportMsg(agentmgr.WebServiceReportData{
		HostAssetID: "agent-01",
		Services: []agentmgr.DiscoveredWebService{
			{ID: "svc-1", Status: "up", HostAssetID: "agent-01"},
		},
	}))

	if _, err := coord.SaveManualService(persistence.WebServiceManual{
		HostAssetID: "agent-01",
		Name:        "Manual Service",
		URL:         "http://host1:9999",
	}); err != nil {
		t.Fatalf("SaveManualService() error: %v", err)
	}

	up, total := coord.SummaryByHosts(map[string]struct{}{"agent-01": {}})
	if up != 1 || total != 1 {
		t.Fatalf("first SummaryByHosts() = (%d,%d), want (1,1) when snapshot load fails", up, total)
	}

	up, total = coord.SummaryByHosts(map[string]struct{}{"agent-01": {}})
	if up != 1 || total != 2 {
		t.Fatalf("second SummaryByHosts() = (%d,%d), want (1,2) after retry succeeds", up, total)
	}

	up, total = coord.SummaryByHosts(map[string]struct{}{"agent-01": {}})
	if up != 1 || total != 2 {
		t.Fatalf("third SummaryByHosts() = (%d,%d), want cached (1,2)", up, total)
	}

	if store.manualListCalls != 2 {
		t.Fatalf("manual list calls = %d, want 2 (failed load retried once)", store.manualListCalls)
	}
	if store.overrideListCalls != 1 {
		t.Fatalf("override list calls = %d, want 1 after successful cache fill", store.overrideListCalls)
	}
}

func TestAttachHealthSummariesIncludesRollingUptimeAndRecentHistory(t *testing.T) {
	coord := NewCoordinator()
	now := time.Date(2026, time.March, 5, 12, 0, 0, 0, time.UTC)
	coord.nowFn = func() time.Time { return now }

	send := func(status string) {
		coord.HandleReport("agent-01", makeReportMsg(agentmgr.WebServiceReportData{
			HostAssetID: "agent-01",
			Services: []agentmgr.DiscoveredWebService{
				{
					ID:          "svc-1",
					Name:        "Grafana",
					Status:      status,
					ResponseMs:  100,
					HostAssetID: "agent-01",
				},
			},
		}))
	}

	send("up")
	now = now.Add(2 * time.Hour)
	send("down")
	now = now.Add(1 * time.Hour)
	send("up")

	services := coord.ListByHost("agent-01")
	coord.AttachHealthSummaries(services)
	if len(services) != 1 {
		t.Fatalf("expected one service, got %d", len(services))
	}
	health := services[0].Health
	if health == nil {
		t.Fatalf("expected health summary")
	}
	if health.Window != "24h" {
		t.Fatalf("window=%q want=%q", health.Window, "24h")
	}
	if health.Checks != 3 {
		t.Fatalf("checks=%d want=%d", health.Checks, 3)
	}
	if health.UpChecks != 2 {
		t.Fatalf("up_checks=%d want=%d", health.UpChecks, 2)
	}
	if health.UptimePercent < 66.6 || health.UptimePercent > 66.8 {
		t.Fatalf("uptime_percent=%f want around 66.7", health.UptimePercent)
	}
	if health.LastCheckedAt != now.UTC().Format(time.RFC3339) {
		t.Fatalf("last_checked_at=%q want=%q", health.LastCheckedAt, now.UTC().Format(time.RFC3339))
	}
	if health.LastChangeAt == "" {
		t.Fatalf("expected last_change_at to be set")
	}
	if len(health.Recent) != 3 {
		t.Fatalf("recent length=%d want=%d", len(health.Recent), 3)
	}

	now = now.Add(23 * time.Hour)
	services = coord.ListByHost("agent-01")
	coord.AttachHealthSummaries(services)
	health = services[0].Health
	if health == nil {
		t.Fatalf("expected health summary after time advance")
	}
	// The first sample should age out of the 24h rolling window.
	if health.Checks != 2 {
		t.Fatalf("checks=%d want=%d after rolling window advance", health.Checks, 2)
	}
}

func TestMarkHostDisconnectedAddsUnknownHealthSample(t *testing.T) {
	coord := NewCoordinator()
	now := time.Date(2026, time.March, 5, 12, 0, 0, 0, time.UTC)
	coord.nowFn = func() time.Time { return now }

	coord.HandleReport("agent-01", makeReportMsg(agentmgr.WebServiceReportData{
		HostAssetID: "agent-01",
		Services: []agentmgr.DiscoveredWebService{
			{ID: "svc-1", Status: "up", HostAssetID: "agent-01"},
		},
	}))
	now = now.Add(5 * time.Minute)
	coord.MarkHostDisconnected("agent-01")

	services := coord.ListByHost("agent-01")
	coord.AttachHealthSummaries(services)
	health := services[0].Health
	if health == nil {
		t.Fatalf("expected health summary")
	}
	if health.Checks != 2 {
		t.Fatalf("checks=%d want=%d", health.Checks, 2)
	}
	if got := health.Recent[len(health.Recent)-1].Status; got != "unknown" {
		t.Fatalf("latest status=%q want=%q", got, "unknown")
	}
}
