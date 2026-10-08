package main

import (
	"context"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/logs"
	"strings"
	"testing"
	"time"
)

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
