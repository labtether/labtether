package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/hubcollector"
)

func TestExecutePortainerCollectorUsesConfiguredClusterNameForSingleEndpoint(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/system/version", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ServerVersion":"2.21.5","DatabaseVersion":"2.21.5","Build":{"BuildNumber":"12345"}}`))
	})
	mux.HandleFunc("/api/endpoints", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"Id":1,"Name":"local","Type":1,"URL":"unix:///var/run/docker.sock","Status":1}]`))
	})
	mux.HandleFunc("/api/endpoints/1/docker/containers/json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{
			"Id":"abc123def456789012345678",
			"Names":["/nginx"],
			"Image":"nginx:latest",
			"State":"running",
			"Status":"Up 4 hours",
			"Created":1710000000,
			"Ports":[{"PrivatePort":80,"PublicPort":8080,"Type":"tcp"}],
			"Labels":{"com.docker.compose.project":"web"}
		}]`))
	})
	mux.HandleFunc("/api/stacks", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{
			"Id":5,
			"Name":"web",
			"Type":2,
			"EndpointId":1,
			"Status":1,
			"EntryPoint":"compose.yml",
			"CreatedBy":"admin",
			"GitConfig":{"URL":"https://github.com/example/web.git"}
		}]`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store

	credentialID := seedPortainerCredentialProfile(t, sut, "svc@local!automation", "ptr-secret-value", server.URL)
	collector := hubcollector.Collector{
		ID:            "collector-portainer-friendly-name",
		AssetID:       "portainer-cluster-friendly-name",
		CollectorType: hubcollector.CollectorTypePortainer,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": credentialID,
			"auth_method":   "api_key",
			"cluster_name":  "Lab Portainer",
			"skip_verify":   true,
		},
	}

	sut.executePortainerCollector(context.Background(), collector)

	asset, exists, err := sut.assetStore.GetAsset("portainer-endpoint-1")
	if err != nil {
		t.Fatalf("GetAsset() error = %v", err)
	}
	if !exists {
		t.Fatalf("expected discovered portainer endpoint asset to be persisted")
	}
	if asset.Name != "Lab Portainer" {
		t.Fatalf("asset.Name = %q, want %q", asset.Name, "Lab Portainer")
	}
	if asset.Metadata["name"] != "Lab Portainer" {
		t.Fatalf("metadata name = %q, want %q", asset.Metadata["name"], "Lab Portainer")
	}
	if asset.Metadata["portainer_endpoint_name"] != "local" {
		t.Fatalf("portainer_endpoint_name = %q, want %q", asset.Metadata["portainer_endpoint_name"], "local")
	}
	if asset.Metadata["portainer_container_count"] != "1" {
		t.Fatalf("portainer_container_count = %q, want %q", asset.Metadata["portainer_container_count"], "1")
	}
	if asset.Metadata["portainer_stack_count"] != "1" {
		t.Fatalf("portainer_stack_count = %q, want %q", asset.Metadata["portainer_stack_count"], "1")
	}
	if asset.Metadata["portainer_version"] != "2.21.5" {
		t.Fatalf("portainer_version = %q, want %q", asset.Metadata["portainer_version"], "2.21.5")
	}

	containerAsset, exists, err := sut.assetStore.GetAsset("portainer-container-1-abc123def456")
	if err != nil {
		t.Fatalf("GetAsset(container) error = %v", err)
	}
	if !exists {
		t.Fatalf("expected discovered portainer container asset to be persisted")
	}
	if containerAsset.Metadata["ports"] != "8080->80/tcp" {
		t.Fatalf("container ports = %q, want %q", containerAsset.Metadata["ports"], "8080->80/tcp")
	}

	stackAsset, exists, err := sut.assetStore.GetAsset("portainer-stack-5")
	if err != nil {
		t.Fatalf("GetAsset(stack) error = %v", err)
	}
	if !exists {
		t.Fatalf("expected discovered portainer stack asset to be persisted")
	}
	if stackAsset.Metadata["portainer_stack_container_count"] != "1" {
		t.Fatalf("portainer_stack_container_count = %q, want %q", stackAsset.Metadata["portainer_stack_container_count"], "1")
	}
}

func TestExecutePortainerCollectorKeepsEndpointNamesWhenMultipleEndpointsExist(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/system/version", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ServerVersion":"2.21.5","DatabaseVersion":"2.21.5","Build":{"BuildNumber":"12345"}}`))
	})
	mux.HandleFunc("/api/endpoints", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"Id":1,"Name":"local","Type":1,"URL":"unix:///var/run/docker.sock","Status":1},
			{"Id":2,"Name":"edge","Type":1,"URL":"tcp://edge:2375","Status":1}
		]`))
	})
	mux.HandleFunc("/api/endpoints/1/docker/containers/json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	mux.HandleFunc("/api/endpoints/2/docker/containers/json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	mux.HandleFunc("/api/stacks", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	sut := newTestAPIServer(t)
	store := newRecordingHubCollectorStore()
	sut.hubCollectorStore = store

	credentialID := seedPortainerCredentialProfile(t, sut, "svc@local!automation", "ptr-secret-value", server.URL)
	collector := hubcollector.Collector{
		ID:            "collector-portainer-multi-endpoint",
		AssetID:       "portainer-cluster-multi-endpoint",
		CollectorType: hubcollector.CollectorTypePortainer,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": credentialID,
			"auth_method":   "api_key",
			"cluster_name":  "Lab Portainer",
			"skip_verify":   true,
		},
	}

	sut.executePortainerCollector(context.Background(), collector)

	localAsset, exists, err := sut.assetStore.GetAsset("portainer-endpoint-1")
	if err != nil {
		t.Fatalf("GetAsset(endpoint-1) error = %v", err)
	}
	if !exists {
		t.Fatalf("expected first discovered portainer endpoint asset to be persisted")
	}
	if localAsset.Name != "local" {
		t.Fatalf("local asset.Name = %q, want %q", localAsset.Name, "local")
	}
	if localAsset.Metadata["portainer_endpoint_name"] != "" {
		t.Fatalf("local portainer_endpoint_name = %q, want empty", localAsset.Metadata["portainer_endpoint_name"])
	}

	edgeAsset, exists, err := sut.assetStore.GetAsset("portainer-endpoint-2")
	if err != nil {
		t.Fatalf("GetAsset(endpoint-2) error = %v", err)
	}
	if !exists {
		t.Fatalf("expected second discovered portainer endpoint asset to be persisted")
	}
	if edgeAsset.Name != "edge" {
		t.Fatalf("edge asset.Name = %q, want %q", edgeAsset.Name, "edge")
	}
}
