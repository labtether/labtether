package main

import (
	"context"
	"github.com/labtether/labtether/internal/connectors/truenas"
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
	"github.com/labtether/labtether/internal/hubcollector"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

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
