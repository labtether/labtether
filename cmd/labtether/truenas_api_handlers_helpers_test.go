package main

import (
	"context"
	"errors"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectors/truenas"
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type failingTrueNASQueryLogStore struct {
	persistence.LogStore
	err error
}

func (s *failingTrueNASQueryLogStore) QueryEvents(req logs.QueryRequest) ([]logs.Event, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.LogStore.QueryEvents(req)
}

type failingTrueNASAssetStore struct {
	persistence.AssetStore
	err error
}

func (s *failingTrueNASAssetStore) GetAsset(assetID string) (assets.Asset, bool, error) {
	if s.err != nil {
		return assets.Asset{}, false, s.err
	}
	return s.AssetStore.GetAsset(assetID)
}

func TestHandleTrueNASAssetsAdditionalErrors(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/truenas/assets/asset-only", nil)
	rec := httptest.NewRecorder()
	sut.handleTrueNASAssets(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing action, got %d", rec.Code)
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "unknown truenas asset action")

	req = httptest.NewRequest(http.MethodGet, "/truenas/assets/asset-1/unknown", nil)
	rec = httptest.NewRecorder()
	sut.handleTrueNASAssets(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown action, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/truenas/assets/missing/events", nil)
	rec = httptest.NewRecorder()
	sut.handleTrueNASAssets(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing asset, got %d", rec.Code)
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), errTrueNASAssetNotFound.Error())

	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "docker-host-1",
		Type:    "container-host",
		Name:    "docker-host-1",
		Source:  "docker",
		Status:  "online",
	})
	if err != nil {
		t.Fatalf("failed to upsert non-truenas asset: %v", err)
	}
	req = httptest.NewRequest(http.MethodGet, "/truenas/assets/docker-host-1/events", nil)
	rec = httptest.NewRecorder()
	sut.handleTrueNASAssets(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-truenas asset, got %d", rec.Code)
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), errAssetNotTrueNAS.Error())

	sut.logStore = nil
	seedTrueNASAsset(t, sut, "truenas-host-omeganas", "collector-truenas-1")
	req = httptest.NewRequest(http.MethodGet, "/truenas/assets/truenas-host-omeganas/events", nil)
	rec = httptest.NewRecorder()
	sut.handleTrueNASAssets(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when log store unavailable, got %d", rec.Code)
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")
}

func TestWriteTrueNASResolveErrorDefaultBranch(t *testing.T) {
	rec := httptest.NewRecorder()
	writeTrueNASResolveError(rec, errors.New("upstream failed"))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 for generic resolve error, got %d", rec.Code)
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")
}

func TestResolveTrueNASAssetRuntimeFallbackAndError(t *testing.T) {
	t.Run("preferred collector fallback to first active", func(t *testing.T) {
		server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			if method == "system.info" {
				return map[string]any{"hostname": "OmegaNAS"}, nil
			}
			return []map[string]any{}, nil
		})
		defer server.Close()

		sut := newTestAPIServer(t)
		createTrueNASCredentialProfile(t, sut, "cred-truenas-1", "api-key-1", server.URL)
		configureTrueNASCollectors(t, sut, hubcollector.Collector{
			ID:            "collector-truenas-1",
			AssetID:       "truenas-cluster-1",
			CollectorType: hubcollector.CollectorTypeTrueNAS,
			Enabled:       true,
			Config: map[string]any{
				"base_url":      server.URL,
				"credential_id": "cred-truenas-1",
				"skip_verify":   true,
			},
		})

		_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: "truenas-host-omeganas",
			Type:    "nas",
			Name:    "omeganas",
			Source:  "truenas",
			Status:  "online",
			Metadata: map[string]string{
				"collector_id": "collector-missing",
			},
		})
		if err != nil {
			t.Fatalf("failed to seed truenas asset: %v", err)
		}

		asset, runtime, err := sut.resolveTrueNASAssetRuntime("truenas-host-omeganas")
		if err != nil {
			t.Fatalf("resolveTrueNASAssetRuntime() error = %v", err)
		}
		if asset.ID != "truenas-host-omeganas" {
			t.Fatalf("asset id = %q", asset.ID)
		}
		if runtime == nil || runtime.CollectorID != "collector-truenas-1" {
			t.Fatalf("expected fallback runtime collector-truenas-1, got %+v", runtime)
		}
	})

	t.Run("runtime load error is wrapped", func(t *testing.T) {
		sut := newTestAPIServer(t)
		seedTrueNASAsset(t, sut, "truenas-host-omeganas", "collector-missing")
		if _, _, err := sut.resolveTrueNASAssetRuntime("truenas-host-omeganas"); err == nil || !strings.Contains(err.Error(), "failed to load truenas runtime") {
			t.Fatalf("expected wrapped runtime load error, got %v", err)
		}
	})
}

func TestTrueNASAPIHelperCoverage(t *testing.T) {
	if got := parseTrueNASEventsWindow(""); got != 24*time.Hour {
		t.Fatalf("default events window = %s, want 24h", got)
	}
	if got := parseTrueNASEventsWindow("bad"); got != 24*time.Hour {
		t.Fatalf("invalid events window = %s, want 24h", got)
	}
	if got := parseTrueNASEventsWindow("10m"); got != time.Hour {
		t.Fatalf("short events window clamp = %s, want 1h", got)
	}
	if got := parseTrueNASEventsWindow("800h"); got != 30*24*time.Hour {
		t.Fatalf("long events window clamp = %s, want 30d", got)
	}

	results := latestSmartResultsByDisk([]map[string]any{
		{"disk": "sda", "status": "SUCCESS", "created_at": "2026-02-22T00:00:00Z"},
		{"disk": map[string]any{"name": "sda"}, "status": "FAILED", "created_at": "2026-02-23T00:00:00Z"},
		{"disk": "sdb", "status": "SUCCESS", "end_time": "2026-02-22T10:00:00Z"},
	})
	if got := strings.TrimSpace(collectorAnyString(results["sda"]["status"])); got != "FAILED" {
		t.Fatalf("latestSmartResultsByDisk(sda).status = %q, want FAILED", got)
	}
	if _, ok := results["sdb"]; !ok {
		t.Fatalf("expected sdb result to be retained")
	}

	if got := deriveTrueNASDiskHealthStatus(trueNASDiskHealth{SmartHealth: "FAILED"}); got != "critical" {
		t.Fatalf("deriveTrueNASDiskHealthStatus failed health = %q", got)
	}
	if got := deriveTrueNASDiskHealthStatus(trueNASDiskHealth{TemperatureCelsius: proxmoxpkg.Float64Ptr(55)}); got != "warning" {
		t.Fatalf("deriveTrueNASDiskHealthStatus warning temp = %q", got)
	}
	if got := deriveTrueNASDiskHealthStatus(trueNASDiskHealth{LastTestStatus: "SUCCESS"}); got != "healthy" {
		t.Fatalf("deriveTrueNASDiskHealthStatus healthy = %q", got)
	}
	if got := deriveTrueNASDiskHealthStatus(trueNASDiskHealth{}); got != "unknown" {
		t.Fatalf("deriveTrueNASDiskHealthStatus empty = %q", got)
	}

	if trueNASDiskHealthSeverity("critical") != 4 || trueNASDiskHealthSeverity("warning") != 3 || trueNASDiskHealthSeverity("unknown") != 2 || trueNASDiskHealthSeverity("healthy") != 1 || trueNASDiskHealthSeverity("other") != 0 {
		t.Fatalf("unexpected disk health severities")
	}

	if anyToFloat64(float32(7.5)) != 7.5 || anyToFloat64("3.25") != 3.25 || anyToFloat64("bad") != 0 {
		t.Fatalf("unexpected anyToFloat64 conversions")
	}

	if parsed, ok := parseAnyBoolLoose("enabled"); !ok || !parsed {
		t.Fatalf("expected enabled => true")
	}
	if parsed, ok := parseAnyBoolLoose("off"); !ok || parsed {
		t.Fatalf("expected off => false")
	}
	if _, ok := parseAnyBoolLoose("maybe"); ok {
		t.Fatalf("expected unknown bool string to fail parse")
	}

	if got := normalizeTrueNASFilesystemPath(" datasets/alpha "); got != "/datasets/alpha" {
		t.Fatalf("normalizeTrueNASFilesystemPath = %q", got)
	}
	if got := normalizeTrueNASFilesystemPath("."); got != "/" {
		t.Fatalf("normalizeTrueNASFilesystemPath(.) = %q", got)
	}
	if got := parentTrueNASFilesystemPath("/"); got != "" {
		t.Fatalf("parentTrueNASFilesystemPath(/) = %q, want empty", got)
	}
	if got := parentTrueNASFilesystemPath("/mnt/data"); got != "/mnt" {
		t.Fatalf("parentTrueNASFilesystemPath(/mnt/data) = %q, want /mnt", got)
	}

	if entries, ok := normalizeTrueNASListDirResult([]any{map[string]any{"name": "a"}, "skip"}); !ok || len(entries) != 1 {
		t.Fatalf("normalizeTrueNASListDirResult array decode failed: %#v ok=%v", entries, ok)
	}
	if entries, ok := normalizeTrueNASListDirResult(map[string]any{"entries": []any{map[string]any{"name": "b"}}}); !ok || len(entries) != 1 {
		t.Fatalf("normalizeTrueNASListDirResult map entries decode failed: %#v ok=%v", entries, ok)
	}
	if _, ok := normalizeTrueNASListDirResult("invalid"); ok {
		t.Fatalf("normalizeTrueNASListDirResult should fail for invalid payload")
	}

	entry := mapTrueNASFilesystemEntry(map[string]any{
		"name":       "photos",
		"type":       "DIRECTORY",
		"mode":       "0755",
		"is_symlink": false,
		"mtime":      "2026-02-23T01:02:03Z",
		"size":       int64(0),
	}, "/mnt")
	if !entry.IsDirectory || entry.Name != "photos" || entry.Path != "/mnt/photos" {
		t.Fatalf("unexpected mapped directory entry: %+v", entry)
	}
	if entry.ModifiedAt == "" {
		t.Fatalf("expected mapped modified timestamp")
	}

	if _, ok := parseAnyTimestamp(time.Now()); !ok {
		t.Fatalf("expected parseAnyTimestamp(time.Time) success")
	}
	if _, ok := parseAnyTimestamp(int64(1700000000)); !ok {
		t.Fatalf("expected parseAnyTimestamp(int64) success")
	}
	if _, ok := parseAnyTimestamp(int(1700000000)); !ok {
		t.Fatalf("expected parseAnyTimestamp(int) success")
	}
	if _, ok := parseAnyTimestamp(float64(1700000000)); !ok {
		t.Fatalf("expected parseAnyTimestamp(float64) success")
	}
	if _, ok := parseAnyTimestamp("2026-02-23T01:02:03.123Z"); !ok {
		t.Fatalf("expected parseAnyTimestamp(RFC3339Nano) success")
	}
	if _, ok := parseAnyTimestamp("1700000000"); !ok {
		t.Fatalf("expected parseAnyTimestamp(unix string) success")
	}
	if _, ok := parseAnyTimestamp("bad"); ok {
		t.Fatalf("expected parseAnyTimestamp(bad) to fail")
	}
}

func TestTrueNASAPIHelpersAdditionalBranches(t *testing.T) {
	t.Run("resolve asset branches", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.assetStore = nil
		if _, err := sut.resolveTrueNASAsset("asset"); err == nil {
			t.Fatalf("expected resolveTrueNASAsset to fail when asset store nil")
		}

		sut = newTestAPIServer(t)
		if _, err := sut.resolveTrueNASAsset("   "); err == nil {
			t.Fatalf("expected resolveTrueNASAsset to fail on empty id")
		}

		sut.assetStore = &failingTrueNASAssetStore{AssetStore: sut.assetStore, err: errors.New("asset get failed")}
		if _, err := sut.resolveTrueNASAsset("asset"); err == nil || !strings.Contains(err.Error(), "failed to load asset") {
			t.Fatalf("expected wrapped GetAsset error, got %v", err)
		}
	})

	t.Run("callTrueNASQueryCompat final retry error", func(t *testing.T) {
		server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			return nil, &trueNASRPCError{Code: -32001, Message: "method call error"}
		})
		defer server.Close()

		client := &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second}
		var out []map[string]any
		if err := callTrueNASQueryCompat(context.Background(), client, "disk.query", &out); err == nil || !truenas.IsMethodCallError(err) {
			t.Fatalf("expected final method call error, got %v", err)
		}
	})

	t.Run("latestSmartResultsByDisk skips empty and compares end_time fallback", func(t *testing.T) {
		out := latestSmartResultsByDisk([]map[string]any{
			{"disk": "", "status": "skip"},
			{"disk": "sda", "status": "old", "created_at": "bad-time", "end_time": "2026-02-22T00:00:00Z"},
			{"disk": "sda", "status": "new", "created_at": "bad-time", "end_time": "2026-02-23T00:00:00Z"},
		})
		if got := strings.TrimSpace(collectorAnyString(out["sda"]["status"])); got != "new" {
			t.Fatalf("expected end_time fallback comparison to pick newest, got %q", got)
		}
	})

	t.Run("helper conversions and parser fallbacks", func(t *testing.T) {
		if got := anyToFloat64(int(5)); got != 5 {
			t.Fatalf("anyToFloat64(int) = %v", got)
		}
		if got := anyToFloat64(int64(6)); got != 6 {
			t.Fatalf("anyToFloat64(int64) = %v", got)
		}

		if parsed, ok := parseAnyBoolLoose(true); !ok || !parsed {
			t.Fatalf("parseAnyBoolLoose(bool) failed")
		}
		if parsed, ok := parseAnyBoolLoose(int(1)); !ok || !parsed {
			t.Fatalf("parseAnyBoolLoose(int) failed")
		}
		if parsed, ok := parseAnyBoolLoose(int64(0)); !ok || parsed {
			t.Fatalf("parseAnyBoolLoose(int64) failed")
		}
		if parsed, ok := parseAnyBoolLoose(float64(1)); !ok || !parsed {
			t.Fatalf("parseAnyBoolLoose(float64) failed")
		}
		if parsed, ok := parseAnyBoolLoose("true"); !ok || !parsed {
			t.Fatalf("parseAnyBoolLoose(\"true\") failed")
		}

		if got := normalizeTrueNASFilesystemPath(""); got != "/mnt" {
			t.Fatalf("normalizeTrueNASFilesystemPath(empty) = %q", got)
		}
		if got := normalizeTrueNASFilesystemPath("mnt/data"); got != "/mnt/data" {
			t.Fatalf("normalizeTrueNASFilesystemPath(relative) = %q", got)
		}
		if got := normalizeTrueNASFilesystemPath("///"); got != "/" {
			t.Fatalf("normalizeTrueNASFilesystemPath(root-ish) = %q", got)
		}

		if entries, ok := normalizeTrueNASListDirResult(map[string]any{"data": []any{map[string]any{"name": "x"}}}); !ok || len(entries) != 1 {
			t.Fatalf("normalizeTrueNASListDirResult(data) failed: %#v ok=%v", entries, ok)
		}

		entry := mapTrueNASFilesystemEntry(map[string]any{
			"path":       "",
			"type":       "",
			"is_dir":     true,
			"modified":   "2026-02-23T01:02:03Z",
			"is_symlink": true,
		}, "/mnt")
		if !entry.IsDirectory || entry.Type != "directory" {
			t.Fatalf("expected inferred directory entry, got %+v", entry)
		}
		if entry.Name == "" {
			t.Fatalf("expected inferred entry name")
		}
		if entry.ModifiedAt == "" {
			t.Fatalf("expected modified fallback timestamp")
		}

		linkEntry := mapTrueNASFilesystemEntry(map[string]any{
			"name": "sym",
			"path": "/mnt/sym",
			"type": "symlink",
		}, "/mnt")
		if !linkEntry.IsSymbolic {
			t.Fatalf("expected symlink type to force IsSymbolic")
		}

		if _, ok := parseAnyTimestamp(""); ok {
			t.Fatalf("expected empty timestamp string to fail parse")
		}
		if _, ok := parseAnyTimestamp("2026-02-23T01:02:03Z"); !ok {
			t.Fatalf("expected RFC3339 timestamp to parse")
		}
	})

	t.Run("callTrueNASListDir exhausted method-call retries", func(t *testing.T) {
		server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			return nil, &trueNASRPCError{Code: -32001, Message: "method call error"}
		})
		defer server.Close()

		client := &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second}
		if _, err := callTrueNASListDir(context.Background(), client, "/mnt"); err == nil || !truenas.IsMethodCallError(err) {
			t.Fatalf("expected exhausted retries to return method call error, got %v", err)
		}
	})
}
