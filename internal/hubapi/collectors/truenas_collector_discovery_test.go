package collectors_test

import (
	"context"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/hubapi/testutil/hubdeps"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/logs"
	"strings"
	"testing"
	"time"
)

func TestExecuteTrueNASCollectorDiscoveryFailure(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)
	server := testutil.NewTrueNASRPCServer(t, func(method string, params []any) (any, *testutil.TrueNASRPCError) {
		if method == "pool.query" {
			return nil, &testutil.TrueNASRPCError{Code: -32000, Message: "pool access denied"}
		}
		return map[string]any{}, nil
	})
	defer server.Close()

	sut := hubdeps.New(t)
	store := testutil.NewRecordingHubCollectorStore()
	sut.HubCollectorStore = store
	testutil.CreateTrueNASCredentialProfile(t, sut.CredentialStore, sut.SecretsManager, "cred-truenas-discovery-fail", "api-key", server.URL)

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
	store.StatusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.CollectorsDeps().ExecuteTrueNASCollector(ctx, collector)

	updated, _, _ := store.GetHubCollector(collector.ID)
	if updated.LastStatus != "error" || !strings.Contains(updated.LastError, "truenas discovery failed") {
		t.Fatalf("expected discovery failure collector status, got %+v", updated)
	}
}

func TestExecuteTrueNASCollectorDiscoveryFailureLog(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)
	server := testutil.NewTrueNASRPCServer(t, func(method string, params []any) (any, *testutil.TrueNASRPCError) {
		if method == "pool.query" {
			return nil, &testutil.TrueNASRPCError{Code: -32000, Message: "pool query denied"}
		}
		return map[string]any{}, nil
	})
	defer server.Close()

	sut := hubdeps.New(t)
	store := testutil.NewRecordingHubCollectorStore()
	sut.HubCollectorStore = store
	testutil.CreateTrueNASCredentialProfile(t, sut.CredentialStore, sut.SecretsManager, "cred-truenas-discovery-log", "api-key", server.URL)

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
	store.StatusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.CollectorsDeps().ExecuteTrueNASCollector(ctx, collector)

	events, err := sut.LogStore.QueryEvents(logs.QueryRequest{
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
	testutil.AllowInsecureTransportForConnectorTests(t)
	server := testutil.NewTrueNASRPCServer(t, func(method string, params []any) (any, *testutil.TrueNASRPCError) {
		switch method {
		case "pool.query":
			return nil, &testutil.TrueNASRPCError{Code: -32000, Message: "pool failure"}
		default:
			return map[string]any{}, nil
		}
	})
	defer server.Close()

	sut := hubdeps.New(t)
	store := testutil.NewRecordingHubCollectorStore()
	sut.HubCollectorStore = store
	testutil.CreateTrueNASCredentialProfile(t, sut.CredentialStore, sut.SecretsManager, "cred-truenas-status-discovery", "api-key", server.URL)
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
	store.StatusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.CollectorsDeps().ExecuteTrueNASCollector(ctx, collector)
	updated, _, _ := store.GetHubCollector(collector.ID)
	if updated.LastStatus != "error" || !strings.Contains(updated.LastError, "truenas discovery failed") {
		t.Fatalf("unexpected discovery failure status: %+v", updated)
	}
}

func TestExecuteTrueNASCollectorDiscoveryFailureUpdatesStatusAndLogs(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)
	server := testutil.NewTrueNASRPCServer(t, func(method string, params []any) (any, *testutil.TrueNASRPCError) {
		if method == "pool.query" {
			return nil, &testutil.TrueNASRPCError{Code: -32000, Message: "pool query failure"}
		}
		return map[string]any{}, nil
	})
	defer server.Close()

	sut := hubdeps.New(t)
	store := testutil.NewRecordingHubCollectorStore()
	sut.HubCollectorStore = store
	testutil.CreateTrueNASCredentialProfile(t, sut.CredentialStore, sut.SecretsManager, "cred-truenas-fail-status-log", "api-key", server.URL)
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
	store.StatusByID[collector.ID] = collector

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sut.CollectorsDeps().ExecuteTrueNASCollector(ctx, collector)
	updated, _, _ := store.GetHubCollector(collector.ID)
	if updated.LastStatus != "error" || !strings.Contains(updated.LastError, "truenas discovery failed") {
		t.Fatalf("unexpected failure status: %+v", updated)
	}
	events, err := sut.LogStore.QueryEvents(logs.QueryRequest{
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
