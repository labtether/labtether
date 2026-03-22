package docker

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newSecureDockerClientFixture(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *dockerClient) {
	t.Helper()
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")

	srv := httptest.NewTLSServer(handler)
	client := NewDockerClient(srv.URL)
	client.httpClient = srv.Client()
	return srv, client
}

func newUnixDockerClientFixture(t *testing.T, handler http.HandlerFunc) *dockerClient {
	t.Helper()

	socketFile, err := os.CreateTemp("/tmp", "labtether-docker-*.sock")
	if err != nil {
		t.Fatalf("create temp socket file: %v", err)
	}
	socketPath := socketFile.Name()
	_ = socketFile.Close()
	if err := os.Remove(socketPath); err != nil {
		t.Fatalf("remove temp socket file: %v", err)
	}
	socketPath = filepath.Clean(socketPath)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix socket: %v", err)
	}

	server := &http.Server{Handler: handler}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = server.Serve(listener)
	}()

	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		_ = listener.Close()
		_ = os.Remove(socketPath)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	})

	return NewDockerClient(socketPath)
}

func TestDockerClientListContainers(t *testing.T) {
	srv, client := newSecureDockerClientFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/containers/json" {
			json.NewEncoder(w).Encode([]map[string]any{
				{
					"Id":      "abc123def456789012",
					"Names":   []string{"/nginx"},
					"Image":   "nginx:1.25",
					"State":   "running",
					"Status":  "Up 3 days",
					"Created": 1708700000,
					"Ports": []map[string]any{
						{"PrivatePort": 80, "PublicPort": 8080, "Type": "tcp"},
					},
					"Labels": map[string]string{"env": "prod"},
					"Mounts": []map[string]string{
						{"Type": "bind", "Source": "/data", "Destination": "/app/data"},
					},
					"NetworkSettings": map[string]any{
						"Networks": map[string]any{"bridge": map[string]any{}},
					},
				},
			})
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	containers, err := client.listContainers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}
	if ContainerName(containers[0].Names) != "nginx" {
		t.Errorf("name = %q, want %q", ContainerName(containers[0].Names), "nginx")
	}
	if containers[0].State != "running" {
		t.Errorf("state = %q, want %q", containers[0].State, "running")
	}
	if len(containers[0].Ports) != 1 || containers[0].Ports[0].PublicPort != 8080 {
		t.Errorf("unexpected ports: %+v", containers[0].Ports)
	}
}

func TestDockerClientListImages(t *testing.T) {
	srv, client := newSecureDockerClientFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/images/json" {
			json.NewEncoder(w).Encode([]map[string]any{
				{"Id": "sha256:abc123", "RepoTags": []string{"nginx:1.25", "nginx:latest"}, "Size": 187654321, "Created": 1706745600},
			})
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	images, err := client.listImages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 1 || images[0].ID != "sha256:abc123" {
		t.Errorf("unexpected images: %+v", images)
	}
}

func TestDockerClientListNetworks(t *testing.T) {
	srv, client := newSecureDockerClientFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/networks" {
			json.NewEncoder(w).Encode([]map[string]string{
				{"Id": "net1", "Name": "bridge", "Driver": "bridge", "Scope": "local"},
			})
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	networks, err := client.listNetworks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(networks) != 1 || networks[0].Name != "bridge" {
		t.Errorf("unexpected networks: %+v", networks)
	}
}

func TestDockerClientListVolumes(t *testing.T) {
	srv, client := newSecureDockerClientFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/volumes" {
			json.NewEncoder(w).Encode(map[string]any{
				"Volumes": []map[string]string{
					{"Name": "pgdata", "Driver": "local", "Mountpoint": "/var/lib/docker/volumes/pgdata/_data"},
				},
			})
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	volumes, err := client.listVolumes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(volumes) != 1 || volumes[0].Name != "pgdata" {
		t.Errorf("unexpected volumes: %+v", volumes)
	}
}

func TestDockerClientVersion(t *testing.T) {
	srv, client := newSecureDockerClientFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/version" {
			json.NewEncoder(w).Encode(map[string]string{
				"Version": "24.0.7", "ApiVersion": "1.43", "Os": "linux", "Arch": "amd64",
			})
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	ver, err := client.version(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ver.Version != "24.0.7" {
		t.Errorf("version = %q, want %q", ver.Version, "24.0.7")
	}
	if ver.APIVersion != "1.43" {
		t.Errorf("api version = %q, want %q", ver.APIVersion, "1.43")
	}
}

func TestDockerClientPing(t *testing.T) {
	srv, client := newSecureDockerClientFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_ping" {
			w.Write([]byte("OK"))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	if err := client.ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDockerClientPingUnixSocketBypassesOutboundURLPolicy(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "false")

	client := newUnixDockerClientFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_ping" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))

	if err := client.ping(context.Background()); err != nil {
		t.Fatalf("expected unix socket ping to bypass outbound URL policy, got %v", err)
	}
}

func TestDockerClientErrorHandling(t *testing.T) {
	srv, client := newSecureDockerClientFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("internal server error"))
	}))
	defer srv.Close()

	_, err := client.listContainers(context.Background())
	if err == nil {
		t.Error("expected error for 500 response")
	}
}
