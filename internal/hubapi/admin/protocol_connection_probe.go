package admin

import (
	"encoding/json"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/protocols"
	"github.com/labtether/labtether/internal/servicehttp"
	"net/http"
	"os"
	"strings"
)

// HandleTestProtocolConnection handles POST /assets/{id}/protocols/{protocol}/test.
func (d *Deps) HandleTestProtocolConnection(w http.ResponseWriter, r *http.Request, assetID, protocol string) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !apiv2.RequireAssetAccess(w, r, assetID) {
		return
	}

	protocol = strings.TrimSpace(protocol)
	if err := protocols.ValidateProtocol(protocol); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	lockKey := assetID + ":" + protocol
	if _, loaded := protocolTestLocks.LoadOrStore(lockKey, struct{}{}); loaded {
		servicehttp.WriteError(w, http.StatusConflict, "test already in progress for this asset and protocol")
		return
	}
	defer protocolTestLocks.Delete(lockKey)

	pc, err := d.DB.GetProtocolConfig(r.Context(), assetID, protocol)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load protocol config")
		return
	}
	if pc == nil {
		servicehttp.WriteError(w, http.StatusNotFound, "protocol config not found")
		return
	}
	if !validateTelnetTransportSecurity(w, protocol, pc.Config, true) {
		return
	}
	if strings.TrimSpace(pc.CredentialProfileID) != "" && !apiv2.RequireScope(w, r, "credentials:use") {
		return
	}

	// Resolve host: protocol config overrides asset host.
	host := strings.TrimSpace(pc.Host)
	if host == "" && d.AssetStore != nil {
		asset, ok, assetErr := d.AssetStore.GetAsset(assetID)
		if assetErr != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load asset")
			return
		}
		if ok {
			host = strings.TrimSpace(asset.Host)
		}
	}
	if host == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "no host configured for this protocol or asset")
		return
	}

	// Decrypt credential if present.
	var password, privateKey string
	if protocol == protocols.ProtocolSSH && pc.CredentialProfileID != "" && d.CredentialStore != nil && d.SecretsManager != nil {
		profile, ok, credErr := d.CredentialStore.GetCredentialProfile(pc.CredentialProfileID)
		if credErr != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load credential profile")
			return
		}
		if ok {
			secret, decErr := d.SecretsManager.DecryptString(profile.SecretCiphertext, profile.ID)
			if decErr != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to decrypt credential")
				return
			}
			switch profile.Kind {
			case credentials.KindSSHPassword:
				password = secret
			case credentials.KindSSHPrivateKey, credentials.KindHubSSHIdentity:
				privateKey = secret
			}
		}
	}

	// Determine guacd address from environment.
	guacdHost := strings.TrimSpace(os.Getenv("GUACD_HOST"))
	guacdPort := strings.TrimSpace(os.Getenv("GUACD_PORT"))
	var guacdAddr string
	if guacdHost != "" {
		if guacdPort == "" {
			guacdPort = "4822"
		}
		guacdAddr = guacdHost + ":" + guacdPort
	}

	// Run the appropriate test.
	var result *protocols.TestResult
	switch protocol {
	case protocols.ProtocolSSH:
		username := strings.TrimSpace(pc.Username)
		var sshCfg protocols.SSHConfig
		if len(pc.Config) > 0 {
			_ = json.Unmarshal(pc.Config, &sshCfg)
		}
		hostKeyCallback, err := shared.BuildSSHHostKeyCallback(sshCfg.StrictHostKey, sshCfg.HostKey)
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "SSH host key verification requires a configured host key or known_hosts entry")
			return
		}
		result = protocols.TestSSH(r.Context(), host, pc.Port, username, password, privateKey, hostKeyCallback)
	case protocols.ProtocolTelnet:
		telnetOptions, decodeErr := protocols.DecodeTelnetConfig(pc.Config)
		if decodeErr != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, decodeErr.Error())
			return
		}
		result = protocols.TestTelnet(r.Context(), host, pc.Port, telnetOptions.AllowInsecureTransport)
	case protocols.ProtocolVNC:
		result = protocols.TestVNC(r.Context(), host, pc.Port)
	case protocols.ProtocolRDP:
		result = protocols.TestRDP(r.Context(), host, pc.Port, guacdAddr)
	case protocols.ProtocolARD:
		result = protocols.TestARD(r.Context(), host, pc.Port)
	default:
		servicehttp.WriteError(w, http.StatusBadRequest, "unsupported protocol")
		return
	}

	// Persist test outcome.
	// Values must match the DB CHECK constraint: 'untested', 'success', 'failed'.
	status := "success"
	testErr := ""
	if !result.Success {
		status = "failed"
		testErr = result.Error
	}
	if dbErr := d.DB.UpdateProtocolTestResult(r.Context(), assetID, protocol, status, testErr); dbErr != nil {
		// Non-fatal: log but continue.
		_ = dbErr
	}

	ev := audit.NewEvent("protocol.test.run")
	ev.ActorID = d.principalActorID(r.Context())
	ev.Target = assetID
	ev.Details = map[string]any{
		"protocol": protocol,
		"success":  result.Success,
	}
	d.appendAuditEventBestEffort(ev, "api warning: failed to append protocol test audit event")

	servicehttp.WriteJSON(w, http.StatusOK, result)
}
