package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/persistence"
)

type failingPBSAssetStore struct {
	persistence.AssetStore
	err error
}

func (s *failingPBSAssetStore) GetAsset(assetID string) (assets.Asset, bool, error) {
	if s.err != nil {
		return assets.Asset{}, false, s.err
	}
	return s.AssetStore.GetAsset(assetID)
}

func TestWritePBSResolveErrorBranches(t *testing.T) {
	rec := httptest.NewRecorder()
	writePBSResolveError(rec, errPBSAssetNotFound)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing asset, got %d", rec.Code)
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), errPBSAssetNotFound.Error())

	rec = httptest.NewRecorder()
	writePBSResolveError(rec, errAssetNotPBS)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-pbs asset, got %d", rec.Code)
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), errAssetNotPBS.Error())

	rec = httptest.NewRecorder()
	writePBSResolveError(rec, errors.New("upstream failure"))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 for generic resolve error, got %d", rec.Code)
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "upstream failure")
}

func TestResolvePBSAssetBranches(t *testing.T) {
	t.Run("asset store unavailable", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.assetStore = nil
		if _, err := sut.resolvePBSAsset("pbs-1"); !errors.Is(err, errPBSAssetNotFound) {
			t.Fatalf("expected errPBSAssetNotFound, got %v", err)
		}
	})

	t.Run("blank asset id", func(t *testing.T) {
		sut := newTestAPIServer(t)
		if _, err := sut.resolvePBSAsset("   "); !errors.Is(err, errPBSAssetNotFound) {
			t.Fatalf("expected errPBSAssetNotFound for blank id, got %v", err)
		}
	})

	t.Run("asset load failure", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.assetStore = &failingPBSAssetStore{
			AssetStore: sut.assetStore,
			err:        errors.New("storage down"),
		}
		if _, err := sut.resolvePBSAsset("pbs-1"); err == nil || !strings.Contains(err.Error(), "failed to load asset") {
			t.Fatalf("expected wrapped load error, got %v", err)
		}
	})

	t.Run("asset not found", func(t *testing.T) {
		sut := newTestAPIServer(t)
		if _, err := sut.resolvePBSAsset("missing"); !errors.Is(err, errPBSAssetNotFound) {
			t.Fatalf("expected errPBSAssetNotFound, got %v", err)
		}
	})

	t.Run("asset is not pbs-backed", func(t *testing.T) {
		sut := newTestAPIServer(t)
		if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: "docker-host-1",
			Type:    "container-host",
			Name:    "docker-host-1",
			Source:  "docker",
			Status:  "online",
		}); err != nil {
			t.Fatalf("seed docker asset: %v", err)
		}
		if _, err := sut.resolvePBSAsset("docker-host-1"); !errors.Is(err, errAssetNotPBS) {
			t.Fatalf("expected errAssetNotPBS, got %v", err)
		}
	})

	t.Run("resolve pbs asset success", func(t *testing.T) {
		sut := newTestAPIServer(t)
		if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: "pbs-datastore-backup",
			Type:    "storage-pool",
			Name:    "backup",
			Source:  "PBS",
			Status:  "online",
		}); err != nil {
			t.Fatalf("seed pbs asset: %v", err)
		}
		asset, err := sut.resolvePBSAsset("  pbs-datastore-backup  ")
		if err != nil {
			t.Fatalf("resolvePBSAsset() error = %v", err)
		}
		if asset.ID != "pbs-datastore-backup" {
			t.Fatalf("asset id = %q", asset.ID)
		}
	})
}

func TestResolvePBSAssetRuntimeFallbackAndErrors(t *testing.T) {
	t.Run("preferred collector resolves directly", func(t *testing.T) {
		sut := newTestAPIServer(t)
		createPBSCredentialProfile(t, sut, "cred-pbs-resolve-1", "root@pam!token-1", "secret-1", "https://pbs.local:8007")
		sut.hubCollectorStore = &errorHubCollectorStore{
			collectors: []hubcollector.Collector{
				{
					ID:            "collector-pbs-1",
					AssetID:       "pbs-server-1",
					CollectorType: hubcollector.CollectorTypePBS,
					Enabled:       true,
					Config: map[string]any{
						"base_url":      "https://pbs.local:8007",
						"credential_id": "cred-pbs-resolve-1",
						"token_id":      "root@pam!token-1",
						"skip_verify":   true,
					},
				},
			},
		}
		if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: "pbs-server-1",
			Type:    "storage-controller",
			Name:    "pbs-1",
			Source:  "pbs",
			Status:  "online",
			Metadata: map[string]string{
				"collector_id": "collector-pbs-1",
			},
		}); err != nil {
			t.Fatalf("seed pbs asset: %v", err)
		}

		asset, runtime, err := sut.resolvePBSAssetRuntime("pbs-server-1")
		if err != nil {
			t.Fatalf("resolvePBSAssetRuntime() error = %v", err)
		}
		if asset.ID != "pbs-server-1" {
			t.Fatalf("asset id = %q", asset.ID)
		}
		if runtime == nil || runtime.CollectorID != "collector-pbs-1" {
			t.Fatalf("expected runtime collector-pbs-1, got %+v", runtime)
		}
	})

	t.Run("preferred collector fallback to first active", func(t *testing.T) {
		sut := newTestAPIServer(t)
		createPBSCredentialProfile(t, sut, "cred-pbs-resolve-2", "root@pam!token-2", "secret-2", "https://pbs.local:8007")
		sut.hubCollectorStore = &errorHubCollectorStore{
			collectors: []hubcollector.Collector{
				{
					ID:            "collector-pbs-2",
					AssetID:       "pbs-server-2",
					CollectorType: hubcollector.CollectorTypePBS,
					Enabled:       true,
					Config: map[string]any{
						"base_url":      "https://pbs.local:8007",
						"credential_id": "cred-pbs-resolve-2",
						"token_id":      "root@pam!token-2",
						"skip_verify":   true,
					},
				},
			},
		}
		if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: "pbs-server-2",
			Type:    "storage-controller",
			Name:    "pbs-2",
			Source:  "pbs",
			Status:  "online",
			Metadata: map[string]string{
				"collector_id": "collector-missing",
			},
		}); err != nil {
			t.Fatalf("seed pbs asset: %v", err)
		}

		_, runtime, err := sut.resolvePBSAssetRuntime("pbs-server-2")
		if err != nil {
			t.Fatalf("resolvePBSAssetRuntime() error = %v", err)
		}
		if runtime == nil || runtime.CollectorID != "collector-pbs-2" {
			t.Fatalf("expected fallback runtime collector-pbs-2, got %+v", runtime)
		}
	})

	t.Run("runtime load error is wrapped", func(t *testing.T) {
		sut := newTestAPIServer(t)
		if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: "pbs-server-error",
			Type:    "storage-controller",
			Name:    "pbs-error",
			Source:  "pbs",
			Status:  "online",
			Metadata: map[string]string{
				"collector_id": "collector-missing",
			},
		}); err != nil {
			t.Fatalf("seed pbs asset: %v", err)
		}
		if _, _, err := sut.resolvePBSAssetRuntime("pbs-server-error"); err == nil || !strings.Contains(err.Error(), "failed to load pbs runtime") {
			t.Fatalf("expected wrapped runtime load error, got %v", err)
		}
	})
}

func TestPBSNodeAndStoreFromAsset(t *testing.T) {
	if got := pbsNodeFromAsset(assets.Asset{Metadata: map[string]string{"node": " node-a "}}); got != "node-a" {
		t.Fatalf("pbsNodeFromAsset(metadata) = %q, want node-a", got)
	}
	if got := pbsNodeFromAsset(assets.Asset{}); got != "localhost" {
		t.Fatalf("pbsNodeFromAsset(default) = %q, want localhost", got)
	}

	if got := pbsStoreFromAsset(assets.Asset{Metadata: map[string]string{"store": " backup "}}); got != "backup" {
		t.Fatalf("pbsStoreFromAsset(metadata) = %q, want backup", got)
	}
	if got := pbsStoreFromAsset(assets.Asset{ID: "pbs-datastore-archive"}); got != "archive" {
		t.Fatalf("pbsStoreFromAsset(id prefix) = %q, want archive", got)
	}
	if got := pbsStoreFromAsset(assets.Asset{ID: "pbs-server-main"}); got != "" {
		t.Fatalf("pbsStoreFromAsset(non-datastore) = %q, want empty", got)
	}
}
