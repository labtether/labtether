package persistence

import (
	"testing"
)

func TestSaveManualWebService_Standalone(t *testing.T) {
	store := NewMemoryWebServiceStore()

	svc := WebServiceManual{
		HostAssetID: "",
		Name:        "My Standalone Service",
		Category:    "Other",
		URL:         "http://standalone.example.com",
	}

	saved, err := store.SaveManualWebService(svc)
	if err != nil {
		t.Fatalf("SaveManualWebService: unexpected error: %v", err)
	}
	if saved.ID == "" {
		t.Fatal("expected a generated ID, got empty string")
	}
	if saved.HostAssetID != "" {
		t.Fatalf("expected empty HostAssetID, got %q", saved.HostAssetID)
	}
}

func TestListManualWebServices_IncludesStandalone(t *testing.T) {
	store := NewMemoryWebServiceStore()

	hosted := WebServiceManual{
		HostAssetID: "asset-1",
		Name:        "Hosted Service",
		Category:    "Other",
		URL:         "http://hosted.example.com",
	}
	standalone := WebServiceManual{
		HostAssetID: "",
		Name:        "Standalone Service",
		Category:    "Other",
		URL:         "http://standalone.example.com",
	}

	if _, err := store.SaveManualWebService(hosted); err != nil {
		t.Fatalf("save hosted: %v", err)
	}
	if _, err := store.SaveManualWebService(standalone); err != nil {
		t.Fatalf("save standalone: %v", err)
	}

	all, err := store.ListManualWebServices("")
	if err != nil {
		t.Fatalf("ListManualWebServices (empty filter): %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 services with empty filter, got %d", len(all))
	}

	byHost, err := store.ListManualWebServices("asset-1")
	if err != nil {
		t.Fatalf("ListManualWebServices (asset-1): %v", err)
	}
	if len(byHost) != 1 {
		t.Fatalf("expected 1 service for asset-1 filter, got %d", len(byHost))
	}
	if byHost[0].HostAssetID != "asset-1" {
		t.Fatalf("expected HostAssetID asset-1, got %q", byHost[0].HostAssetID)
	}
}

func TestPromoteManualServicesToStandalone(t *testing.T) {
	store := NewMemoryWebServiceStore()

	for i, tc := range []struct {
		host string
		name string
		url  string
	}{
		{"asset-1", "Service A", "http://a.example.com"},
		{"asset-1", "Service B", "http://b.example.com"},
		{"asset-2", "Service C", "http://c.example.com"},
	} {
		if _, err := store.SaveManualWebService(WebServiceManual{
			HostAssetID: tc.host,
			Name:        tc.name,
			Category:    "Other",
			URL:         tc.url,
		}); err != nil {
			t.Fatalf("save service %d: %v", i, err)
		}
	}

	if err := store.PromoteManualServicesToStandalone("asset-1"); err != nil {
		t.Fatalf("PromoteManualServicesToStandalone: %v", err)
	}

	all, err := store.ListManualWebServices("")
	if err != nil {
		t.Fatalf("ListManualWebServices: %v", err)
	}

	promoted := 0
	unchanged := 0
	for _, svc := range all {
		switch svc.HostAssetID {
		case "":
			promoted++
		case "asset-2":
			unchanged++
		default:
			t.Errorf("unexpected HostAssetID %q after promotion", svc.HostAssetID)
		}
	}

	if promoted != 2 {
		t.Errorf("expected 2 promoted (empty host) services, got %d", promoted)
	}
	if unchanged != 1 {
		t.Errorf("expected 1 unchanged (asset-2) service, got %d", unchanged)
	}
}
