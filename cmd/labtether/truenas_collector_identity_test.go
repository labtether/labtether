package main

import (
	"context"
	"fmt"
	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/hubcollector"
	"testing"
	"time"
)

func TestExecuteTwoTrueNASCollectorsKeepRepeatedMainPoolDistinct(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		switch method {
		case "system.info":
			return map[string]any{"hostname": "nas", "version": "25.04.0"}, nil
		case "pool.query":
			return []map[string]any{{"id": 1, "name": "MainPool", "status": "ONLINE", "healthy": true}}, nil
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

	collectorIDs := []string{"collector-truenas-omega", "collector-truenas-tau"}
	for index, collectorID := range collectorIDs {
		credentialID := fmt.Sprintf("cred-truenas-scope-%d", index)
		createTrueNASCredentialProfile(t, sut, credentialID, "api-key", server.URL)
		collector := hubcollector.Collector{
			ID:            collectorID,
			AssetID:       "truenas-cluster-" + collectorID,
			CollectorType: hubcollector.CollectorTypeTrueNAS,
			Enabled:       true,
			Config: map[string]any{
				"base_url":      server.URL,
				"credential_id": credentialID,
				"skip_verify":   true,
			},
		}
		store.statusByID[collector.ID] = collector
		ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
		sut.executeTrueNASCollector(ctx, collector)
		cancel()
	}

	for _, collectorID := range collectorIDs {
		id := assetid.ScopeCollectorAssetID("truenas-storage-pool-mainpool", collectorID)
		asset, ok, err := sut.assetStore.GetAsset(id)
		if err != nil || !ok {
			t.Fatalf("expected scoped pool %s: ok=%v err=%v", id, ok, err)
		}
		if asset.Metadata["collector_id"] != collectorID {
			t.Fatalf("pool %s collector_id = %q", id, asset.Metadata["collector_id"])
		}
	}
	if _, ok, err := sut.assetStore.GetAsset("truenas-storage-pool-mainpool"); err != nil || ok {
		t.Fatalf("unexpected ambiguous legacy pool: ok=%v err=%v", ok, err)
	}
}

func TestExecuteTrueNASCollectorAssetMetadataEndpointIdentity(t *testing.T) {
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
	createTrueNASCredentialProfile(t, sut, "cred-truenas-meta", "api-key", server.URL)
	collector := hubcollector.Collector{
		ID:            "collector-truenas-meta",
		AssetID:       "truenas-cluster-meta",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "cred-truenas-meta",
		},
	}
	store.statusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.executeTrueNASCollector(ctx, collector)

	poolAsset, exists, err := sut.assetStore.GetAsset(assetid.ScopeCollectorAssetID("truenas-storage-pool-mainpool", collector.ID))
	if err != nil || !exists {
		t.Fatalf("expected pool asset to exist, exists=%v err=%v", exists, err)
	}
	if poolAsset.Metadata["collector_base_url"] == "" || poolAsset.Metadata["collector_id"] != collector.ID {
		t.Fatalf("expected collector metadata enrichment, got %#v", poolAsset.Metadata)
	}
}

func TestExecuteTrueNASCollectorHandlesEndpointIdentityMetadata(t *testing.T) {
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
	createTrueNASCredentialProfile(t, sut, "cred-truenas-endpoint-meta", "api-key", server.URL)
	collector := hubcollector.Collector{
		ID:            "collector-truenas-endpoint-meta",
		AssetID:       "truenas-cluster-endpoint-meta",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "cred-truenas-endpoint-meta",
		},
	}
	store.statusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.executeTrueNASCollector(ctx, collector)

	poolAsset, exists, err := sut.assetStore.GetAsset(assetid.ScopeCollectorAssetID("truenas-storage-pool-mainpool", collector.ID))
	if err != nil || !exists {
		t.Fatalf("expected pool asset to exist, exists=%v err=%v", exists, err)
	}
	if poolAsset.Metadata["collector_endpoint_host"] == "" {
		t.Fatalf("expected collector endpoint host metadata")
	}
}

func TestExecuteTrueNASCollectorClusterHeartbeatMetadata(t *testing.T) {
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
	createTrueNASCredentialProfile(t, sut, "cred-truenas-cluster-meta", "api-key", server.URL)
	collector := hubcollector.Collector{
		ID:            "collector-truenas-cluster-meta",
		AssetID:       "truenas-cluster-meta",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "cred-truenas-cluster-meta",
		},
	}
	store.statusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.executeTrueNASCollector(ctx, collector)

	clusterAsset, exists, err := sut.assetStore.GetAsset(collector.AssetID)
	if err != nil || !exists {
		t.Fatalf("expected cluster asset heartbeat, exists=%v err=%v", exists, err)
	}
	if clusterAsset.Metadata["connector_type"] != "truenas" {
		t.Fatalf("expected connector_type metadata, got %#v", clusterAsset.Metadata)
	}
}

func TestExecuteTrueNASCollectorRefreshesCollectorAssetIDHeartbeat(t *testing.T) {
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
	createTrueNASCredentialProfile(t, sut, "cred-truenas-cluster-heartbeat", "api-key", server.URL)
	collector := hubcollector.Collector{
		ID:            "collector-truenas-cluster-heartbeat",
		AssetID:       "truenas-cluster-heartbeat",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "cred-truenas-cluster-heartbeat",
		},
	}
	store.statusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.executeTrueNASCollector(ctx, collector)
	clusterAsset, exists, err := sut.assetStore.GetAsset(collector.AssetID)
	if err != nil || !exists {
		t.Fatalf("expected collector asset heartbeat refresh, exists=%v err=%v", exists, err)
	}
	if clusterAsset.Status != "online" {
		t.Fatalf("expected cluster asset status online, got %q", clusterAsset.Status)
	}
}

func TestExecuteTrueNASCollectorClusterAssetOptionalWhenAssetIDEmpty(t *testing.T) {
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
	createTrueNASCredentialProfile(t, sut, "cred-truenas-empty-asset-id", "api-key", server.URL)
	collector := hubcollector.Collector{
		ID:            "collector-truenas-empty-asset-id",
		AssetID:       "",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "cred-truenas-empty-asset-id",
		},
	}
	store.statusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.executeTrueNASCollector(ctx, collector)
	updated, _, _ := store.GetHubCollector(collector.ID)
	if updated.LastStatus != "ok" {
		t.Fatalf("expected collector success with empty collector asset id, got %+v", updated)
	}
}

func TestExecuteTrueNASCollectorUsesAssetHeartbeatStatusNormalization(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		switch method {
		case "system.info":
			return map[string]any{"hostname": "OmegaNAS"}, nil
		case "pool.query":
			return []map[string]any{{"id": 1, "name": "faultedpool", "status": "FAULTED", "size": 100, "allocated": 90, "free": 10}}, nil
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
	createTrueNASCredentialProfile(t, sut, "cred-truenas-status-normalization", "api-key", server.URL)
	collector := hubcollector.Collector{
		ID:            "collector-truenas-status-normalization",
		AssetID:       "truenas-cluster-status-normalization",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "cred-truenas-status-normalization",
		},
	}
	store.statusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.executeTrueNASCollector(ctx, collector)

	poolAsset, exists, err := sut.assetStore.GetAsset(assetid.ScopeCollectorAssetID("truenas-storage-pool-faultedpool", collector.ID))
	if err != nil || !exists {
		t.Fatalf("expected faulted pool asset, exists=%v err=%v", exists, err)
	}
	if poolAsset.Status != "offline" {
		t.Fatalf("expected normalized offline status for faulted pool, got %q", poolAsset.Status)
	}
}
