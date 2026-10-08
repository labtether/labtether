package main

import (
	"context"
	"github.com/labtether/labtether/internal/connectors/truenas"
	truenaspkg "github.com/labtether/labtether/internal/hubapi/truenas"
	"strings"
	"testing"
	"time"
)

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
