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

	targetHost, targetPort, username, password := d.ResolveRDPTarget(session.Target)
	client, err := guacamole.Connect(guacdHost, guacdPort)
	if err != nil {
		log.Printf("rdp: guacd connect failed: %v", err)
		servicehttp.WriteError(w, http.StatusBadGateway, "guacd unavailable")
		return
	}

	if err := client.SelectProtocol("rdp"); err != nil {
		log.Printf("rdp: select protocol failed: %v", err)
		servicehttp.WriteError(w, http.StatusBadGateway, "guacd protocol negotiation failed")
		return
	}

	// guacd responds with "args" after protocol selection.
	if _, _, err := client.ReadInstruction(); err != nil {
		log.Printf("rdp: read args failed: %v", err)
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
	if err := client.SendConnect(params); err != nil {
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

	browserWS, err := d.TerminalWebSocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		_ = client.Close()
		return
	}
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
			_ = client.Close()
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

	_ = client.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}

// ResolveRDPTarget resolves the RDP connection target for an asset.
func (d *Deps) ResolveRDPTarget(assetID string) (host string, port int, username string, password string) {
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
