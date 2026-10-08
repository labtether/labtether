package portainer

import (
	"context"
	"github.com/labtether/labtether/internal/connectorsdk"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestConnectorID(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	c := &Connector{}
	if c.ID() != "portainer" {
		t.Fatalf("expected ID 'portainer', got %q", c.ID())
	}
	if c.DisplayName() != "Portainer" {
		t.Fatalf("expected DisplayName 'Portainer', got %q", c.DisplayName())
	}
}

func TestConnectorUnconfiguredModeFailsClosed(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	c := NewWithClient(nil)

	assets, err := c.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}
	if assets == nil || len(assets) != 0 {
		t.Fatalf("expected non-nil empty unconfigured inventory, got %+v", assets)
	}

	health, err := c.TestConnection(context.Background())
	if err != nil {
		t.Fatalf("TestConnection failed: %v", err)
	}
	if health.Status != "failed" {
		t.Fatalf("expected status 'failed', got %q", health.Status)
	}
	if !strings.Contains(health.Message, "not configured") {
		t.Fatalf("expected unconfigured message, got %q", health.Message)
	}

	for _, descriptor := range c.Actions() {
		for _, dryRun := range []bool{false, true} {
			result, execErr := c.ExecuteAction(context.Background(), descriptor.ID, connectorsdk.ActionRequest{DryRun: dryRun})
			if execErr != nil || result.Status != "failed" || !strings.Contains(result.Message, "not configured") {
				t.Fatalf("ExecuteAction(%q, dry_run=%v) = %+v, err=%v, want fail-closed unconfigured result", descriptor.ID, dryRun, result, execErr)
			}
		}
	}
}

func TestConnectorDryRun(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	// Server should NOT be called during dry run.
	serverCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Timeout: 5 * time.Second,
	})
	c := NewWithClient(client)

	result, err := c.ExecuteAction(context.Background(), "container.restart", connectorsdk.ActionRequest{
		TargetID: "portainer-container-1-abc123def456",
		DryRun:   true,
	})
	if err != nil {
		t.Fatalf("ExecuteAction failed: %v", err)
	}
	if result.Status != "succeeded" {
		t.Fatalf("expected status 'succeeded', got %q: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Output, "would execute") {
		t.Fatalf("expected 'would execute' in output, got %q", result.Output)
	}
	if serverCalled {
		t.Fatal("server should not be called during dry run")
	}
}

func TestCapabilitiesAndActions(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	c := &Connector{}

	caps := c.Capabilities()
	if !caps.DiscoverAssets || !caps.CollectMetrics || !caps.CollectEvents || !caps.ExecuteActions {
		t.Fatalf("unexpected capabilities: %+v", caps)
	}

	actions := c.Actions()
	if len(actions) != 11 {
		t.Fatalf("expected 11 actions, got %d", len(actions))
	}

	var hasContainerRemove bool
	var hasStackRedeploy bool
	for _, action := range actions {
		switch action.ID {
		case "container.remove":
			hasContainerRemove = true
			if len(action.Parameters) != 1 || action.Parameters[0].Key != "force" {
				t.Fatalf("unexpected container.remove params: %+v", action.Parameters)
			}
		case "stack.redeploy":
			hasStackRedeploy = true
			if len(action.Parameters) != 1 || action.Parameters[0].Key != "pull_image" {
				t.Fatalf("unexpected stack.redeploy params: %+v", action.Parameters)
			}
		}
	}
	if !hasContainerRemove {
		t.Fatalf("expected container.remove action")
	}
	if !hasStackRedeploy {
		t.Fatalf("expected stack.redeploy action")
	}
}

func TestNewUsesEnvironmentConfiguration(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	t.Setenv("PORTAINER_BASE_URL", " https://portainer.lab:9443/ ")
	t.Setenv("PORTAINER_API_KEY", "  ptr_key  ")
	t.Setenv("PORTAINER_SKIP_VERIFY", "true")
	t.Setenv("PORTAINER_HTTP_TIMEOUT", "17s")

	c := New()
	if c == nil || c.client == nil {
		t.Fatalf("expected configured client")
	}
	if c.client.baseURL != "https://portainer.lab:9443" {
		t.Fatalf("unexpected baseURL: %q", c.client.baseURL)
	}
	if c.client.apiKey != "ptr_key" {
		t.Fatalf("unexpected api key: %q", c.client.apiKey)
	}
	if c.client.httpClient.Timeout != 17*time.Second {
		t.Fatalf("unexpected timeout: %v", c.client.httpClient.Timeout)
	}

	transport, ok := c.client.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", c.client.httpClient.Transport)
	}
	if transport.TLSClientConfig == nil || !transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatalf("expected skip verify enabled")
	}
}

func TestNewInvalidEnvironmentFallsBackToDefaults(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	t.Setenv("PORTAINER_BASE_URL", "https://portainer.lab:9443")
	t.Setenv("PORTAINER_API_KEY", "ptr_key")
	t.Setenv("PORTAINER_SKIP_VERIFY", "definitely-not-bool")
	t.Setenv("PORTAINER_HTTP_TIMEOUT", "bad-duration")

	c := New()
	if c == nil || c.client == nil {
		t.Fatalf("expected configured client")
	}
	if c.client.httpClient.Timeout != 10*time.Second {
		t.Fatalf("expected default timeout 10s, got %v", c.client.httpClient.Timeout)
	}

	transport, ok := c.client.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", c.client.httpClient.Transport)
	}
	if transport.TLSClientConfig == nil {
		t.Fatalf("expected TLS config")
	}
	if transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatalf("expected skip verify to remain false on invalid env value")
	}
}

func TestTestConnectionConfiguredPaths(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	t.Run("version included", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/system/version", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"ServerVersion":"2.30.0"}`))
		})
		mux.HandleFunc("/api/endpoints", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`[{"Id":1,"Name":"edge"}]`))
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		c := NewWithClient(NewClient(Config{
			BaseURL: server.URL,
			APIKey:  "test-key",
			Timeout: 5 * time.Second,
		}))

		health, err := c.TestConnection(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if health.Status != "ok" {
			t.Fatalf("expected ok health, got %+v", health)
		}
		if !strings.Contains(health.Message, "v2.30.0") {
			t.Fatalf("expected version in message, got %q", health.Message)
		}
		if !strings.Contains(health.Message, "1 endpoint available") {
			t.Fatalf("expected endpoint count in message, got %q", health.Message)
		}
	})

	t.Run("empty version still healthy when endpoints are visible", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/system/version", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"ServerVersion":""}`))
		})
		mux.HandleFunc("/api/endpoints", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`[{"Id":1,"Name":"edge"},{"Id":2,"Name":"lab"}]`))
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		c := NewWithClient(NewClient(Config{
			BaseURL: server.URL,
			APIKey:  "test-key",
			Timeout: 5 * time.Second,
		}))

		health, err := c.TestConnection(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if health.Message != "portainer API reachable (2 endpoints available)" {
			t.Fatalf("unexpected health message: %q", health.Message)
		}
	})

	t.Run("api failure returns failed status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"message":"backend down"}`))
		}))
		defer server.Close()

		c := NewWithClient(NewClient(Config{
			BaseURL: server.URL,
			APIKey:  "test-key",
			Timeout: 5 * time.Second,
		}))

		health, err := c.TestConnection(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if health.Status != "failed" {
			t.Fatalf("expected failed status, got %+v", health)
		}
		if !strings.Contains(strings.ToLower(health.Message), "portainer api returned") {
			t.Fatalf("unexpected failed message: %q", health.Message)
		}
	})

	t.Run("missing endpoint visibility returns failed status", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/system/version", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"ServerVersion":"2.30.0"}`))
		})
		mux.HandleFunc("/api/endpoints", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`[]`))
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		c := NewWithClient(NewClient(Config{
			BaseURL: server.URL,
			APIKey:  "test-key",
			Timeout: 5 * time.Second,
		}))

		health, err := c.TestConnection(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if health.Status != "failed" {
			t.Fatalf("expected failed status, got %+v", health)
		}
		if !strings.Contains(health.Message, "no endpoints are visible") {
			t.Fatalf("unexpected failed message: %q", health.Message)
		}
	})
}

func TestExecuteActionFailureBranches(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	t.Run("not configured connector", func(t *testing.T) {
		c := &Connector{}
		result, err := c.ExecuteAction(context.Background(), "container.restart", connectorsdk.ActionRequest{
			TargetID: "portainer-container-1-abc123",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Status != "failed" {
			t.Fatalf("expected failed status, got %+v", result)
		}
	})

	t.Run("unsupported action type", func(t *testing.T) {
		c := NewWithClient(NewClient(Config{
			BaseURL: "https://127.0.0.1:65535",
			APIKey:  "test-key",
		}))
		result, err := c.ExecuteAction(context.Background(), "database.repair", connectorsdk.ActionRequest{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Status != "failed" || !strings.Contains(result.Message, "unsupported action") {
			t.Fatalf("unexpected result: %+v", result)
		}
	})

	t.Run("stack action requires endpoint id", func(t *testing.T) {
		c := NewWithClient(NewClient(Config{
			BaseURL: "https://127.0.0.1:65535",
			APIKey:  "test-key",
		}))
		result, err := c.ExecuteAction(context.Background(), "stack.stop", connectorsdk.ActionRequest{
			TargetID: "portainer-stack-5",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Status != "failed" || !strings.Contains(result.Message, "endpoint_id") {
			t.Fatalf("unexpected result: %+v", result)
		}
	})

	t.Run("stack action rejects invalid endpoint id", func(t *testing.T) {
		c := NewWithClient(NewClient(Config{
			BaseURL: "https://127.0.0.1:65535",
			APIKey:  "test-key",
		}))
		result, err := c.ExecuteAction(context.Background(), "stack.stop", connectorsdk.ActionRequest{
			TargetID: "portainer-stack-5",
			Params: map[string]string{
				"endpoint_id": "abc",
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Status != "failed" || !strings.Contains(result.Message, "invalid endpoint_id") {
			t.Fatalf("unexpected result: %+v", result)
		}
	})
}

func TestTypeAndStatusStringMappings(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "endpoint docker", got: endpointTypeString(1), want: "docker"},
		{name: "endpoint agent", got: endpointTypeString(2), want: "agent"},
		{name: "endpoint azure", got: endpointTypeString(3), want: "azure"},
		{name: "endpoint edge-agent", got: endpointTypeString(4), want: "edge-agent"},
		{name: "endpoint kubernetes", got: endpointTypeString(5), want: "kubernetes"},
		{name: "endpoint unknown", got: endpointTypeString(99), want: "unknown(99)"},
		{name: "endpoint status up", got: endpointStatusString(1), want: "up"},
		{name: "endpoint status down", got: endpointStatusString(2), want: "down"},
		{name: "endpoint status unknown", got: endpointStatusString(8), want: "unknown(8)"},
		{name: "stack type swarm", got: stackTypeString(1), want: "swarm"},
		{name: "stack type compose", got: stackTypeString(2), want: "compose"},
		{name: "stack type kubernetes", got: stackTypeString(3), want: "kubernetes"},
		{name: "stack type unknown", got: stackTypeString(6), want: "unknown(6)"},
		{name: "stack status active", got: stackStatusString(1), want: "active"},
		{name: "stack status inactive", got: stackStatusString(2), want: "inactive"},
		{name: "stack status unknown", got: stackStatusString(9), want: "unknown(9)"},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Fatalf("%s: expected %q, got %q", tt.name, tt.want, tt.got)
		}
	}
}
