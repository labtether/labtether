package webservice

import (
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/persistence"
	"testing"
)

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
