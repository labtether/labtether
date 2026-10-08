package main

import (
	"context"
	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/logs"
	"strings"
	"testing"
	"time"
)

func TestExecuteTrueNASCollectorNoAssetsAndSuccess(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	t.Run("no assets discovered returns partial status", func(t *testing.T) {
		server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			switch method {
			case "system.info":
				return nil, &trueNASRPCError{Code: -32000, Message: "system unavailable"}
			case "pool.query":
				return []map[string]any{}, nil
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
		createTrueNASCredentialProfile(t, sut, "cred-truenas-empty", "api-key-empty", server.URL)

		collector := hubcollector.Collector{
			ID:            "collector-truenas-empty",
			AssetID:       "truenas-cluster-empty",
			CollectorType: hubcollector.CollectorTypeTrueNAS,
			Enabled:       true,
			Config: map[string]any{
				"base_url":      server.URL,
				"credential_id": "cred-truenas-empty",
				"skip_verify":   true,
			},
		}
		store.statusByID[collector.ID] = collector

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		sut.executeTrueNASCollector(ctx, collector)

		updated, _, _ := store.GetHubCollector(collector.ID)
		if updated.LastStatus != "partial" || !strings.Contains(updated.LastError, "no assets discovered") {
			t.Fatalf("expected no-assets partial status, got %+v", updated)
		}
	})

	t.Run("success updates assets and status", func(t *testing.T) {
		server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			switch method {
			case "system.info":
				return map[string]any{"hostname": "OmegaNAS", "version": "25.04.0"}, nil
			case "pool.query":
				return []map[string]any{
					{"id": 1, "name": "mainpool", "status": "ONLINE", "healthy": true, "size": 1000, "allocated": 250, "free": 750},
				}, nil
			case "alert.list":
				return []map[string]any{
					{"uuid": "alert-collector-1", "formatted": "Pool healthy", "level": "INFO", "datetime": "2026-02-23T00:00:00Z"},
				}, nil
			default:
				return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
			}
		})
		defer server.Close()

		sut := newTestAPIServer(t)
		store := newRecordingHubCollectorStore()
		sut.hubCollectorStore = store
		createTrueNASCredentialProfile(t, sut, "cred-truenas-success", "api-key-success", server.URL)

		collector := hubcollector.Collector{
			ID:            "collector-truenas-success",
			AssetID:       "truenas-cluster-success",
			CollectorType: hubcollector.CollectorTypeTrueNAS,
			Enabled:       true,
			Config: map[string]any{
				"base_url":      server.URL,
				"credential_id": "cred-truenas-success",
				"skip_verify":   true,
			},
		}
		store.statusByID[collector.ID] = collector

		ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
		defer cancel()
		sut.executeTrueNASCollector(ctx, collector)

		updated, ok, err := store.GetHubCollector(collector.ID)
		if err != nil || !ok {
			t.Fatalf("collector status lookup failed: ok=%v err=%v", ok, err)
		}
		if updated.LastStatus != "ok" || updated.LastError != "" {
			t.Fatalf("expected successful collector status, got %+v", updated)
		}

		poolAsset, exists, assetErr := sut.assetStore.GetAsset(assetid.ScopeCollectorAssetID("truenas-storage-pool-mainpool", collector.ID))
		if assetErr != nil || !exists {
			t.Fatalf("expected pool asset to be upserted, exists=%v err=%v", exists, assetErr)
		}
		if poolAsset.Metadata["collector_id"] != collector.ID {
			t.Fatalf("expected collector metadata on pool asset, got %#v", poolAsset.Metadata)
		}

		clusterAsset, exists, assetErr := sut.assetStore.GetAsset(collector.AssetID)
		if assetErr != nil || !exists {
			t.Fatalf("expected cluster heartbeat asset, exists=%v err=%v", exists, assetErr)
		}
		if clusterAsset.Metadata["discovered"] == "" {
			t.Fatalf("expected discovered metadata on cluster asset")
		}
	})
}

func TestExecuteTrueNASCollectorDiscoveryFailure(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		if method == "pool.query" {
			return nil, &trueNASRPCError{Code: -32000, Message: "pool access denied"}
		}
		return map[string]any{}, nil
	})
	defer server.Close()

	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store
	createTrueNASCredentialProfile(t, sut, "cred-truenas-discovery-fail", "api-key", server.URL)

	collector := hubcollector.Collector{
		ID:            "collector-truenas-discovery-fail",
		AssetID:       "truenas-cluster-discovery-fail",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "cred-truenas-discovery-fail",
			"skip_verify":   true,
		},
	}
	store.statusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.executeTrueNASCollector(ctx, collector)

	updated, _, _ := store.GetHubCollector(collector.ID)
	if updated.LastStatus != "error" || !strings.Contains(updated.LastError, "truenas discovery failed") {
		t.Fatalf("expected discovery failure collector status, got %+v", updated)
	}
}

func TestExecuteTrueNASCollectorNoAssetsLog(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		switch method {
		case "system.info":
			return nil, &trueNASRPCError{Code: -32000, Message: "system unavailable"}
		case "pool.query":
			return []map[string]any{}, nil
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
	createTrueNASCredentialProfile(t, sut, "cred-truenas-noassets-log", "api-key", server.URL)

	collector := hubcollector.Collector{
		ID:            "collector-truenas-log-4",
		AssetID:       "truenas-cluster-log-4",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "cred-truenas-noassets-log",
		},
	}
	store.statusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.executeTrueNASCollector(ctx, collector)

	events, err := sut.logStore.QueryEvents(logs.QueryRequest{
		Source: "truenas",
		From:   time.Unix(0, 0).UTC(),
		To:     time.Now().UTC().Add(365 * 24 * time.Hour),
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("QueryEvents() error = %v", err)
	}
	found := false
	for _, event := range events {
		if strings.Contains(event.Message, "collector run partial: no assets discovered from TrueNAS") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected no assets discovered log event")
	}
}

func TestExecuteTrueNASCollectorDiscoveryFailureLog(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		if method == "pool.query" {
			return nil, &trueNASRPCError{Code: -32000, Message: "pool query denied"}
		}
		return map[string]any{}, nil
	})
	defer server.Close()

	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store
	createTrueNASCredentialProfile(t, sut, "cred-truenas-discovery-log", "api-key", server.URL)

	collector := hubcollector.Collector{
		ID:            "collector-truenas-log-5",
		AssetID:       "truenas-cluster-log-5",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "cred-truenas-discovery-log",
		},
	}
	store.statusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.executeTrueNASCollector(ctx, collector)

	events, err := sut.logStore.QueryEvents(logs.QueryRequest{
		Source: "truenas",
		From:   time.Unix(0, 0).UTC(),
		To:     time.Now().UTC().Add(365 * 24 * time.Hour),
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("QueryEvents() error = %v", err)
	}
	found := false
	for _, event := range events {
		if strings.Contains(event.Message, "truenas discovery failed") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected discovery failed log event")
	}
}

func TestExecuteTrueNASCollectorDiscoveryFailureStatus(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		switch method {
		case "pool.query":
			return nil, &trueNASRPCError{Code: -32000, Message: "pool failure"}
		default:
			return map[string]any{}, nil
		}
	})
	defer server.Close()

	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store
	createTrueNASCredentialProfile(t, sut, "cred-truenas-status-discovery", "api-key", server.URL)
	collector := hubcollector.Collector{
		ID:            "collector-truenas-status-3",
		AssetID:       "truenas-cluster-status-3",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "cred-truenas-status-discovery",
		},
	}
	store.statusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.executeTrueNASCollector(ctx, collector)
	updated, _, _ := store.GetHubCollector(collector.ID)
	if updated.LastStatus != "error" || !strings.Contains(updated.LastError, "truenas discovery failed") {
		t.Fatalf("unexpected discovery failure status: %+v", updated)
	}
}

func TestExecuteTrueNASCollectorNoAssetsStatus(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		switch method {
		case "system.info":
			return nil, &trueNASRPCError{Code: -32000, Message: "system unavailable"}
		case "pool.query":
			return []map[string]any{}, nil
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
	createTrueNASCredentialProfile(t, sut, "cred-truenas-status-noassets", "api-key", server.URL)
	collector := hubcollector.Collector{
		ID:            "collector-truenas-status-4",
		AssetID:       "truenas-cluster-status-4",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "cred-truenas-status-noassets",
		},
	}
	store.statusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.executeTrueNASCollector(ctx, collector)
	updated, _, _ := store.GetHubCollector(collector.ID)
	if updated.LastStatus != "partial" || !strings.Contains(updated.LastError, "no assets discovered") {
		t.Fatalf("unexpected no-assets status: %+v", updated)
	}
}

func TestExecuteTrueNASCollectorDeDupeStubAssetIgnored(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		switch method {
		case "system.info":
			return nil, &trueNASRPCError{Code: -32000, Message: "system unavailable"}
		case "pool.query":
			return []map[string]any{}, nil
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
	createTrueNASCredentialProfile(t, sut, "cred-truenas-stub-ignore", "api-key", server.URL)
	collector := hubcollector.Collector{
		ID:            "collector-truenas-stub-ignore",
		AssetID:       "truenas-cluster-stub-ignore",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "cred-truenas-stub-ignore",
		},
	}
	store.statusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.executeTrueNASCollector(ctx, collector)

	_, exists, err := sut.assetStore.GetAsset("truenas-controller-stub")
	if err != nil {
		t.Fatalf("GetAsset(truenas-controller-stub) error = %v", err)
	}
	if exists {
		t.Fatalf("expected stub asset to be ignored during collector ingest")
	}
}

func TestExecuteTrueNASCollectorDiscoveryFailureUpdatesStatusAndLogs(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		if method == "pool.query" {
			return nil, &trueNASRPCError{Code: -32000, Message: "pool query failure"}
		}
		return map[string]any{}, nil
	})
	defer server.Close()

	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store
	createTrueNASCredentialProfile(t, sut, "cred-truenas-fail-status-log", "api-key", server.URL)
	collector := hubcollector.Collector{
		ID:            "collector-truenas-fail-status-log",
		AssetID:       "truenas-cluster-fail-status-log",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "cred-truenas-fail-status-log",
		},
	}
	store.statusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.executeTrueNASCollector(ctx, collector)
	updated, _, _ := store.GetHubCollector(collector.ID)
	if updated.LastStatus != "error" || !strings.Contains(updated.LastError, "truenas discovery failed") {
		t.Fatalf("unexpected failure status: %+v", updated)
	}
	events, err := sut.logStore.QueryEvents(logs.QueryRequest{
		Source: "truenas",
		From:   time.Unix(0, 0).UTC(),
		To:     time.Now().UTC().Add(365 * 24 * time.Hour),
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("QueryEvents() error = %v", err)
	}
	found := false
	for _, event := range events {
		if strings.Contains(event.Message, "truenas discovery failed") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected discovery failure log")
	}
}
