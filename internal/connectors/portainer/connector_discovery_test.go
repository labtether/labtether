package portainer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

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

func TestDiscoverFallbackAndFailures(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	t.Run("empty discovery returns no synthetic inventory", func(t *testing.T) {
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
		if assets == nil || len(assets) != 0 {
			t.Fatalf("expected non-nil empty inventory, got %+v", assets)
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
