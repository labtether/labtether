package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/hubcollector"
)

func TestExecuteTwoProxmoxCollectorsKeepRepeatedVMIDDistinct(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/cluster/resources":
			_, _ = w.Write([]byte(`{"data":[{"type":"node","node":"pve","status":"online"},{"type":"qemu","id":"qemu/101","node":"pve","vmid":101,"name":"vm-101","status":"running"}]}`))
		case "/api2/json/nodes/pve/qemu/101/config":
			_, _ = w.Write([]byte(`{"data":{}}`))
		case "/api2/json/cluster/tasks":
			_, _ = w.Write([]byte(`{"data":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store
	collectorIDs := []string{"collector-proxmox-delta", "collector-proxmox-gamma"}
	for index, collectorID := range collectorIDs {
		credentialID := "cred-proxmox-scope-" + string(rune('a'+index))
		createProxmoxCredentialProfile(t, sut, credentialID, "labtether@pve!agent", "token-secret", server.URL)
		collector := hubcollector.Collector{
			ID:            collectorID,
			AssetID:       "proxmox-cluster-" + collectorID,
			CollectorType: hubcollector.CollectorTypeProxmox,
			Enabled:       true,
			Config: map[string]any{
				"base_url":      server.URL,
				"credential_id": credentialID,
				"token_id":      "labtether@pve!agent",
				"auth_method":   "api_token",
				"skip_verify":   true,
			},
		}
		store.StatusByID[collector.ID] = collector
		sut.executeCollector(context.Background(), collector)
	}

	for _, collectorID := range collectorIDs {
		id := assetid.ScopeCollectorAssetID("proxmox-vm-101", collectorID)
		asset, ok, err := sut.assetStore.GetAsset(id)
		if err != nil || !ok {
			t.Fatalf("expected scoped VM %s: ok=%v err=%v", id, ok, err)
		}
		if asset.Metadata["collector_id"] != collectorID || asset.Metadata["vmid"] != "101" {
			t.Fatalf("unexpected VM metadata for %s: %#v", id, asset.Metadata)
		}
	}
	if _, ok, err := sut.assetStore.GetAsset("proxmox-vm-101"); err != nil || ok {
		t.Fatalf("unexpected ambiguous legacy VM: ok=%v err=%v", ok, err)
	}
}
