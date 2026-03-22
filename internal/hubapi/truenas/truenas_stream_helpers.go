package truenas

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	tnconnector "github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/terminal"
)

// truenasShellTarget holds the resolved connection info for a TrueNAS shell session.
type truenasShellTarget struct {
	BaseURL    string
	APIKey     string // #nosec G117 -- Runtime connector credential, not a hardcoded secret.
	SkipVerify bool
	Timeout    time.Duration
	Options    map[string]any // vm_id, app_name, container_id
}

var (
	TruenasTerminalWriteToNAS = func(conn *websocket.Conn, msgType int, data []byte) error {
		return conn.WriteMessage(msgType, data)
	}
	TruenasTerminalWriteToBrowser = func(conn *websocket.Conn, msgType int, data []byte) error {
		return conn.WriteMessage(msgType, data)
	}
)

// resolveTrueNASSessionTarget checks if the terminal session's target asset is
// backed by a TrueNAS collector and returns the connection details.
func (d *Deps) ResolveTrueNASSessionTarget(assetID string) (TruenasShellTarget, bool, error) {
	if d.AssetStore == nil {
		return TruenasShellTarget{}, false, nil
	}

	asset, ok, err := d.AssetStore.GetAsset(strings.TrimSpace(assetID))
	if err != nil {
		return TruenasShellTarget{}, false, err
	}
	if !ok || !strings.EqualFold(strings.TrimSpace(asset.Source), "truenas") {
		return TruenasShellTarget{}, false, nil
	}

	preferredCollectorID := strings.TrimSpace(asset.Metadata["collector_id"])
	runtime, runtimeErr := d.LoadTrueNASRuntime(preferredCollectorID)
	if runtimeErr != nil && preferredCollectorID != "" {
		// Asset metadata may be stale after collector recreation; fall back to first active collector.
		runtime, runtimeErr = d.LoadTrueNASRuntime("")
	}
	if runtimeErr != nil {
		return TruenasShellTarget{}, false, runtimeErr
	}

	target := TruenasShellTarget{
		BaseURL:    runtime.BaseURL,
		APIKey:     runtime.APIKey,
		SkipVerify: runtime.SkipVerify,
		Timeout:    runtime.Timeout,
	}

	// Determine shell options based on asset type.
	switch asset.Type {
	case "vm":
		if vmID := strings.TrimSpace(asset.Metadata["vm_id"]); vmID != "" {
			target.Options = map[string]any{"vm_id": vmID}
		}
	case "app":
		target.Options = map[string]any{"app_name": asset.Name}
	}

	return target, true, nil
}

// tryTrueNASTerminalStream attempts to proxy a terminal session through TrueNAS
// WebSocket shell. Returns nil on success (including if it wrote an HTTP error),
// or an error if the proxy could not be established.
func (d *Deps) TryTrueNASTerminalStream(w http.ResponseWriter, r *http.Request, session terminal.Session, target TruenasShellTarget) error {
	ctx := r.Context()

	// Step 1: Get an auth token from TrueNAS.
	client := &tnconnector.Client{
		BaseURL:    target.BaseURL,
		APIKey:     target.APIKey,
		SkipVerify: target.SkipVerify,
		Timeout:    target.Timeout,
	}

	var authToken string
	if err := client.Call(ctx, "auth.generate_token", []any{300}, &authToken); err != nil {
		return fmt.Errorf("truenas auth.generate_token failed: %w", err)
	}

	// Step 2: Upgrade browser connection to WebSocket.
	upgrader := websocket.Upgrader{
		CheckOrigin: d.CheckSameOrigin,
	}
	browserWS, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return fmt.Errorf("browser ws upgrade: %w", err)
	}
	defer browserWS.Close()

	// Step 3: Connect to TrueNAS shell endpoint.
	shellURL := TruenasShellEndpoint(target.BaseURL)
	if _, err := securityruntime.ValidateOutboundURL(shellURL); err != nil {
		return fmt.Errorf("truenas shell endpoint blocked by runtime policy: %w", err)
	}
	dialer := websocket.Dialer{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			// #nosec G402 -- user-controlled homelab setting for self-signed certs.
			InsecureSkipVerify: target.SkipVerify, //nolint:gosec // #nosec G402 -- user-controlled homelab setting
		},
	}

	nasWS, _, err := dialer.DialContext(ctx, shellURL, nil)
	if err != nil {
		_ = browserWS.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Failed to connect to TrueNAS shell: %v\r\n", err)))
		return fmt.Errorf("truenas shell dial: %w", err)
	}
	defer nasWS.Close()

	// Step 4: Send auth message with token and options.
	authMsg := map[string]any{"token": authToken}
	if target.Options != nil {
		authMsg["options"] = target.Options
	}
	if err := nasWS.WriteJSON(authMsg); err != nil {
		return fmt.Errorf("truenas shell auth send: %w", err)
	}

	// Step 5: Wait for connected response.
	_, msg, err := nasWS.ReadMessage()
	if err != nil {
		return fmt.Errorf("truenas shell auth read: %w", err)
	}

	var connResp map[string]any
	if err := json.Unmarshal(msg, &connResp); err == nil {
		if connResp["msg"] == "failed" {
			errInfo, _ := json.Marshal(connResp["error"])
			return fmt.Errorf("truenas shell auth failed: %s", string(errInfo))
		}
	}

	securityruntime.Logf("terminal-truenas: shell connected for session %s (asset=%s)", session.ID, session.Target)

	// Step 6: Bidirectional proxy.
	var browserWriteMu sync.Mutex
	stopKeepalive := d.StartBrowserWebSocketKeepalive(browserWS, &browserWriteMu, "terminal-truenas:"+session.ID)
	defer stopKeepalive()

	var wg sync.WaitGroup
	wg.Add(2)

	// Browser -> TrueNAS
	go func() {
		defer wg.Done()
		for {
			msgType, data, err := browserWS.ReadMessage()
			if err != nil {
				_ = nasWS.Close()
				return
			}
			_ = d.TouchBrowserWebSocketReadDeadline(browserWS)
			if err := TruenasTerminalWriteToNAS(nasWS, msgType, data); err != nil {
				return
			}
		}
	}()

	// TrueNAS -> Browser
	go func() {
		defer wg.Done()
		for {
			msgType, data, err := nasWS.ReadMessage()
			if err != nil {
				_ = browserWS.Close()
				return
			}
			browserWriteMu.Lock()
			err = TruenasTerminalWriteToBrowser(browserWS, msgType, data)
			browserWriteMu.Unlock()
			if err != nil {
				return
			}
		}
	}()

	wg.Wait()
	return nil
}

// TruenasShellEndpoint converts a TrueNAS base URL to the WebSocket shell endpoint.
func TruenasShellEndpoint(baseURL string) string {
	raw := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	u, err := url.Parse(raw)
	if err != nil {
		return "wss://" + raw + "/websocket/shell"
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	case "wss", "ws":
		// already correct
	default:
		u.Scheme = "wss"
	}
	u.Path = "/websocket/shell"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
