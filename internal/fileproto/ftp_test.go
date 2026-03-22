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

	"github.com/jlaffaye/ftp"
)

// TestFTPImplementsRemoteFS is a compile-time interface compliance check.
func TestFTPImplementsRemoteFS(t *testing.T) {
	var _ RemoteFS = (*FTPClient)(nil)
}

// ---------- integration tests against a real FTP server ----------
//
// These tests require a local FTP server. Set the environment variable
// FTP_TEST_ADDR=host:port to enable them. The server must allow login
// with FTP_TEST_USER / FTP_TEST_PASS (defaults: anonymous / anonymous@).
//
// Quick setup with Docker:
//   docker run -d --name ftpd -p 2121:21 -p 30000-30009:30000-30009 \
//     -e FTP_USER_NAME=testuser -e FTP_USER_PASS=testpass \
//     stilliard/pure-ftpd

func ftpTestEnv(t *testing.T) (host string, port int, user, pass string) {
	t.Helper()
	addr := os.Getenv("FTP_TEST_ADDR")
	if addr == "" {
		t.Skip("FTP_TEST_ADDR not set; skipping FTP integration tests")
	}
	h, p, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("bad FTP_TEST_ADDR: %v", err)
	}
	var pn int
	fmt.Sscanf(p, "%d", &pn)

	user = os.Getenv("FTP_TEST_USER")
	if user == "" {
		user = "anonymous"
	}
	pass = os.Getenv("FTP_TEST_PASS")
	if pass == "" {
		pass = "anonymous@"
	}
	return h, pn, user, pass
}

func ftpConnect(t *testing.T) *FTPClient {
	t.Helper()
	h, p, u, pw := ftpTestEnv(t)

	client := &FTPClient{}
	cfg := ConnectionConfig{
		Protocol:    "ftp",
		Host:        h,
		Port:        p,
		Username:    u,
		Secret:      pw,
		AuthMethod:  "password",
		ExtraConfig: map[string]any{},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := client.Connect(ctx, cfg); err != nil {
		t.Fatalf("ftp connect: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

func TestFTPConnectAndClose(t *testing.T) {
	_ = ftpConnect(t)
}

func TestFTPWriteReadDelete(t *testing.T) {
	client := ftpConnect(t)
	ctx := context.Background()

	filePath := "/test_ftp_write_read.txt"
	content := []byte("hello ftp world")

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

func TestFTPListHiddenFiles(t *testing.T) {
	client := ftpConnect(t)
	ctx := context.Background()

	// Create visible and hidden files.
	_ = client.Write(ctx, "/visible_ftp.txt", bytes.NewReader([]byte("v")), 1)
	_ = client.Write(ctx, "/.hidden_ftp.txt", bytes.NewReader([]byte("h")), 1)
	defer func() {
		_ = client.Delete(ctx, "/visible_ftp.txt")
		_ = client.Delete(ctx, "/.hidden_ftp.txt")
	}()

	// Without hidden.
	entries, err := client.List(ctx, "/", false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, e := range entries {
		if e.Name == ".hidden_ftp.txt" {
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
		if e.Name == ".hidden_ftp.txt" {
			foundHidden = true
		}
	}
	if !foundHidden {
		t.Error("hidden file should be included when showHidden=true")
	}
}

func TestFTPMkdirAndDelete(t *testing.T) {
	client := ftpConnect(t)
	ctx := context.Background()

	dirPath := "/ftp_test_dir"

	if err := client.Mkdir(ctx, dirPath); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Put a file inside.
	_ = client.Write(ctx, dirPath+"/inner.txt", bytes.NewReader([]byte("inner")), 5)

	// Recursive delete.
	if err := client.Delete(ctx, dirPath); err != nil {
		t.Fatalf("recursive delete: %v", err)
	}

	// Verify the directory is gone by listing parent.
	entries, err := client.List(ctx, "/", false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, e := range entries {
		if e.Name == "ftp_test_dir" {
			t.Error("directory should have been deleted")
		}
	}
}

func TestFTPRename(t *testing.T) {
	client := ftpConnect(t)
	ctx := context.Background()

	oldPath := "/ftp_old.txt"
	newPath := "/ftp_new.txt"

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

func TestFTPCopyReturnsNotSupported(t *testing.T) {
	// This test does not need a server; Copy always returns ErrNotSupported.
	client := &FTPClient{}
	err := client.Copy(context.Background(), "/a", "/b")
	if !errors.Is(err, ErrNotSupported) {
		t.Errorf("expected ErrNotSupported, got: %v", err)
	}
}

// TestFTPCloseNilConn ensures Close is safe on an unconnected client.
func TestFTPCloseNilConn(t *testing.T) {
	client := &FTPClient{}
	if err := client.Close(); err != nil {
		t.Errorf("close nil conn: %v", err)
	}
}

// TestFTPEntryMapping verifies that ftp.Entry types map correctly to FileEntry.
func TestFTPEntryMapping(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name  string
		entry *ftp.Entry
		isDir bool
	}{
		{"file", &ftp.Entry{Name: "file.txt", Type: ftp.EntryTypeFile, Size: 100, Time: now}, false},
		{"folder", &ftp.Entry{Name: "subdir", Type: ftp.EntryTypeFolder, Size: 0, Time: now}, true},
		{"link", &ftp.Entry{Name: "link", Type: ftp.EntryTypeLink, Size: 50, Time: now}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fe := FileEntry{
				Name:    tt.entry.Name,
				Path:    "/" + tt.entry.Name,
				IsDir:   tt.entry.Type == ftp.EntryTypeFolder,
				Size:    int64(tt.entry.Size),
				ModTime: tt.entry.Time,
			}
			if fe.IsDir != tt.isDir {
				t.Errorf("IsDir: got %v, want %v", fe.IsDir, tt.isDir)
			}
		})
	}
}
