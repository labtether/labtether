package docker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/securityruntime"
)

const maxExecSessions = 10

// dockerExecSession tracks an active exec session in a container.
type dockerExecSession struct {
	sessionID string
	execID    string
	conn      net.Conn // hijacked connection from Docker exec start
	cancel    context.CancelFunc
	done      chan struct{}

	mu          sync.Mutex
	closeReason string
}

// dockerExecManager manages Docker exec sessions on the agent.
type DockerExecManager struct {
	mu       sync.Mutex
	sessions map[string]*dockerExecSession
	client   *dockerClient
}

func NewDockerExecManager(client *dockerClient) *DockerExecManager {
	return &DockerExecManager{
		sessions: make(map[string]*dockerExecSession),
		client:   client,
	}
}

func (em *DockerExecManager) HandleExecStart(transport Transport, msg agentmgr.Message) {
	var req agentmgr.DockerExecStartData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("docker-exec: invalid start request: %v", err)
		return
	}

	if req.SessionID == "" || req.ContainerID == "" {
		log.Printf("docker-exec: missing session_id or container_id")
		return
	}
	if !isValidContainerID(req.ContainerID) {
		log.Printf("docker-exec: invalid container_id %q", req.ContainerID)
		sendExecClosed(transport, req.SessionID, "invalid container ID")
		return
	}

	em.mu.Lock()
	if len(em.sessions) >= maxExecSessions {
		em.mu.Unlock()
		log.Printf("docker-exec: max sessions (%d) reached", maxExecSessions)
		sendExecClosed(transport, req.SessionID, "max exec sessions reached")
		return
	}
	if _, exists := em.sessions[req.SessionID]; exists {
		em.mu.Unlock()
		log.Printf("docker-exec: session %s already exists", req.SessionID)
		return
	}
	em.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())

	// Create exec instance
	cmd := req.Command
	if len(cmd) == 0 {
		cmd = []string{"/bin/sh"}
	}
	execID, err := em.client.createExec(ctx, req.ContainerID, cmd, req.TTY)
	if err != nil {
		cancel()
		log.Printf("docker-exec: failed to create exec: %v", err)
		sendExecClosed(transport, req.SessionID, "failed to create exec: "+err.Error())
		return
	}

	// Start exec and hijack the connection for bidirectional I/O.
	conn, err := em.startAndHijack(ctx, execID, req.TTY)
	if err != nil {
		cancel()
		log.Printf("docker-exec: failed to start exec: %v", err)
		sendExecClosed(transport, req.SessionID, "failed to start exec: "+err.Error())
		return
	}

	sess := &dockerExecSession{
		sessionID: req.SessionID,
		execID:    execID,
		conn:      conn,
		cancel:    cancel,
		done:      make(chan struct{}),
	}

	em.mu.Lock()
	em.sessions[req.SessionID] = sess
	em.mu.Unlock()

	// Notify hub that exec is ready.
	sendExecStarted(transport, req.SessionID)

	// Stream output → hub.
	go em.streamExecOutput(transport, sess)

	// Wait for connection to close and clean up.
	go func() {
		<-sess.done
		em.cleanup(req.SessionID)
		sendExecClosed(transport, req.SessionID, sess.reasonOr("exec ended"))
	}()
}

// startAndHijack starts an exec instance and returns the hijacked connection.
// This uses a raw HTTP request to get the underlying TCP connection for I/O.
func (em *DockerExecManager) startAndHijack(ctx context.Context, execID string, tty bool) (net.Conn, error) {
	body := map[string]any{"Detach": false, "Tty": tty}
	jsonBody, _ := json.Marshal(body)

	// For unix-socket endpoints, use the raw hijack path directly.
	// This avoids double-starting the same exec ID across mixed fallback paths.
	if em != nil && em.client != nil && strings.TrimSpace(em.client.unixPath) != "" {
		return em.startAndHijackRawUnix(ctx, execID, jsonBody)
	}

	req, err := securityruntime.NewOutboundRequestWithContext(ctx, http.MethodPost,
		em.client.baseURL+"/exec/"+execID+"/start",
		bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "tcp")

	resp, err := securityruntime.DoOutboundRequest(em.client.httpClient, req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if closeErr := resp.Body.Close(); closeErr != nil {
			return nil, fmt.Errorf("exec start returned %d: %s (close body: %v)", resp.StatusCode, string(body), closeErr)
		}
		return nil, fmt.Errorf("exec start returned %d: %s", resp.StatusCode, string(body))
	}

	// In production with a real Docker socket the connection would be hijacked
	// for true bidirectional I/O. net/http exposes the upgraded stream through
	// resp.Body and, for upgraded responses, Body also implements io.Writer.
	writer, ok := resp.Body.(io.Writer)
	if !ok {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("exec start stream is read-only; writable upgraded stream unavailable")
	}
	return &execConn{body: resp.Body, writer: writer}, nil
}

// startAndHijackRawUnix opens a raw unix socket to Docker and performs an
// HTTP upgrade request manually, returning a bidirectional stream connection.
func (em *DockerExecManager) startAndHijackRawUnix(ctx context.Context, execID string, payload []byte) (net.Conn, error) {
	if em == nil || em.client == nil || strings.TrimSpace(em.client.unixPath) == "" {
		return nil, fmt.Errorf("docker unix socket is not configured")
	}

	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "unix", em.client.unixPath)
	if err != nil {
		return nil, err
	}

	request := fmt.Sprintf(
		"POST /exec/%s/start HTTP/1.1\r\nHost: docker\r\nConnection: Upgrade\r\nUpgrade: tcp\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n",
		execID,
		len(payload),
	)
	if _, err := conn.Write([]byte(request)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if _, err := conn.Write(payload); err != nil {
		_ = conn.Close()
		return nil, err
	}

	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	statusLine = strings.TrimSpace(statusLine)
	parts := strings.SplitN(statusLine, " ", 3)
	if len(parts) < 2 {
		_ = conn.Close()
		return nil, fmt.Errorf("unexpected docker exec status line: %q", statusLine)
	}
	statusCode, err := strconv.Atoi(parts[1])
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("invalid docker exec status code in %q", statusLine)
	}

	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			_ = conn.Close()
			return nil, readErr
		}
		if line == "\r\n" || line == "\n" {
			break
		}
	}

	if statusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(reader, 1024))
		_ = conn.Close()
		return nil, fmt.Errorf("exec start returned %d: %s", statusCode, string(body))
	}

	if statusCode != http.StatusSwitchingProtocols && statusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(reader, 1024))
		_ = conn.Close()
		return nil, fmt.Errorf("exec start returned unexpected status %d: %s", statusCode, string(body))
	}

	buffered := reader.Buffered()
	if buffered == 0 {
		return conn, nil
	}
	prefix, err := reader.Peek(buffered)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	remaining := make([]byte, len(prefix))
	copy(remaining, prefix)
	_, _ = reader.Discard(buffered)

	return &hijackedConn{
		Conn:   conn,
		reader: io.MultiReader(bytes.NewReader(remaining), conn),
	}, nil
}

// execConn wraps an io.ReadCloser as a net.Conn for exec I/O.
// Only Read, Write, and Close are used; the embedded net.Conn satisfies the
// interface for unused methods and will panic if called unexpectedly.
type execConn struct {
	body   io.ReadCloser
	writer io.Writer
	net.Conn
}

func (ec *execConn) Read(p []byte) (int, error) {
	return ec.body.Read(p)
}

// Write forwards stdin data to the exec connection.
func (ec *execConn) Write(p []byte) (int, error) {
	if ec.writer == nil {
		return 0, io.ErrClosedPipe
	}
	return ec.writer.Write(p)
}

func (ec *execConn) Close() error {
	return ec.body.Close()
}

// hijackedConn reads from a buffered reader first, then the underlying socket.
type hijackedConn struct {
	net.Conn
	reader io.Reader
}

func (hc *hijackedConn) Read(p []byte) (int, error) {
	return hc.reader.Read(p)
}

func (em *DockerExecManager) HandleExecInput(msg agentmgr.Message) {
	var payload agentmgr.DockerExecInputData
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		return
	}

	em.mu.Lock()
	sess, ok := em.sessions[payload.SessionID]
	em.mu.Unlock()
	if !ok {
		return
	}

	decoded, err := base64.StdEncoding.DecodeString(payload.Data)
	if err != nil {
		log.Printf("docker-exec: failed to decode stdin for session %s: %v", payload.SessionID, err)
		return
	}
	if len(decoded) == 0 {
		return
	}
	if _, err := sess.conn.Write(decoded); err != nil {
		sess.setReasonIfUnset("stdin write failed: " + err.Error())
		log.Printf("docker-exec: stdin write failed for session %s: %v", payload.SessionID, err)
		_ = sess.conn.Close()
		sess.cancel()
	}
}

func (em *DockerExecManager) HandleExecResize(msg agentmgr.Message) {
	var req agentmgr.DockerExecResizeData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		return
	}

	em.mu.Lock()
	sess, ok := em.sessions[req.SessionID]
	em.mu.Unlock()
	if !ok {
		return
	}

	if req.Cols <= 0 || req.Rows <= 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := em.client.post(ctx, fmt.Sprintf("/exec/%s/resize?h=%d&w=%d", sess.execID, req.Rows, req.Cols))
	if err != nil {
		log.Printf("docker-exec: resize failed for session %s: %v", req.SessionID, err)
	}
}

func (em *DockerExecManager) HandleExecClose(msg agentmgr.Message) {
	var req agentmgr.DockerExecCloseData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		return
	}

	em.mu.Lock()
	sess, ok := em.sessions[req.SessionID]
	em.mu.Unlock()
	if !ok {
		return
	}

	sess.setReasonIfUnset("closed by hub")
	_ = sess.conn.Close()
	sess.cancel()
}

func (em *DockerExecManager) streamExecOutput(transport Transport, sess *dockerExecSession) {
	defer close(sess.done)

	buf := make([]byte, 4096)
	for {
		n, err := sess.conn.Read(buf)
		if n > 0 {
			encoded := base64.StdEncoding.EncodeToString(buf[:n])
			data, _ := json.Marshal(agentmgr.DockerExecDataPayload{
				SessionID: sess.sessionID,
				Data:      encoded,
			})
			_ = transport.Send(agentmgr.Message{
				Type: agentmgr.MsgDockerExecData,
				ID:   sess.sessionID,
				Data: data,
			})
		}
		if err != nil {
			if err != io.EOF {
				sess.setReasonIfUnset("stdout read failed: " + err.Error())
				log.Printf("docker-exec: output stream ended for session %s: %v", sess.sessionID, err)
			}
			return
		}
	}
}

func (s *dockerExecSession) setReasonIfUnset(reason string) {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closeReason == "" {
		s.closeReason = trimmed
	}
}

func (s *dockerExecSession) reasonOr(fallback string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(s.closeReason) != "" {
		return s.closeReason
	}
	return fallback
}

func (em *DockerExecManager) cleanup(sessionID string) {
	em.mu.Lock()
	defer em.mu.Unlock()
	if sess, ok := em.sessions[sessionID]; ok {
		_ = sess.conn.Close()
		sess.cancel()
		delete(em.sessions, sessionID)
	}
}

func (em *DockerExecManager) CloseAll() {
	em.mu.Lock()
	defer em.mu.Unlock()
	for id, sess := range em.sessions {
		_ = sess.conn.Close()
		sess.cancel()
		delete(em.sessions, id)
	}
}

func sendExecStarted(transport Transport, sessionID string) {
	data, _ := json.Marshal(agentmgr.DockerExecStartedData{SessionID: sessionID})
	_ = transport.Send(agentmgr.Message{
		Type: agentmgr.MsgDockerExecStarted,
		ID:   sessionID,
		Data: data,
	})
}

func sendExecClosed(transport Transport, sessionID, reason string) {
	data, _ := json.Marshal(agentmgr.DockerExecCloseData{SessionID: sessionID, Reason: reason})
	_ = transport.Send(agentmgr.Message{
		Type: agentmgr.MsgDockerExecClosed,
		ID:   sessionID,
		Data: data,
	})
}
