package main

import (
	"context"
	"errors"
	"github.com/labtether/labtether/internal/assets"
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

func TestExecuteTrueNASCollectorRemainingErrorBranches(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		switch method {
		case "system.info":
			return map[string]any{"hostname": "VaultNAS", "version": "25.04.0"}, nil
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
			"truenas-host-vaultnas":              errors.New("forced asset upsert failure"),
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
