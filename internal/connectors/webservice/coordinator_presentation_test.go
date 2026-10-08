package webservice

import (
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/persistence"
	"testing"
)

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
