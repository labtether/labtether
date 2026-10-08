package portainer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

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
