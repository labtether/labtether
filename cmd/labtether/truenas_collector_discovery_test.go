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
		store.StatusByID[collector.ID] = collector

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
				return map[string]any{"hostname": "VaultNAS", "version": "25.04.0"}, nil
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
		store.StatusByID[collector.ID] = collector

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
	store.StatusByID[collector.ID] = collector

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
	store.StatusByID[collector.ID] = collector

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
	store.StatusByID[collector.ID] = collector

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
