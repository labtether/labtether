package main

import (
	"context"
	"github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/logs"
	"strings"
	"testing"
	"time"
)

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
