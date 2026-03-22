package fileproto

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"testing"
	"time"
)

// TestSMBImplementsRemoteFS is a compile-time interface compliance check.
func TestSMBImplementsRemoteFS(t *testing.T) {
	var _ RemoteFS = (*SMBClient)(nil)
}

// ---------- unit tests (no server required) ----------

// TestSMBCopyReturnsNotSupported verifies Copy always returns ErrNotSupported.
func TestSMBCopyReturnsNotSupported(t *testing.T) {
	client := &SMBClient{}
	err := client.Copy(context.Background(), "/a", "/b")
	if !errors.Is(err, ErrNotSupported) {
		t.Errorf("expected ErrNotSupported, got: %v", err)
	}
}

// TestSMBCloseNilHandles ensures Close is safe on an unconnected client.
func TestSMBCloseNilHandles(t *testing.T) {
	client := &SMBClient{}
	if err := client.Close(); err != nil {
		t.Errorf("close nil handles: %v", err)
	}
}

// TestSMBConnectMissingShare verifies that Connect fails when smb_share is not
// provided in ExtraConfig.
func TestSMBConnectMissingShare(t *testing.T) {
	client := &SMBClient{}
	cfg := ConnectionConfig{
		Protocol:    "smb",
		Host:        "127.0.0.1",
		Port:        445,
		Username:    "user",
		Secret:      "pass",
		ExtraConfig: map[string]any{},
	}
	err := client.Connect(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error when smb_share is missing")
	}
	if !contains(err.Error(), "smb_share") {
		t.Errorf("error should mention smb_share, got: %v", err)
	}
}

// TestSMBConnectNilExtraConfig verifies that Connect fails gracefully when
// ExtraConfig is nil.
func TestSMBConnectNilExtraConfig(t *testing.T) {
	client := &SMBClient{}
	cfg := ConnectionConfig{
		Protocol: "smb",
		Host:     "127.0.0.1",
		Port:     445,
		Username: "user",
		Secret:   "pass",
	}
	err := client.Connect(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error when ExtraConfig is nil")
	}
}

// TestSMBPathConversion verifies the smbPath helper.
func TestSMBPathConversion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/documents/report.txt", "documents/report.txt"},
		{"/", "."},
		{"", "."},
		{"relative/path", "relative/path"},
		{"/leading/slash", "leading/slash"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := smbPath(tt.input)
			if got != tt.want {
				t.Errorf("smbPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestSMBConnectDialFailure verifies that a connection to an unreachable host
// fails with a sensible error.
func TestSMBConnectDialFailure(t *testing.T) {
	client := &SMBClient{}
	cfg := ConnectionConfig{
		Protocol:    "smb",
		Host:        "127.0.0.1",
		Port:        1, // unlikely to have an SMB server
		Username:    "user",
		Secret:      "pass",
		ExtraConfig: map[string]any{"smb_share": "share"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := client.Connect(ctx, cfg)
	if err == nil {
		client.Close()
		t.Fatal("expected dial error")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsAt(s, substr)
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ---------- integration tests against a real SMB server ----------
//
// These tests require a local SMB server. Set the environment variable
// SMB_TEST_ADDR=host:port to enable them. The server must allow login
// with SMB_TEST_USER / SMB_TEST_PASS and have the share SMB_TEST_SHARE
// available for read/write.
//
// Quick setup with Docker:
//   docker run -d --name samba -p 445:445 \
//     -e USER=testuser -e PASS=testpass -e SHARE=testshare \
//     dperson/samba -s "testshare;/share;yes;no;no;testuser"

func smbTestEnv(t *testing.T) (host string, port int, user, pass, share string) {
	t.Helper()
	addr := os.Getenv("SMB_TEST_ADDR")
	if addr == "" {
		t.Skip("SMB_TEST_ADDR not set; skipping SMB integration tests")
	}
	h, p, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("bad SMB_TEST_ADDR: %v", err)
	}
	var pn int
	fmt.Sscanf(p, "%d", &pn)

	user = os.Getenv("SMB_TEST_USER")
	if user == "" {
		user = "testuser"
	}
	pass = os.Getenv("SMB_TEST_PASS")
	if pass == "" {
		pass = "testpass"
	}
	share = os.Getenv("SMB_TEST_SHARE")
	if share == "" {
		share = "testshare"
	}
	return h, pn, user, pass, share
}

func smbConnect(t *testing.T) *SMBClient {
	t.Helper()
	h, p, u, pw, sh := smbTestEnv(t)

	client := &SMBClient{}
	cfg := ConnectionConfig{
		Protocol:   "smb",
		Host:       h,
		Port:       p,
		Username:   u,
		Secret:     pw,
		AuthMethod: "password",
		ExtraConfig: map[string]any{
			"smb_share": sh,
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := client.Connect(ctx, cfg); err != nil {
		t.Fatalf("smb connect: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

func TestSMBConnectAndClose(t *testing.T) {
	_ = smbConnect(t)
}

func TestSMBWriteReadDelete(t *testing.T) {
	client := smbConnect(t)
	ctx := context.Background()

	filePath := "/test_smb_write_read.txt"
	content := []byte("hello smb world")

	// Write
	if err := client.Write(ctx, filePath, bytes.NewReader(content), int64(len(content))); err != nil {
		t.Fatalf("write: %v", err)
	}
	defer func() { _ = client.Delete(ctx, filePath) }()

	// Read back
	rc, size, err := client.Read(ctx, filePath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()

	if size != int64(len(content)) {
		t.Errorf("size mismatch: got %d, want %d", size, len(content))
	}
	if !bytes.Equal(got, content) {
		t.Errorf("content mismatch: got %q, want %q", got, content)
	}

	// Delete
	if err := client.Delete(ctx, filePath); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestSMBListHiddenFiles(t *testing.T) {
	client := smbConnect(t)
	ctx := context.Background()

	// Create visible and hidden files.
	_ = client.Write(ctx, "/visible_smb.txt", bytes.NewReader([]byte("v")), 1)
	_ = client.Write(ctx, "/.hidden_smb.txt", bytes.NewReader([]byte("h")), 1)
	defer func() {
		_ = client.Delete(ctx, "/visible_smb.txt")
		_ = client.Delete(ctx, "/.hidden_smb.txt")
	}()

	// Without hidden.
	entries, err := client.List(ctx, "/", false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, e := range entries {
		if e.Name == ".hidden_smb.txt" {
			t.Error("hidden file should be excluded when showHidden=false")
		}
	}

	// With hidden.
	entries, err = client.List(ctx, "/", true)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	foundHidden := false
	for _, e := range entries {
		if e.Name == ".hidden_smb.txt" {
			foundHidden = true
		}
	}
	if !foundHidden {
		t.Error("hidden file should be included when showHidden=true")
	}
}

func TestSMBMkdirAndRecursiveDelete(t *testing.T) {
	client := smbConnect(t)
	ctx := context.Background()

	dirPath := "/smb_test_dir/nested/deep"

	if err := client.Mkdir(ctx, dirPath); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Put a file inside.
	_ = client.Write(ctx, "/smb_test_dir/nested/deep/inner.txt", bytes.NewReader([]byte("inner")), 5)

	// Recursive delete from the top.
	if err := client.Delete(ctx, "/smb_test_dir"); err != nil {
		t.Fatalf("recursive delete: %v", err)
	}

	// Verify the directory is gone by listing root.
	entries, err := client.List(ctx, "/", false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, e := range entries {
		if e.Name == "smb_test_dir" {
			t.Error("directory should have been deleted")
		}
	}
}

func TestSMBRename(t *testing.T) {
	client := smbConnect(t)
	ctx := context.Background()

	oldPath := "/smb_old.txt"
	newPath := "/smb_new.txt"

	_ = client.Write(ctx, oldPath, bytes.NewReader([]byte("data")), 4)
	defer func() {
		_ = client.Delete(ctx, newPath)
		_ = client.Delete(ctx, oldPath)
	}()

	if err := client.Rename(ctx, oldPath, newPath); err != nil {
		t.Fatalf("rename: %v", err)
	}

	// New file should be readable.
	rc, _, err := client.Read(ctx, newPath)
	if err != nil {
		t.Fatalf("read new: %v", err)
	}
	rc.Close()
}

func TestSMBDomainAuth(t *testing.T) {
	// Verify that the domain field is accepted without error during config
	// construction. Full integration requires a domain-joined server.
	client := &SMBClient{}
	cfg := ConnectionConfig{
		Protocol:    "smb",
		Host:        "127.0.0.1",
		Port:        1,
		Username:    "user",
		Secret:      "pass",
		ExtraConfig: map[string]any{"smb_share": "share", "smb_domain": "MYDOMAIN"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// We expect a dial error (no server), not a config error.
	err := client.Connect(ctx, cfg)
	if err == nil {
		client.Close()
		t.Fatal("expected dial error")
	}
	// The error should be about dialing, not about domain config.
	if contains(err.Error(), "smb_share") || contains(err.Error(), "smb_domain") {
		t.Errorf("expected dial error, got config error: %v", err)
	}
}
