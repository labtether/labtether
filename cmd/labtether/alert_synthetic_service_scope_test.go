package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/assetstatus"
	"github.com/labtether/labtether/internal/connectors/webservice"
	"github.com/labtether/labtether/internal/persistence"
)

type failedSyntheticManualLookup struct{ persistence.WebServiceStore }

func (failedSyntheticManualLookup) GetManualWebServiceContext(context.Context, string) (persistence.WebServiceManual, bool, error) {
	return persistence.WebServiceManual{}, false, errors.New("manual lookup unavailable")
}

// Exercise the production dependency callback with actual coordinator reports
// and stores. A shared service ID must never invent an incident asset witness.
func TestSyntheticServiceScopeUsesUniqueCurrentOwner(t *testing.T) {
	for _, tc := range []struct {
		name        string
		reportHost  []string
		manual      *persistence.WebServiceManual
		serviceID   string
		wantID      string
		storeError  bool
		cancelled   bool
		deleted     bool
		legacyOnly  bool
		assetLegacy bool
	}{
		{name: "discovered", reportHost: []string{"host-a"}, serviceID: "svc", wantID: "host-a"},
		{name: "manual", manual: &persistence.WebServiceManual{ID: "svc", HostAssetID: "host-a"}, serviceID: "svc", wantID: "host-a"},
		{name: "same owner", reportHost: []string{"host-a"}, manual: &persistence.WebServiceManual{ID: "svc", HostAssetID: "host-a"}, serviceID: "svc", wantID: "host-a"},
		{name: "two reports", reportHost: []string{"host-a", "host-b"}, serviceID: "svc"},
		{name: "manual conflict", reportHost: []string{"host-a"}, manual: &persistence.WebServiceManual{ID: "svc", HostAssetID: "host-b"}, serviceID: "svc"},
		{name: "standalone conflict", reportHost: []string{"host-a"}, manual: &persistence.WebServiceManual{ID: "svc"}, serviceID: "svc"},
		{name: "missing asset", reportHost: []string{"absent-host"}, serviceID: "svc"},
		{name: "same URL is not identity", reportHost: []string{"host-a"}, serviceID: "other-svc"},
		{name: "exact ID only", reportHost: []string{"host-a"}, serviceID: " svc "},
		{name: "store uncertainty", reportHost: []string{"host-a"}, serviceID: "svc", storeError: true},
		{name: "cancelled", reportHost: []string{"host-a"}, serviceID: "svc", cancelled: true},
		{name: "stale manual cache", manual: &persistence.WebServiceManual{ID: "svc", HostAssetID: "host-a"}, serviceID: "svc", deleted: true},
		{name: "no context capability", reportHost: []string{"host-a"}, serviceID: "svc", legacyOnly: true},
		{name: "no asset context capability", reportHost: []string{"host-a"}, serviceID: "svc", assetLegacy: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sut := newTestAPIServer(t)
			for _, id := range []string{"host-a", "host-b"} {
				if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: id, Type: "node", Name: id, Source: "agent", Status: "online", GroupID: "group-" + id}); err != nil {
					t.Fatal(err)
				}
			}
			store := persistence.NewMemoryWebServiceStore()
			if tc.manual != nil {
				if _, err := store.SaveManualWebService(*tc.manual); err != nil {
					t.Fatal(err)
				}
			}
			sut.webServiceCoordinator = webservice.NewCoordinator(store)
			if tc.legacyOnly {
				sut.webServiceCoordinator = webservice.NewCoordinator(struct{ persistence.WebServiceStore }{store})
			}
			if tc.storeError {
				sut.webServiceCoordinator = webservice.NewCoordinator(failedSyntheticManualLookup{store})
			}
			for _, host := range tc.reportHost {
				data, err := json.Marshal(agentmgr.WebServiceReportData{Services: []agentmgr.DiscoveredWebService{{ID: "svc", URL: "https://same.example.test/", HostAssetID: host}}})
				if err != nil {
					t.Fatal(err)
				}
				sut.webServiceCoordinator.HandleReport(host, agentmgr.Message{Data: data})
			}
			if tc.deleted {
				if len(sut.webServiceCoordinator.ListAll()) != 1 {
					t.Fatal("manual cache was not primed")
				}
				if err := store.DeleteManualWebService("svc"); err != nil {
					t.Fatal(err)
				}
			}
			// Production uses this decorator; its forwarding must retain the
			// cancellable lookup instead of dropping to the legacy AssetStore.
			if tc.assetLegacy {
				sut.assetStore = struct{ persistence.AssetStore }{sut.assetStore}
			}
			sut.assetStore = assetstatus.NewStore(sut.assetStore, nil)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancelled {
				cancel()
			}
			entry, found, err := sut.buildAlertingDeps().ResolveSyntheticServiceAsset(ctx, tc.serviceID)
			// The status decorator reports a missing exact-read capability as
			// unknown (an error); the evaluator then attaches no witness.
			if (err != nil) != (tc.storeError || tc.cancelled || tc.legacyOnly || tc.assetLegacy) {
				t.Fatalf("unexpected lookup error: %v", err)
			}
			if found != (tc.wantID != "") || entry.ID != tc.wantID {
				t.Fatalf("owner=%q found=%v, want %q", entry.ID, found, tc.wantID)
			}
			if found && entry.GroupID != "group-"+tc.wantID {
				t.Fatalf("final asset group witness lost: %q", entry.GroupID)
			}
		})
	}
}
