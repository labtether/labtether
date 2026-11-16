package collectors_test

import (
	"context"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/hubapi/testutil/hubdeps"
	"github.com/labtether/labtether/internal/hubcollector"
	"testing"
	"time"
)

func TestExecuteTrueNASCollectorAlertIngestionNonFatal(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)
	server := testutil.NewTrueNASRPCServer(t, func(method string, params []any) (any, *testutil.TrueNASRPCError) {
		switch method {
		case "system.info":
			return map[string]any{"hostname": "VaultNAS"}, nil
		case "pool.query":
			return []map[string]any{{"id": 1, "name": "mainpool", "status": "ONLINE", "size": 100, "allocated": 10, "free": 90}}, nil
		case "alert.list":
			return nil, &testutil.TrueNASRPCError{Code: -32000, Message: "alerts unavailable"}
		default:
			return nil, &testutil.TrueNASRPCError{Code: -32601, Message: "Method not found"}
		}
	})
	defer server.Close()

	sut := hubdeps.New(t)
	store := testutil.NewRecordingHubCollectorStore()
	sut.HubCollectorStore = store
	testutil.CreateTrueNASCredentialProfile(t, sut.CredentialStore, sut.SecretsManager, "cred-truenas-alert-nonfatal", "api-key", server.URL)
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
	store.StatusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.CollectorsDeps().ExecuteTrueNASCollector(ctx, collector)
	updated, _, _ := store.GetHubCollector(collector.ID)
	if updated.LastStatus != "ok" {
		t.Fatalf("expected collector success despite alert ingestion failure, got %+v", updated)
	}
}

func TestExecuteTrueNASCollectorUpdatesClusterAssetEvenOnAlertFailure(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)
	server := testutil.NewTrueNASRPCServer(t, func(method string, params []any) (any, *testutil.TrueNASRPCError) {
		switch method {
		case "system.info":
			return map[string]any{"hostname": "VaultNAS"}, nil
		case "pool.query":
			return []map[string]any{{"id": 1, "name": "mainpool", "status": "ONLINE", "size": 100, "allocated": 10, "free": 90}}, nil
		case "alert.list":
			return nil, &testutil.TrueNASRPCError{Code: -32000, Message: "alert list unavailable"}
		default:
			return nil, &testutil.TrueNASRPCError{Code: -32601, Message: "Method not found"}
		}
	})
	defer server.Close()

	sut := hubdeps.New(t)
	store := testutil.NewRecordingHubCollectorStore()
	sut.HubCollectorStore = store
	testutil.CreateTrueNASCredentialProfile(t, sut.CredentialStore, sut.SecretsManager, "cred-truenas-cluster-refresh", "api-key", server.URL)
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
	store.StatusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.CollectorsDeps().ExecuteTrueNASCollector(ctx, collector)

	clusterAsset, exists, err := sut.AssetStore.GetAsset(collector.AssetID)
	if err != nil || !exists {
		t.Fatalf("expected cluster asset refresh, exists=%v err=%v", exists, err)
	}
	if clusterAsset.Type != "connector-cluster" {
		t.Fatalf("unexpected cluster asset type %q", clusterAsset.Type)
	}
}

func TestExecuteTrueNASCollectorAlertListMethodNotFoundNonFatal(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)
	server := testutil.NewTrueNASRPCServer(t, func(method string, params []any) (any, *testutil.TrueNASRPCError) {
		switch method {
		case "system.info":
			return map[string]any{"hostname": "VaultNAS"}, nil
		case "pool.query":
			return []map[string]any{{"id": 1, "name": "mainpool", "status": "ONLINE", "size": 100, "allocated": 10, "free": 90}}, nil
		case "alert.list":
			return nil, &testutil.TrueNASRPCError{Code: -32601, Message: "Method not found"}
		default:
			return nil, &testutil.TrueNASRPCError{Code: -32601, Message: "Method not found"}
		}
	})
	defer server.Close()

	sut := hubdeps.New(t)
	store := testutil.NewRecordingHubCollectorStore()
	sut.HubCollectorStore = store
	testutil.CreateTrueNASCredentialProfile(t, sut.CredentialStore, sut.SecretsManager, "cred-truenas-alert-method-not-found", "api-key", server.URL)
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
	store.StatusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.CollectorsDeps().ExecuteTrueNASCollector(ctx, collector)
	updated, _, _ := store.GetHubCollector(collector.ID)
	if updated.LastStatus != "ok" {
		t.Fatalf("expected collector success when alert.list missing, got %+v", updated)
	}
}

func TestExecuteTrueNASCollectorConfigTimeoutParsing(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)
	server := testutil.NewTrueNASRPCServer(t, func(method string, params []any) (any, *testutil.TrueNASRPCError) {
		switch method {
		case "system.info":
			return map[string]any{"hostname": "VaultNAS"}, nil
		case "pool.query":
			return []map[string]any{{"id": 1, "name": "mainpool", "status": "ONLINE", "size": 100, "allocated": 10, "free": 90}}, nil
		case "alert.list":
			return []map[string]any{}, nil
		default:
			return nil, &testutil.TrueNASRPCError{Code: -32601, Message: "Method not found"}
		}
	})
	defer server.Close()

	sut := hubdeps.New(t)
	store := testutil.NewRecordingHubCollectorStore()
	sut.HubCollectorStore = store
	testutil.CreateTrueNASCredentialProfile(t, sut.CredentialStore, sut.SecretsManager, "cred-truenas-timeout", "api-key", server.URL)
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
	store.StatusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.CollectorsDeps().ExecuteTrueNASCollector(ctx, collector)
	updated, _, _ := store.GetHubCollector(collector.ID)
	if updated.LastStatus != "ok" {
		t.Fatalf("expected collector success with timeout config, got %+v", updated)
	}
}

func TestExecuteTrueNASCollectorSuccessWithSkipVerifyDefaultFalse(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)
	server := testutil.NewTrueNASRPCServer(t, func(method string, params []any) (any, *testutil.TrueNASRPCError) {
		switch method {
		case "system.info":
			return map[string]any{"hostname": "VaultNAS"}, nil
		case "pool.query":
			return []map[string]any{{"id": 1, "name": "mainpool", "status": "ONLINE", "size": 100, "allocated": 10, "free": 90}}, nil
		case "alert.list":
			return []map[string]any{}, nil
		default:
			return nil, &testutil.TrueNASRPCError{Code: -32601, Message: "Method not found"}
		}
	})
	defer server.Close()

	sut := hubdeps.New(t)
	store := testutil.NewRecordingHubCollectorStore()
	sut.HubCollectorStore = store
	testutil.CreateTrueNASCredentialProfile(t, sut.CredentialStore, sut.SecretsManager, "cred-truenas-skipverify-default", "api-key", server.URL)
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
	store.StatusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.CollectorsDeps().ExecuteTrueNASCollector(ctx, collector)
	updated, _, _ := store.GetHubCollector(collector.ID)
	if updated.LastStatus != "ok" {
		t.Fatalf("expected collector success with default skip_verify=false, got %+v", updated)
	}
}

func TestExecuteTrueNASCollectorUsesCollectorConfigTimeoutInt(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)
	server := testutil.NewTrueNASRPCServer(t, func(method string, params []any) (any, *testutil.TrueNASRPCError) {
		switch method {
		case "system.info":
			return map[string]any{"hostname": "VaultNAS"}, nil
		case "pool.query":
			return []map[string]any{{"id": 1, "name": "mainpool", "status": "ONLINE", "size": 100, "allocated": 10, "free": 90}}, nil
		case "alert.list":
			return []map[string]any{}, nil
		default:
			return nil, &testutil.TrueNASRPCError{Code: -32601, Message: "Method not found"}
		}
	})
	defer server.Close()

	sut := hubdeps.New(t)
	store := testutil.NewRecordingHubCollectorStore()
	sut.HubCollectorStore = store
	testutil.CreateTrueNASCredentialProfile(t, sut.CredentialStore, sut.SecretsManager, "cred-truenas-timeout-int", "api-key", server.URL)
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
	store.StatusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.CollectorsDeps().ExecuteTrueNASCollector(ctx, collector)
	updated, _, _ := store.GetHubCollector(collector.ID)
	if updated.LastStatus != "ok" {
		t.Fatalf("expected collector success with int timeout config, got %+v", updated)
	}
}

func TestExecuteTrueNASCollectorSkipVerifyFalsePath(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)
	server := testutil.NewTrueNASRPCServer(t, func(method string, params []any) (any, *testutil.TrueNASRPCError) {
		switch method {
		case "system.info":
			return map[string]any{"hostname": "VaultNAS"}, nil
		case "pool.query":
			return []map[string]any{{"id": 1, "name": "mainpool", "status": "ONLINE", "size": 100, "allocated": 10, "free": 90}}, nil
		case "alert.list":
			return []map[string]any{}, nil
		default:
			return nil, &testutil.TrueNASRPCError{Code: -32601, Message: "Method not found"}
		}
	})
	defer server.Close()

	sut := hubdeps.New(t)
	store := testutil.NewRecordingHubCollectorStore()
	sut.HubCollectorStore = store
	testutil.CreateTrueNASCredentialProfile(t, sut.CredentialStore, sut.SecretsManager, "cred-truenas-skipverify-false", "api-key", server.URL)
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
	store.StatusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.CollectorsDeps().ExecuteTrueNASCollector(ctx, collector)
	updated, _, _ := store.GetHubCollector(collector.ID)
	if updated.LastStatus != "ok" {
		t.Fatalf("expected success with skip_verify=false, got %+v", updated)
	}
}

func TestExecuteTrueNASCollectorStatusOKWithAlertMethodNotFound(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)
	server := testutil.NewTrueNASRPCServer(t, func(method string, params []any) (any, *testutil.TrueNASRPCError) {
		switch method {
		case "system.info":
			return map[string]any{"hostname": "VaultNAS"}, nil
		case "pool.query":
			return []map[string]any{{"id": 1, "name": "mainpool", "status": "ONLINE", "size": 100, "allocated": 10, "free": 90}}, nil
		case "alert.list":
			return nil, &testutil.TrueNASRPCError{Code: -32601, Message: "Method not found"}
		default:
			return nil, &testutil.TrueNASRPCError{Code: -32601, Message: "Method not found"}
		}
	})
	defer server.Close()

	sut := hubdeps.New(t)
	store := testutil.NewRecordingHubCollectorStore()
	sut.HubCollectorStore = store
	testutil.CreateTrueNASCredentialProfile(t, sut.CredentialStore, sut.SecretsManager, "cred-truenas-method-not-found-ok", "api-key", server.URL)
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
	store.StatusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.CollectorsDeps().ExecuteTrueNASCollector(ctx, collector)
	updated, _, _ := store.GetHubCollector(collector.ID)
	if updated.LastStatus != "ok" {
		t.Fatalf("expected status ok on method-not-found alert ingestion, got %+v", updated)
	}
}
