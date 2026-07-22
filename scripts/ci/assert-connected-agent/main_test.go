package main

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func trustedTestClient(t *testing.T, server *httptest.Server) *http.Client {
	t.Helper()
	caPath := filepath.Join(t.TempDir(), "ca.crt")
	encoded := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(caPath, encoded, 0o600); err != nil {
		t.Fatalf("write test CA: %v", err)
	}
	client, err := newStrictHTTPClient(caPath)
	if err != nil {
		t.Fatalf("newStrictHTTPClient: %v", err)
	}
	return client
}

func TestWaitForConnectedAgentRequiresExactAuthenticatedAsset(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/agents/connected" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer qa-token" {
			t.Errorf("Authorization = %q", got)
		}
		response.Header().Set("Content-Type", "application/json")
		if calls.Add(1) < 2 {
			_, _ = response.Write([]byte(`{"assets":["labtether-ci-agent-similar"]}`))
			return
		}
		_, _ = response.Write([]byte(`{"assets":["labtether-ci-agent"]}`))
	}))
	defer server.Close()

	endpoint, err := url.Parse(server.URL + "/agents/connected")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := waitForConnectedAgent(ctx, trustedTestClient(t, server), endpoint, "qa-token", "labtether-ci-agent", 5*time.Millisecond); err != nil {
		t.Fatalf("waitForConnectedAgent: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("probe calls = %d, want 2", calls.Load())
	}
}

func TestWaitForConnectedAgentFailsFastOnAuthenticationError(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	endpoint, _ := url.Parse(server.URL + "/agents/connected")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := waitForConnectedAgent(ctx, trustedTestClient(t, server), endpoint, "bad-token", "labtether-ci-agent", time.Hour)
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("error = %v, want authentication failure", err)
	}
}

func TestParseHubBaseURLRejectsInsecureOrAmbiguousURLs(t *testing.T) {
	for _, raw := range []string{
		"http://localhost:8443",
		"https://user:secret@localhost:8443",
		"https://localhost:8443/api",
		"https://localhost:8443?next=https://example.com",
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := parseHubBaseURL(raw); err == nil {
				t.Fatalf("parseHubBaseURL(%q) unexpectedly succeeded", raw)
			}
		})
	}
}

func TestStrictHTTPClientRequiresCA(t *testing.T) {
	if _, err := newStrictHTTPClient(""); err == nil {
		t.Fatal("newStrictHTTPClient unexpectedly accepted an empty CA path")
	}
}
