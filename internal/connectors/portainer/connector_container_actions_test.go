package portainer

import (
	"context"
	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/connectorsdk"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

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
			name:      "valid collector-scoped id",
			target:    assetid.ScopeCollectorAssetID("portainer-container-2-abc123def456", "collector-portainer-a"),
			wantEpID:  2,
			wantCtrID: "abc123def456",
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

func TestParseContainerTargetEmptyContainerID(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	_, _, err := parseContainerTarget("portainer-container-1-")
	if err == nil || !strings.Contains(err.Error(), "empty container ID") {
		t.Fatalf("expected empty container ID error, got %v", err)
	}
}
