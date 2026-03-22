package docker

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

func TestDockerExecCreate(t *testing.T) {
	srv, client := newSecureDockerClientFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/containers/container1/exec" {
			json.NewEncoder(w).Encode(map[string]string{"Id": "exec-abc123"})
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	execID, err := client.createExec(context.Background(), "container1", []string{"/bin/sh"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if execID != "exec-abc123" {
		t.Errorf("execID = %q, want %q", execID, "exec-abc123")
	}
}

func TestDockerExecManagerMaxSessions(t *testing.T) {
	client := NewDockerClient("http://localhost:1") // won't be called
	em := NewDockerExecManager(client)

	// Fill up sessions to the max
	em.mu.Lock()
	for i := 0; i < maxExecSessions; i++ {
		em.sessions[string(rune('a'+i))] = &dockerExecSession{
			sessionID: string(rune('a' + i)),
			cancel:    func() {},
			done:      make(chan struct{}),
		}
	}
	em.mu.Unlock()

	if len(em.sessions) != maxExecSessions {
		t.Fatalf("expected %d sessions, got %d", maxExecSessions, len(em.sessions))
	}
}

func TestDockerExecManagerCloseAll(t *testing.T) {
	client := NewDockerClient("http://localhost:1")
	em := NewDockerExecManager(client)
	em.CloseAll() // should not panic on empty

	if len(em.sessions) != 0 {
		t.Errorf("expected 0 sessions after closeAll, got %d", len(em.sessions))
	}
}

func TestExecConnWriteForwardsBytes(t *testing.T) {
	stream := &mockReadWriteCloser{
		reader: bytes.NewReader(nil),
	}
	conn := &execConn{
		body:   stream,
		writer: stream,
	}

	input := []byte("ls -la\n")
	n, err := conn.Write(input)
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n != len(input) {
		t.Fatalf("wrote %d bytes, want %d", n, len(input))
	}
	if got := stream.writes.String(); got != string(input) {
		t.Fatalf("unexpected forwarded bytes: got %q want %q", got, string(input))
	}
}

func TestExecConnWriteFailsWithoutWriter(t *testing.T) {
	conn := &execConn{
		body: io.NopCloser(bytes.NewReader(nil)),
	}
	n, err := conn.Write([]byte("pwd\n"))
	if n != 0 {
		t.Fatalf("wrote %d bytes, want 0", n)
	}
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("expected io.ErrClosedPipe, got %v", err)
	}
}

func TestDockerExecInputWritesToSessionConn(t *testing.T) {
	client := NewDockerClient("http://localhost:1")
	em := NewDockerExecManager(client)

	local, remote := net.Pipe()
	defer local.Close()
	defer remote.Close()

	sessionID := "sess-1"
	em.mu.Lock()
	em.sessions[sessionID] = &dockerExecSession{
		sessionID: sessionID,
		conn:      local,
		cancel:    func() {},
		done:      make(chan struct{}),
	}
	em.mu.Unlock()

	expected := "echo test\n"
	raw, _ := json.Marshal(agentmgr.DockerExecInputData{
		SessionID: sessionID,
		Data:      base64.StdEncoding.EncodeToString([]byte(expected)),
	})

	received := make(chan string, 1)
	go func() {
		buf := make([]byte, len(expected))
		n, err := io.ReadFull(remote, buf)
		if err != nil {
			received <- "ERR:" + err.Error()
			return
		}
		received <- string(buf[:n])
	}()

	em.HandleExecInput(agentmgr.Message{
		Type: agentmgr.MsgDockerExecInput,
		ID:   sessionID,
		Data: raw,
	})

	select {
	case got := <-received:
		if strings.HasPrefix(got, "ERR:") {
			t.Fatal(got)
		}
		if got != expected {
			t.Fatalf("got %q want %q", got, expected)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for forwarded exec input")
	}
}

func TestDockerExecSessionReasonSetOnce(t *testing.T) {
	sess := &dockerExecSession{}
	sess.setReasonIfUnset("stdin write failed: broken pipe")
	sess.setReasonIfUnset("closed by hub")

	got := sess.reasonOr("exec ended")
	want := "stdin write failed: broken pipe"
	if got != want {
		t.Fatalf("reason = %q, want %q", got, want)
	}
}

func TestStartAndHijackUnixPrefersRawPathWithoutHTTPFallback(t *testing.T) {
	rt := &countingRoundTripper{}
	client := &dockerClient{
		httpClient: &http.Client{Transport: rt},
		baseURL:    "http://localhost",
		unixPath:   "/tmp/labtether-nonexistent.sock",
	}
	em := NewDockerExecManager(client)

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	_, err := em.startAndHijack(ctx, "exec-id", true)
	if err == nil {
		t.Fatal("expected startAndHijack to fail for missing unix socket")
	}
	if got := atomic.LoadInt32(&rt.calls); got != 0 {
		t.Fatalf("expected no HTTP fallback calls, got %d", got)
	}
}

type mockReadWriteCloser struct {
	reader *bytes.Reader
	writes bytes.Buffer
}

func (m *mockReadWriteCloser) Read(p []byte) (int, error) {
	if m.reader == nil {
		return 0, io.EOF
	}
	return m.reader.Read(p)
}

func (m *mockReadWriteCloser) Write(p []byte) (int, error) {
	return m.writes.Write(p)
}

func (m *mockReadWriteCloser) Close() error {
	return nil
}

type countingRoundTripper struct {
	calls int32
}

func (c *countingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	atomic.AddInt32(&c.calls, 1)
	return nil, errors.New("unexpected http transport usage")
}
