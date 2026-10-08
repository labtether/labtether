package main

import (
	"errors"
	"github.com/labtether/labtether/internal/hubcollector"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
		assertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")
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
