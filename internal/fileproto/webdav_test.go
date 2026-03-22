package fileproto

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"testing"
	"time"
)

// TestWebDAVImplementsRemoteFS is a compile-time interface compliance check.
func TestWebDAVImplementsRemoteFS(t *testing.T) {
	var _ RemoteFS = (*WebDAVClient)(nil)
}

// ---------- unit tests (no server required) ----------

// TestWebDAVCloseIsNoop ensures Close is safe and returns nil.
func TestWebDAVCloseIsNoop(t *testing.T) {
	client := &WebDAVClient{}
	if err := client.Close(); err != nil {
		t.Errorf("close should be no-op, got: %v", err)
	}
}

// TestWebDAVConnectBuildURL verifies URL construction from config parameters.
func TestWebDAVConnectBuildURL(t *testing.T) {
	tests := []struct {
		name        string
		port        int
		extraConfig map[string]any
		wantScheme  string
	}{
		{
			name:        "default_port_443_uses_https",
			port:        0, // defaults to 443
			extraConfig: map[string]any{},
			wantScheme:  "https",
		},
		{
			name:        "explicit_port_443_uses_https",
			port:        443,
			extraConfig: map[string]any{},
			wantScheme:  "https",
		},
		{
			name:        "port_80_uses_http",
			port:        80,
			extraConfig: map[string]any{},
			wantScheme:  "http",
		},
		{
			name:        "webdav_tls_true_overrides_http",
			port:        8080,
			extraConfig: map[string]any{"webdav_tls": true},
			wantScheme:  "https",
		},
		{
			name:        "webdav_tls_false_keeps_http",
			port:        8080,
			extraConfig: map[string]any{"webdav_tls": false},
			wantScheme:  "http",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &WebDAVClient{}
			cfg := ConnectionConfig{
				Protocol:    "webdav",
				Host:        "192.0.2.1", // TEST-NET, will fail to connect
				Port:        tt.port,
				Username:    "user",
				Secret:      "pass",
				ExtraConfig: tt.extraConfig,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			// We expect a connection error, not a config error.
			// This validates URL construction without needing a server.
			err := client.Connect(ctx, cfg)
			if err == nil {
				client.Close()
				t.Fatal("expected connection error to unreachable host")
			}
			// The error message should contain the expected scheme.
			if !contains(err.Error(), tt.wantScheme+"://") {
				t.Errorf("expected scheme %s in error, got: %v", tt.wantScheme, err)
			}
		})
	}
}

// TestWebDAVConnectWithBasePath verifies that webdav_base_path is appended to
// the URL correctly.
func TestWebDAVConnectWithBasePath(t *testing.T) {
	client := &WebDAVClient{}
	cfg := ConnectionConfig{
		Protocol: "webdav",
		Host:     "192.0.2.1",
		Port:     80,
		Username: "user",
		Secret:   "pass",
		ExtraConfig: map[string]any{
			"webdav_base_path": "/remote.php/dav/files/user",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := client.Connect(ctx, cfg)
	if err == nil {
		client.Close()
		t.Fatal("expected connection error")
	}
	// The error should contain the base path as part of the URL.
	if !contains(err.Error(), "/remote.php/dav/files/user") {
		t.Errorf("expected base path in error, got: %v", err)
	}
}

// TestWebDAVConnectDialFailure verifies that a connection to an unreachable
// host fails with a sensible error.
func TestWebDAVConnectDialFailure(t *testing.T) {
	client := &WebDAVClient{}
	cfg := ConnectionConfig{
		Protocol:    "webdav",
		Host:        "127.0.0.1",
		Port:        1, // unlikely to have a WebDAV server
		Username:    "user",
		Secret:      "pass",
		ExtraConfig: map[string]any{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := client.Connect(ctx, cfg)
	if err == nil {
		client.Close()
		t.Fatal("expected dial error")
	}
}

// TestWebDAVConnectNilExtraConfig verifies Connect handles nil ExtraConfig
// gracefully (defaults to HTTPS on port 443).
func TestWebDAVConnectNilExtraConfig(t *testing.T) {
	client := &WebDAVClient{}
	cfg := ConnectionConfig{
		Protocol: "webdav",
		Host:     "192.0.2.1",
		Port:     0,
		Username: "user",
		Secret:   "pass",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := client.Connect(ctx, cfg)
	if err == nil {
		client.Close()
		t.Fatal("expected connection error")
	}
	// Should default to HTTPS on port 443.
	if !contains(err.Error(), "https://192.0.2.1:443") {
		t.Errorf("expected https://192.0.2.1:443 in error, got: %v", err)
	}
}

// ---------- integration tests against a real WebDAV server ----------
//
// These tests require a local WebDAV server. Set the environment variable
// WEBDAV_TEST_ADDR=host:port to enable them. The server must allow login
// with WEBDAV_TEST_USER / WEBDAV_TEST_PASS.
//
// Quick setup with Docker:
//   docker run -d --name webdav -p 8080:80 \
//     -e USERNAME=testuser -e PASSWORD=testpass \
//     bytemark/webdav

func webdavTestEnv(t *testing.T) (host string, port int, user, pass string) {
	t.Helper()
	addr := os.Getenv("WEBDAV_TEST_ADDR")
	if addr == "" {
		t.Skip("WEBDAV_TEST_ADDR not set; skipping WebDAV integration tests")
	}
	h, p, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("bad WEBDAV_TEST_ADDR: %v", err)
	}
	var pn int
	fmt.Sscanf(p, "%d", &pn)

	user = os.Getenv("WEBDAV_TEST_USER")
	if user == "" {
		user = "testuser"
	}
	pass = os.Getenv("WEBDAV_TEST_PASS")
	if pass == "" {
		pass = "testpass"
	}
	return h, pn, user, pass
}

func webdavConnect(t *testing.T) *WebDAVClient {
	t.Helper()
	h, p, u, pw := webdavTestEnv(t)

	extra := map[string]any{}
	if basePath := os.Getenv("WEBDAV_TEST_BASE_PATH"); basePath != "" {
		extra["webdav_base_path"] = basePath
	}
	if p != 443 {
		// Non-443 ports default to HTTP; integration test servers are typically
		// plain HTTP.
	}

	client := &WebDAVClient{}
	cfg := ConnectionConfig{
		Protocol:    "webdav",
		Host:        h,
		Port:        p,
		Username:    u,
		Secret:      pw,
		AuthMethod:  "password",
		ExtraConfig: extra,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := client.Connect(ctx, cfg); err != nil {
		t.Fatalf("webdav connect: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

func TestWebDAVConnectAndClose(t *testing.T) {
	_ = webdavConnect(t)
}

func TestWebDAVWriteReadDelete(t *testing.T) {
	client := webdavConnect(t)
	ctx := context.Background()

	filePath := "/test_webdav_write_read.txt"
	content := []byte("hello webdav world")

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

func TestWebDAVListHiddenFiles(t *testing.T) {
	client := webdavConnect(t)
	ctx := context.Background()

	// Create visible and hidden files.
	_ = client.Write(ctx, "/visible_webdav.txt", bytes.NewReader([]byte("v")), 1)
	_ = client.Write(ctx, "/.hidden_webdav.txt", bytes.NewReader([]byte("h")), 1)
	defer func() {
		_ = client.Delete(ctx, "/visible_webdav.txt")
		_ = client.Delete(ctx, "/.hidden_webdav.txt")
	}()

	// Without hidden.
	entries, err := client.List(ctx, "/", false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, e := range entries {
		if e.Name == ".hidden_webdav.txt" {
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
		if e.Name == ".hidden_webdav.txt" {
			foundHidden = true
		}
	}
	if !foundHidden {
		t.Error("hidden file should be included when showHidden=true")
	}
}

func TestWebDAVMkdirAndDelete(t *testing.T) {
	client := webdavConnect(t)
	ctx := context.Background()

	dirPath := "/webdav_test_dir/nested/deep"

	if err := client.Mkdir(ctx, dirPath); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Put a file inside.
	_ = client.Write(ctx, "/webdav_test_dir/nested/deep/inner.txt", bytes.NewReader([]byte("inner")), 5)

	// Recursive delete from the top.
	if err := client.Delete(ctx, "/webdav_test_dir"); err != nil {
		t.Fatalf("recursive delete: %v", err)
	}

	// Verify the directory is gone by listing root.
	entries, err := client.List(ctx, "/", false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, e := range entries {
		if e.Name == "webdav_test_dir" {
			t.Error("directory should have been deleted")
		}
	}
}

func TestWebDAVRename(t *testing.T) {
	client := webdavConnect(t)
	ctx := context.Background()

	oldPath := "/webdav_old.txt"
	newPath := "/webdav_new.txt"

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

func TestWebDAVCopy(t *testing.T) {
	client := webdavConnect(t)
	ctx := context.Background()

	content := []byte("copy me webdav")
	srcPath := "/webdav_src.txt"
	dstPath := "/webdav_dst.txt"

	_ = client.Write(ctx, srcPath, bytes.NewReader(content), int64(len(content)))
	defer func() {
		_ = client.Delete(ctx, srcPath)
		_ = client.Delete(ctx, dstPath)
	}()

	if err := client.Copy(ctx, srcPath, dstPath); err != nil {
		t.Fatalf("copy: %v", err)
	}

	// Read the copy.
	rc, size, err := client.Read(ctx, dstPath)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()

	if size != int64(len(content)) {
		t.Errorf("copy size mismatch: got %d, want %d", size, len(content))
	}
	if !bytes.Equal(got, content) {
		t.Errorf("copy content mismatch: got %q, want %q", got, content)
	}
}

func TestWebDAVWriteZeroSizeStream(t *testing.T) {
	client := webdavConnect(t)
	ctx := context.Background()

	filePath := "/webdav_zero_size.txt"
	content := []byte("stream without size hint")

	// Write with size=0 (should use WriteStream instead of WriteStreamWithLength).
	if err := client.Write(ctx, filePath, bytes.NewReader(content), 0); err != nil {
		t.Fatalf("write zero size: %v", err)
	}
	defer func() { _ = client.Delete(ctx, filePath) }()

	rc, _, err := client.Read(ctx, filePath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()

	if !bytes.Equal(got, content) {
		t.Errorf("content mismatch: got %q, want %q", got, content)
	}
}
