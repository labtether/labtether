package portainer

import (
	"context"
	"encoding/json"
	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/connectorsdk"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

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
			name:   "valid collector-scoped id",
			target: assetid.ScopeCollectorAssetID("portainer-stack-5", "collector-portainer-a"),
			wantID: 5,
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
