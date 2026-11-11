package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubcollector"
)

// The Remove dialog did not say what goes with a device. Its preview must
// count exactly the devices the delete then removes, and name the login.
func TestDeletePreviewMatchesWhatRemovingAConnectionDeletes(t *testing.T) {
	sut := newTestAPIServer(t)
	if _, err := sut.credentialStore.CreateCredentialProfile(credentials.Profile{
		ID: "cred-nas01", Name: "nas01 key", Kind: credentials.KindTrueNASAPIKey, SecretCiphertext: "x",
	}); err != nil {
		t.Fatalf("create saved login: %v", err)
	}
	sut.hubCollectorStore = &deleteAssetHubCollectorStoreSpy{collectors: []hubcollector.Collector{
		{ID: "hc-nas01", AssetID: "truenas-nas01", CollectorType: hubcollector.CollectorTypeTrueNAS,
			Enabled: true, Config: map[string]any{"credential_id": "cred-nas01"}},
	}}
	for _, req := range []assets.HeartbeatRequest{
		{AssetID: "truenas-nas01", Type: "connector-cluster", Name: "nas01", Source: "truenas"},
		{AssetID: assetid.ScopeCollectorAssetID("truenas-host-nas01", "hc-nas01"), Type: "nas", Name: "nas01", Source: "truenas"},
		{AssetID: "truenas-pool-tank", Type: "storage-pool", Name: "tank", Source: "truenas",
			Metadata: map[string]string{"collector_id": "hc-nas01"}},
		{AssetID: "agent-nas01", Type: "host", Name: "nas01", Source: "agent",
			Metadata: map[string]string{"collector_id": "hc-nas01"}},
	} {
		if _, err := sut.assetStore.UpsertAssetHeartbeat(req); err != nil {
			t.Fatalf("seed %s: %v", req.AssetID, err)
		}
	}
	before, _ := sut.assetStore.ListAssets()

	rec := httptest.NewRecorder()
	sut.handleAssetActions(rec, httptest.NewRequest(http.MethodGet, "/assets/truenas-nas01/delete-preview", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", rec.Code, rec.Body.String())
	}
	var preview struct {
		Devices    int  `json:"devices"`
		Connection bool `json:"connection"`
		SavedLogin bool `json:"saved_login"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if preview.Devices != 2 || !preview.Connection || !preview.SavedLogin {
		t.Fatalf("preview = %+v, want 2 devices, a connection and its login", preview)
	}

	rec = httptest.NewRecorder()
	sut.handleAssetActions(rec, httptest.NewRequest(http.MethodDelete, "/assets/truenas-nas01", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	after, _ := sut.assetStore.ListAssets()
	if removedWith := len(before) - len(after) - 1; removedWith != preview.Devices {
		t.Fatalf("delete removed %d other devices, preview said %d", removedWith, preview.Devices)
	}
}
