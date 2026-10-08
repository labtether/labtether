package main

import (
	"context"
	"errors"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
	"strings"
	"testing"
	"time"
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
