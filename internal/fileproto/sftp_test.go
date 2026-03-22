package fileproto

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"path"
	"testing"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// TestSFTPImplementsRemoteFS is a compile-time interface compliance check.
func TestSFTPImplementsRemoteFS(t *testing.T) {
	var _ RemoteFS = (*SFTPClient)(nil)
}

// ---------- in-process SFTP server for integration tests ----------

// testSFTPEnv holds the pieces needed to run tests against an in-process SFTP
// server backed by the real filesystem (under a temp directory).
type testSFTPEnv struct {
	Addr       string // host:port
	HostPubKey string // authorized_key format
	RootDir    string // temp dir used as the filesystem root
	Cleanup    func()
}

// startTestSFTPServer spins up an SSH server with an SFTP subsystem on a
// random port. The server's filesystem root is a temp directory that is
// cleaned up automatically.
func startTestSFTPServer(t *testing.T) testSFTPEnv {
	t.Helper()

	// Generate an ephemeral host key.
	_, hostPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	hostSigner, err := ssh.NewSignerFromKey(hostPriv)
	if err != nil {
		t.Fatalf("host signer: %v", err)
	}
	hostPubKey := string(ssh.MarshalAuthorizedKey(hostSigner.PublicKey()))

	config := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if c.User() == "testuser" && string(pass) == "testpass" {
				return nil, nil
			}
			return nil, fmt.Errorf("auth failed")
		},
	}
	config.AddHostKey(hostSigner)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	rootDir := t.TempDir()
	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handleSSHConn(conn, config)
		}
	}()

	return testSFTPEnv{
		Addr:       listener.Addr().String(),
		HostPubKey: hostPubKey,
		RootDir:    rootDir,
		Cleanup: func() {
			listener.Close()
			<-done
		},
	}
}

func handleSSHConn(conn net.Conn, config *ssh.ServerConfig) {
	defer conn.Close()

	sconn, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return
	}
	defer sconn.Close()
	go ssh.DiscardRequests(reqs)

	for newCh := range chans {
		if newCh.ChannelType() != "session" {
			newCh.Reject(ssh.UnknownChannelType, "unsupported")
			continue
		}
		ch, reqs, err := newCh.Accept()
		if err != nil {
			return
		}
		go func() {
			for req := range reqs {
				if req.Type == "subsystem" && string(req.Payload[4:]) == "sftp" {
					req.Reply(true, nil)

					server, err := sftp.NewServer(ch)
					if err != nil {
						ch.Close()
						return
					}
					_ = server.Serve()
					server.Close()
					return
				}
				if req.WantReply {
					req.Reply(false, nil)
				}
			}
		}()
	}
}

// ---------- test helpers ----------

// testConnect creates a connected SFTPClient against the test server.
func testConnect(t *testing.T, env testSFTPEnv) *SFTPClient {
	t.Helper()

	host, port, _ := net.SplitHostPort(env.Addr)
	var portNum int
	fmt.Sscanf(port, "%d", &portNum)

	client := &SFTPClient{}
	cfg := ConnectionConfig{
		Protocol:    "sftp",
		Host:        host,
		Port:        portNum,
		Username:    "testuser",
		Secret:      "testpass",
		AuthMethod:  "password",
		ExtraConfig: map[string]any{"host_key": env.HostPubKey},
	}

	if err := client.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

// p joins the test root with a relative path to form an absolute path on the
// server's real filesystem.
func p(env testSFTPEnv, rel string) string {
	return path.Join(env.RootDir, rel)
}

// ---------- integration tests ----------

func TestSFTPConnectAndClose(t *testing.T) {
	env := startTestSFTPServer(t)
	defer env.Cleanup()

	_ = testConnect(t, env)
}

func TestSFTPTOFU(t *testing.T) {
	env := startTestSFTPServer(t)
	defer env.Cleanup()

	host, port, _ := net.SplitHostPort(env.Addr)
	var portNum int
	fmt.Sscanf(port, "%d", &portNum)

	client := &SFTPClient{}
	// No host_key in ExtraConfig => TOFU mode.
	cfg := ConnectionConfig{
		Protocol:   "sftp",
		Host:       host,
		Port:       portNum,
		Username:   "testuser",
		Secret:     "testpass",
		AuthMethod: "password",
	}
	if err := client.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	if client.CapturedHostKey == "" {
		t.Error("expected CapturedHostKey to be populated in TOFU mode")
	}
	if client.CapturedFingerprint == "" {
		t.Error("expected CapturedFingerprint to be populated in TOFU mode")
	}
}

func TestSFTPWriteReadDelete(t *testing.T) {
	env := startTestSFTPServer(t)
	defer env.Cleanup()

	client := testConnect(t, env)
	ctx := context.Background()

	filePath := p(env, "test.txt")

	// Write a file.
	content := []byte("hello sftp world")
	if err := client.Write(ctx, filePath, bytes.NewReader(content), int64(len(content))); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Read it back.
	rc, size, err := client.Read(ctx, filePath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()

	if int64(len(content)) != size {
		t.Errorf("size mismatch: got %d, want %d", size, len(content))
	}
	if !bytes.Equal(got, content) {
		t.Errorf("content mismatch: got %q, want %q", got, content)
	}

	// Delete the file.
	if err := client.Delete(ctx, filePath); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Confirm it's gone.
	_, _, err = client.Read(ctx, filePath)
	if err == nil {
		t.Error("expected error reading deleted file")
	}
}

func TestSFTPListHiddenFiles(t *testing.T) {
	env := startTestSFTPServer(t)
	defer env.Cleanup()

	client := testConnect(t, env)
	ctx := context.Background()

	// Create visible and hidden files.
	_ = client.Write(ctx, p(env, "visible.txt"), bytes.NewReader([]byte("v")), 1)
	_ = client.Write(ctx, p(env, ".hidden.txt"), bytes.NewReader([]byte("h")), 1)

	// List without hidden.
	entries, err := client.List(ctx, env.RootDir, false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, e := range entries {
		if e.Name == ".hidden.txt" {
			t.Error("hidden file should be excluded when showHidden=false")
		}
	}

	// List with hidden.
	entries, err = client.List(ctx, env.RootDir, true)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	foundHidden := false
	for _, e := range entries {
		if e.Name == ".hidden.txt" {
			foundHidden = true
		}
	}
	if !foundHidden {
		t.Error("hidden file should be included when showHidden=true")
	}
}

func TestSFTPMkdirAndRecursiveDelete(t *testing.T) {
	env := startTestSFTPServer(t)
	defer env.Cleanup()

	client := testConnect(t, env)
	ctx := context.Background()

	nestedDir := p(env, "a/b/c")

	// Create nested dirs.
	if err := client.Mkdir(ctx, nestedDir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Put a file inside.
	_ = client.Write(ctx, p(env, "a/b/c/deep.txt"), bytes.NewReader([]byte("deep")), 4)

	// Recursively delete /a.
	if err := client.Delete(ctx, p(env, "a")); err != nil {
		t.Fatalf("recursive delete: %v", err)
	}

	// Verify /a is gone.
	entries, err := client.List(ctx, env.RootDir, false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, e := range entries {
		if e.Name == "a" {
			t.Error("directory 'a' should have been deleted")
		}
	}
}

func TestSFTPRename(t *testing.T) {
	env := startTestSFTPServer(t)
	defer env.Cleanup()

	client := testConnect(t, env)
	ctx := context.Background()

	oldPath := p(env, "old.txt")
	newPath := p(env, "new.txt")

	_ = client.Write(ctx, oldPath, bytes.NewReader([]byte("data")), 4)

	if err := client.Rename(ctx, oldPath, newPath); err != nil {
		t.Fatalf("rename: %v", err)
	}

	// old should be gone
	_, _, err := client.Read(ctx, oldPath)
	if err == nil {
		t.Error("expected error reading old path after rename")
	}

	// new should exist
	rc, _, err := client.Read(ctx, newPath)
	if err != nil {
		t.Fatalf("read new: %v", err)
	}
	rc.Close()
}

func TestSFTPCopyFile(t *testing.T) {
	env := startTestSFTPServer(t)
	defer env.Cleanup()

	client := testConnect(t, env)
	ctx := context.Background()

	content := []byte("copy me")
	srcPath := p(env, "src.txt")
	dstPath := p(env, "dst.txt")

	_ = client.Write(ctx, srcPath, bytes.NewReader(content), int64(len(content)))

	if err := client.Copy(ctx, srcPath, dstPath); err != nil {
		t.Fatalf("copy: %v", err)
	}

	rc, _, err := client.Read(ctx, dstPath)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()

	if !bytes.Equal(got, content) {
		t.Errorf("copy content mismatch: got %q, want %q", got, content)
	}
}

func TestSFTPCopyDirectoryReturnsNotSupported(t *testing.T) {
	env := startTestSFTPServer(t)
	defer env.Cleanup()

	client := testConnect(t, env)
	ctx := context.Background()

	dirPath := p(env, "mydir")
	_ = client.Mkdir(ctx, dirPath)

	err := client.Copy(ctx, dirPath, p(env, "mydir2"))
	if !errors.Is(err, ErrNotSupported) {
		t.Errorf("expected ErrNotSupported for directory copy, got: %v", err)
	}
}
