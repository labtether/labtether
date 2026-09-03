package desktop

import (
	"context"
	"errors"
	"fmt"
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
	"github.com/labtether/labtether/internal/protocols"
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

	target, targetErr := d.ResolveRDPTarget(r.Context(), session)
	if targetErr != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid RDP configuration: "+d.SanitizeUpstreamError(targetErr.Error()))
		return
	}
	// Guacd performs the target connection out-of-process. Resolve and validate
	// every managed or direct target here, then pass guacd only the approved IP
	// literal. This prevents a second DNS lookup/rebinding hop and applies the
	// loopback/link-local/private-network policy uniformly to asset metadata.
	resolvedHost, resolveErr := securityruntime.ResolveOutboundTCPHost(r.Context(), target.Host, target.Port)
	if resolveErr != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid RDP target: "+d.SanitizeUpstreamError(resolveErr.Error()))
		return
	}
	target.Host = resolvedHost
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

	params := rdpGuacdParams(target)
	params["hostname"] = target.Host
	params["port"] = strconv.Itoa(target.Port)
	params["username"] = target.Username
	params["password"] = target.Password
	for key, value := range map[string]string{
		"disable-auth":      "",
		"width":             "1920",
		"height":            "1080",
		"dpi":               "96",
		"enable-audio":      "true",
		"enable-drive":      "false",
		"enable-printing":   "false",
		"drive-path":        "",
		"create-drive-path": "",
	} {
		params[key] = value
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

// RDPConnectionTarget is the fully resolved connection configuration sent to guacd.
type RDPConnectionTarget struct {
	Host                    string
	Port                    int
	Username                string
	Password                string
	Domain                  string
	NLAEnabled              bool
	IgnoreCertificate       bool
	AllowLegacySecurity     bool
	CertificateFingerprints string
}

func rdpGuacdParams(target RDPConnectionTarget) map[string]string {
	security := "tls"
	if target.NLAEnabled {
		security = "nla"
	}
	if target.AllowLegacySecurity {
		security = "rdp"
	}
	return map[string]string{
		"domain":            target.Domain,
		"security":          security,
		"ignore-cert":       strconv.FormatBool(target.IgnoreCertificate),
		"cert-fingerprints": target.CertificateFingerprints,
	}
}

// ResolveRDPTarget resolves the RDP connection target for an asset.
func (d *Deps) ResolveRDPTarget(ctx context.Context, session terminal.Session) (RDPConnectionTarget, error) {
	opts := d.GetDesktopSessionOptions(session.ID)
	if opts.Direct {
		if (opts.RDPIgnoreCertificate || opts.RDPAllowLegacySecurity) && !securityruntime.InsecureTransportAllowed() {
			return RDPConnectionTarget{}, errors.New("unsafe RDP options cannot be used unless insecure transport is enabled")
		}
		cfg := protocols.RDPConfig{
			IgnoreCertificate:       opts.RDPIgnoreCertificate,
			AllowLegacySecurity:     opts.RDPAllowLegacySecurity,
			CertificateFingerprints: opts.RDPCertificateFingerprints,
		}
		if err := protocols.ValidateRDPConfigOptions(cfg); err != nil {
			return RDPConnectionTarget{}, err
		}
		return RDPConnectionTarget{
			Host:                    opts.DirectHost,
			Port:                    opts.DirectPort,
			Username:                opts.DirectUsername,
			Password:                opts.DirectPassword,
			NLAEnabled:              !opts.RDPAllowLegacySecurity,
			IgnoreCertificate:       opts.RDPIgnoreCertificate,
			AllowLegacySecurity:     opts.RDPAllowLegacySecurity,
			CertificateFingerprints: strings.TrimSpace(opts.RDPCertificateFingerprints),
		}, nil
	}
	assetID := session.Target
	target := RDPConnectionTarget{Host: strings.TrimSpace(assetID), Port: protocols.DefaultPort(protocols.ProtocolRDP)}

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
					target.Host = candidate
					break
				}
			}
			if rawPort := strings.TrimSpace(assetEntry.Metadata["rdp_port"]); rawPort != "" {
				if parsed, err := strconv.Atoi(rawPort); err == nil && parsed > 0 {
					target.Port = parsed
				}
			}
		}
	}

	credentialProfileID := ""
	if d.GetProtocolConfig != nil {
		pc, err := d.GetProtocolConfig(ctx, assetID, protocols.ProtocolRDP)
		if err != nil {
			return RDPConnectionTarget{}, fmt.Errorf("failed to load RDP protocol config: %w", err)
		}
		if pc != nil && pc.Enabled {
			if host := strings.TrimSpace(pc.Host); host != "" {
				target.Host = host
			}
			if pc.Port > 0 {
				target.Port = pc.Port
			}
			target.Username = strings.TrimSpace(pc.Username)
			credentialProfileID = strings.TrimSpace(pc.CredentialProfileID)
			cfg, err := protocols.DecodeRDPConfig(pc.Config)
			if err != nil {
				return RDPConnectionTarget{}, fmt.Errorf("invalid saved RDP protocol config: %w", err)
			}
			target.Domain = strings.TrimSpace(cfg.Domain)
			target.NLAEnabled = cfg.NLAEnabled
			target.IgnoreCertificate = cfg.IgnoreCertificate
			target.AllowLegacySecurity = cfg.AllowLegacySecurity
			target.CertificateFingerprints = strings.TrimSpace(cfg.CertificateFingerprints)
		}
	}
	if (target.IgnoreCertificate || target.AllowLegacySecurity) && !securityruntime.InsecureTransportAllowed() {
		return RDPConnectionTarget{}, errors.New("unsafe RDP options cannot be used unless insecure transport is enabled")
	}

	if d.CredentialStore == nil || d.SecretsManager == nil {
		return target, nil
	}
	if credentialProfileID == "" {
		cfg, ok, err := d.CredentialStore.GetDesktopConfig(assetID)
		if err != nil || !ok {
			return target, nil
		}
		credentialProfileID = strings.TrimSpace(cfg.CredentialProfileID)
	}
	if credentialProfileID == "" {
		return target, nil
	}
	profile, found, err := d.CredentialStore.GetCredentialProfile(credentialProfileID)
	if err != nil || !found {
		return target, nil
	}
	if target.Username == "" {
		target.Username = strings.TrimSpace(profile.Username)
	}
	secret, err := d.SecretsManager.DecryptString(profile.SecretCiphertext, profile.ID)
	if err != nil {
		return target, nil
	}
	target.Password = secret
	return target, nil
}
