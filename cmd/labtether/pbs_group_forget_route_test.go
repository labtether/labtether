package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubcollector"
)

func TestHandlePBSGroupForgetRouteAllowsDelete(t *testing.T) {
	seen := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api2/json/admin/datastore/backup/groups" {
			t.Fatalf("unexpected pbs request: %s %s", r.Method, r.URL.Path)
		}
		for key, want := range map[string]string{
			"backup-type": "vm",
			"backup-id":   "900",
		} {
			if got := r.URL.Query().Get(key); got != want {
				t.Fatalf("expected %s=%q, got %q", key, want, got)
			}
		}
		seen <- struct{}{}
		_, _ = w.Write([]byte(`{"data":null}`))
	}))
	defer server.Close()

	sut := newTestAPIServer(t)
	createPBSCredentialProfile(t, sut, "cred-pbs-group-forget", "root@pam!labtether", "secret-group-forget", server.URL)
	sut.hubCollectorStore = &stubHubCollectorStore{collectors: []hubcollector.Collector{{
		ID:            "collector-pbs-group-forget",
		AssetID:       "pbs-server-group-forget",
		CollectorType: hubcollector.CollectorTypePBS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"token_id":      "root@pam!labtether",
			"credential_id": "cred-pbs-group-forget",
			"skip_verify":   true,
		},
	}}}
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "pbs-datastore-backup",
		Type:    "storage-pool",
		Name:    "backup",
		Source:  "pbs",
		Status:  "online",
		Metadata: map[string]string{
			"store":        "backup",
			"collector_id": "collector-pbs-group-forget",
		},
	}); err != nil {
		t.Fatalf("seed pbs datastore asset: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/pbs/assets/pbs-datastore-backup/groups/forget?store=backup&backup-type=vm&backup-id=900", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "owner", "owner"))
	rec := httptest.NewRecorder()
	sut.handlePBSAssets(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from group forget, got %d body=%s", rec.Code, rec.Body.String())
	}
	select {
	case <-seen:
	default:
		t.Fatal("group forget did not reach PBS")
	}
}
