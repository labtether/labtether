package main

import (
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectors/pbs"
	"github.com/labtether/labtether/internal/hubcollector"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandlePBSAssetsGuardAndErrorBranches(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/pbs/assets//details", nil)
	rec := httptest.NewRecorder()
	sut.handlePBSAssets(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for empty asset id, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/pbs/assets/missing/details", nil)
	rec = httptest.NewRecorder()
	sut.handlePBSAssets(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing pbs asset, got %d", rec.Code)
	}

	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "pbs-datastore-runtime-missing",
		Type:    "storage-pool",
		Name:    "backup",
		Source:  "pbs",
		Status:  "online",
		Metadata: map[string]string{
			"store": "backup",
		},
	}); err != nil {
		t.Fatalf("seed pbs datastore asset: %v", err)
	}
	req = httptest.NewRequest(http.MethodGet, "/pbs/assets/pbs-datastore-runtime-missing/details", nil)
	rec = httptest.NewRecorder()
	sut.handlePBSAssets(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when runtime missing, got %d", rec.Code)
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/version":
			_, _ = w.Write([]byte(`{"data":{"release":"3.2-1"}}`))
		case "/api2/json/admin/datastore/backup/status":
			http.Error(w, `{"errors":"status failed"}`, http.StatusBadGateway)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	sut = newTestAPIServer(t)
	createPBSCredentialProfile(t, sut, "cred-pbs-asset-error", "root@pam!asset", "secret-asset", server.URL)
	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{
			{
				ID:            "collector-pbs-asset-error",
				AssetID:       "pbs-server-asset-error",
				CollectorType: hubcollector.CollectorTypePBS,
				Enabled:       true,
				Config: map[string]any{
					"base_url":      server.URL,
					"credential_id": "cred-pbs-asset-error",
					"token_id":      "root@pam!asset",
					"skip_verify":   true,
				},
			},
		},
	}
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "pbs-datastore-backup",
		Type:    "storage-pool",
		Name:    "backup",
		Source:  "pbs",
		Status:  "offline",
		Metadata: map[string]string{
			"store":        "backup",
			"collector_id": "collector-pbs-asset-error",
		},
	}); err != nil {
		t.Fatalf("seed pbs datastore asset: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/pbs/assets/pbs-datastore-backup/details", nil)
	rec = httptest.NewRecorder()
	sut.handlePBSAssets(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when detail load fails, got %d", rec.Code)
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")
	failedAsset, exists, err := sut.assetStore.GetAsset("pbs-datastore-backup")
	if err != nil || !exists {
		t.Fatalf("load pbs asset after failed refresh: exists=%v err=%v", exists, err)
	}
	if failedAsset.Status != "offline" {
		t.Fatalf("expected failed details refresh not to improve offline status, got %q", failedAsset.Status)
	}

	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "pbs-datastore-online-backup",
		Type:    "storage-pool",
		Name:    "backup",
		Source:  "pbs",
		Status:  "online",
		Metadata: map[string]string{
			"store":        "backup",
			"collector_id": "collector-pbs-asset-error",
		},
	}); err != nil {
		t.Fatalf("seed online pbs datastore asset: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/pbs/assets/pbs-datastore-online-backup/details", nil)
	rec = httptest.NewRecorder()
	sut.handlePBSAssets(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when online asset detail refresh fails, got %d", rec.Code)
	}
	failedOnlineAsset, exists, err := sut.assetStore.GetAsset("pbs-datastore-online-backup")
	if err != nil || !exists {
		t.Fatalf("load online pbs asset after failed refresh: exists=%v err=%v", exists, err)
	}
	if failedOnlineAsset.Status != "unresponsive" {
		t.Fatalf("expected failed details refresh to mark online asset unresponsive, got %q", failedOnlineAsset.Status)
	}
	if failedOnlineAsset.Metadata["store"] != "backup" || failedOnlineAsset.Metadata["collector_id"] != "collector-pbs-asset-error" {
		t.Fatalf("expected failed details refresh to retain inventory identity metadata, got %#v", failedOnlineAsset.Metadata)
	}
}

func TestPBSAPIHelperFunctions(t *testing.T) {
	if node, upid, ok := parsePBSTaskPath("/pbs/tasks/node-a/UPID-1/status", "status"); !ok || node != "node-a" || upid != "UPID-1" {
		t.Fatalf("parsePBSTaskPath success mismatch: ok=%v node=%q upid=%q", ok, node, upid)
	}
	if _, _, ok := parsePBSTaskPath("/tasks/node-a/UPID-1/status", "status"); ok {
		t.Fatalf("expected parse failure for invalid prefix")
	}
	if _, _, ok := parsePBSTaskPath("/pbs/tasks/", "status"); ok {
		t.Fatalf("expected parse failure for empty task path")
	}
	if _, _, ok := parsePBSTaskPath("/pbs/tasks/node-a/UPID-1/log", "status"); ok {
		t.Fatalf("expected parse failure for action mismatch")
	}
	if _, _, ok := parsePBSTaskPath("/pbs/tasks/node-a", "status"); ok {
		t.Fatalf("expected parse failure for short task path")
	}

	tasks := []pbs.Task{
		{UPID: "UPID-A", WorkerID: "backup:vm/101", StartTime: 10},
		{UPID: "UPID-Z", WorkerID: "backup:vm/102", StartTime: 20},
		{UPID: "UPID-M", WorkerID: "other:vm/103", StartTime: 30},
		{UPID: "UPID:node:123:backup:extra:", WorkerID: "", StartTime: 20},
	}

	filtered := filterAndSortPBSTasks(tasks, "backup", 2)
	if len(filtered) != 2 {
		t.Fatalf("expected limited filtered tasks=2, got %d", len(filtered))
	}
	if filtered[0].UPID != "UPID:node:123:backup:extra:" || filtered[1].UPID != "UPID-Z" {
		t.Fatalf("unexpected filtered/sorted order: %+v", filtered)
	}

	all := filterAndSortPBSTasks(tasks, "", 0)
	if len(all) != len(tasks) {
		t.Fatalf("expected no-filter tasks=%d, got %d", len(tasks), len(all))
	}
	if all[0].StartTime < all[1].StartTime {
		t.Fatalf("expected descending sort by start time")
	}

	if got := dedupeNonEmptyWarnings(nil); got != nil {
		t.Fatalf("expected nil output for nil warnings, got %v", got)
	}
	if got := dedupeNonEmptyWarnings([]string{" ", "\t"}); got != nil {
		t.Fatalf("expected nil output for empty warnings, got %v", got)
	}
	warnings := dedupeNonEmptyWarnings([]string{" warning A ", "warning a", "warning B"})
	if len(warnings) != 2 || warnings[0] != "warning A" || warnings[1] != "warning B" {
		t.Fatalf("unexpected dedupe output: %v", warnings)
	}
}

func configurePBSTaskRuntime(t *testing.T, sut *apiServer, collectorID, credentialID, tokenID, baseURL string) {
	t.Helper()

	createPBSCredentialProfile(t, sut, credentialID, tokenID, "secret-"+collectorID, baseURL)
	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{
			{
				ID:            collectorID,
				AssetID:       "pbs-server-" + collectorID,
				CollectorType: hubcollector.CollectorTypePBS,
				Enabled:       true,
				Config: map[string]any{
					"base_url":      baseURL,
					"credential_id": credentialID,
					"token_id":      tokenID,
					"skip_verify":   true,
				},
			},
		},
	}
}

func mustNewPBSClient(t *testing.T, baseURL string) *pbs.Client {
	t.Helper()
	allowInsecureTransportForConnectorTests(t)

	client, err := pbs.NewClient(pbs.Config{
		BaseURL:     baseURL,
		TokenID:     "root@pam!labtether",
		TokenSecret: "secret",
		SkipVerify:  true,
		Timeout:     2 * time.Second,
	})
	if err != nil {
		t.Fatalf("new pbs client: %v", err)
	}
	return client
}
