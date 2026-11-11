package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubcollector"
)

// Removing a connection from its device page (DELETE on the connection's own
// device) left the devices it had found outside its direct children and its
// saved login with the secret. They must go with it; agent devices and other
// connections' devices must stay.
func TestDeleteConnectionDeviceRemovesItsDevicesAndUnusedLogin(t *testing.T) {
	sut := newTestAPIServer(t)
	const loginID = "cred-truenas-ltqa"
	if _, err := sut.credentialStore.CreateCredentialProfile(credentials.Profile{
		ID: loginID, Name: "nas01 key", Kind: credentials.KindTrueNASAPIKey, SecretCiphertext: "x",
	}); err != nil {
		t.Fatalf("create saved login: %v", err)
	}
	sut.hubCollectorStore = &deleteAssetHubCollectorStoreSpy{collectors: []hubcollector.Collector{
		{ID: "hc-truenas-1", AssetID: "truenas-nas01", CollectorType: hubcollector.CollectorTypeTrueNAS,
			Enabled: true, Config: map[string]any{"credential_id": loginID}},
		{ID: "hc-truenas-2", AssetID: "truenas-nas02", CollectorType: hubcollector.CollectorTypeTrueNAS,
			Enabled: true, Config: map[string]any{}},
	}}

	scopedHost := assetid.ScopeCollectorAssetID("truenas-host-nas01", "hc-truenas-1")
	otherHost := assetid.ScopeCollectorAssetID("truenas-host-nas02", "hc-truenas-2")
	seed := []assets.HeartbeatRequest{
		{AssetID: "truenas-nas01", Type: "connector-cluster", Name: "nas01", Source: "truenas"},
		{AssetID: scopedHost, Type: "nas", Name: "nas01", Source: "truenas"},
		{AssetID: "truenas-pool-tank", Type: "storage-pool", Name: "tank", Source: "truenas",
			Metadata: map[string]string{"collector_id": "hc-truenas-1"}},
		{AssetID: "agent-nas01", Type: "host", Name: "nas01", Source: "agent",
			Metadata: map[string]string{"collector_id": "hc-truenas-1"}},
		{AssetID: "truenas-nas02", Type: "connector-cluster", Name: "nas02", Source: "truenas"},
		{AssetID: otherHost, Type: "nas", Name: "nas02", Source: "truenas"},
	}
	for _, req := range seed {
		if _, err := sut.assetStore.UpsertAssetHeartbeat(req); err != nil {
			t.Fatalf("seed %s: %v", req.AssetID, err)
		}
	}

	rec := httptest.NewRecorder()
	sut.handleAssetActions(rec, httptest.NewRequest(http.MethodDelete, "/assets/truenas-nas01", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	for _, id := range []string{"truenas-nas01", scopedHost, "truenas-pool-tank"} {
		if _, ok, _ := sut.assetStore.GetAsset(id); ok {
			t.Fatalf("expected %s to go with its connection", id)
		}
	}
	for _, id := range []string{"agent-nas01", "truenas-nas02", otherHost} {
		if _, ok, _ := sut.assetStore.GetAsset(id); !ok {
			t.Fatalf("expected %s to stay", id)
		}
	}
	if _, ok, _ := sut.credentialStore.GetCredentialProfile(loginID); ok {
		t.Fatalf("expected the connection's saved login to be removed")
	}
}
