package pbs_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/assets"
	pbspkg "github.com/labtether/labtether/internal/hubapi/pbs"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/hubapi/testutil/hubdeps"
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
	pbspkg.WritePBSResolveError(rec, pbspkg.ErrPBSAssetNotFound)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing asset, got %d", rec.Code)
	}
	testutil.AssertErrorBodyContains(t, rec.Body.Bytes(), pbspkg.ErrPBSAssetNotFound.Error())

	rec = httptest.NewRecorder()
	pbspkg.WritePBSResolveError(rec, pbspkg.ErrAssetNotPBS)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-pbs asset, got %d", rec.Code)
	}
	testutil.AssertErrorBodyContains(t, rec.Body.Bytes(), pbspkg.ErrAssetNotPBS.Error())

	rec = httptest.NewRecorder()
	pbspkg.WritePBSResolveError(rec, errors.New("upstream failure"))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 for generic resolve error, got %d", rec.Code)
	}
	testutil.AssertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")
}

func TestResolvePBSAssetBranches(t *testing.T) {
	t.Run("asset store unavailable", func(t *testing.T) {
		sut := hubdeps.New(t)
		sut.AssetStore = nil
		if _, err := sut.PBSDeps().ResolvePBSAsset("pbs-1"); !errors.Is(err, pbspkg.ErrPBSAssetNotFound) {
			t.Fatalf("expected errPBSAssetNotFound, got %v", err)
		}
	})

	t.Run("blank asset id", func(t *testing.T) {
		sut := hubdeps.New(t)
		if _, err := sut.PBSDeps().ResolvePBSAsset("   "); !errors.Is(err, pbspkg.ErrPBSAssetNotFound) {
			t.Fatalf("expected errPBSAssetNotFound for blank id, got %v", err)
		}
	})

	t.Run("asset load failure", func(t *testing.T) {
		sut := hubdeps.New(t)
		sut.AssetStore = &failingPBSAssetStore{
			AssetStore: sut.AssetStore,
			err:        errors.New("storage down"),
		}
		if _, err := sut.PBSDeps().ResolvePBSAsset("pbs-1"); err == nil || !strings.Contains(err.Error(), "failed to load asset") {
			t.Fatalf("expected wrapped load error, got %v", err)
		}
	})

	t.Run("asset not found", func(t *testing.T) {
		sut := hubdeps.New(t)
		if _, err := sut.PBSDeps().ResolvePBSAsset("missing"); !errors.Is(err, pbspkg.ErrPBSAssetNotFound) {
			t.Fatalf("expected errPBSAssetNotFound, got %v", err)
		}
	})

	t.Run("asset is not pbs-backed", func(t *testing.T) {
		sut := hubdeps.New(t)
		if _, err := sut.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: "docker-host-1",
			Type:    "container-host",
			Name:    "docker-host-1",
			Source:  "docker",
			Status:  "online",
		}); err != nil {
			t.Fatalf("seed docker asset: %v", err)
		}
		if _, err := sut.PBSDeps().ResolvePBSAsset("docker-host-1"); !errors.Is(err, pbspkg.ErrAssetNotPBS) {
			t.Fatalf("expected errAssetNotPBS, got %v", err)
		}
	})

	t.Run("resolve pbs asset success", func(t *testing.T) {
		sut := hubdeps.New(t)
		if _, err := sut.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: "pbs-datastore-backup",
			Type:    "storage-pool",
			Name:    "backup",
			Source:  "PBS",
			Status:  "online",
		}); err != nil {
			t.Fatalf("seed pbs asset: %v", err)
		}
		asset, err := sut.PBSDeps().ResolvePBSAsset("  pbs-datastore-backup  ")
		if err != nil {
			t.Fatalf("resolvePBSAsset() error = %v", err)
		}
		if asset.ID != "pbs-datastore-backup" {
			t.Fatalf("asset id = %q", asset.ID)
		}
	})
}

func TestResolvePBSAssetRuntimeHonorsCollectorOwner(t *testing.T) {
	t.Run("preferred collector resolves directly", func(t *testing.T) {
		sut := hubdeps.New(t)
		createPBSCredentialProfile(t, sut, "cred-pbs-resolve-1", "root@pam!token-1", "secret-1", "https://pbs.local:8007")
		sut.HubCollectorStore = &testutil.ErrorHubCollectorStore{
			Collectors: []hubcollector.Collector{
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
		if _, err := sut.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
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

		asset, runtime, err := sut.PBSDeps().ResolvePBSAssetRuntime("pbs-server-1")
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

	t.Run("missing preferred collector cannot fall back to another owner", func(t *testing.T) {
		sut := hubdeps.New(t)
		createPBSCredentialProfile(t, sut, "cred-pbs-resolve-2", "root@pam!token-2", "secret-2", "https://pbs.local:8007")
		sut.HubCollectorStore = &testutil.ErrorHubCollectorStore{
			Collectors: []hubcollector.Collector{
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
		if _, err := sut.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
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

		if _, err := sut.PBSDeps().LoadPBSRuntime("collector-pbs-2"); err != nil {
			t.Fatalf("replacement collector must be usable: %v", err)
		}
		if _, runtime, err := sut.PBSDeps().ResolvePBSAssetRuntime("pbs-server-2"); err == nil || runtime != nil {
			t.Fatalf("missing owner must not resolve a replacement: runtime=%v err=%v", runtime, err)
		}
	})

	t.Run("runtime load error is wrapped", func(t *testing.T) {
		sut := hubdeps.New(t)
		if _, err := sut.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
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
		if _, _, err := sut.PBSDeps().ResolvePBSAssetRuntime("pbs-server-error"); err == nil || !strings.Contains(err.Error(), "failed to load pbs runtime") {
			t.Fatalf("expected wrapped runtime load error, got %v", err)
		}
	})
}

func TestPBSNodeAndStoreFromAsset(t *testing.T) {
	if got := pbspkg.PBSNodeFromAsset(assets.Asset{Metadata: map[string]string{"node": " node-a "}}); got != "node-a" {
		t.Fatalf("pbsNodeFromAsset(metadata) = %q, want node-a", got)
	}
	if got := pbspkg.PBSNodeFromAsset(assets.Asset{}); got != "localhost" {
		t.Fatalf("pbsNodeFromAsset(default) = %q, want localhost", got)
	}

	if got := pbspkg.PBSStoreFromAsset(assets.Asset{Metadata: map[string]string{"store": " backup "}}); got != "backup" {
		t.Fatalf("pbsStoreFromAsset(metadata) = %q, want backup", got)
	}
	if got := pbspkg.PBSStoreFromAsset(assets.Asset{ID: "pbs-datastore-archive"}); got != "archive" {
		t.Fatalf("pbsStoreFromAsset(id prefix) = %q, want archive", got)
	}
	if got := pbspkg.PBSStoreFromAsset(assets.Asset{ID: "pbs-server-main"}); got != "" {
		t.Fatalf("pbsStoreFromAsset(non-datastore) = %q, want empty", got)
	}
}
