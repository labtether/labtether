package portainer

import (
	"github.com/labtether/labtether/internal/hubapi/shared"
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
)

// handlePortainerContainerExec upgrades the HTTP connection to WebSocket and
// proxies an exec session into a Docker container via the Portainer WebSocket
// exec endpoint.
//
// Route: GET /portainer/assets/{assetID}/containers/{containerID}/exec
//
// Flow:
//  1. Resolve asset + Portainer runtime.
//  2. Create a TTY exec instance via POST to Portainer's Docker exec API.
//  3. Obtain the Portainer WebSocket exec URL (requires JWT auth).
//  4. Dial Portainer's exec WebSocket.
//  5. Upgrade the browser connection to WebSocket.
//  6. Bidirectionally proxy data between browser and Portainer.
func (d *Deps) HandlePortainerContainerExec(w http.ResponseWriter, r *http.Request, assetID, containerID string) {
	securityruntime.Logf("portainer-exec: request asset=%s container=%.12s", assetID, containerID)

	if !d.RequireAdminAuth(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !websocket.IsWebSocketUpgrade(r) {
		servicehttp.WriteError(w, http.StatusBadRequest, "websocket upgrade required")
		return
	}

	asset, runtime, err := d.ResolvePortainerAssetRuntime(assetID)
	if err != nil {
		writePortainerResolveError(w, err)
		return
	}

	epID, err := portainerEndpointID(asset)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	shell := portainerExecShellFromQuery(r.URL.Query().Get("shell"))

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	execID, err := runtime.Client.CreateExec(ctx, epID, containerID, shell)
	cancel()
	if err != nil {
		securityruntime.Logf("portainer-exec: create exec failed: %v", err)
		servicehttp.WriteError(w, http.StatusBadGateway, shared.SanitizeUpstreamError(err.Error()))
		return
	}
	securityruntime.Logf("portainer-exec: exec instance created exec_id=%.16s", execID)

	ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
	wsURL, _, wsErr := runtime.Client.ExecWebSocketURL(ctx2, epID, execID)
	cancel2()
	if wsErr != nil {
		securityruntime.Logf("portainer-exec: get ws url failed: %v", wsErr)
		servicehttp.WriteError(w, http.StatusBadGateway, shared.SanitizeUpstreamError(wsErr.Error()))
		return
	}

	portainerConn, err := dialPortainerExecSocket(wsURL, runtime.SkipVerify)
	if err != nil {
		securityruntime.Logf("portainer-exec: dial portainer ws failed: %v", err)
		servicehttp.WriteError(w, http.StatusBadGateway, shared.SanitizeUpstreamError(err.Error()))
		return
	}
	defer portainerConn.Close()
	securityruntime.Logf("portainer-exec: portainer ws connected")

	// Upgrade the browser connection. Past this point we own the response.
	browserConn, err := d.TerminalWebSocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade already wrote an error response.
		return
	}
	defer browserConn.Close()

	securityruntime.Logf("portainer-exec: bridge started asset=%s container=%.12s", assetID, containerID)
	d.bridgePortainerExec(browserConn, portainerConn)
	securityruntime.Logf("portainer-exec: bridge ended asset=%s container=%.12s", assetID, containerID)
}

// dialPortainerExecSocket dials the Portainer exec WebSocket URL.
// The URL already contains the JWT token as a query parameter.
func dialPortainerExecSocket(wsURL string, skipVerify bool) (*websocket.Conn, error) {
	if _, err := securityruntime.ValidateOutboundURL(
		strings.Replace(strings.Replace(wsURL, "wss://", "https://", 1), "ws://", "http://", 1),
	); err != nil {
		return nil, fmt.Errorf("exec websocket url rejected: %w", err)
	}

	dialer := &websocket.Dialer{
		HandshakeTimeout: 15 * time.Second,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			// #nosec G402 -- runtime-configurable for self-signed homelab certs.
			InsecureSkipVerify: skipVerify, //nolint:gosec // #nosec G402 -- user-controlled homelab setting
		},
	}

	conn, httpResp, err := dialer.Dial(wsURL, nil)
	if err != nil {
		detail := err.Error()
		if httpResp != nil {
			body := make([]byte, 512)
			n, _ := httpResp.Body.Read(body)
			_ = httpResp.Body.Close()
			if n > 0 {
				detail = fmt.Sprintf("%s (HTTP %d: %s)", detail, httpResp.StatusCode, strings.TrimSpace(string(body[:n])))
			} else {
				detail = fmt.Sprintf("%s (HTTP %d)", detail, httpResp.StatusCode)
			}
		}
		return nil, fmt.Errorf("dial portainer exec: %s", detail)
	}
	return conn, nil
}

// bridgePortainerExec bidirectionally proxies data between the browser WebSocket
// and the Portainer exec WebSocket. It uses a mutex-protected write path for
// the browser connection to coordinate with the keepalive goroutine.
func (d *Deps) bridgePortainerExec(browserConn, portainerConn *websocket.Conn) {
	var browserWriteMu sync.Mutex
	stopKeepalive := d.StartBrowserWebSocketKeepalive(browserConn, &browserWriteMu, "portainer-exec")
	defer stopKeepalive()

	done := make(chan struct{})

	// Portainer → Browser
	go func() {
		defer close(done)
		for {
			_ = portainerConn.SetReadDeadline(time.Now().Add(90 * time.Second))
			msgType, payload, err := portainerConn.ReadMessage()
			if err != nil {
				return
			}
			browserWriteMu.Lock()
			err = browserConn.WriteMessage(msgType, payload)
			browserWriteMu.Unlock()
			if err != nil {
				return
			}
		}
	}()

	// Browser → Portainer
	for {
		select {
		case <-done:
			return
		default:
		}

		msgType, payload, err := browserConn.ReadMessage()
		if err != nil {
			return
		}
		_ = d.TouchBrowserWebSocketReadDeadline(browserConn)
		if msgType != websocket.TextMessage && msgType != websocket.BinaryMessage {
			continue
		}
		if err := portainerConn.WriteMessage(msgType, payload); err != nil {
			return
		}
	}
}

// portainerExecShellFromQuery parses the ?shell= query parameter into a command
// slice. Accepts a single shell name (e.g. "bash") or a simple space-separated
// command. Falls back to ["/bin/sh"] when absent or invalid.
func portainerExecShellFromQuery(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil // caller will default to /bin/sh
	}
	fields := strings.Fields(trimmed)
	const maxTokens = 8
	const maxTokenLen = 128
	if len(fields) > maxTokens {
		fields = fields[:maxTokens]
	}
	result := make([]string, 0, len(fields))
	for _, f := range fields {
		if t := strings.TrimSpace(f); t != "" && len(t) <= maxTokenLen {
			result = append(result, t)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
