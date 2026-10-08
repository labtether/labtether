package main

import (
	"context"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/logs"
	"strings"
	"testing"
	"time"
)

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
