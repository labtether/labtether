package desktop

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/guacamole"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
)

const (
	defaultGUACDHost = "127.0.0.1"
	defaultGUACDPort = 4822
)

// HandleGuacdDesktopStream handles RDP desktop streams through guacd.
func (d *Deps) HandleGuacdDesktopStream(w http.ResponseWriter, r *http.Request, session terminal.Session) {
	guacdHost := strings.TrimSpace(os.Getenv("GUACD_HOST"))
	if guacdHost == "" {
		guacdHost = defaultGUACDHost
	}
	guacdPort := defaultGUACDPort
	if raw := strings.TrimSpace(os.Getenv("GUACD_PORT")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			guacdPort = parsed
		}
	}

	targetHost, targetPort, username, password := d.ResolveRDPTarget(session)
	// Guacd performs the target connection out-of-process. Resolve and validate
	// every managed or direct target here, then pass guacd only the approved IP
	// literal. This prevents a second DNS lookup/rebinding hop and applies the
	// loopback/link-local/private-network policy uniformly to asset metadata.
	resolvedHost, resolveErr := securityruntime.ResolveOutboundTCPHost(r.Context(), targetHost, targetPort)
	if resolveErr != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid RDP target: "+d.SanitizeUpstreamError(resolveErr.Error()))
		return
	}
	targetHost = resolvedHost
	client, err := guacamole.Connect(guacdHost, guacdPort)
	if err != nil {
		log.Printf("rdp: guacd connect failed: %v", err)
		servicehttp.WriteError(w, http.StatusBadGateway, "guacd unavailable")
		return
	}
	defer client.Close()
	if err := client.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "guacd unavailable")
		return
	}

	if err := client.SelectProtocol("rdp"); err != nil {
		log.Printf("rdp: select protocol failed: %v", err)
		servicehttp.WriteError(w, http.StatusBadGateway, "guacd protocol negotiation failed")
		return
	}

	// guacd responds with "args" after protocol selection. Preserve the
	// advertised order; it is version-dependent.
	opcode, argNames, err := client.ReadInstruction()
	if err != nil {
		log.Printf("rdp: read args failed: %v", err)
		servicehttp.WriteError(w, http.StatusBadGateway, "guacd handshake failed")
		return
	}
	if opcode != "args" || len(argNames) == 0 {
		log.Printf("rdp: unexpected guacd handshake opcode=%s", opcode) // #nosec G706 -- Value comes from the reviewed guacd control channel, not direct user input.
		servicehttp.WriteError(w, http.StatusBadGateway, "guacd handshake failed")
		return
	}

	params := map[string]string{
		"hostname":          targetHost,
		"port":              strconv.Itoa(targetPort),
		"username":          username,
		"password":          password,
		"domain":            "",
		"security":          "any",
		"ignore-cert":       "true",
		"disable-auth":      "",
		"width":             "1920",
		"height":            "1080",
		"dpi":               "96",
		"enable-audio":      "true",
		"enable-drive":      "false",
		"enable-printing":   "false",
		"drive-path":        "",
		"create-drive-path": "",
	}
	if err := client.SendHandshake(argNames, params, guacamole.ClientInformation{
		Width:          1920,
		Height:         1080,
		DPI:            96,
		AudioMIMETypes: []string{"audio/L16", "audio/L8"},
		ImageMIMETypes: []string{"image/png", "image/jpeg"},
		Name:           "LabTether",
	}); err != nil {
		log.Printf("rdp: connect instruction failed: %v", err)
		servicehttp.WriteError(w, http.StatusBadGateway, "guacd connect failed")
		return
	}

	opcode, args, err := client.ReadInstruction()
	if err != nil {
		log.Printf("rdp: ready read failed: %v", err)
		servicehttp.WriteError(w, http.StatusBadGateway, "guacd session setup failed")
		return
	}
	if opcode != "ready" {
		log.Printf("rdp: unexpected guacd opcode=%s args=%v", opcode, args) // #nosec G706 -- Values come from the reviewed guacd control channel, not direct user-controlled log text.
		servicehttp.WriteError(w, http.StatusBadGateway, "guacd did not return ready")
		return
	}
	if err := client.SetDeadline(time.Time{}); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "guacd session setup failed")
		return
	}

	browserWS, err := shared.UpgradeWebSocket(d.TerminalWebSocketUpgrader, w, r, nil)
	if err != nil {
		return
	}
	shared.LimitBrowserInteractiveMessages(browserWS)
	defer browserWS.Close()
	var writeMu sync.Mutex
	stopKeepalive := d.StartBrowserWSKeepalive(browserWS, &writeMu, "desktop-rdp:"+session.ID)
	defer stopKeepalive()

	done := make(chan struct{})
	var doneOnce sync.Once
	closeDone := func() { doneOnce.Do(func() { close(done) }) }
	go func() {
		defer closeDone()
		buf := make([]byte, 32768)
		for {
			n, readErr := client.Read(buf)
			if n > 0 {
				writeMu.Lock()
				_ = browserWS.SetWriteDeadline(time.Now().Add(10 * time.Second))
				writeErr := browserWS.WriteMessage(websocket.TextMessage, buf[:n])
				writeMu.Unlock()
				if writeErr != nil {
					return
				}
			}
			if readErr != nil {
				return
			}
		}
	}()

	stopCloser := make(chan struct{})
	defer close(stopCloser)
	go func() {
		select {
		case <-done:
			_ = browserWS.SetReadDeadline(time.Now())
			_ = browserWS.Close()
		case <-stopCloser:
		}
	}()

	for {
		select {
		case <-done:
			return
		default:
		}
		messageType, payload, readErr := browserWS.ReadMessage()
		if readErr != nil {
			break
		}
		_ = d.TouchBrowserWSReadDeadline(browserWS)
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			continue
		}
		if _, writeErr := client.Write(payload); writeErr != nil {
			break
		}
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}

// ResolveRDPTarget resolves the RDP connection target for an asset.
func (d *Deps) ResolveRDPTarget(session terminal.Session) (host string, port int, username string, password string) {
	opts := d.GetDesktopSessionOptions(session.ID)
	if opts.Direct {
		return opts.DirectHost, opts.DirectPort, opts.DirectUsername, opts.DirectPassword
	}
	assetID := session.Target
	host = strings.TrimSpace(assetID)
	port = 3389

	if d.AssetStore != nil {
		if assetEntry, ok, err := d.AssetStore.GetAsset(assetID); err == nil && ok {
			candidates := []string{
				strings.TrimSpace(assetEntry.Metadata["rdp_host"]),
				strings.TrimSpace(assetEntry.Metadata["host"]),
				strings.TrimSpace(assetEntry.Metadata["hostname"]),
				strings.TrimSpace(assetEntry.Metadata["ip"]),
				strings.TrimSpace(assetEntry.Metadata["address"]),
			}
			for _, candidate := range candidates {
				if candidate != "" {
					host = candidate
					break
				}
			}
			if rawPort := strings.TrimSpace(assetEntry.Metadata["rdp_port"]); rawPort != "" {
				if parsed, err := strconv.Atoi(rawPort); err == nil && parsed > 0 {
					port = parsed
				}
			}
		}
	}

	if d.CredentialStore == nil || d.SecretsManager == nil {
		return host, port, "", ""
	}
	cfg, ok, err := d.CredentialStore.GetDesktopConfig(assetID)
	if err != nil || !ok || strings.TrimSpace(cfg.CredentialProfileID) == "" {
		return host, port, "", ""
	}
	profile, found, err := d.CredentialStore.GetCredentialProfile(cfg.CredentialProfileID)
	if err != nil || !found {
		return host, port, "", ""
	}
	secret, err := d.SecretsManager.DecryptString(profile.SecretCiphertext, profile.ID)
	if err != nil {
		return host, port, profile.Username, ""
	}
	return host, port, profile.Username, secret
}
