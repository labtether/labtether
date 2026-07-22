package homeassistant

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/connectorsdk"
)

func TestConnectorTestConnectionSuccess(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	connector := NewWithConfig(Config{
		BaseURL:    server.URL,
		Token:      "token-1",
		SkipVerify: true,
		Timeout:    2 * time.Second,
	})

	health, err := connector.TestConnection(context.Background())
	if err != nil {
		t.Fatalf("TestConnection() unexpected error: %v", err)
	}
	if health.Status != "ok" {
		t.Fatalf("TestConnection().Status = %q, want ok", health.Status)
	}
}

func TestConnectorExecuteActionAcceptsCollectorScopedEntityTarget(t *testing.T) {
	t.Parallel()
	connector := NewWithConfig(Config{BaseURL: "https://homeassistant.invalid", Token: "disposable"})
	target := assetid.ScopeCollectorAssetID("light.office", "collector-ha-a")
	result, err := connector.ExecuteAction(context.Background(), "entity.toggle", connectorsdk.ActionRequest{
		TargetID: target,
		DryRun:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "succeeded" || !strings.Contains(result.Output, "light.office") || strings.Contains(result.Output, "--ltc-") {
		t.Fatalf("scoped entity target was not normalized: %+v", result)
	}
}

func TestConnectorTestConnectionRejectsDisallowedOutboundURL(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOWED_HOSTS", "allowed.example.com")

	connector := NewWithConfig(Config{
		BaseURL: "https://blocked.example.net",
		Token:   "token-1",
		Timeout: 2 * time.Second,
	})

	health, err := connector.TestConnection(context.Background())
	if err != nil {
		t.Fatalf("TestConnection() unexpected error: %v", err)
	}
	if health.Status != "failed" {
		t.Fatalf("TestConnection().Status = %q, want failed", health.Status)
	}
	if !strings.Contains(health.Message, "not allowlisted") {
		t.Fatalf("expected outbound policy error, got %q", health.Message)
	}
}

func TestConnectorTestConnectionRejectsOversizedResponse(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")

	payload := bytes.Repeat([]byte("a"), maxResponseBytes+1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	connector := NewWithConfig(Config{
		BaseURL:    server.URL,
		Token:      "token-1",
		SkipVerify: true,
		Timeout:    2 * time.Second,
	})

	health, err := connector.TestConnection(context.Background())
	if err != nil {
		t.Fatalf("TestConnection() unexpected error: %v", err)
	}
	if health.Status != "failed" {
		t.Fatalf("TestConnection().Status = %q, want failed", health.Status)
	}
	if !strings.Contains(health.Message, "response exceeded") {
		t.Fatalf("expected oversized response error, got %q", health.Message)
	}
}

func TestConnectorExecuteServiceCallUsesSafeServicePath(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/services/light/turn_on" {
			t.Fatalf("unexpected path: %s", r.URL.EscapedPath())
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body["entity_id"] != "light.office" {
			t.Fatalf("entity_id = %q, want light.office", body["entity_id"])
		}
		if body["brightness"] != "128" {
			t.Fatalf("brightness = %q, want 128", body["brightness"])
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	connector := NewWithConfig(Config{
		BaseURL:    server.URL,
		Token:      "token-1",
		SkipVerify: true,
		Timeout:    2 * time.Second,
	})

	result, err := connector.ExecuteAction(context.Background(), "service.call", connectorsdk.ActionRequest{
		TargetID: "light.office",
		Params: map[string]string{
			"service":    "light.turn_on",
			"brightness": "128",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteAction() unexpected error: %v", err)
	}
	if result.Status != "succeeded" {
		t.Fatalf("ExecuteAction().Status = %q, want succeeded: %s", result.Status, result.Message)
	}
}

func TestConnectorExecuteServiceCallRejectsUnsafeServicePath(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")

	called := false
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		t.Fatalf("unexpected request to %s", r.URL.EscapedPath())
	}))
	defer server.Close()

	connector := NewWithConfig(Config{
		BaseURL:    server.URL,
		Token:      "token-1",
		SkipVerify: true,
		Timeout:    2 * time.Second,
	})

	result, err := connector.ExecuteAction(context.Background(), "service.call", connectorsdk.ActionRequest{
		Params: map[string]string{
			"service": "light.turn_on/unsafe",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteAction() unexpected error: %v", err)
	}
	if result.Status != "failed" {
		t.Fatalf("ExecuteAction().Status = %q, want failed", result.Status)
	}
	if !strings.Contains(result.Message, "service domain/action") {
		t.Fatalf("expected unsafe service validation error, got %q", result.Message)
	}
	if called {
		t.Fatal("unsafe service should be rejected before making a request")
	}
}

func TestConnectorServiceCallRequiresAdvertisedServiceParameter(t *testing.T) {
	connector := NewWithConfig(Config{BaseURL: "https://homeassistant.invalid", Token: "disposable"})
	for _, dryRun := range []bool{false, true} {
		result, err := connector.ExecuteAction(context.Background(), "service.call", connectorsdk.ActionRequest{DryRun: dryRun})
		if err != nil || result.Status != "failed" || !strings.Contains(result.Message, "service is required") {
			t.Fatalf("missing service dry_run=%v result=%+v err=%v", dryRun, result, err)
		}
	}
}

func TestConnectorActionErrorDoesNotReflectUpstreamBody(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	const reflectedSecret = "qa-ha-long-lived-token"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "reflected "+reflectedSecret, http.StatusInternalServerError)
	}))
	defer server.Close()

	connector := NewWithConfig(Config{
		BaseURL: server.URL, Token: reflectedSecret, SkipVerify: true, Timeout: 2 * time.Second,
	})
	result, err := connector.ExecuteAction(context.Background(), "entity.toggle", connectorsdk.ActionRequest{TargetID: "light.qa"})
	if err != nil || result.Status != "failed" || strings.Contains(result.Message, reflectedSecret) || strings.Contains(result.Message, "reflected") {
		t.Fatalf("upstream body reached action error: result=%+v err=%v", result, err)
	}
}

func TestConnectorActionSuccessRedactsReflectedToken(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	const reflectedSecret = "qa-ha-long-lived-token"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"token":"` + reflectedSecret + `"}`))
	}))
	defer server.Close()

	connector := NewWithConfig(Config{
		BaseURL: server.URL, Token: reflectedSecret, SkipVerify: true, Timeout: 2 * time.Second,
	})
	result, err := connector.ExecuteAction(context.Background(), "entity.toggle", connectorsdk.ActionRequest{TargetID: "light.qa"})
	if err != nil || result.Status != "succeeded" || strings.Contains(result.Output, reflectedSecret) || !strings.Contains(result.Output, "[REDACTED]") {
		t.Fatalf("reflected token was not redacted: result=%+v err=%v", result, err)
	}
}

func TestTruncatePayloadRedactsTokenAcrossOutputBoundary(t *testing.T) {
	const secret = "boundary-secret-token"
	payload := []byte(strings.Repeat("x", 500) + secret + strings.Repeat("y", 500))
	output := truncatePayloadRedacted(payload, secret)
	if strings.Contains(output, secret) || strings.Contains(output, "boundary-secr") || !strings.Contains(output, "[REDACTED]") {
		t.Fatalf("boundary token was partially exposed: %q", output)
	}
}

func TestValidateTokenRejectsEmptyOversizeAndControlCharacters(t *testing.T) {
	if err := validateToken("valid-disposable-token"); err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	for _, token := range []string{"", strings.Repeat("a", maxTokenBytes+1), "token\nheader"} {
		if err := validateToken(token); err == nil {
			t.Fatalf("invalid token of length %d accepted", len(token))
		}
	}
}

func TestConnectorRejectsInvalidTokenBeforeOutboundRequest(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	defer server.Close()

	connector := NewWithConfig(Config{BaseURL: server.URL, Token: "token\nforged-header"})
	result, err := connector.ExecuteAction(context.Background(), "entity.toggle", connectorsdk.ActionRequest{TargetID: "light.qa"})
	if err != nil || result.Status != "failed" || !strings.Contains(result.Message, "invalid home assistant token") || called {
		t.Fatalf("invalid token was not rejected before network use: result=%+v err=%v called=%v", result, err, called)
	}
}

func TestConnectorActionResponseHasSmallerBoundThanInventory(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maxActionResponseBytes+1)))
	}))
	defer server.Close()

	connector := NewWithConfig(Config{BaseURL: server.URL, Token: "disposable", SkipVerify: true})
	result, err := connector.ExecuteAction(context.Background(), "entity.toggle", connectorsdk.ActionRequest{TargetID: "light.qa"})
	if err != nil || result.Status != "failed" || !strings.Contains(result.Message, "exceeded") {
		t.Fatalf("oversized action response was not rejected: result=%+v err=%v", result, err)
	}
}

func TestConnectorDiscoverCapturesEntityMetadata(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")

	statesPayload := []map[string]any{
		{
			"entity_id":    "sensor.office_temp",
			"state":        "23.5",
			"last_changed": "2026-03-09T09:15:00Z",
			"last_updated": "2026-03-09T09:16:00Z",
			"attributes": map[string]any{
				"friendly_name":         "Office Temperature",
				"unit_of_measurement":   "C",
				"device_class":          "temperature",
				"state_class":           "measurement",
				"entity_category":       "diagnostic",
				"supported_color_modes": []string{"brightness", "color_temp"},
			},
		},
	}
	payload, err := json.Marshal(statesPayload)
	if err != nil {
		t.Fatalf("Marshal(statesPayload) error = %v", err)
	}

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/states" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	connector := NewWithConfig(Config{
		BaseURL:    server.URL,
		Token:      "token-1",
		SkipVerify: true,
		Timeout:    2 * time.Second,
	})

	assets, err := connector.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() unexpected error: %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("Discover() asset count = %d, want 1", len(assets))
	}

	asset := assets[0]
	if asset.Name != "Office Temperature" {
		t.Fatalf("asset.Name = %q, want Office Temperature", asset.Name)
	}
	if asset.Metadata["entity_id"] != "sensor.office_temp" {
		t.Fatalf("entity_id metadata = %q, want sensor.office_temp", asset.Metadata["entity_id"])
	}
	if asset.Metadata["domain"] != "sensor" {
		t.Fatalf("domain metadata = %q, want sensor", asset.Metadata["domain"])
	}
	if asset.Metadata["state"] != "23.5" {
		t.Fatalf("state metadata = %q, want 23.5", asset.Metadata["state"])
	}
	if asset.Metadata["last_changed"] != "2026-03-09T09:15:00Z" {
		t.Fatalf("last_changed metadata = %q", asset.Metadata["last_changed"])
	}
	if asset.Metadata["last_updated"] != "2026-03-09T09:16:00Z" {
		t.Fatalf("last_updated metadata = %q", asset.Metadata["last_updated"])
	}
	if asset.Metadata["unit_of_measurement"] != "C" {
		t.Fatalf("unit_of_measurement metadata = %q, want C", asset.Metadata["unit_of_measurement"])
	}
	if asset.Metadata["device_class"] != "temperature" {
		t.Fatalf("device_class metadata = %q, want temperature", asset.Metadata["device_class"])
	}
	if asset.Metadata["state_class"] != "measurement" {
		t.Fatalf("state_class metadata = %q, want measurement", asset.Metadata["state_class"])
	}
	if asset.Metadata["entity_category"] != "diagnostic" {
		t.Fatalf("entity_category metadata = %q, want diagnostic", asset.Metadata["entity_category"])
	}
	if asset.Metadata["supported_color_modes"] != `["brightness","color_temp"]` {
		t.Fatalf("supported_color_modes metadata = %q", asset.Metadata["supported_color_modes"])
	}
}

func TestConnectorFetchSupervisorStatsReturnsCPUMemDisk(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/hassio/core/stats":
			resp := map[string]any{
				"result": "ok",
				"data": map[string]any{
					"cpu_percent":  38.5,
					"memory_usage": 1073741824,
					"memory_limit": 2147483648,
				},
			}
			payload, _ := json.Marshal(resp)
			_, _ = w.Write(payload)
		case "/api/hassio/host/info":
			resp := map[string]any{
				"result": "ok",
				"data": map[string]any{
					"hostname":         "homeassistant",
					"operating_system": "Home Assistant OS 12.1",
					"disk_total":       32.0,
					"disk_used":        14.4,
					"disk_free":        17.6,
				},
			}
			payload, _ := json.Marshal(resp)
			_, _ = w.Write(payload)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	connector := NewWithConfig(Config{
		BaseURL:    server.URL,
		Token:      "token-1",
		SkipVerify: true,
		Timeout:    2 * time.Second,
	})

	stats, err := connector.FetchSupervisorStats(context.Background())
	if err != nil {
		t.Fatalf("FetchSupervisorStats() unexpected error: %v", err)
	}
	if !stats.Available {
		t.Fatal("Available = false, want true")
	}
	if stats.CPUPercent < 38 || stats.CPUPercent > 39 {
		t.Fatalf("CPUPercent = %f, want ~38.5", stats.CPUPercent)
	}
	if stats.MemoryUsedPercent < 49 || stats.MemoryUsedPercent > 51 {
		t.Fatalf("MemoryUsedPercent = %f, want ~50", stats.MemoryUsedPercent)
	}
	if stats.DiskUsedPercent < 44 || stats.DiskUsedPercent > 46 {
		t.Fatalf("DiskUsedPercent = %f, want ~45", stats.DiskUsedPercent)
	}
	if stats.OSName != "Home Assistant OS 12.1" {
		t.Fatalf("OSName = %q, want Home Assistant OS 12.1", stats.OSName)
	}
	if stats.Hostname != "homeassistant" {
		t.Fatalf("Hostname = %q, want homeassistant", stats.Hostname)
	}
}

func TestConnectorFetchSupervisorStatsUnavailable(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	connector := NewWithConfig(Config{
		BaseURL:    server.URL,
		Token:      "token-1",
		SkipVerify: true,
		Timeout:    2 * time.Second,
	})

	stats, err := connector.FetchSupervisorStats(context.Background())
	if err != nil {
		t.Fatalf("FetchSupervisorStats() should not error on 404: %v", err)
	}
	if stats.Available {
		t.Fatal("Available = true, want false for non-supervised install")
	}
}

func TestConnectorFetchConfigReturnsVersionAndLocation(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")

	configPayload := map[string]any{
		"version":       "2025.3.1",
		"location_name": "My Home",
		"time_zone":     "Australia/Melbourne",
		"unit_system":   map[string]any{"temperature": "°C"},
	}
	payload, err := json.Marshal(configPayload)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/config" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	connector := NewWithConfig(Config{
		BaseURL:    server.URL,
		Token:      "token-1",
		SkipVerify: true,
		Timeout:    2 * time.Second,
	})

	config, err := connector.FetchConfig(context.Background())
	if err != nil {
		t.Fatalf("FetchConfig() unexpected error: %v", err)
	}
	if config.Version != "2025.3.1" {
		t.Fatalf("Version = %q, want 2025.3.1", config.Version)
	}
	if config.LocationName != "My Home" {
		t.Fatalf("LocationName = %q, want My Home", config.LocationName)
	}
}

func TestConnectorUnconfiguredModeFailsClosed(t *testing.T) {
	connector := NewWithConfig(Config{})

	health, err := connector.TestConnection(context.Background())
	if err != nil || health.Status != "failed" || !strings.Contains(health.Message, "not configured") {
		t.Fatalf("TestConnection() = %+v, err=%v, want failed unconfigured health", health, err)
	}

	assets, err := connector.Discover(context.Background())
	if err != nil || assets == nil || len(assets) != 0 {
		t.Fatalf("Discover() = %+v, err=%v, want non-nil empty inventory", assets, err)
	}

	for _, descriptor := range connector.Actions() {
		for _, dryRun := range []bool{false, true} {
			result, execErr := connector.ExecuteAction(context.Background(), descriptor.ID, connectorsdk.ActionRequest{
				TargetID: "light.disposable",
				Params:   map[string]string{"service": "light.turn_on"},
				DryRun:   dryRun,
			})
			if execErr != nil || result.Status != "failed" || !strings.Contains(result.Message, "not configured") {
				t.Fatalf("ExecuteAction(%q, dry_run=%v) = %+v, err=%v, want fail-closed unconfigured result", descriptor.ID, dryRun, result, execErr)
			}
		}
	}
}
