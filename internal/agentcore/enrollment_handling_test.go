package agentcore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadTokenRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent-token")

	err := saveTokenToFile(path, "test-token-abc")
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := loadTokenFromFile(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if loaded != "test-token-abc" {
		t.Fatalf("expected 'test-token-abc', got %q", loaded)
	}

	// Verify restrictive permissions.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("expected 0600 permissions, got %o", info.Mode().Perm())
	}
}

func TestEnrollmentHeadersWhenNoToken(t *testing.T) {
	transport := newWSTransport("ws://localhost:8080/ws/agent", "", "test-host", "linux", "dev", nil, "/tmp/test-token", nil)

	// Verify the transport was created with empty token.
	if transport.token != "" {
		t.Fatalf("expected empty token, got %q", transport.token)
	}
	if transport.tokenFilePath != "/tmp/test-token" {
		t.Fatalf("expected tokenFilePath '/tmp/test-token', got %q", transport.tokenFilePath)
	}
}

func TestUpdateTokenResetsAuthFailures(t *testing.T) {
	transport := newWSTransport("ws://localhost:8080/ws/agent", "", "test-host", "linux", "dev", nil, "", nil)
	transport.consecutiveAuthFailures = 5
	transport.lastError = "auth_failed"

	transport.updateToken("new-token")

	if transport.token != "new-token" {
		t.Fatalf("expected 'new-token', got %q", transport.token)
	}
	if transport.consecutiveAuthFailures != 0 {
		t.Fatalf("expected 0 auth failures after updateToken, got %d", transport.consecutiveAuthFailures)
	}
	if transport.lastError != "" {
		t.Fatalf("expected empty lastError, got %q", transport.lastError)
	}
}
