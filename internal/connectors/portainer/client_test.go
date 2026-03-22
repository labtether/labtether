package portainer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func allowInsecureTransportForPortainerTests(t *testing.T) {
	t.Helper()
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
}

func TestIsConfigured(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	tests := []struct {
		name   string
		client *Client
		want   bool
	}{
		{"nil client", nil, false},
		{"empty config", NewClient(Config{}), false},
		{"baseURL only", NewClient(Config{BaseURL: "https://localhost:9443"}), false},
		{"apiKey auth", NewClient(Config{BaseURL: "https://localhost:9443", APIKey: "key123"}), true},
		{"jwt auth", NewClient(Config{BaseURL: "https://localhost:9443", Username: "admin", Password: "pass"}), true},
		{"username without password", NewClient(Config{BaseURL: "https://localhost:9443", Username: "admin"}), false},
		{"password without username", NewClient(Config{BaseURL: "https://localhost:9443", Password: "pass"}), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.client.IsConfigured(); got != tt.want {
				t.Fatalf("IsConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAPIKeyAuth(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	const apiKey = "ptr_abc123"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != apiKey {
			t.Fatalf("expected X-API-Key=%q, got %q", apiKey, got)
		}
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Fatalf("expected no Authorization header, got %q", auth)
		}
		_, _ = w.Write([]byte(`{"ServerVersion":"2.21.0","DatabaseVersion":"100","Build":{"BuildNumber":"1234","GoVersion":"go1.21"}}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  apiKey,
		Timeout: 5 * time.Second,
	})

	info, err := client.GetVersion(context.Background())
	if err != nil {
		t.Fatalf("GetVersion failed: %v", err)
	}
	if info.ServerVersion != "2.21.0" {
		t.Fatalf("unexpected server version: %s", info.ServerVersion)
	}
}

func TestJWTAuth(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	var authCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth":
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST for /api/auth, got %s", r.Method)
			}
			var creds struct {
				Username string `json:"Username"`
				Password string `json:"Password"`
			}
			if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
				t.Fatalf("decode auth body: %v", err)
			}
			if creds.Username != "admin" || creds.Password != "secret" {
				t.Fatalf("unexpected credentials: %+v", creds)
			}
			authCalls.Add(1)
			_, _ = w.Write([]byte(`{"jwt":"eyJhbGciOiJIUzI1NiJ9.test.sig"}`))

		case "/api/system/version":
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				t.Fatalf("expected Bearer token, got %q", auth)
			}
			_, _ = w.Write([]byte(`{"ServerVersion":"2.21.0","DatabaseVersion":"100","Build":{"BuildNumber":"1234","GoVersion":"go1.21"}}`))

		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL:  server.URL,
		Username: "admin",
		Password: "secret",
		Timeout:  5 * time.Second,
	})

	// First call should trigger auth.
	info, err := client.GetVersion(context.Background())
	if err != nil {
		t.Fatalf("first GetVersion failed: %v", err)
	}
	if info.ServerVersion != "2.21.0" {
		t.Fatalf("unexpected server version: %s", info.ServerVersion)
	}

	// Second call should reuse cached JWT (no additional auth call).
	_, err = client.GetVersion(context.Background())
	if err != nil {
		t.Fatalf("second GetVersion failed: %v", err)
	}

	if authCalls.Load() != 1 {
		t.Fatalf("expected exactly 1 auth call, got %d", authCalls.Load())
	}
}

func TestGetEndpoints(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/endpoints" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[
			{"Id":1,"Name":"local","Type":1,"URL":"unix:///var/run/docker.sock","Status":1},
			{"Id":2,"Name":"remote","Type":2,"URL":"tcp://192.168.1.100:2375","Status":1}
		]`))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "key",
		Timeout: 5 * time.Second,
	})

	endpoints, err := client.GetEndpoints(context.Background())
	if err != nil {
		t.Fatalf("GetEndpoints failed: %v", err)
	}
	if len(endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(endpoints))
	}
	if endpoints[0].ID != 1 || endpoints[0].Name != "local" {
		t.Fatalf("unexpected endpoint[0]: %+v", endpoints[0])
	}
	if endpoints[1].ID != 2 || endpoints[1].Name != "remote" {
		t.Fatalf("unexpected endpoint[1]: %+v", endpoints[1])
	}
}

func TestGetContainers(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/endpoints/1/docker/containers/json" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("all") != "true" {
			t.Fatalf("expected all=true, got %s", r.URL.Query().Get("all"))
		}
		_, _ = w.Write([]byte(`[
			{
				"Id":"abc123def456",
				"Names":["/nginx-proxy"],
				"Image":"nginx:latest",
				"State":"running",
				"Status":"Up 2 hours",
				"Labels":{"com.docker.compose.project":"web"}
			}
		]`))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "key",
		Timeout: 5 * time.Second,
	})

	containers, err := client.GetContainers(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetContainers failed: %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}
	c := containers[0]
	if c.ID != "abc123def456" {
		t.Fatalf("unexpected container ID: %s", c.ID)
	}
	if len(c.Names) != 1 || c.Names[0] != "/nginx-proxy" {
		t.Fatalf("unexpected container names: %v", c.Names)
	}
	if c.Image != "nginx:latest" {
		t.Fatalf("unexpected image: %s", c.Image)
	}
	if c.State != "running" {
		t.Fatalf("unexpected state: %s", c.State)
	}
	if c.Labels["com.docker.compose.project"] != "web" {
		t.Fatalf("unexpected labels: %v", c.Labels)
	}
}

func TestGetStacks(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/stacks" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[
			{
				"Id":5,
				"Name":"monitoring",
				"Type":2,
				"EndpointId":1,
				"Status":1,
				"EntryPoint":"docker-compose.yml",
				"CreatedBy":"admin",
				"GitConfig":{"URL":"https://github.com/example/monitoring.git"}
			}
		]`))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "key",
		Timeout: 5 * time.Second,
	})

	stacks, err := client.GetStacks(context.Background())
	if err != nil {
		t.Fatalf("GetStacks failed: %v", err)
	}
	if len(stacks) != 1 {
		t.Fatalf("expected 1 stack, got %d", len(stacks))
	}
	s := stacks[0]
	if s.ID != 5 || s.Name != "monitoring" {
		t.Fatalf("unexpected stack: %+v", s)
	}
	if s.EndpointID != 1 {
		t.Fatalf("unexpected endpoint ID: %d", s.EndpointID)
	}
	if s.GitConfig == nil || s.GitConfig.URL != "https://github.com/example/monitoring.git" {
		t.Fatalf("unexpected git config: %+v", s.GitConfig)
	}
}

func TestContainerAction(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		expectedPath := "/api/endpoints/1/docker/containers/abc123/restart"
		if r.URL.Path != expectedPath {
			t.Fatalf("expected path %s, got %s", expectedPath, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "key",
		Timeout: 5 * time.Second,
	})

	err := client.ContainerAction(context.Background(), 1, "abc123", "restart")
	if err != nil {
		t.Fatalf("ContainerAction failed: %v", err)
	}
}

func TestRemoveContainer(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("expected DELETE, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/docker/containers/abc123") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("force") != "true" {
			t.Fatalf("expected force=true, got %s", r.URL.Query().Get("force"))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "key",
		Timeout: 5 * time.Second,
	})

	err := client.RemoveContainer(context.Background(), 1, "abc123", true)
	if err != nil {
		t.Fatalf("RemoveContainer failed: %v", err)
	}
}

func TestStackOperations(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/stacks/5/start"):
			if r.URL.Query().Get("endpointId") != "1" {
				t.Fatalf("expected endpointId=1, got %s", r.URL.Query().Get("endpointId"))
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))

		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/stacks/5/stop"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))

		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/stacks/5/git/redeploy"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))

		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/stacks/5"):
			if r.URL.Query().Get("endpointId") != "1" {
				t.Fatalf("expected endpointId=1, got %s", r.URL.Query().Get("endpointId"))
			}
			w.WriteHeader(http.StatusNoContent)

		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "key",
		Timeout: 5 * time.Second,
	})

	if err := client.StartStack(context.Background(), 5, 1); err != nil {
		t.Fatalf("StartStack failed: %v", err)
	}
	if err := client.StopStack(context.Background(), 5, 1); err != nil {
		t.Fatalf("StopStack failed: %v", err)
	}
	if err := client.RedeployStack(context.Background(), 5, 1, true); err != nil {
		t.Fatalf("RedeployStack failed: %v", err)
	}
	if err := client.RemoveStack(context.Background(), 5, 1); err != nil {
		t.Fatalf("RemoveStack failed: %v", err)
	}
}

func TestNewClientDefaults(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	client := NewClient(Config{
		BaseURL: "https://portainer.local:9443",
		APIKey:  "key",
	})
	if client.httpClient.Timeout != 10*time.Second {
		t.Fatalf("expected default timeout 10s, got %v", client.httpClient.Timeout)
	}
	transport, ok := client.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.httpClient.Transport)
	}
	if transport.MaxIdleConns != 10 {
		t.Fatalf("expected MaxIdleConns=10, got %d", transport.MaxIdleConns)
	}
	if transport.MaxIdleConnsPerHost != 5 {
		t.Fatalf("expected MaxIdleConnsPerHost=5, got %d", transport.MaxIdleConnsPerHost)
	}
	if transport.IdleConnTimeout != 90*time.Second {
		t.Fatalf("expected IdleConnTimeout=90s, got %v", transport.IdleConnTimeout)
	}
	if transport.TLSClientConfig.MinVersion != 0x0303 { // TLS 1.2
		t.Fatalf("expected TLS 1.2 min, got %x", transport.TLSClientConfig.MinVersion)
	}
}

func TestJWTRetryOn401(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	var requestCount atomic.Int32
	var authCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth":
			authCount.Add(1)
			_, _ = w.Write([]byte(`{"jwt":"fresh-token"}`))

		case "/api/system/version":
			count := requestCount.Add(1)
			if count == 1 {
				// First request: return 401 to trigger re-auth.
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"message":"Token is expired"}`))
				return
			}
			// Second request (after re-auth): succeed.
			_, _ = w.Write([]byte(`{"ServerVersion":"2.21.0","DatabaseVersion":"100","Build":{}}`))

		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL:  server.URL,
		Username: "admin",
		Password: "secret",
		Timeout:  5 * time.Second,
	})

	info, err := client.GetVersion(context.Background())
	if err != nil {
		t.Fatalf("GetVersion failed: %v", err)
	}
	if info.ServerVersion != "2.21.0" {
		t.Fatalf("unexpected version: %s", info.ServerVersion)
	}

	// Should have called auth twice: initial + retry after 401.
	if authCount.Load() != 2 {
		t.Fatalf("expected 2 auth calls, got %d", authCount.Load())
	}
}

func TestGetVersionParsing(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"ServerVersion":"2.21.0",
			"DatabaseVersion":"100",
			"Build":{
				"BuildNumber":"1234",
				"GoVersion":"go1.21.5"
			}
		}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "key",
		Timeout: 5 * time.Second,
	})

	info, err := client.GetVersion(context.Background())
	if err != nil {
		t.Fatalf("GetVersion failed: %v", err)
	}
	if info.ServerVersion != "2.21.0" {
		t.Fatalf("unexpected ServerVersion: %s", info.ServerVersion)
	}
	if info.DatabaseVersion != "100" {
		t.Fatalf("unexpected DatabaseVersion: %s", info.DatabaseVersion)
	}
	if info.Build.BuildNumber != "1234" {
		t.Fatalf("unexpected BuildNumber: %s", info.Build.BuildNumber)
	}
	if info.Build.GoVersion != "go1.21.5" {
		t.Fatalf("unexpected GoVersion: %s", info.Build.GoVersion)
	}
}

func TestCustomTimeout(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	client := NewClient(Config{
		BaseURL: "https://portainer.local:9443",
		APIKey:  "key",
		Timeout: 30 * time.Second,
	})
	if client.httpClient.Timeout != 30*time.Second {
		t.Fatalf("expected timeout 30s, got %v", client.httpClient.Timeout)
	}
}

func TestBaseURLTrailingSlash(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	client := NewClient(Config{
		BaseURL: "https://portainer.local:9443/",
		APIKey:  "key",
	})
	if strings.HasSuffix(client.baseURL, "/") {
		t.Fatalf("expected trailing slash removed, got %s", client.baseURL)
	}
}

func TestJWTRetryOn401PreservesJSONBody(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	var requestCount atomic.Int32
	var authCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth":
			authCount.Add(1)
			_, _ = w.Write([]byte(`{"jwt":"fresh-token"}`))

		case "/api/stacks/5/git/redeploy":
			call := requestCount.Add(1)
			if call == 1 {
				// Force retry path.
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"message":"token expired"}`))
				return
			}

			rawBody, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("failed to read request body: %v", err)
			}
			if len(strings.TrimSpace(string(rawBody))) == 0 {
				t.Fatalf("expected JSON body on retry request, got empty payload")
			}

			var payload map[string]any
			if err := json.Unmarshal(rawBody, &payload); err != nil {
				t.Fatalf("failed to decode retry payload: %v", err)
			}
			if pullImage, ok := payload["pullImage"].(bool); !ok || !pullImage {
				t.Fatalf("expected pullImage=true in retry payload, got %#v", payload["pullImage"])
			}

			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))

		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL:  server.URL,
		Username: "admin",
		Password: "secret",
		Timeout:  5 * time.Second,
	})

	if err := client.RedeployStack(context.Background(), 5, 1, true); err != nil {
		t.Fatalf("RedeployStack failed: %v", err)
	}

	if requestCount.Load() != 2 {
		t.Fatalf("expected 2 stack requests, got %d", requestCount.Load())
	}
	if authCount.Load() != 2 {
		t.Fatalf("expected 2 auth calls (initial + retry), got %d", authCount.Load())
	}
}

func TestAuthenticateErrorPaths(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	t.Run("create auth request error", func(t *testing.T) {
		client := NewClient(Config{
			BaseURL:  "https://example.com",
			Username: "admin",
			Password: "secret",
		})
		client.baseURL = "://bad-url"

		if err := client.authenticate(context.Background()); err == nil || !strings.Contains(err.Error(), "create auth request") {
			t.Fatalf("expected create auth request error, got %v", err)
		}
	})

	t.Run("auth request transport error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		baseURL := server.URL
		server.Close()

		client := NewClient(Config{
			BaseURL:  baseURL,
			Username: "admin",
			Password: "secret",
			Timeout:  50 * time.Millisecond,
		})
		if err := client.authenticate(context.Background()); err == nil || !strings.Contains(err.Error(), "auth request") {
			t.Fatalf("expected auth request error, got %v", err)
		}
	})

	t.Run("read auth response error", func(t *testing.T) {
		client := NewClient(Config{
			BaseURL:  "https://portainer.local",
			Username: "admin",
			Password: "secret",
		})
		client.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       errorReadCloser{},
				Header:     make(http.Header),
			}, nil
		})

		if err := client.authenticate(context.Background()); err == nil || !strings.Contains(err.Error(), "read auth response") {
			t.Fatalf("expected read auth response error, got %v", err)
		}
	})

	t.Run("auth non-success status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"invalid credentials"}`))
		}))
		defer server.Close()

		client := NewClient(Config{
			BaseURL:  server.URL,
			Username: "admin",
			Password: "wrong",
		})

		if err := client.authenticate(context.Background()); err == nil || !strings.Contains(err.Error(), "auth failed") {
			t.Fatalf("expected auth failed error, got %v", err)
		}
	})

	t.Run("decode auth response error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{not-json`))
		}))
		defer server.Close()

		client := NewClient(Config{
			BaseURL:  server.URL,
			Username: "admin",
			Password: "secret",
		})
		if err := client.authenticate(context.Background()); err == nil || !strings.Contains(err.Error(), "decode auth response") {
			t.Fatalf("expected decode auth response error, got %v", err)
		}
	})

	t.Run("empty jwt response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"jwt":""}`))
		}))
		defer server.Close()

		client := NewClient(Config{
			BaseURL:  server.URL,
			Username: "admin",
			Password: "secret",
		})
		if err := client.authenticate(context.Background()); err == nil || !strings.Contains(err.Error(), "empty JWT") {
			t.Fatalf("expected empty JWT error, got %v", err)
		}
	})
}

func TestRequestAndDoRequestErrorPaths(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	t.Run("request body read failure", func(t *testing.T) {
		client := NewClient(Config{
			BaseURL: "https://portainer.local",
			APIKey:  "key",
		})
		_, err := client.request(context.Background(), http.MethodPost, "/api/system/version", failingReader{}, "application/json")
		if err == nil || !strings.Contains(err.Error(), "read request body") {
			t.Fatalf("expected read request body error, got %v", err)
		}
	})

	t.Run("request returns response status errors", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"downstream"}`))
		}))
		defer server.Close()

		client := NewClient(Config{
			BaseURL: server.URL,
			APIKey:  "key",
		})
		_, err := client.request(context.Background(), http.MethodGet, "/api/system/version", nil, "")
		if err == nil || !strings.Contains(err.Error(), "portainer api returned 502") {
			t.Fatalf("expected status error, got %v", err)
		}
	})

	t.Run("request returns initial doRequest error", func(t *testing.T) {
		client := NewClient(Config{
			BaseURL: "https://portainer.local",
			APIKey:  "key",
		})
		client.baseURL = "://bad-url"

		_, err := client.request(context.Background(), http.MethodGet, "/api/system/version", nil, "")
		if err == nil || !strings.Contains(err.Error(), "missing protocol scheme") {
			t.Fatalf("expected initial doRequest error, got %v", err)
		}
	})

	t.Run("request retry propagates second doRequest error", func(t *testing.T) {
		var systemCalls atomic.Int32
		client := NewClient(Config{
			BaseURL:  "https://portainer.local",
			Username: "admin",
			Password: "secret",
		})
		client.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/api/auth":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"jwt":"fresh-token"}`)),
					Header:     make(http.Header),
				}, nil
			case "/api/system/version":
				call := systemCalls.Add(1)
				if call == 1 {
					return &http.Response{
						StatusCode: http.StatusUnauthorized,
						Body:       io.NopCloser(strings.NewReader(`{"message":"expired token"}`)),
						Header:     make(http.Header),
					}, nil
				}
				return nil, errors.New("retry transport failure")
			default:
				return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
			}
		})

		_, err := client.request(context.Background(), http.MethodGet, "/api/system/version", nil, "")
		if err == nil || !strings.Contains(err.Error(), "retry transport failure") {
			t.Fatalf("expected retry doRequest error, got %v", err)
		}
	})

	t.Run("doRequest acquire jwt failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/auth" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"message":"bad auth"}`))
				return
			}
			t.Fatalf("unexpected path %s", r.URL.Path)
		}))
		defer server.Close()

		client := NewClient(Config{
			BaseURL:  server.URL,
			Username: "admin",
			Password: "wrong",
		})

		_, _, err := client.doRequest(context.Background(), http.MethodGet, "api/system/version", nil, "")
		if err == nil || !strings.Contains(err.Error(), "acquire JWT") {
			t.Fatalf("expected acquire JWT error, got %v", err)
		}
	})

	t.Run("doRequest transport failure", func(t *testing.T) {
		client := NewClient(Config{
			BaseURL: "https://portainer.local",
			APIKey:  "key",
		})
		client.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("dial failed")
		})

		_, _, err := client.doRequest(context.Background(), http.MethodGet, "/api/system/version", nil, "")
		if err == nil || !strings.Contains(err.Error(), "dial failed") {
			t.Fatalf("expected transport failure, got %v", err)
		}
	})

	t.Run("doRequest read body failure", func(t *testing.T) {
		client := NewClient(Config{
			BaseURL: "https://portainer.local",
			APIKey:  "key",
		})
		client.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       errorReadCloser{},
				Header:     make(http.Header),
			}, nil
		})

		_, _, err := client.doRequest(context.Background(), http.MethodGet, "/api/system/version", nil, "")
		if err == nil || !strings.Contains(err.Error(), "forced read failure") {
			t.Fatalf("expected response read failure, got %v", err)
		}
	})

	t.Run("doRequest new request error", func(t *testing.T) {
		client := NewClient(Config{
			BaseURL: "https://portainer.local",
			APIKey:  "key",
		})
		client.baseURL = "://bad-url"

		_, _, err := client.doRequest(context.Background(), http.MethodGet, "/api/system/version", nil, "")
		if err == nil || !strings.Contains(err.Error(), "missing protocol scheme") {
			t.Fatalf("expected new request error, got %v", err)
		}
	})
}

func TestMarshalAndDecodeErrorPaths(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	client := NewClient(Config{
		BaseURL: "https://portainer.local",
		APIKey:  "key",
	})

	t.Run("post marshal error", func(t *testing.T) {
		_, err := client.post(context.Background(), "/api/test", map[string]any{"invalid": make(chan int)})
		if err == nil || !strings.Contains(err.Error(), "marshal request body") {
			t.Fatalf("expected marshal request body error, got %v", err)
		}
	})

	t.Run("put marshal error", func(t *testing.T) {
		_, err := client.put(context.Background(), "/api/test", map[string]any{"invalid": make(chan int)})
		if err == nil || !strings.Contains(err.Error(), "marshal request body") {
			t.Fatalf("expected marshal request body error, got %v", err)
		}
	})

	t.Run("post with nil body succeeds", func(t *testing.T) {
		var contentType string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			contentType = r.Header.Get("Content-Type")
			w.WriteHeader(http.StatusNoContent)
		}))
		defer server.Close()

		client := NewClient(Config{
			BaseURL: server.URL,
			APIKey:  "key",
		})
		if _, err := client.post(context.Background(), "/api/test", nil); err != nil {
			t.Fatalf("post with nil body failed: %v", err)
		}
		if contentType != "" {
			t.Fatalf("expected no content type for nil body, got %q", contentType)
		}
	})

	t.Run("post with json body sets content type", func(t *testing.T) {
		var contentType string
		var body map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			contentType = r.Header.Get("Content-Type")
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}))
		defer server.Close()

		client := NewClient(Config{
			BaseURL: server.URL,
			APIKey:  "key",
		})
		if _, err := client.post(context.Background(), "/api/test", map[string]any{"hello": "world"}); err != nil {
			t.Fatalf("post with json body failed: %v", err)
		}
		if contentType != "application/json" {
			t.Fatalf("expected application/json content type, got %q", contentType)
		}
		if body["hello"] != "world" {
			t.Fatalf("unexpected request body: %#v", body)
		}
	})
}

func TestAPIResponseDecodeAndTruncationPaths(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	t.Run("GetVersion decode error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`not-json`))
		}))
		defer server.Close()

		client := NewClient(Config{BaseURL: server.URL, APIKey: "key"})
		_, err := client.GetVersion(context.Background())
		if err == nil || !strings.Contains(err.Error(), "decode version response") {
			t.Fatalf("expected version decode error, got %v", err)
		}
	})

	t.Run("GetEndpoints decode error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{invalid`))
		}))
		defer server.Close()

		client := NewClient(Config{BaseURL: server.URL, APIKey: "key"})
		_, err := client.GetEndpoints(context.Background())
		if err == nil || !strings.Contains(err.Error(), "decode endpoints response") {
			t.Fatalf("expected endpoints decode error, got %v", err)
		}
	})

	t.Run("GetEndpoints truncates to max", func(t *testing.T) {
		endpoints := make([]Endpoint, 0, maxEndpoints+5)
		for i := 1; i <= maxEndpoints+5; i++ {
			endpoints = append(endpoints, Endpoint{ID: i, Name: fmt.Sprintf("ep-%d", i), Type: 1, URL: "unix:///var/run/docker.sock", Status: 1})
		}
		payload, _ := json.Marshal(endpoints)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(payload)
		}))
		defer server.Close()

		client := NewClient(Config{BaseURL: server.URL, APIKey: "key"})
		got, err := client.GetEndpoints(context.Background())
		if err != nil {
			t.Fatalf("GetEndpoints failed: %v", err)
		}
		if len(got) != maxEndpoints {
			t.Fatalf("expected %d endpoints after truncation, got %d", maxEndpoints, len(got))
		}
	})

	t.Run("GetContainers decode error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{invalid`))
		}))
		defer server.Close()

		client := NewClient(Config{BaseURL: server.URL, APIKey: "key"})
		_, err := client.GetContainers(context.Background(), 1)
		if err == nil || !strings.Contains(err.Error(), "decode containers response") {
			t.Fatalf("expected containers decode error, got %v", err)
		}
	})

	t.Run("GetContainers truncates to max", func(t *testing.T) {
		containers := make([]Container, 0, maxContainersPerEndpoint+7)
		for i := 0; i < maxContainersPerEndpoint+7; i++ {
			containers = append(containers, Container{
				ID:    fmt.Sprintf("id-%d", i),
				Names: []string{fmt.Sprintf("/c-%d", i)},
			})
		}
		payload, _ := json.Marshal(containers)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(payload)
		}))
		defer server.Close()

		client := NewClient(Config{BaseURL: server.URL, APIKey: "key"})
		got, err := client.GetContainers(context.Background(), 1)
		if err != nil {
			t.Fatalf("GetContainers failed: %v", err)
		}
		if len(got) != maxContainersPerEndpoint {
			t.Fatalf("expected %d containers after truncation, got %d", maxContainersPerEndpoint, len(got))
		}
	})

	t.Run("GetStacks decode error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{invalid`))
		}))
		defer server.Close()

		client := NewClient(Config{BaseURL: server.URL, APIKey: "key"})
		_, err := client.GetStacks(context.Background())
		if err == nil || !strings.Contains(err.Error(), "decode stacks response") {
			t.Fatalf("expected stacks decode error, got %v", err)
		}
	})
}

type failingReader struct{}

func (failingReader) Read(p []byte) (int, error) {
	return 0, errors.New("forced read failure")
}

type errorReadCloser struct{}

func (errorReadCloser) Read(p []byte) (int, error) {
	return 0, errors.New("forced read failure")
}

func (errorReadCloser) Close() error { return nil }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
