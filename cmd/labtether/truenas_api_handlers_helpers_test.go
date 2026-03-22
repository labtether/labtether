package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectors/truenas"
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
	truenaspkg "github.com/labtether/labtether/internal/hubapi/truenas"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
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
	assertErrorBodyContains(t, rec.Body.Bytes(), "log store unavailable")
}

func TestWriteTrueNASResolveErrorDefaultBranch(t *testing.T) {
	rec := httptest.NewRecorder()
	writeTrueNASResolveError(rec, errors.New("upstream failed"))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 for generic resolve error, got %d", rec.Code)
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "upstream failed")
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

func TestCallTrueNASQueryCompatAndListDirBranches(t *testing.T) {
	t.Run("query compat retries method call error", func(t *testing.T) {
		attempts := 0
		server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			attempts++
			if method != "disk.query" {
				return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
			}
			if attempts == 1 {
				return nil, &trueNASRPCError{Code: -32001, Message: "Method call error"}
			}
			return []map[string]any{{"name": "sda"}}, nil
		})
		defer server.Close()

		client := &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second}
		var disks []map[string]any
		if err := callTrueNASQueryCompat(context.Background(), client, "disk.query", &disks); err != nil {
			t.Fatalf("callTrueNASQueryCompat() error = %v", err)
		}
		if attempts != 2 || len(disks) != 1 {
			t.Fatalf("unexpected compat retry result: attempts=%d disks=%#v", attempts, disks)
		}
	})

	t.Run("query compat non-method error returns immediately", func(t *testing.T) {
		server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			return nil, &trueNASRPCError{Code: -32000, Message: "permission denied"}
		})
		defer server.Close()

		client := &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second}
		var out []map[string]any
		if err := callTrueNASQueryCompat(context.Background(), client, "disk.query", &out); err == nil || !strings.Contains(err.Error(), "permission denied") {
			t.Fatalf("expected permission denied error, got %v", err)
		}
	})

	t.Run("listdir retries method call error and succeeds", func(t *testing.T) {
		attempts := 0
		server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			if method != "filesystem.listdir" {
				return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
			}
			attempts++
			if attempts < 3 {
				return nil, &trueNASRPCError{Code: -32001, Message: "Method call error"}
			}
			return map[string]any{
				"entries": []any{
					map[string]any{"name": "photos", "path": "/mnt/photos", "type": "DIRECTORY"},
				},
			}, nil
		})
		defer server.Close()

		client := &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second}
		entries, err := callTrueNASListDir(context.Background(), client, "/mnt")
		if err != nil {
			t.Fatalf("callTrueNASListDir() error = %v", err)
		}
		if attempts != 3 || len(entries) != 1 {
			t.Fatalf("unexpected listdir retry result: attempts=%d entries=%#v", attempts, entries)
		}
	})

	t.Run("listdir unexpected payload", func(t *testing.T) {
		server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			return "invalid", nil
		})
		defer server.Close()

		client := &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second}
		if _, err := callTrueNASListDir(context.Background(), client, "/mnt"); err == nil || !strings.Contains(err.Error(), "unexpected payload") {
			t.Fatalf("expected unexpected payload error, got %v", err)
		}
	})

	t.Run("listdir non-method error short-circuits", func(t *testing.T) {
		server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			return nil, &trueNASRPCError{Code: -32000, Message: "permission denied"}
		})
		defer server.Close()

		client := &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second}
		if _, err := callTrueNASListDir(context.Background(), client, "/mnt"); err == nil || !strings.Contains(err.Error(), "permission denied") {
			t.Fatalf("expected permission denied error, got %v", err)
		}
	})
}

func TestTrueNASReadRetryHelpers(t *testing.T) {
	previousBackoff := truenaspkg.TrueNASMethodCallRetryBackoff
	truenaspkg.TrueNASMethodCallRetryBackoff = 0
	t.Cleanup(func() {
		truenaspkg.TrueNASMethodCallRetryBackoff = previousBackoff
	})

	t.Run("query with retries recovers after exhausted compat pass", func(t *testing.T) {
		attempts := 0
		server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			if method != "disk.query" {
				return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
			}
			attempts++
			if attempts <= 4 {
				return nil, &trueNASRPCError{Code: -32001, Message: "Method call error"}
			}
			return []map[string]any{{"name": "sda"}}, nil
		})
		defer server.Close()

		client := &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second}
		var out []map[string]any
		if err := callTrueNASQueryWithRetries(context.Background(), client, "disk.query", &out); err != nil {
			t.Fatalf("callTrueNASQueryWithRetries() error = %v", err)
		}
		if attempts != 5 || len(out) != 1 {
			t.Fatalf("unexpected query retry result: attempts=%d out=%#v", attempts, out)
		}
	})

	t.Run("query with retries returns final method-call error", func(t *testing.T) {
		attempts := 0
		server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			if method != "disk.query" {
				return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
			}
			attempts++
			return nil, &trueNASRPCError{Code: -32001, Message: "Method call error"}
		})
		defer server.Close()

		client := &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second}
		var out []map[string]any
		err := callTrueNASQueryWithRetries(context.Background(), client, "disk.query", &out)
		if err == nil || !truenas.IsMethodCallError(err) {
			t.Fatalf("expected method call error after retries, got %v", err)
		}
		if attempts != truenaspkg.TrueNASMethodCallRetryAttempts*3 {
			t.Fatalf("attempts = %d, want %d", attempts, truenaspkg.TrueNASMethodCallRetryAttempts*3)
		}
	})

	t.Run("method with retries recovers on transient method-call error", func(t *testing.T) {
		attempts := 0
		server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			if method != "disk.temperatures" {
				return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
			}
			attempts++
			if attempts < 3 {
				return nil, &trueNASRPCError{Code: -32001, Message: "Method call error"}
			}
			return map[string]any{"sda": 40.0}, nil
		})
		defer server.Close()

		client := &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second}
		temps := map[string]any{}
		if err := callTrueNASMethodWithRetries(context.Background(), client, "disk.temperatures", nil, &temps); err != nil {
			t.Fatalf("callTrueNASMethodWithRetries() error = %v", err)
		}
		if attempts != 3 || len(temps) != 1 {
			t.Fatalf("unexpected method retry result: attempts=%d temps=%#v", attempts, temps)
		}
	})

	t.Run("listdir with retries recovers after exhausted parameter shapes", func(t *testing.T) {
		attempts := 0
		server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			if method != "filesystem.listdir" {
				return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
			}
			attempts++
			if attempts <= 4 {
				return nil, &trueNASRPCError{Code: -32001, Message: "Method call error"}
			}
			return []map[string]any{{"name": "photos", "path": "/mnt/photos", "type": "DIRECTORY"}}, nil
		})
		defer server.Close()

		client := &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second}
		entries, err := callTrueNASListDirWithRetries(context.Background(), client, "/mnt")
		if err != nil {
			t.Fatalf("callTrueNASListDirWithRetries() error = %v", err)
		}
		if attempts != 5 || len(entries) != 1 {
			t.Fatalf("unexpected listdir retry result: attempts=%d entries=%#v", attempts, entries)
		}
	})
}

func TestTrueNASReadRetryAndCacheHelperBranches(t *testing.T) {
	t.Run("method/query/listdir retries stop on non-method errors", func(t *testing.T) {
		previousBackoff := truenaspkg.TrueNASMethodCallRetryBackoff
		truenaspkg.TrueNASMethodCallRetryBackoff = 0
		t.Cleanup(func() {
			truenaspkg.TrueNASMethodCallRetryBackoff = previousBackoff
		})

		server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			return nil, &trueNASRPCError{Code: -32000, Message: "permission denied"}
		})
		defer server.Close()

		client := &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second}

		var temps map[string]any
		if err := callTrueNASMethodWithRetries(context.Background(), client, "disk.temperatures", nil, &temps); err == nil || !strings.Contains(err.Error(), "permission denied") {
			t.Fatalf("expected non-method error from callTrueNASMethodWithRetries, got %v", err)
		}

		var disks []map[string]any
		if err := callTrueNASQueryWithRetries(context.Background(), client, "disk.query", &disks); err == nil || !strings.Contains(err.Error(), "permission denied") {
			t.Fatalf("expected non-method error from callTrueNASQueryWithRetries, got %v", err)
		}

		if _, err := callTrueNASListDirWithRetries(context.Background(), client, "/mnt"); err == nil || !strings.Contains(err.Error(), "permission denied") {
			t.Fatalf("expected non-method error from callTrueNASListDirWithRetries, got %v", err)
		}
	})

	t.Run("method/listdir retries return method-call errors after exhaustion", func(t *testing.T) {
		previousBackoff := truenaspkg.TrueNASMethodCallRetryBackoff
		truenaspkg.TrueNASMethodCallRetryBackoff = 0
		t.Cleanup(func() {
			truenaspkg.TrueNASMethodCallRetryBackoff = previousBackoff
		})

		server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			return nil, &trueNASRPCError{Code: -32001, Message: "Method call error"}
		})
		defer server.Close()

		client := &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second}

		var temps map[string]any
		if err := callTrueNASMethodWithRetries(context.Background(), client, "disk.temperatures", nil, &temps); err == nil || !truenas.IsMethodCallError(err) {
			t.Fatalf("expected method-call error from callTrueNASMethodWithRetries, got %v", err)
		}

		if _, err := callTrueNASListDirWithRetries(context.Background(), client, "/mnt"); err == nil || !truenas.IsMethodCallError(err) {
			t.Fatalf("expected method-call error from callTrueNASListDirWithRetries, got %v", err)
		}
	})

	t.Run("warning and wait helpers cover empty and cancelled branches", func(t *testing.T) {
		if got := staleTrueNASReadWarning("", ""); !strings.Contains(got, "showing cached data") {
			t.Fatalf("expected stale warning fallback, got %q", got)
		}

		warnings := appendTrueNASWarning(nil, "  ")
		if len(warnings) != 0 {
			t.Fatalf("expected blank warning to be ignored, got %#v", warnings)
		}
		warnings = appendTrueNASWarning(warnings, "disk warning")
		warnings = appendTrueNASWarning(warnings, "DISK WARNING")
		if len(warnings) != 1 {
			t.Fatalf("expected case-insensitive warning dedupe, got %#v", warnings)
		}

		if waitForTrueNASMethodRetry(context.Background(), truenaspkg.TrueNASMethodCallRetryAttempts-1) {
			t.Fatalf("expected retry wait false on final attempt")
		}

		previousBackoff := truenaspkg.TrueNASMethodCallRetryBackoff
		truenaspkg.TrueNASMethodCallRetryBackoff = 0
		if !waitForTrueNASMethodRetry(context.Background(), 0) {
			t.Fatalf("expected retry wait true when backoff disabled")
		}
		truenaspkg.TrueNASMethodCallRetryBackoff = previousBackoff

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if waitForTrueNASMethodRetry(ctx, 0) {
			t.Fatalf("expected retry wait false when context already cancelled")
		}
	})

	t.Run("smart cache helpers cover empty keys and collector fallback", func(t *testing.T) {
		if key := trueNASSmartAssetCacheKey("   "); key != "" {
			t.Fatalf("expected empty smart asset key for blank input, got %q", key)
		}
		if key := trueNASSmartCollectorCacheKey(""); key != "" {
			t.Fatalf("expected empty smart collector key for blank input, got %q", key)
		}

		sut := &apiServer{}
		if _, ok := sut.getCachedTrueNASSMART("", ""); ok {
			t.Fatalf("expected no smart cache hit for empty keys")
		}
		if _, ok := sut.getCachedTrueNASSMART("asset-1", "collector-1"); ok {
			t.Fatalf("expected no smart cache hit for nil cache map")
		}

		sut.setCachedTrueNASSMART("", "", trueNASAssetSMARTResponse{AssetID: "ignored"})
		if sut.ensureTruenasDeps().TruenasSmartCache != nil {
			t.Fatalf("expected blank smart cache set to no-op")
		}

		sut.setCachedTrueNASSMART("asset-1", "collector-1", trueNASAssetSMARTResponse{AssetID: "asset-1"})
		if _, ok := sut.getCachedTrueNASSMART("asset-1", ""); !ok {
			t.Fatalf("expected smart cache hit by asset key")
		}
		if _, ok := sut.getCachedTrueNASSMART("missing-asset", ""); ok {
			t.Fatalf("expected no smart cache hit for missing asset without collector fallback")
		}
		if _, ok := sut.getCachedTrueNASSMART("missing-asset", "collector-1"); !ok {
			t.Fatalf("expected smart cache hit by collector fallback")
		}
	})

	t.Run("filesystem cache helpers cover empty keys and collector fallback", func(t *testing.T) {
		if key := trueNASFilesystemCacheKey("", "collector-1", "/mnt"); key != "" {
			t.Fatalf("expected empty filesystem key for blank scope, got %q", key)
		}
		if key := trueNASFilesystemCacheKey("collector", "", "/mnt"); key != "" {
			t.Fatalf("expected empty filesystem key for blank id, got %q", key)
		}

		sut := &apiServer{}
		if _, ok := sut.getCachedTrueNASFilesystem("", "", "/mnt"); ok {
			t.Fatalf("expected no filesystem cache hit for empty keys")
		}
		if _, ok := sut.getCachedTrueNASFilesystem("asset-1", "collector-1", "/mnt"); ok {
			t.Fatalf("expected no filesystem cache hit for nil cache map")
		}

		sut.setCachedTrueNASFilesystem("", "", "/mnt", trueNASFilesystemResponse{AssetID: "ignored"})
		if sut.ensureTruenasDeps().TruenasFSCache != nil {
			t.Fatalf("expected blank filesystem cache set to no-op")
		}

		sut.setCachedTrueNASFilesystem("asset-1", "collector-1", "/mnt", trueNASFilesystemResponse{AssetID: "asset-1"})
		if _, ok := sut.getCachedTrueNASFilesystem("asset-1", "", "/mnt"); !ok {
			t.Fatalf("expected filesystem cache hit by asset key")
		}
		if _, ok := sut.getCachedTrueNASFilesystem("missing-asset", "collector-1", "/mnt"); !ok {
			t.Fatalf("expected filesystem cache hit by collector fallback")
		}
	})
}

func TestTrueNASAPIHandlersAdditionalBranchCoverage(t *testing.T) {
	t.Run("events path clamps limit and handles query error", func(t *testing.T) {
		sut := newTestAPIServer(t)
		seedTrueNASAsset(t, sut, "truenas-host-omeganas", "collector-truenas-1")

		req := httptest.NewRequest(http.MethodGet, "/truenas/assets/truenas-host-omeganas/events?limit=9999", nil)
		rec := httptest.NewRecorder()
		sut.handleTrueNASAssets(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for clamped events query, got %d", rec.Code)
		}

		sut.logStore = &failingTrueNASQueryLogStore{
			LogStore: sut.logStore,
			err:      errors.New("query failed"),
		}
		req = httptest.NewRequest(http.MethodGet, "/truenas/assets/truenas-host-omeganas/events", nil)
		rec = httptest.NewRecorder()
		sut.handleTrueNASAssets(rec, req)
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected 502 for log query failure, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "failed to load truenas events")
	})

	t.Run("smart handler error and warning paths", func(t *testing.T) {
		server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			switch method {
			case "disk.query":
				return []map[string]any{
					{"name": "", "status": "ONLINE"},
					{"name": "sda", "togglesmart": "yes", "status": "DEGRADED", "size": 1000},
					{"name": "sdb", "smart_enabled": false, "size": 2000},
					{"name": "sdc", "smart_enabled": true, "size": 3000},
					{"name": "sdd", "smart_enabled": true, "size": 4000},
				}, nil
			case "disk.temperatures":
				return nil, &trueNASRPCError{Code: -32000, Message: "temps unavailable"}
			case "smart.test.results":
				return []map[string]any{
					{"disk": "sdc", "status": "WARN", "type": "SHORT", "created_at": "bad-time", "end_time": "2026-02-23T01:00:00Z"},
					{"disk": "sdd", "status": "FAILED", "type": "LONG"},
				}, nil
			default:
				return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
			}
		})
		defer server.Close()

		sut := newTestAPIServer(t)
		createTrueNASCredentialProfile(t, sut, "cred-truenas-smart-branches", "api-key", server.URL)
		configureTrueNASCollectors(t, sut, hubcollector.Collector{
			ID:            "collector-truenas-smart-branches",
			AssetID:       "truenas-cluster-smart-branches",
			CollectorType: hubcollector.CollectorTypeTrueNAS,
			Enabled:       true,
			Config: map[string]any{
				"base_url":      server.URL,
				"credential_id": "cred-truenas-smart-branches",
			},
		})
		seedTrueNASAsset(t, sut, "truenas-host-omeganas", "collector-truenas-smart-branches")

		req := httptest.NewRequest(http.MethodGet, "/truenas/assets/truenas-host-omeganas/smart", nil)
		rec := httptest.NewRecorder()
		sut.handleTrueNASAssets(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 smart response, got %d body=%s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !strings.Contains(body, "disk temperatures unavailable") {
			t.Fatalf("expected disk temperature warning, got %s", body)
		}
		if !strings.Contains(body, `"warning"`) || !strings.Contains(body, `"critical"`) || !strings.Contains(body, `"unknown"`) {
			t.Fatalf("expected summary statuses to include warning/critical/unknown, got %s", body)
		}
	})

	t.Run("smart disk query failure", func(t *testing.T) {
		server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			if method == "disk.query" {
				return nil, &trueNASRPCError{Code: -32000, Message: "disk query denied"}
			}
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		})
		defer server.Close()

		sut := newTestAPIServer(t)
		createTrueNASCredentialProfile(t, sut, "cred-truenas-smart-error", "api-key", server.URL)
		configureTrueNASCollectors(t, sut, hubcollector.Collector{
			ID:            "collector-truenas-smart-error",
			AssetID:       "truenas-cluster-smart-error",
			CollectorType: hubcollector.CollectorTypeTrueNAS,
			Enabled:       true,
			Config: map[string]any{
				"base_url":      server.URL,
				"credential_id": "cred-truenas-smart-error",
			},
		})
		seedTrueNASAsset(t, sut, "truenas-host-omeganas", "collector-truenas-smart-error")

		req := httptest.NewRequest(http.MethodGet, "/truenas/assets/truenas-host-omeganas/smart", nil)
		rec := httptest.NewRecorder()
		sut.handleTrueNASAssets(rec, req)
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected 502 for smart disk query error, got %d", rec.Code)
		}
	})

	t.Run("filesystem handler errors and truncation", func(t *testing.T) {
		server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			if method != "filesystem.listdir" {
				return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
			}
			return []map[string]any{
				{"name": "", "path": "", "type": "", "is_dir": true, "size": 10, "modified": "2026-02-23T01:02:03Z"},
				{"name": ".", "path": "/mnt/.", "type": "FILE"},
				{"name": "link", "path": "/mnt/link", "type": "symlink", "is_symlink": false, "realpath": "/mnt/real"},
				{"name": "zeta", "path": "/mnt/zeta", "type": "FILE"},
				{"name": "alpha", "path": "/mnt/alpha", "type": "FILE"},
			}, nil
		})
		defer server.Close()

		sut := newTestAPIServer(t)
		createTrueNASCredentialProfile(t, sut, "cred-truenas-fs-branches", "api-key", server.URL)
		configureTrueNASCollectors(t, sut, hubcollector.Collector{
			ID:            "collector-truenas-fs-branches",
			AssetID:       "truenas-cluster-fs-branches",
			CollectorType: hubcollector.CollectorTypeTrueNAS,
			Enabled:       true,
			Config: map[string]any{
				"base_url":      server.URL,
				"credential_id": "cred-truenas-fs-branches",
			},
		})
		seedTrueNASAsset(t, sut, "truenas-host-omeganas", "collector-truenas-fs-branches")

		req := httptest.NewRequest(http.MethodGet, "/truenas/assets/truenas-host-omeganas/filesystem?path=.&limit=2", nil)
		rec := httptest.NewRecorder()
		sut.handleTrueNASAssets(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 filesystem response, got %d body=%s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if strings.Count(body, `"path"`) < 2 {
			t.Fatalf("expected filesystem entries in body, got %s", body)
		}

		errServer := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			return nil, &trueNASRPCError{Code: -32000, Message: "listdir denied"}
		})
		defer errServer.Close()
		createTrueNASCredentialProfile(t, sut, "cred-truenas-fs-error", "api-key", errServer.URL)
		configureTrueNASCollectors(t, sut, hubcollector.Collector{
			ID:            "collector-truenas-fs-error",
			AssetID:       "truenas-cluster-fs-error",
			CollectorType: hubcollector.CollectorTypeTrueNAS,
			Enabled:       true,
			Config: map[string]any{
				"base_url":      errServer.URL,
				"credential_id": "cred-truenas-fs-error",
			},
		})
		seedTrueNASAsset(t, sut, "truenas-host-fs-error", "collector-truenas-fs-error")
		req = httptest.NewRequest(http.MethodGet, "/truenas/assets/truenas-host-fs-error/filesystem", nil)
		rec = httptest.NewRecorder()
		sut.handleTrueNASAssets(rec, req)
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected 502 filesystem error, got %d", rec.Code)
		}
	})
}

func TestTrueNASAPIHandlersCacheFallback(t *testing.T) {
	t.Run("smart endpoint serves cached payload on transient rpc failure", func(t *testing.T) {
		failQueries := false
		server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			switch method {
			case "disk.query":
				if failQueries {
					return nil, &trueNASRPCError{Code: -32001, Message: "Method call error"}
				}
				return []map[string]any{{"name": "sda", "model": "test-disk"}}, nil
			case "disk.temperatures":
				return map[string]any{"sda": 40.0}, nil
			case "smart.test.results":
				return []map[string]any{}, nil
			default:
				return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
			}
		})
		defer server.Close()

		sut := newTestAPIServer(t)
		createTrueNASCredentialProfile(t, sut, "cred-truenas-cache-smart", "api-key", server.URL)
		configureTrueNASCollectors(t, sut, hubcollector.Collector{
			ID:            "collector-truenas-cache-smart",
			AssetID:       "truenas-cluster-cache-smart",
			CollectorType: hubcollector.CollectorTypeTrueNAS,
			Enabled:       true,
			Config: map[string]any{
				"base_url":      server.URL,
				"credential_id": "cred-truenas-cache-smart",
			},
		})
		seedTrueNASAsset(t, sut, "truenas-host-cache-smart", "collector-truenas-cache-smart")

		req := httptest.NewRequest(http.MethodGet, "/truenas/assets/truenas-host-cache-smart/smart", nil)
		rec := httptest.NewRecorder()
		sut.handleTrueNASAssets(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected initial smart response 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"name":"sda"`) {
			t.Fatalf("expected initial smart payload to include disk entry, got %s", rec.Body.String())
		}

		failQueries = true
		req = httptest.NewRequest(http.MethodGet, "/truenas/assets/truenas-host-cache-smart/smart", nil)
		rec = httptest.NewRecorder()
		sut.handleTrueNASAssets(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected cached smart response 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "showing cached data") {
			t.Fatalf("expected cached warning in smart payload, got %s", rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"name":"sda"`) {
			t.Fatalf("expected cached smart payload to include disk entry, got %s", rec.Body.String())
		}
	})

	t.Run("filesystem endpoint serves cached payload on transient rpc failure", func(t *testing.T) {
		failListDir := false
		server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			switch method {
			case "filesystem.listdir":
				if failListDir {
					return nil, &trueNASRPCError{Code: -32001, Message: "Method call error"}
				}
				return []map[string]any{
					{"name": "photos", "path": "/mnt/photos", "type": "DIRECTORY", "is_dir": true},
				}, nil
			default:
				return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
			}
		})
		defer server.Close()

		sut := newTestAPIServer(t)
		createTrueNASCredentialProfile(t, sut, "cred-truenas-cache-fs", "api-key", server.URL)
		configureTrueNASCollectors(t, sut, hubcollector.Collector{
			ID:            "collector-truenas-cache-fs",
			AssetID:       "truenas-cluster-cache-fs",
			CollectorType: hubcollector.CollectorTypeTrueNAS,
			Enabled:       true,
			Config: map[string]any{
				"base_url":      server.URL,
				"credential_id": "cred-truenas-cache-fs",
			},
		})
		seedTrueNASAsset(t, sut, "truenas-host-cache-fs", "collector-truenas-cache-fs")

		req := httptest.NewRequest(http.MethodGet, "/truenas/assets/truenas-host-cache-fs/filesystem?path=/mnt", nil)
		rec := httptest.NewRecorder()
		sut.handleTrueNASAssets(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected initial filesystem response 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"name":"photos"`) {
			t.Fatalf("expected initial filesystem payload to include directory entry, got %s", rec.Body.String())
		}

		failListDir = true
		req = httptest.NewRequest(http.MethodGet, "/truenas/assets/truenas-host-cache-fs/filesystem?path=/mnt", nil)
		rec = httptest.NewRecorder()
		sut.handleTrueNASAssets(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected cached filesystem response 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "showing cached data") {
			t.Fatalf("expected cached warning in filesystem payload, got %s", rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"name":"photos"`) {
			t.Fatalf("expected cached filesystem payload to include directory entry, got %s", rec.Body.String())
		}
	})
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

func TestTrueNASAPIHandlersFinalCoverageBranches(t *testing.T) {
	t.Run("asset route rejects empty asset id segment", func(t *testing.T) {
		sut := newTestAPIServer(t)
		req := httptest.NewRequest(http.MethodGet, "/truenas/assets//events", nil)
		rec := httptest.NewRecorder()
		sut.handleTrueNASAssets(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for empty asset id segment, got %d", rec.Code)
		}
	})

	t.Run("smart and filesystem handlers propagate resolve errors", func(t *testing.T) {
		sut := newTestAPIServer(t)

		req := httptest.NewRequest(http.MethodGet, "/truenas/assets/missing/smart", nil)
		rec := httptest.NewRecorder()
		sut.handleTrueNASAssets(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for missing smart asset, got %d", rec.Code)
		}

		req = httptest.NewRequest(http.MethodGet, "/truenas/assets/missing/filesystem", nil)
		rec = httptest.NewRecorder()
		sut.handleTrueNASAssets(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for missing filesystem asset, got %d", rec.Code)
		}
	})

	t.Run("smart history warning and end_time fallback are handled", func(t *testing.T) {
		warningServer := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			switch method {
			case "disk.query":
				return []map[string]any{
					{"name": "sda", "smart_enabled": true, "size": 1000},
				}, nil
			case "disk.temperatures":
				return map[string]any{}, nil
			case "smart.test.results":
				return nil, &trueNASRPCError{Code: -32000, Message: "permission denied"}
			default:
				return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
			}
		})
		defer warningServer.Close()

		sut := newTestAPIServer(t)
		createTrueNASCredentialProfile(t, sut, "cred-truenas-smart-warning", "api-key", warningServer.URL)
		configureTrueNASCollectors(t, sut, hubcollector.Collector{
			ID:            "collector-truenas-smart-warning",
			AssetID:       "truenas-cluster-smart-warning",
			CollectorType: hubcollector.CollectorTypeTrueNAS,
			Enabled:       true,
			Config: map[string]any{
				"base_url":      warningServer.URL,
				"credential_id": "cred-truenas-smart-warning",
			},
		})
		seedTrueNASAsset(t, sut, "truenas-host-omeganas-warning", "collector-truenas-smart-warning")

		req := httptest.NewRequest(http.MethodGet, "/truenas/assets/truenas-host-omeganas-warning/smart", nil)
		rec := httptest.NewRecorder()
		sut.handleTrueNASAssets(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 smart response, got %d body=%s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "SMART test history unavailable") {
			t.Fatalf("expected smart history warning, got %s", rec.Body.String())
		}

		fallbackServer := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			switch method {
			case "disk.query":
				return []map[string]any{
					{"name": "sda", "smart_enabled": true, "size": 1000},
					{"name": "sdb", "smart_enabled": true, "size": 2000},
				}, nil
			case "disk.temperatures":
				return map[string]any{}, nil
			case "smart.test.results":
				return []map[string]any{
					{"disk": "sdb", "type": "SHORT", "status": "SUCCESS", "created_at": "0001-01-01T00:00:00Z", "end_time": "2026-02-23T03:00:00Z"},
				}, nil
			default:
				return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
			}
		})
		defer fallbackServer.Close()

		createTrueNASCredentialProfile(t, sut, "cred-truenas-smart-fallback", "api-key", fallbackServer.URL)
		configureTrueNASCollectors(t, sut, hubcollector.Collector{
			ID:            "collector-truenas-smart-fallback",
			AssetID:       "truenas-cluster-smart-fallback",
			CollectorType: hubcollector.CollectorTypeTrueNAS,
			Enabled:       true,
			Config: map[string]any{
				"base_url":      fallbackServer.URL,
				"credential_id": "cred-truenas-smart-fallback",
			},
		})
		seedTrueNASAsset(t, sut, "truenas-host-omeganas-fallback", "collector-truenas-smart-fallback")

		req = httptest.NewRequest(http.MethodGet, "/truenas/assets/truenas-host-omeganas-fallback/smart", nil)
		rec = httptest.NewRecorder()
		sut.handleTrueNASAssets(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 smart fallback response, got %d body=%s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !strings.Contains(body, `"unknown":1`) {
			t.Fatalf("expected one unknown disk in summary, got %s", body)
		}
		if !strings.Contains(body, `"last_test_at":"2026-02-23T03:00:00Z"`) {
			t.Fatalf("expected end_time fallback timestamp, got %s", body)
		}
	})

	t.Run("filesystem limit clamp and helper branches", func(t *testing.T) {
		server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			if method == "filesystem.listdir" {
				return []map[string]any{
					{"name": "a", "path": "/mnt/a"},
					{"name": "b", "path": "/mnt/b"},
				}, nil
			}
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		})
		defer server.Close()

		sut := newTestAPIServer(t)
		createTrueNASCredentialProfile(t, sut, "cred-truenas-fs-final", "api-key", server.URL)
		configureTrueNASCollectors(t, sut, hubcollector.Collector{
			ID:            "collector-truenas-fs-final",
			AssetID:       "truenas-cluster-fs-final",
			CollectorType: hubcollector.CollectorTypeTrueNAS,
			Enabled:       true,
			Config: map[string]any{
				"base_url":      server.URL,
				"credential_id": "cred-truenas-fs-final",
			},
		})
		seedTrueNASAsset(t, sut, "truenas-host-fs-final", "collector-truenas-fs-final")

		req := httptest.NewRequest(http.MethodGet, "/truenas/assets/truenas-host-fs-final/filesystem?limit=999999", nil)
		rec := httptest.NewRecorder()
		sut.handleTrueNASAssets(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 filesystem response, got %d body=%s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"entries"`) {
			t.Fatalf("expected filesystem entries payload, got %s", rec.Body.String())
		}

		file := mapTrueNASFilesystemEntry(map[string]any{
			"name":   "plain-file",
			"path":   "/mnt/plain-file",
			"type":   "",
			"is_dir": false,
		}, "/mnt")
		if file.Type != "file" || file.IsDirectory {
			t.Fatalf("expected inferred file entry, got %+v", file)
		}
	})

	t.Run("query compat returns non-method-call retry error", func(t *testing.T) {
		attempts := 0
		server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			if method != "disk.query" {
				return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
			}
			attempts++
			if attempts == 1 {
				return nil, &trueNASRPCError{Code: -32001, Message: "method call error"}
			}
			return nil, &trueNASRPCError{Code: -32000, Message: "permission denied"}
		})
		defer server.Close()

		client := &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second}
		var out []map[string]any
		if err := callTrueNASQueryCompat(context.Background(), client, "disk.query", &out); err == nil || !strings.Contains(err.Error(), "permission denied") {
			t.Fatalf("expected non-method-call retry error, got %v", err)
		}
		if attempts != 2 {
			t.Fatalf("expected two attempts, got %d", attempts)
		}
	})

	t.Run("additional helper branches", func(t *testing.T) {
		if got := deriveTrueNASDiskHealthStatus(trueNASDiskHealth{TemperatureCelsius: proxmoxpkg.Float64Ptr(60)}); got != "critical" {
			t.Fatalf("expected critical status at 60C, got %q", got)
		}
		if _, _, err := newTestAPIServer(t).resolveTrueNASAssetRuntime("missing"); err == nil {
			t.Fatalf("expected resolveTrueNASAssetRuntime to fail for missing asset")
		}
	})
}
