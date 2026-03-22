package portainer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/connectorsdk"
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

func TestConnectorStubMode(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	// New() with no env vars should return a stub connector.
	c := New()

	assets, err := c.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("expected 1 stub asset, got %d", len(assets))
	}
	if assets[0].Source != "portainer" {
		t.Fatalf("expected source 'portainer', got %q", assets[0].Source)
	}
	if assets[0].ID != "portainer-endpoint-stub" {
		t.Fatalf("expected stub asset ID, got %q", assets[0].ID)
	}
	if assets[0].Type != "container-host" {
		t.Fatalf("expected type 'container-host', got %q", assets[0].Type)
	}
	if assets[0].Metadata["note"] == "" {
		t.Fatal("expected stub asset to have a 'note' metadata field")
	}

	// TestConnection in stub mode should return ok.
	health, err := c.TestConnection(context.Background())
	if err != nil {
		t.Fatalf("TestConnection failed: %v", err)
	}
	if health.Status != "ok" {
		t.Fatalf("expected status 'ok', got %q", health.Status)
	}
	if !strings.Contains(health.Message, "stub mode") {
		t.Fatalf("expected stub mode message, got %q", health.Message)
	}
}

func TestConnectorDiscover(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	mux := http.NewServeMux()

	// Endpoints
	mux.HandleFunc("/api/endpoints", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Endpoint{
			{ID: 1, Name: "local", Type: 1, URL: "unix:///var/run/docker.sock", Status: 1},
		})
	})

	// Containers for endpoint 1
	mux.HandleFunc("/api/endpoints/1/docker/containers/json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Container{
			{
				ID:      "abc123def456789012345678",
				Names:   []string{"/nginx-proxy"},
				Image:   "nginx:latest",
				State:   "running",
				Status:  "Up 2 hours",
				Created: 1710000000,
				Ports: []ContainerPort{
					{PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
				},
				Labels: map[string]string{"com.docker.compose.project": "web", "app": "gateway"},
			},
		})
	})

	// Stacks
	mux.HandleFunc("/api/stacks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Stack{
			{
				ID:         5,
				Name:       "monitoring",
				Type:       2,
				EndpointID: 1,
				Status:     1,
				EntryPoint: "docker-compose.yml",
				CreatedBy:  "admin",
				GitConfig: &struct {
					URL string `json:"URL"`
				}{URL: "https://github.com/example/monitoring.git"},
			},
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Timeout: 5 * time.Second,
	})
	c := NewWithClient(client)

	assets, err := c.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	// Should have 3 assets: 1 endpoint + 1 container + 1 stack.
	if len(assets) != 3 {
		t.Fatalf("expected 3 assets, got %d", len(assets))
	}

	// Verify endpoint asset.
	ep := assets[0]
	if ep.Type != "container-host" {
		t.Fatalf("expected type 'container-host', got %q", ep.Type)
	}
	if ep.ID != "portainer-endpoint-1" {
		t.Fatalf("expected ID 'portainer-endpoint-1', got %q", ep.ID)
	}
	if ep.Metadata["type"] != "docker" {
		t.Fatalf("expected metadata type 'docker', got %q", ep.Metadata["type"])
	}
	if ep.Metadata["status"] != "up" {
		t.Fatalf("expected metadata status 'up', got %q", ep.Metadata["status"])
	}

	// Verify container asset.
	ctr := assets[1]
	if ctr.Type != "container" {
		t.Fatalf("expected type 'container', got %q", ctr.Type)
	}
	if ctr.ID != "portainer-container-1-abc123def456" {
		t.Fatalf("expected ID 'portainer-container-1-abc123def456', got %q", ctr.ID)
	}
	if ctr.Name != "nginx-proxy" {
		t.Fatalf("expected name 'nginx-proxy', got %q", ctr.Name)
	}
	if ctr.Metadata["image"] != "nginx:latest" {
		t.Fatalf("expected image 'nginx:latest', got %q", ctr.Metadata["image"])
	}
	if ctr.Metadata["stack"] != "web" {
		t.Fatalf("expected stack 'web', got %q", ctr.Metadata["stack"])
	}
	if ctr.Metadata["container_id"] != "abc123def456789012345678" {
		t.Fatalf("expected full container_id, got %q", ctr.Metadata["container_id"])
	}
	if ctr.Metadata["created_at"] != "2024-03-09T16:00:00Z" {
		t.Fatalf("expected created_at metadata, got %q", ctr.Metadata["created_at"])
	}
	if ctr.Metadata["ports"] != "8080->80/tcp" {
		t.Fatalf("expected ports metadata, got %q", ctr.Metadata["ports"])
	}
	if !strings.Contains(ctr.Metadata["labels_json"], "\"app\":\"gateway\"") {
		t.Fatalf("expected labels_json metadata, got %q", ctr.Metadata["labels_json"])
	}

	// Verify stack asset.
	stk := assets[2]
	if stk.Type != "stack" {
		t.Fatalf("expected type 'stack', got %q", stk.Type)
	}
	if stk.ID != "portainer-stack-5" {
		t.Fatalf("expected ID 'portainer-stack-5', got %q", stk.ID)
	}
	if stk.Metadata["type"] != "compose" {
		t.Fatalf("expected metadata type 'compose', got %q", stk.Metadata["type"])
	}
	if stk.Metadata["status"] != "active" {
		t.Fatalf("expected metadata status 'active', got %q", stk.Metadata["status"])
	}
	if stk.Metadata["git_url"] != "https://github.com/example/monitoring.git" {
		t.Fatalf("expected git_url, got %q", stk.Metadata["git_url"])
	}
}

func TestConnectorContainerRestart(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	var capturedPath string
	var capturedMethod string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
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
	})
	if err != nil {
		t.Fatalf("ExecuteAction failed: %v", err)
	}
	if result.Status != "succeeded" {
		t.Fatalf("expected status 'succeeded', got %q: %s", result.Status, result.Message)
	}
	if capturedMethod != http.MethodPost {
		t.Fatalf("expected POST, got %s", capturedMethod)
	}
	expectedPath := "/api/endpoints/1/docker/containers/abc123def456/restart"
	if capturedPath != expectedPath {
		t.Fatalf("expected path %q, got %q", expectedPath, capturedPath)
	}
}

func TestConnectorStackStop(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	var capturedPath string
	var capturedMethod string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedMethod = r.Method
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Timeout: 5 * time.Second,
	})
	c := NewWithClient(client)

	result, err := c.ExecuteAction(context.Background(), "stack.stop", connectorsdk.ActionRequest{
		TargetID: "portainer-stack-5",
		Params: map[string]string{
			"endpoint_id": "1",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteAction failed: %v", err)
	}
	if result.Status != "succeeded" {
		t.Fatalf("expected status 'succeeded', got %q: %s", result.Status, result.Message)
	}
	if capturedMethod != http.MethodPost {
		t.Fatalf("expected POST, got %s", capturedMethod)
	}
	expectedPath := "/api/stacks/5/stop"
	if capturedPath != expectedPath {
		t.Fatalf("expected path %q, got %q", expectedPath, capturedPath)
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

func TestParseContainerTarget(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	tests := []struct {
		name        string
		target      string
		wantEpID    int
		wantCtrID   string
		wantErr     bool
		errContains string
	}{
		{
			name:      "valid short id",
			target:    "portainer-container-1-abc123def456",
			wantEpID:  1,
			wantCtrID: "abc123def456",
		},
		{
			name:      "valid multi-digit endpoint",
			target:    "portainer-container-42-deadbeef1234",
			wantEpID:  42,
			wantCtrID: "deadbeef1234",
		},
		{
			name:      "valid full container id with dashes",
			target:    "portainer-container-1-abc123def456-extra-chars",
			wantEpID:  1,
			wantCtrID: "abc123def456-extra-chars",
		},
		{
			name:        "missing prefix",
			target:      "container-1-abc123",
			wantErr:     true,
			errContains: "invalid container target format",
		},
		{
			name:        "wrong prefix",
			target:      "portainer-stack-1-abc123",
			wantErr:     true,
			errContains: "invalid container target format",
		},
		{
			name:        "no endpoint id",
			target:      "portainer-container-",
			wantErr:     true,
			errContains: "invalid container target format",
		},
		{
			name:        "non-numeric endpoint id",
			target:      "portainer-container-abc-def",
			wantErr:     true,
			errContains: "invalid endpoint ID",
		},
		{
			name:        "empty target",
			target:      "",
			wantErr:     true,
			errContains: "invalid container target format",
		},
		{
			name:        "endpoint only no container",
			target:      "portainer-container-1",
			wantErr:     true,
			errContains: "invalid container target format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			epID, ctrID, err := parseContainerTarget(tt.target)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if epID != tt.wantEpID {
				t.Fatalf("expected endpoint ID %d, got %d", tt.wantEpID, epID)
			}
			if ctrID != tt.wantCtrID {
				t.Fatalf("expected container ID %q, got %q", tt.wantCtrID, ctrID)
			}
		})
	}
}

func TestParseStackTarget(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	tests := []struct {
		name        string
		target      string
		wantID      int
		wantErr     bool
		errContains string
	}{
		{
			name:   "valid",
			target: "portainer-stack-5",
			wantID: 5,
		},
		{
			name:   "valid large id",
			target: "portainer-stack-999",
			wantID: 999,
		},
		{
			name:        "missing prefix",
			target:      "stack-5",
			wantErr:     true,
			errContains: "invalid stack target format",
		},
		{
			name:        "wrong prefix",
			target:      "portainer-container-5",
			wantErr:     true,
			errContains: "invalid stack target format",
		},
		{
			name:        "non-numeric id",
			target:      "portainer-stack-abc",
			wantErr:     true,
			errContains: "invalid stack ID",
		},
		{
			name:        "empty target",
			target:      "",
			wantErr:     true,
			errContains: "invalid stack target format",
		},
		{
			name:        "no id",
			target:      "portainer-stack-",
			wantErr:     true,
			errContains: "invalid stack ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := parseStackTarget(tt.target)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if id != tt.wantID {
				t.Fatalf("expected stack ID %d, got %d", tt.wantID, id)
			}
		})
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

func TestDiscoverFallbackAndFailures(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	t.Run("empty discovery falls back to stub", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/endpoints", func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode([]Endpoint{})
		})
		mux.HandleFunc("/api/stacks", func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode([]Stack{})
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		c := NewWithClient(NewClient(Config{
			BaseURL: server.URL,
			APIKey:  "test-key",
			Timeout: 5 * time.Second,
		}))
		assets, err := c.Discover(context.Background())
		if err != nil {
			t.Fatalf("unexpected discover error: %v", err)
		}
		if len(assets) != 1 || assets[0].ID != "portainer-endpoint-stub" {
			t.Fatalf("expected stub asset fallback, got %+v", assets)
		}
	})

	t.Run("endpoint listing failure is fatal", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/endpoints" {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"message":"broken"}`))
				return
			}
			_ = json.NewEncoder(w).Encode([]Stack{})
		}))
		defer server.Close()

		c := NewWithClient(NewClient(Config{
			BaseURL: server.URL,
			APIKey:  "test-key",
			Timeout: 5 * time.Second,
		}))
		_, err := c.Discover(context.Background())
		if err == nil || !strings.Contains(err.Error(), "portainer endpoints") {
			t.Fatalf("expected endpoints failure, got %v", err)
		}
	})

	t.Run("container and stack failures still return endpoint asset", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/endpoints", func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode([]Endpoint{
				{ID: 1, Name: "edge", Type: 4, URL: "tcp://edge:2375", Status: 2},
			})
		})
		mux.HandleFunc("/api/endpoints/1/docker/containers/json", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"message":"containers unavailable"}`))
		})
		mux.HandleFunc("/api/stacks", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"message":"stacks unavailable"}`))
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		c := NewWithClient(NewClient(Config{
			BaseURL: server.URL,
			APIKey:  "test-key",
			Timeout: 5 * time.Second,
		}))

		assets, err := c.Discover(context.Background())
		if err != nil {
			t.Fatalf("unexpected discover error: %v", err)
		}
		if len(assets) != 1 {
			t.Fatalf("expected only endpoint asset, got %d", len(assets))
		}
		if assets[0].Metadata["type"] != "edge-agent" {
			t.Fatalf("expected endpoint type edge-agent, got %q", assets[0].Metadata["type"])
		}
		if assets[0].Metadata["status"] != "down" {
			t.Fatalf("expected endpoint status down, got %q", assets[0].Metadata["status"])
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

func TestDiscoverContainerNameFallback(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/endpoints", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Endpoint{
			{ID: 1, Name: "fallback", Type: 1, URL: "unix:///var/run/docker.sock", Status: 1},
		})
	})
	mux.HandleFunc("/api/endpoints/1/docker/containers/json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Container{
			{
				ID:     "abcdef1234567890",
				Names:  []string{},
				Image:  "busybox:latest",
				State:  "running",
				Status: "Up 1m",
				Labels: map[string]string{},
			},
		})
	})
	mux.HandleFunc("/api/stacks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Stack{})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	c := NewWithClient(NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Timeout: 5 * time.Second,
	}))
	assets, err := c.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if len(assets) != 2 {
		t.Fatalf("expected endpoint + container assets, got %d", len(assets))
	}
	container := assets[1]
	if container.Name != "abcdef123456" {
		t.Fatalf("expected container name fallback to short id, got %q", container.Name)
	}
}

func TestExecuteContainerActionBranches(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	t.Run("invalid target returns failed result", func(t *testing.T) {
		c := NewWithClient(NewClient(Config{
			BaseURL: "https://127.0.0.1:65535",
			APIKey:  "test-key",
		}))
		result, err := c.ExecuteAction(context.Background(), "container.restart", connectorsdk.ActionRequest{
			TargetID: "portainer-container-invalid",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Status != "failed" || !strings.Contains(result.Message, "invalid container target format") {
			t.Fatalf("unexpected result: %+v", result)
		}
	})

	t.Run("remove action forwards force parameter", func(t *testing.T) {
		var capturedQuery string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedQuery = r.URL.RawQuery
			w.WriteHeader(http.StatusNoContent)
		}))
		defer server.Close()

		c := NewWithClient(NewClient(Config{
			BaseURL: server.URL,
			APIKey:  "test-key",
			Timeout: 5 * time.Second,
		}))
		result, err := c.ExecuteAction(context.Background(), "container.remove", connectorsdk.ActionRequest{
			TargetID: "portainer-container-1-abc123def456",
			Params:   map[string]string{"force": "true"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Status != "succeeded" {
			t.Fatalf("unexpected result: %+v", result)
		}
		if !strings.Contains(capturedQuery, "force=true") {
			t.Fatalf("expected force=true query, got %q", capturedQuery)
		}
	})

	t.Run("container remove api error surfaces failed result", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"message":"remove failed"}`))
		}))
		defer server.Close()

		c := NewWithClient(NewClient(Config{
			BaseURL: server.URL,
			APIKey:  "test-key",
			Timeout: 5 * time.Second,
		}))
		result, err := c.ExecuteAction(context.Background(), "container.remove", connectorsdk.ActionRequest{
			TargetID: "portainer-container-1-abc123def456",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Status != "failed" || !strings.Contains(result.Message, "portainer api returned 502") {
			t.Fatalf("unexpected result: %+v", result)
		}
	})

	t.Run("container action api error surfaces failed result", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"message":"action failed"}`))
		}))
		defer server.Close()

		c := NewWithClient(NewClient(Config{
			BaseURL: server.URL,
			APIKey:  "test-key",
			Timeout: 5 * time.Second,
		}))
		result, err := c.ExecuteAction(context.Background(), "container.pause", connectorsdk.ActionRequest{
			TargetID: "portainer-container-1-abc123def456",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Status != "failed" || !strings.Contains(result.Message, "portainer api returned 502") {
			t.Fatalf("unexpected result: %+v", result)
		}
	})
}

func TestExecuteStackActionBranches(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	t.Run("invalid stack target", func(t *testing.T) {
		c := NewWithClient(NewClient(Config{
			BaseURL: "https://127.0.0.1:65535",
			APIKey:  "test-key",
		}))
		result, err := c.ExecuteAction(context.Background(), "stack.start", connectorsdk.ActionRequest{
			TargetID: "stack-1",
			Params:   map[string]string{"endpoint_id": "1"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Status != "failed" || !strings.Contains(result.Message, "invalid stack target format") {
			t.Fatalf("unexpected result: %+v", result)
		}
	})

	t.Run("stack dry run", func(t *testing.T) {
		c := NewWithClient(NewClient(Config{
			BaseURL: "https://127.0.0.1:65535",
			APIKey:  "test-key",
		}))
		result, err := c.ExecuteAction(context.Background(), "stack.start", connectorsdk.ActionRequest{
			TargetID: "portainer-stack-5",
			DryRun:   true,
			Params:   map[string]string{"endpoint_id": "1"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Status != "succeeded" || !strings.Contains(result.Output, "would execute") {
			t.Fatalf("unexpected dry-run result: %+v", result)
		}
	})

	t.Run("stack start succeeds", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/stacks/5/start" {
				t.Fatalf("unexpected path %s", r.URL.Path)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}))
		defer server.Close()

		c := NewWithClient(NewClient(Config{
			BaseURL: server.URL,
			APIKey:  "test-key",
			Timeout: 5 * time.Second,
		}))
		result, err := c.ExecuteAction(context.Background(), "stack.start", connectorsdk.ActionRequest{
			TargetID: "portainer-stack-5",
			Params:   map[string]string{"endpoint_id": "1"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Status != "succeeded" {
			t.Fatalf("unexpected result: %+v", result)
		}
	})

	t.Run("stack start api error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"message":"start failed"}`))
		}))
		defer server.Close()

		c := NewWithClient(NewClient(Config{
			BaseURL: server.URL,
			APIKey:  "test-key",
			Timeout: 5 * time.Second,
		}))
		result, err := c.ExecuteAction(context.Background(), "stack.start", connectorsdk.ActionRequest{
			TargetID: "portainer-stack-5",
			Params:   map[string]string{"endpoint_id": "1"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Status != "failed" || !strings.Contains(result.Message, "portainer api returned 502") {
			t.Fatalf("unexpected result: %+v", result)
		}
	})

	t.Run("stack stop api error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"message":"stop failed"}`))
		}))
		defer server.Close()

		c := NewWithClient(NewClient(Config{
			BaseURL: server.URL,
			APIKey:  "test-key",
			Timeout: 5 * time.Second,
		}))
		result, err := c.ExecuteAction(context.Background(), "stack.stop", connectorsdk.ActionRequest{
			TargetID: "portainer-stack-5",
			Params:   map[string]string{"endpoint_id": "1"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Status != "failed" || !strings.Contains(result.Message, "portainer api returned 502") {
			t.Fatalf("unexpected result: %+v", result)
		}
	})

	t.Run("stack redeploy supports pull_image=false", func(t *testing.T) {
		var capturedBody map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPut {
				t.Fatalf("expected PUT, got %s", r.Method)
			}
			if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}))
		defer server.Close()

		c := NewWithClient(NewClient(Config{
			BaseURL: server.URL,
			APIKey:  "test-key",
			Timeout: 5 * time.Second,
		}))
		result, err := c.ExecuteAction(context.Background(), "stack.redeploy", connectorsdk.ActionRequest{
			TargetID: "portainer-stack-5",
			Params: map[string]string{
				"endpoint_id": "1",
				"pull_image":  "false",
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Status != "succeeded" {
			t.Fatalf("unexpected result: %+v", result)
		}
		if capturedBody["pullImage"] != false {
			t.Fatalf("expected pullImage=false, got %#v", capturedBody["pullImage"])
		}
	})

	t.Run("stack redeploy api error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"message":"redeploy failed"}`))
		}))
		defer server.Close()

		c := NewWithClient(NewClient(Config{
			BaseURL: server.URL,
			APIKey:  "test-key",
			Timeout: 5 * time.Second,
		}))
		result, err := c.ExecuteAction(context.Background(), "stack.redeploy", connectorsdk.ActionRequest{
			TargetID: "portainer-stack-5",
			Params: map[string]string{
				"endpoint_id": "1",
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Status != "failed" || !strings.Contains(result.Message, "portainer api returned 502") {
			t.Fatalf("unexpected result: %+v", result)
		}
	})

	t.Run("stack remove api error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"message":"remove failed"}`))
		}))
		defer server.Close()

		c := NewWithClient(NewClient(Config{
			BaseURL: server.URL,
			APIKey:  "test-key",
			Timeout: 5 * time.Second,
		}))
		result, err := c.ExecuteAction(context.Background(), "stack.remove", connectorsdk.ActionRequest{
			TargetID: "portainer-stack-5",
			Params:   map[string]string{"endpoint_id": "1"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Status != "failed" || !strings.Contains(result.Message, "portainer api returned 502") {
			t.Fatalf("unexpected result: %+v", result)
		}
	})

	t.Run("unsupported stack action", func(t *testing.T) {
		c := NewWithClient(NewClient(Config{
			BaseURL: "https://127.0.0.1:65535",
			APIKey:  "test-key",
		}))
		result, err := c.ExecuteAction(context.Background(), "stack.unknown", connectorsdk.ActionRequest{
			TargetID: "portainer-stack-5",
			Params:   map[string]string{"endpoint_id": "1"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Status != "failed" || !strings.Contains(result.Message, "unsupported stack action") {
			t.Fatalf("unexpected result: %+v", result)
		}
	})
}

func TestParseContainerTargetEmptyContainerID(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	_, _, err := parseContainerTarget("portainer-container-1-")
	if err == nil || !strings.Contains(err.Error(), "empty container ID") {
		t.Fatalf("expected empty container ID error, got %v", err)
	}
}
