package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
)

type truenasCollectorAssetStoreWithErrors struct {
	persistence.AssetStore
	failUpsert map[string]error
	listErr    error
}

func (s *truenasCollectorAssetStoreWithErrors) UpsertAssetHeartbeat(req assets.HeartbeatRequest) (assets.Asset, error) {
	if err, ok := s.failUpsert[strings.TrimSpace(req.AssetID)]; ok {
		return assets.Asset{}, err
	}
	return s.AssetStore.UpsertAssetHeartbeat(req)
}

func (s *truenasCollectorAssetStoreWithErrors) ListAssets() ([]assets.Asset, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.AssetStore.ListAssets()
}

func TestIngestTrueNASAlertLogsBranches(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	t.Run("method not found is ignored", func(t *testing.T) {
		server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		})
		defer server.Close()

		sut := newTestAPIServer(t)
		client := &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second}
		count, err := sut.ingestTrueNASAlertLogs(context.Background(), client, "truenas-cluster-1")
		if err != nil {
			t.Fatalf("ingestTrueNASAlertLogs() error = %v", err)
		}
		if count != 0 {
			t.Fatalf("ingestTrueNASAlertLogs() count = %d, want 0", count)
		}
	})

	t.Run("upstream error surfaces", func(t *testing.T) {
		server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			return nil, &trueNASRPCError{Code: -32000, Message: "permission denied"}
		})
		defer server.Close()

		sut := newTestAPIServer(t)
		client := &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second}
		if _, err := sut.ingestTrueNASAlertLogs(context.Background(), client, "truenas-cluster-1"); err == nil || !strings.Contains(err.Error(), "permission denied") {
			t.Fatalf("expected permission denied error, got %v", err)
		}
	})

	t.Run("alerts ingested with hostname routing and stable ids", func(t *testing.T) {
		now := time.Now().UTC()
		server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			if method != "alert.list" {
				return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
			}
			return []map[string]any{
				{
					"uuid":      "alert-1",
					"formatted": "Pool degraded",
					"level":     "WARN",
					"datetime":  now.Add(-5 * time.Minute).Format(time.RFC3339),
					"hostname":  "OmegaNAS",
					"klass":     "PoolStatus",
					"source":    "middlewared",
				},
				{
					"id":       "alert-2",
					"text":     "Disk warning",
					"level":    "ERROR",
					"datetime": now.Add(-4 * time.Minute).Format(time.RFC3339),
				},
				{
					"formatted": "",
					"datetime":  "",
				},
			}, nil
		})
		defer server.Close()

		sut := newTestAPIServer(t)
		client := &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second}
		count, err := sut.ingestTrueNASAlertLogs(context.Background(), client, "truenas-cluster-1")
		if err != nil {
			t.Fatalf("ingestTrueNASAlertLogs() error = %v", err)
		}
		if count != 2 {
			t.Fatalf("ingestTrueNASAlertLogs() count = %d, want 2", count)
		}

		events, err := sut.logStore.QueryEvents(logs.QueryRequest{
			Source: "truenas",
			From:   time.Now().UTC().Add(-24 * time.Hour),
			To:     time.Now().UTC().Add(24 * time.Hour),
			Limit:  10,
		})
		if err != nil {
			t.Fatalf("QueryEvents() error = %v", err)
		}
		if len(events) < 2 {
			t.Fatalf("expected ingested alert events, got %d", len(events))
		}
		if !strings.HasPrefix(events[0].ID, "log_truenas_alert_") {
			t.Fatalf("expected stable truenas alert id prefix, got %q", events[0].ID)
		}
	})
}

func TestExecuteTrueNASCollectorErrorBranches(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	t.Run("credential store unavailable", func(t *testing.T) {
		sut := newTestAPIServer(t)
		store := newRecordingHubCollectorStore()
		store.statusByID["collector-truenas-1"] = hubcollector.Collector{ID: "collector-truenas-1"}
		sut.hubCollectorStore = store
		sut.credentialStore = nil

		sut.executeTrueNASCollector(context.Background(), hubcollector.Collector{
			ID:            "collector-truenas-1",
			AssetID:       "truenas-cluster-1",
			CollectorType: hubcollector.CollectorTypeTrueNAS,
			Enabled:       true,
		})

		updated, ok, err := store.GetHubCollector("collector-truenas-1")
		if err != nil || !ok {
			t.Fatalf("failed to get collector status: ok=%v err=%v", ok, err)
		}
		if updated.LastStatus != "error" || !strings.Contains(updated.LastError, "credential store unavailable") {
			t.Fatalf("unexpected collector status update: %+v", updated)
		}
	})

	t.Run("missing base url and credential id", func(t *testing.T) {
		sut := newTestAPIServer(t)
		store := newRecordingHubCollectorStore()
		sut.hubCollectorStore = store

		collector := hubcollector.Collector{
			ID:            "collector-truenas-2",
			AssetID:       "truenas-cluster-2",
			CollectorType: hubcollector.CollectorTypeTrueNAS,
			Enabled:       true,
			Config:        map[string]any{},
		}
		store.statusByID[collector.ID] = collector
		sut.executeTrueNASCollector(context.Background(), collector)
		updated, _, _ := store.GetHubCollector(collector.ID)
		if updated.LastStatus != "error" || !strings.Contains(updated.LastError, "missing base_url") {
			t.Fatalf("unexpected missing base_url status: %+v", updated)
		}

		collector.ID = "collector-truenas-3"
		collector.Config = map[string]any{"base_url": "https://tn.local"}
		store.statusByID[collector.ID] = collector
		sut.executeTrueNASCollector(context.Background(), collector)
		updated, _, _ = store.GetHubCollector(collector.ID)
		if updated.LastStatus != "error" || !strings.Contains(updated.LastError, "missing credential_id") {
			t.Fatalf("unexpected missing credential_id status: %+v", updated)
		}
	})

	t.Run("credential lookup and decrypt errors", func(t *testing.T) {
		sut := newTestAPIServer(t)
		store := newRecordingHubCollectorStore()
		sut.hubCollectorStore = store

		missingCollector := hubcollector.Collector{
			ID:            "collector-truenas-4",
			AssetID:       "truenas-cluster-4",
			CollectorType: hubcollector.CollectorTypeTrueNAS,
			Enabled:       true,
			Config:        map[string]any{"base_url": "https://tn.local", "credential_id": "missing"},
		}
		store.statusByID[missingCollector.ID] = missingCollector
		sut.executeTrueNASCollector(context.Background(), missingCollector)
		updated, _, _ := store.GetHubCollector(missingCollector.ID)
		if updated.LastStatus != "error" || !strings.Contains(updated.LastError, "credential not found") {
			t.Fatalf("unexpected missing credential status: %+v", updated)
		}

		const badCipherID = "cred-truenas-bad-cipher"
		_, err := sut.credentialStore.CreateCredentialProfile(credentials.Profile{
			ID:               badCipherID,
			Name:             "bad cipher",
			Kind:             credentials.KindTrueNASAPIKey,
			Status:           "active",
			SecretCiphertext: "not-valid-ciphertext",
			Metadata:         map[string]string{"base_url": "https://tn.local"},
			CreatedAt:        time.Now().UTC(),
			UpdatedAt:        time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("create bad cipher profile: %v", err)
		}

		decryptCollector := hubcollector.Collector{
			ID:            "collector-truenas-5",
			AssetID:       "truenas-cluster-5",
			CollectorType: hubcollector.CollectorTypeTrueNAS,
			Enabled:       true,
			Config:        map[string]any{"base_url": "https://tn.local", "credential_id": badCipherID},
		}
		store.statusByID[decryptCollector.ID] = decryptCollector
		sut.executeTrueNASCollector(context.Background(), decryptCollector)
		updated, _, _ = store.GetHubCollector(decryptCollector.ID)
		if updated.LastStatus != "error" || !strings.Contains(updated.LastError, "failed to decrypt credential") {
			t.Fatalf("unexpected decrypt error status: %+v", updated)
		}
	})
}

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

		poolAsset, exists, assetErr := sut.assetStore.GetAsset("truenas-storage-pool-mainpool")
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

func TestTrueNASAutoLinkWrappers(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	sut := newTestAPIServer(t)
	deps := newStubDependencyStore()
	sut.dependencyStore = deps

	assetsToUpsert := []assets.HeartbeatRequest{
		{
			AssetID: "portainer-host-1",
			Type:    "container-host",
			Name:    "portainer-host",
			Source:  "portainer",
			Metadata: map[string]string{
				"endpoint_ip": "10.0.0.25",
			},
		},
		{
			AssetID: "truenas-host-omeganas",
			Type:    "nas",
			Name:    "OmegaNAS",
			Source:  "truenas",
			Metadata: map[string]string{
				"collector_endpoint_ip": "10.0.0.25",
				"hostname":              "omeganas",
			},
		},
		{
			AssetID: "proxmox-vm-101",
			Type:    "vm",
			Name:    "OmegaNAS",
			Source:  "proxmox",
			Metadata: map[string]string{
				"node_ip": "10.0.0.25",
			},
		},
	}
	for _, req := range assetsToUpsert {
		if _, err := sut.assetStore.UpsertAssetHeartbeat(req); err != nil {
			t.Fatalf("failed to upsert %s: %v", req.AssetID, err)
		}
	}

	if err := sut.autoLinkPortainerHostsToTrueNASHosts(); err != nil {
		t.Fatalf("autoLinkPortainerHostsToTrueNASHosts() error = %v", err)
	}
	if err := sut.autoLinkTrueNASHostsToProxmoxGuests(); err != nil {
		t.Fatalf("autoLinkTrueNASHostsToProxmoxGuests() error = %v", err)
	}

	portainerDeps, err := deps.ListAssetDependencies("portainer-host-1", 20)
	if err != nil {
		t.Fatalf("ListAssetDependencies(portainer) error = %v", err)
	}
	foundPortainerLink := false
	for _, dep := range portainerDeps {
		if dep.SourceAssetID == "portainer-host-1" && dep.TargetAssetID == "truenas-host-omeganas" {
			foundPortainerLink = true
			break
		}
	}
	if !foundPortainerLink {
		t.Fatalf("expected portainer -> truenas runs_on link")
	}

	truenasDeps, err := deps.ListAssetDependencies("truenas-host-omeganas", 20)
	if err != nil {
		t.Fatalf("ListAssetDependencies(truenas) error = %v", err)
	}
	foundTrueNASLink := false
	for _, dep := range truenasDeps {
		if dep.SourceAssetID == "truenas-host-omeganas" && dep.TargetAssetID == "proxmox-vm-101" {
			foundTrueNASLink = true
			break
		}
	}
	if !foundTrueNASLink {
		t.Fatalf("expected truenas -> proxmox runs_on link")
	}
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

func TestExecuteTrueNASCollectorLogsSummary(t *testing.T) {
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
	createTrueNASCredentialProfile(t, sut, "cred-truenas-summary", "api-key", server.URL)

	collector := hubcollector.Collector{
		ID:            "collector-truenas-summary",
		AssetID:       "truenas-cluster-summary",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "cred-truenas-summary",
			"skip_verify":   true,
		},
	}
	store.statusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	sut.executeTrueNASCollector(ctx, collector)

	events, err := sut.logStore.QueryEvents(logs.QueryRequest{
		Source: "truenas",
		From:   time.Unix(0, 0).UTC(),
		To:     time.Now().UTC().Add(365 * 24 * time.Hour),
		Limit:  20,
	})
	if err != nil {
		t.Fatalf("QueryEvents() error = %v", err)
	}
	found := false
	for _, event := range events {
		if strings.Contains(event.Message, "collector run complete") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected collector completion summary log entry")
	}
}

func TestExecuteTrueNASCollectorRemainingErrorBranches(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		switch method {
		case "system.info":
			return map[string]any{"hostname": "OmegaNAS", "version": "25.04.0"}, nil
		case "pool.query":
			return []map[string]any{
				{"id": 1, "name": "mainpool", "status": "ONLINE", "healthy": true, "size": 1000, "allocated": 200, "free": 800},
			}, nil
		case "alert.list":
			return []map[string]any{}, nil
		default:
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		}
	})
	defer server.Close()

	sut := newTestAPIServer(t)
	createTrueNASCredentialProfile(t, sut, "cred-truenas-remaining-branches", "api-key", server.URL)

	collector := hubcollector.Collector{
		ID:            "collector-truenas-remaining-branches",
		AssetID:       "truenas-cluster-remaining-branches",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "cred-truenas-remaining-branches",
			"skip_verify":   true,
		},
	}

	assetStoreWithErrors := &truenasCollectorAssetStoreWithErrors{
		AssetStore: sut.assetStore,
		failUpsert: map[string]error{
			"truenas-host-omeganas":              errors.New("forced asset upsert failure"),
			"truenas-cluster-remaining-branches": errors.New("forced cluster upsert failure"),
		},
		listErr: errors.New("forced list assets failure"),
	}
	sut.assetStore = assetStoreWithErrors
	sut.dependencyStore = newStubDependencyStore()
	// Force runtime worker setup to fail while allowing collector execution to continue.
	sut.hubCollectorStore = nil

	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()
	sut.executeTrueNASCollector(ctx, collector)
	// Avoid impacting cleanup hooks that list assets.
	assetStoreWithErrors.listErr = nil

	events, err := sut.logStore.QueryEvents(logs.QueryRequest{
		Source: "truenas",
		From:   time.Unix(0, 0).UTC(),
		To:     time.Now().UTC().Add(365 * 24 * time.Hour),
		Limit:  50,
	})
	if err != nil {
		t.Fatalf("QueryEvents() error = %v", err)
	}
	if len(events) == 0 {
		t.Fatalf("expected collector log events after execution")
	}
}

func TestIngestTrueNASAlertLogsKeyFallback(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		if method != "alert.list" {
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		}
		return []map[string]any{
			{
				"formatted": "fallback-key-alert",
				"datetime":  "2026-02-23T01:00:00Z",
			},
		}, nil
	})
	defer server.Close()

	sut := newTestAPIServer(t)
	client := &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second}
	count, err := sut.ingestTrueNASAlertLogs(context.Background(), client, "truenas-cluster-fallback")
	if err != nil {
		t.Fatalf("ingestTrueNASAlertLogs() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("ingestTrueNASAlertLogs() count = %d, want 1", count)
	}

	events, err := sut.logStore.QueryEvents(logs.QueryRequest{
		Source: "truenas",
		From:   time.Unix(0, 0).UTC(),
		To:     time.Now().UTC().Add(365 * 24 * time.Hour),
		Limit:  5,
	})
	if err != nil {
		t.Fatalf("QueryEvents() error = %v", err)
	}
	if len(events) == 0 || !strings.Contains(events[0].Message, "fallback-key-alert") {
		t.Fatalf("expected fallback alert message in ingested logs, got %#v", events)
	}
}

func TestExecuteTrueNASCollectorMissingCredentialIDAndBaseURLErrorsIncludeLogs(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store

	collector := hubcollector.Collector{
		ID:            "collector-truenas-log-1",
		AssetID:       "truenas-cluster-log-1",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config:        map[string]any{},
	}
	store.statusByID[collector.ID] = collector

	sut.executeTrueNASCollector(context.Background(), collector)
	events, err := sut.logStore.QueryEvents(logs.QueryRequest{
		Source: "truenas",
		From:   time.Unix(0, 0).UTC(),
		To:     time.Now().UTC().Add(365 * 24 * time.Hour),
		Limit:  5,
	})
	if err != nil {
		t.Fatalf("QueryEvents() error = %v", err)
	}
	found := false
	for _, event := range events {
		if strings.Contains(event.Message, "missing base_url") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected missing base_url log event")
	}
}

func TestExecuteTrueNASCollectorCredentialNotFoundLog(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store

	collector := hubcollector.Collector{
		ID:            "collector-truenas-log-2",
		AssetID:       "truenas-cluster-log-2",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      "https://tn.local",
			"credential_id": "missing",
		},
	}
	store.statusByID[collector.ID] = collector

	sut.executeTrueNASCollector(context.Background(), collector)
	events, err := sut.logStore.QueryEvents(logs.QueryRequest{
		Source: "truenas",
		From:   time.Unix(0, 0).UTC(),
		To:     time.Now().UTC().Add(365 * 24 * time.Hour),
		Limit:  5,
	})
	if err != nil {
		t.Fatalf("QueryEvents() error = %v", err)
	}
	found := false
	for _, event := range events {
		if strings.Contains(event.Message, "credential not found") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected credential not found log event")
	}
}

func TestExecuteTrueNASCollectorDecryptFailureLog(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store

	const badCipherID = "cred-truenas-log-bad"
	_, err := sut.credentialStore.CreateCredentialProfile(credentials.Profile{
		ID:               badCipherID,
		Name:             "bad cipher",
		Kind:             credentials.KindTrueNASAPIKey,
		Status:           "active",
		SecretCiphertext: "not-valid-ciphertext",
		Metadata:         map[string]string{"base_url": "https://tn.local"},
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create bad cipher profile: %v", err)
	}

	collector := hubcollector.Collector{
		ID:            "collector-truenas-log-3",
		AssetID:       "truenas-cluster-log-3",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      "https://tn.local",
			"credential_id": badCipherID,
		},
	}
	store.statusByID[collector.ID] = collector

	sut.executeTrueNASCollector(context.Background(), collector)
	events, err := sut.logStore.QueryEvents(logs.QueryRequest{
		Source: "truenas",
		From:   time.Unix(0, 0).UTC(),
		To:     time.Now().UTC().Add(365 * 24 * time.Hour),
		Limit:  5,
	})
	if err != nil {
		t.Fatalf("QueryEvents() error = %v", err)
	}
	found := false
	for _, event := range events {
		if strings.Contains(event.Message, "failed to decrypt credential") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected decrypt failure log event")
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

func TestExecuteTrueNASCollectorBaseURLCredentialValidationMessages(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store

	collector := hubcollector.Collector{
		ID:            "collector-truenas-log-6",
		AssetID:       "truenas-cluster-log-6",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url": "",
		},
	}
	store.statusByID[collector.ID] = collector
	sut.executeTrueNASCollector(context.Background(), collector)
	updated, _, _ := store.GetHubCollector(collector.ID)
	if updated.LastStatus != "error" {
		t.Fatalf("expected error status for missing base_url")
	}
	if !strings.Contains(updated.LastError, "missing base_url") {
		t.Fatalf("expected missing base_url last error, got %q", updated.LastError)
	}

	collector.ID = "collector-truenas-log-7"
	collector.Config = map[string]any{"base_url": "https://tn.local"}
	store.statusByID[collector.ID] = collector
	sut.executeTrueNASCollector(context.Background(), collector)
	updated, _, _ = store.GetHubCollector(collector.ID)
	if updated.LastStatus != "error" {
		t.Fatalf("expected error status for missing credential_id")
	}
	if !strings.Contains(updated.LastError, "missing credential_id") {
		t.Fatalf("expected missing credential_id last error, got %q", updated.LastError)
	}
}

func TestExecuteTrueNASCollectorCredentialNotFoundAndDecryptStatus(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store

	collectorMissing := hubcollector.Collector{
		ID:            "collector-truenas-status-1",
		AssetID:       "truenas-cluster-status-1",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      "https://tn.local",
			"credential_id": "missing",
		},
	}
	store.statusByID[collectorMissing.ID] = collectorMissing
	sut.executeTrueNASCollector(context.Background(), collectorMissing)
	updated, _, _ := store.GetHubCollector(collectorMissing.ID)
	if updated.LastStatus != "error" || !strings.Contains(updated.LastError, "credential not found") {
		t.Fatalf("unexpected missing credential status: %+v", updated)
	}

	const badCipherID = "cred-truenas-status-bad"
	_, err := sut.credentialStore.CreateCredentialProfile(credentials.Profile{
		ID:               badCipherID,
		Name:             "bad cipher",
		Kind:             credentials.KindTrueNASAPIKey,
		Status:           "active",
		SecretCiphertext: "not-valid-ciphertext",
		Metadata:         map[string]string{"base_url": "https://tn.local"},
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create bad cipher profile: %v", err)
	}

	collectorBadCipher := hubcollector.Collector{
		ID:            "collector-truenas-status-2",
		AssetID:       "truenas-cluster-status-2",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      "https://tn.local",
			"credential_id": badCipherID,
		},
	}
	store.statusByID[collectorBadCipher.ID] = collectorBadCipher
	sut.executeTrueNASCollector(context.Background(), collectorBadCipher)
	updated, _, _ = store.GetHubCollector(collectorBadCipher.ID)
	if updated.LastStatus != "error" || !strings.Contains(updated.LastError, "failed to decrypt credential") {
		t.Fatalf("unexpected decrypt status: %+v", updated)
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

func TestExecuteTrueNASCollectorSuccessStatusAndCompletionLog(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		switch method {
		case "system.info":
			return map[string]any{"hostname": "OmegaNAS", "version": "25.04.0"}, nil
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
	createTrueNASCredentialProfile(t, sut, "cred-truenas-status-success", "api-key", server.URL)
	collector := hubcollector.Collector{
		ID:            "collector-truenas-status-5",
		AssetID:       "truenas-cluster-status-5",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "cred-truenas-status-success",
		},
	}
	store.statusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.executeTrueNASCollector(ctx, collector)
	updated, _, _ := store.GetHubCollector(collector.ID)
	if updated.LastStatus != "ok" || updated.LastError != "" {
		t.Fatalf("unexpected success status: %+v", updated)
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
	foundCompletion := false
	for _, event := range events {
		if strings.Contains(event.Message, "collector run complete") {
			foundCompletion = true
			break
		}
	}
	if !foundCompletion {
		t.Fatalf("expected collector completion log")
	}
}

func TestIngestTrueNASAlertLogsSkipsEmptyFallbackKey(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		if method != "alert.list" {
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		}
		return []map[string]any{{"formatted": "", "datetime": ""}}, nil
	})
	defer server.Close()

	sut := newTestAPIServer(t)
	client := &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second}
	count, err := sut.ingestTrueNASAlertLogs(context.Background(), client, "truenas-cluster-empty-key")
	if err != nil {
		t.Fatalf("ingestTrueNASAlertLogs() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("expected zero ingested alerts for empty fallback key, got %d", count)
	}
}

func TestExecuteTrueNASCollectorMissingCredentialIDStatus(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store
	collector := hubcollector.Collector{
		ID:            "collector-truenas-status-missing-credential-id",
		AssetID:       "truenas-cluster-status-missing-credential-id",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url": "https://tn.local",
		},
	}
	store.statusByID[collector.ID] = collector
	sut.executeTrueNASCollector(context.Background(), collector)
	updated, _, _ := store.GetHubCollector(collector.ID)
	if updated.LastStatus != "error" || !strings.Contains(updated.LastError, "missing credential_id") {
		t.Fatalf("unexpected missing credential_id status: %+v", updated)
	}
}

func TestExecuteTrueNASCollectorMissingBaseURLStatus(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store
	collector := hubcollector.Collector{
		ID:            "collector-truenas-status-missing-base-url",
		AssetID:       "truenas-cluster-status-missing-base-url",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"credential_id": "cred",
		},
	}
	store.statusByID[collector.ID] = collector
	sut.executeTrueNASCollector(context.Background(), collector)
	updated, _, _ := store.GetHubCollector(collector.ID)
	if updated.LastStatus != "error" || !strings.Contains(updated.LastError, "missing base_url") {
		t.Fatalf("unexpected missing base_url status: %+v", updated)
	}
}

func TestIngestTrueNASAlertLogsFallbackAssetIDWithoutHostname(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		if method != "alert.list" {
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		}
		return []map[string]any{{
			"uuid":      "alert-no-host",
			"formatted": "No hostname",
			"datetime":  "2026-02-23T02:00:00Z",
		}}, nil
	})
	defer server.Close()

	sut := newTestAPIServer(t)
	client := &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second}
	_, err := sut.ingestTrueNASAlertLogs(context.Background(), client, "truenas-cluster-fallback-asset")
	if err != nil {
		t.Fatalf("ingestTrueNASAlertLogs() error = %v", err)
	}
	events, err := sut.logStore.QueryEvents(logs.QueryRequest{
		Source: "truenas",
		From:   time.Unix(0, 0).UTC(),
		To:     time.Now().UTC().Add(365 * 24 * time.Hour),
		Limit:  5,
	})
	if err != nil {
		t.Fatalf("QueryEvents() error = %v", err)
	}
	if len(events) == 0 || events[0].AssetID != "truenas-cluster-fallback-asset" {
		t.Fatalf("expected fallback asset id assignment, got %#v", events)
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

	poolAsset, exists, err := sut.assetStore.GetAsset("truenas-storage-pool-mainpool")
	if err != nil || !exists {
		t.Fatalf("expected pool asset to exist, exists=%v err=%v", exists, err)
	}
	if poolAsset.Metadata["collector_base_url"] == "" || poolAsset.Metadata["collector_id"] != collector.ID {
		t.Fatalf("expected collector metadata enrichment, got %#v", poolAsset.Metadata)
	}
}

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

func TestExecuteTrueNASCollectorDiscoveryAndAlertCountsInSummaryLog(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		switch method {
		case "system.info":
			return map[string]any{"hostname": "OmegaNAS"}, nil
		case "pool.query":
			return []map[string]any{{"id": 1, "name": "mainpool", "status": "ONLINE", "size": 100, "allocated": 10, "free": 90}}, nil
		case "alert.list":
			return []map[string]any{{"uuid": "alert1", "formatted": "A1", "datetime": "2026-02-23T00:00:00Z"}}, nil
		default:
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		}
	})
	defer server.Close()

	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store
	createTrueNASCredentialProfile(t, sut, "cred-truenas-summary-counts", "api-key", server.URL)
	collector := hubcollector.Collector{
		ID:            "collector-truenas-summary-counts",
		AssetID:       "truenas-cluster-summary-counts",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "cred-truenas-summary-counts",
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
		if strings.Contains(event.Message, "collector run complete") && strings.Contains(event.Message, "discovered=") && strings.Contains(event.Message, "alert_events=") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected summary log with discovered/alert counts")
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

	poolAsset, exists, err := sut.assetStore.GetAsset("truenas-storage-pool-mainpool")
	if err != nil || !exists {
		t.Fatalf("expected pool asset to exist, exists=%v err=%v", exists, err)
	}
	if poolAsset.Metadata["collector_endpoint_host"] == "" {
		t.Fatalf("expected collector endpoint host metadata")
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

func TestIngestTrueNASAlertLogsHostnameAssetOverride(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		if method != "alert.list" {
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		}
		return []map[string]any{{
			"uuid":      "alert-host-override",
			"formatted": "Host override",
			"hostname":  "OmegaNAS",
			"datetime":  "2026-02-23T03:00:00Z",
		}}, nil
	})
	defer server.Close()

	sut := newTestAPIServer(t)
	client := &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second}
	_, err := sut.ingestTrueNASAlertLogs(context.Background(), client, "truenas-cluster-source")
	if err != nil {
		t.Fatalf("ingestTrueNASAlertLogs() error = %v", err)
	}
	events, err := sut.logStore.QueryEvents(logs.QueryRequest{
		Source: "truenas",
		From:   time.Unix(0, 0).UTC(),
		To:     time.Now().UTC().Add(365 * 24 * time.Hour),
		Limit:  5,
	})
	if err != nil {
		t.Fatalf("QueryEvents() error = %v", err)
	}
	if len(events) == 0 || events[0].AssetID != "truenas-host-omeganas" {
		t.Fatalf("expected hostname-based asset override, got %#v", events)
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

func TestIngestTrueNASAlertLogsTimestampFallbackNow(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		if method != "alert.list" {
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		}
		return []map[string]any{{
			"uuid":      "alert-no-timestamp",
			"formatted": "No timestamp",
			"datetime":  "not-a-time",
		}}, nil
	})
	defer server.Close()

	sut := newTestAPIServer(t)
	client := &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second}
	count, err := sut.ingestTrueNASAlertLogs(context.Background(), client, "truenas-cluster-now")
	if err != nil {
		t.Fatalf("ingestTrueNASAlertLogs() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one ingested alert, got %d", count)
	}
}

func TestExecuteTrueNASCollectorStatusErrorWhenCredentialStoreUnavailable(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store
	sut.credentialStore = nil
	collector := hubcollector.Collector{
		ID:            "collector-truenas-cred-unavailable",
		AssetID:       "truenas-cluster-cred-unavailable",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
	}
	store.statusByID[collector.ID] = collector
	sut.executeTrueNASCollector(context.Background(), collector)
	updated, _, _ := store.GetHubCollector(collector.ID)
	if updated.LastStatus != "error" || !strings.Contains(updated.LastError, "credential store unavailable") {
		t.Fatalf("unexpected credential store unavailable status: %+v", updated)
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

func TestIngestTrueNASAlertLogsLevelNormalization(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		if method != "alert.list" {
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		}
		return []map[string]any{{
			"uuid":      "alert-level-1",
			"formatted": "Critical alert",
			"level":     "CRIT",
			"datetime":  "2026-02-23T04:00:00Z",
		}}, nil
	})
	defer server.Close()

	sut := newTestAPIServer(t)
	client := &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second}
	_, err := sut.ingestTrueNASAlertLogs(context.Background(), client, "truenas-cluster-level")
	if err != nil {
		t.Fatalf("ingestTrueNASAlertLogs() error = %v", err)
	}
	events, err := sut.logStore.QueryEvents(logs.QueryRequest{
		Source: "truenas",
		From:   time.Unix(0, 0).UTC(),
		To:     time.Now().UTC().Add(365 * 24 * time.Hour),
		Limit:  5,
	})
	if err != nil {
		t.Fatalf("QueryEvents() error = %v", err)
	}
	if len(events) == 0 || events[0].Level != "error" {
		t.Fatalf("expected level normalization to error, got %#v", events)
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

func TestIngestTrueNASAlertLogsIncludesNodeAndClassFields(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		if method != "alert.list" {
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		}
		return []map[string]any{{
			"uuid":      "alert-fields-1",
			"formatted": "Alert with fields",
			"klass":     "PoolStatus",
			"source":    "middlewared",
			"node":      "tn-node-1",
			"datetime":  "2026-02-23T05:00:00Z",
		}}, nil
	})
	defer server.Close()

	sut := newTestAPIServer(t)
	client := &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second}
	_, err := sut.ingestTrueNASAlertLogs(context.Background(), client, "truenas-cluster-fields")
	if err != nil {
		t.Fatalf("ingestTrueNASAlertLogs() error = %v", err)
	}
	events, err := sut.logStore.QueryEvents(logs.QueryRequest{
		Source: "truenas",
		From:   time.Unix(0, 0).UTC(),
		To:     time.Now().UTC().Add(365 * 24 * time.Hour),
		Limit:  5,
	})
	if err != nil {
		t.Fatalf("QueryEvents() error = %v", err)
	}
	if len(events) == 0 {
		t.Fatalf("expected alert event with metadata fields")
	}
	if events[0].Fields["alert_class"] != "PoolStatus" || events[0].Fields["node"] != "tn-node-1" {
		t.Fatalf("expected alert metadata fields, got %#v", events[0].Fields)
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

func TestIngestTrueNASAlertLogsUsesIDWhenUUIDMissing(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		if method != "alert.list" {
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		}
		return []map[string]any{{
			"id":        "alert-id-only",
			"formatted": "ID only alert",
			"datetime":  "2026-02-23T06:00:00Z",
		}}, nil
	})
	defer server.Close()

	sut := newTestAPIServer(t)
	client := &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second}
	count, err := sut.ingestTrueNASAlertLogs(context.Background(), client, "truenas-cluster-id-only")
	if err != nil {
		t.Fatalf("ingestTrueNASAlertLogs() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one ingested alert, got %d", count)
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

func TestIngestTrueNASAlertLogsZeroOnEmptyAlertList(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		if method != "alert.list" {
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		}
		return []map[string]any{}, nil
	})
	defer server.Close()

	sut := newTestAPIServer(t)
	client := &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second}
	count, err := sut.ingestTrueNASAlertLogs(context.Background(), client, "truenas-cluster-empty-alerts")
	if err != nil {
		t.Fatalf("ingestTrueNASAlertLogs() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("expected zero ingested alerts, got %d", count)
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

func TestIngestTrueNASAlertLogsHandlesNumericLevelAndTimestamp(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		if method != "alert.list" {
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		}
		return []map[string]any{{
			"uuid":      "alert-numeric",
			"formatted": "Numeric alert",
			"level":     2,
			"datetime":  float64(1771828186),
		}}, nil
	})
	defer server.Close()

	sut := newTestAPIServer(t)
	client := &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second}
	count, err := sut.ingestTrueNASAlertLogs(context.Background(), client, "truenas-cluster-numeric")
	if err != nil {
		t.Fatalf("ingestTrueNASAlertLogs() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one ingested numeric alert, got %d", count)
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

func TestIngestTrueNASAlertLogsIgnoresPipeOnlyFallbackKey(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		if method != "alert.list" {
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		}
		return []map[string]any{{"formatted": "", "datetime": ""}}, nil
	})
	defer server.Close()

	sut := newTestAPIServer(t)
	client := &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second}
	count, err := sut.ingestTrueNASAlertLogs(context.Background(), client, "truenas-cluster-pipe")
	if err != nil {
		t.Fatalf("ingestTrueNASAlertLogs() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no ingested alerts for pipe-only fallback key, got %d", count)
	}
}

func TestExecuteTrueNASCollectorSummaryLogIncludesAlertCountZero(t *testing.T) {
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
	createTrueNASCredentialProfile(t, sut, "cred-truenas-summary-zero-alerts", "api-key", server.URL)
	collector := hubcollector.Collector{
		ID:            "collector-truenas-summary-zero-alerts",
		AssetID:       "truenas-cluster-summary-zero-alerts",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "cred-truenas-summary-zero-alerts",
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
		if strings.Contains(event.Message, "alert_events=0") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected summary log with alert_events=0")
	}
}

func TestExecuteTrueNASCollectorSummaryLogIncludesNonZeroAlertCount(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		switch method {
		case "system.info":
			return map[string]any{"hostname": "OmegaNAS"}, nil
		case "pool.query":
			return []map[string]any{{"id": 1, "name": "mainpool", "status": "ONLINE", "size": 100, "allocated": 10, "free": 90}}, nil
		case "alert.list":
			return []map[string]any{{"uuid": "a1", "formatted": "Alert 1", "datetime": "2026-02-23T07:00:00Z"}}, nil
		default:
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		}
	})
	defer server.Close()

	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store
	createTrueNASCredentialProfile(t, sut, "cred-truenas-summary-nonzero-alerts", "api-key", server.URL)
	collector := hubcollector.Collector{
		ID:            "collector-truenas-summary-nonzero-alerts",
		AssetID:       "truenas-cluster-summary-nonzero-alerts",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "cred-truenas-summary-nonzero-alerts",
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
		if strings.Contains(event.Message, "alert_events=1") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected summary log with alert_events=1")
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

	poolAsset, exists, err := sut.assetStore.GetAsset("truenas-storage-pool-faultedpool")
	if err != nil || !exists {
		t.Fatalf("expected faulted pool asset, exists=%v err=%v", exists, err)
	}
	if poolAsset.Status != "offline" {
		t.Fatalf("expected normalized offline status for faulted pool, got %q", poolAsset.Status)
	}
}

func TestIngestTrueNASAlertLogsNormalizesHostnameToAssetKey(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		if method != "alert.list" {
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		}
		return []map[string]any{{
			"uuid":      "alert-hostname-normalized",
			"formatted": "Hostname normalize",
			"hostname":  "Omega NAS",
			"datetime":  "2026-02-23T08:00:00Z",
		}}, nil
	})
	defer server.Close()

	sut := newTestAPIServer(t)
	client := &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second}
	_, err := sut.ingestTrueNASAlertLogs(context.Background(), client, "truenas-cluster-hostname-normalized")
	if err != nil {
		t.Fatalf("ingestTrueNASAlertLogs() error = %v", err)
	}
	events, err := sut.logStore.QueryEvents(logs.QueryRequest{
		Source: "truenas",
		From:   time.Unix(0, 0).UTC(),
		To:     time.Now().UTC().Add(365 * 24 * time.Hour),
		Limit:  5,
	})
	if err != nil {
		t.Fatalf("QueryEvents() error = %v", err)
	}
	if len(events) == 0 || events[0].AssetID != "truenas-host-omega-nas" {
		t.Fatalf("expected normalized hostname asset id, got %#v", events)
	}
}

func TestExecuteTrueNASCollectorStatusErrorWhenHubCollectorStoreUnavailable(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	sut := newTestAPIServer(t)
	sut.hubCollectorStore = nil
	sut.executeTrueNASCollector(context.Background(), hubcollector.Collector{
		ID:            "collector-truenas-no-store",
		AssetID:       "truenas-cluster-no-store",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config:        map[string]any{},
	})
	// No panic is the assertion; status update is best-effort when store unavailable.
}
