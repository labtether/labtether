package main

import (
	"context"
	"github.com/labtether/labtether/internal/hubcollector"
	"testing"
	"time"
)

func TestExecuteTrueNASCollectorAlertIngestionNonFatal(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		switch method {
		case "system.info":
			return map[string]any{"hostname": "OmegaNAS"}, nil
		case "pool.query":
			return []map[string]any{{"id": 1, "name": "mainpool", "status": "ONLINE", "size": 100, "allocated": 10, "free": 90}}, nil
		case "alert.list":
			return nil, &trueNASRPCError{Code: -32000, Message: "alerts unavailable"}
		default:
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		}
	})
	defer server.Close()

	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store
	createTrueNASCredentialProfile(t, sut, "cred-truenas-alert-nonfatal", "api-key", server.URL)
	collector := hubcollector.Collector{
		ID:            "collector-truenas-alert-nonfatal",
		AssetID:       "truenas-cluster-alert-nonfatal",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "cred-truenas-alert-nonfatal",
		},
	}
	store.statusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.executeTrueNASCollector(ctx, collector)
	updated, _, _ := store.GetHubCollector(collector.ID)
	if updated.LastStatus != "ok" {
		t.Fatalf("expected collector success despite alert ingestion failure, got %+v", updated)
	}
}

func TestExecuteTrueNASCollectorUpdatesClusterAssetEvenOnAlertFailure(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		switch method {
		case "system.info":
			return map[string]any{"hostname": "OmegaNAS"}, nil
		case "pool.query":
			return []map[string]any{{"id": 1, "name": "mainpool", "status": "ONLINE", "size": 100, "allocated": 10, "free": 90}}, nil
		case "alert.list":
			return nil, &trueNASRPCError{Code: -32000, Message: "alert list unavailable"}
		default:
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		}
	})
	defer server.Close()

	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store
	createTrueNASCredentialProfile(t, sut, "cred-truenas-cluster-refresh", "api-key", server.URL)
	collector := hubcollector.Collector{
		ID:            "collector-truenas-cluster-refresh",
		AssetID:       "truenas-cluster-refresh",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "cred-truenas-cluster-refresh",
		},
	}
	store.statusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.executeTrueNASCollector(ctx, collector)

	clusterAsset, exists, err := sut.assetStore.GetAsset(collector.AssetID)
	if err != nil || !exists {
		t.Fatalf("expected cluster asset refresh, exists=%v err=%v", exists, err)
	}
	if clusterAsset.Type != "connector-cluster" {
		t.Fatalf("unexpected cluster asset type %q", clusterAsset.Type)
	}
}

func TestExecuteTrueNASCollectorAlertListMethodNotFoundNonFatal(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		switch method {
		case "system.info":
			return map[string]any{"hostname": "OmegaNAS"}, nil
		case "pool.query":
			return []map[string]any{{"id": 1, "name": "mainpool", "status": "ONLINE", "size": 100, "allocated": 10, "free": 90}}, nil
		case "alert.list":
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		default:
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		}
	})
	defer server.Close()

	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store
	createTrueNASCredentialProfile(t, sut, "cred-truenas-alert-method-not-found", "api-key", server.URL)
	collector := hubcollector.Collector{
		ID:            "collector-truenas-alert-method-not-found",
		AssetID:       "truenas-cluster-alert-method-not-found",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "cred-truenas-alert-method-not-found",
		},
	}
	store.statusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.executeTrueNASCollector(ctx, collector)
	updated, _, _ := store.GetHubCollector(collector.ID)
	if updated.LastStatus != "ok" {
		t.Fatalf("expected collector success when alert.list missing, got %+v", updated)
	}
}

func TestExecuteTrueNASCollectorConfigTimeoutParsing(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		switch method {
		case "system.info":
			return map[string]any{"hostname": "OmegaNAS"}, nil
		case "pool.query":
			return []map[string]any{{"id": 1, "name": "mainpool", "status": "ONLINE", "size": 100, "allocated": 10, "free": 90}}, nil
		case "alert.list":
			return []map[string]any{}, nil
		default:
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		}
	})
	defer server.Close()

	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store
	createTrueNASCredentialProfile(t, sut, "cred-truenas-timeout", "api-key", server.URL)
	collector := hubcollector.Collector{
		ID:            "collector-truenas-timeout",
		AssetID:       "truenas-cluster-timeout",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "cred-truenas-timeout",
			"timeout":       "5s",
		},
	}
	store.statusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.executeTrueNASCollector(ctx, collector)
	updated, _, _ := store.GetHubCollector(collector.ID)
	if updated.LastStatus != "ok" {
		t.Fatalf("expected collector success with timeout config, got %+v", updated)
	}
}

func TestExecuteTrueNASCollectorSuccessWithSkipVerifyDefaultFalse(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		switch method {
		case "system.info":
			return map[string]any{"hostname": "OmegaNAS"}, nil
		case "pool.query":
			return []map[string]any{{"id": 1, "name": "mainpool", "status": "ONLINE", "size": 100, "allocated": 10, "free": 90}}, nil
		case "alert.list":
			return []map[string]any{}, nil
		default:
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		}
	})
	defer server.Close()

	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store
	createTrueNASCredentialProfile(t, sut, "cred-truenas-skipverify-default", "api-key", server.URL)
	collector := hubcollector.Collector{
		ID:            "collector-truenas-skipverify-default",
		AssetID:       "truenas-cluster-skipverify-default",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "cred-truenas-skipverify-default",
		},
	}
	store.statusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.executeTrueNASCollector(ctx, collector)
	updated, _, _ := store.GetHubCollector(collector.ID)
	if updated.LastStatus != "ok" {
		t.Fatalf("expected collector success with default skip_verify=false, got %+v", updated)
	}
}

func TestExecuteTrueNASCollectorUsesCollectorConfigTimeoutInt(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		switch method {
		case "system.info":
			return map[string]any{"hostname": "OmegaNAS"}, nil
		case "pool.query":
			return []map[string]any{{"id": 1, "name": "mainpool", "status": "ONLINE", "size": 100, "allocated": 10, "free": 90}}, nil
		case "alert.list":
			return []map[string]any{}, nil
		default:
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		}
	})
	defer server.Close()

	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store
	createTrueNASCredentialProfile(t, sut, "cred-truenas-timeout-int", "api-key", server.URL)
	collector := hubcollector.Collector{
		ID:            "collector-truenas-timeout-int",
		AssetID:       "truenas-cluster-timeout-int",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "cred-truenas-timeout-int",
			"timeout":       5,
		},
	}
	store.statusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.executeTrueNASCollector(ctx, collector)
	updated, _, _ := store.GetHubCollector(collector.ID)
	if updated.LastStatus != "ok" {
		t.Fatalf("expected collector success with int timeout config, got %+v", updated)
	}
}

func TestExecuteTrueNASCollectorSkipVerifyFalsePath(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		switch method {
		case "system.info":
			return map[string]any{"hostname": "OmegaNAS"}, nil
		case "pool.query":
			return []map[string]any{{"id": 1, "name": "mainpool", "status": "ONLINE", "size": 100, "allocated": 10, "free": 90}}, nil
		case "alert.list":
			return []map[string]any{}, nil
		default:
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		}
	})
	defer server.Close()

	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store
	createTrueNASCredentialProfile(t, sut, "cred-truenas-skipverify-false", "api-key", server.URL)
	collector := hubcollector.Collector{
		ID:            "collector-truenas-skipverify-false",
		AssetID:       "truenas-cluster-skipverify-false",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "cred-truenas-skipverify-false",
			"skip_verify":   false,
		},
	}
	store.statusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.executeTrueNASCollector(ctx, collector)
	updated, _, _ := store.GetHubCollector(collector.ID)
	if updated.LastStatus != "ok" {
		t.Fatalf("expected success with skip_verify=false, got %+v", updated)
	}
}

func TestExecuteTrueNASCollectorStatusOKWithAlertMethodNotFound(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		switch method {
		case "system.info":
			return map[string]any{"hostname": "OmegaNAS"}, nil
		case "pool.query":
			return []map[string]any{{"id": 1, "name": "mainpool", "status": "ONLINE", "size": 100, "allocated": 10, "free": 90}}, nil
		case "alert.list":
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		default:
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		}
	})
	defer server.Close()

	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store
	createTrueNASCredentialProfile(t, sut, "cred-truenas-method-not-found-ok", "api-key", server.URL)
	collector := hubcollector.Collector{
		ID:            "collector-truenas-method-not-found-ok",
		AssetID:       "truenas-cluster-method-not-found-ok",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "cred-truenas-method-not-found-ok",
		},
	}
	store.statusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.executeTrueNASCollector(ctx, collector)
	updated, _, _ := store.GetHubCollector(collector.ID)
	if updated.LastStatus != "ok" {
		t.Fatalf("expected status ok on method-not-found alert ingestion, got %+v", updated)
	}
}
