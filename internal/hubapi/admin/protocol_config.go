package admin

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/protocols"
	"github.com/labtether/labtether/internal/servicehttp"
)

// protocolTestLocks guards against concurrent test runs for the same
// asset+protocol combination. Keys are "assetID:protocol".
var protocolTestLocks sync.Map

var validSSHPubKeyRe = regexp.MustCompile(`^ssh-\S+ [A-Za-z0-9+/=]+(?: \S+)?$`)

// HandleListProtocolConfigs handles GET /assets/{id}/protocols.
func (d *Deps) HandleListProtocolConfigs(w http.ResponseWriter, r *http.Request, assetID string) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	configs, err := d.DB.ListProtocolConfigs(r.Context(), assetID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list protocol configs")
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"protocols": configs})
}

type protocolConfigRequest struct {
	Protocol            string          `json:"protocol"`
	Host                string          `json:"host"`
	Port                int             `json:"port"`
	Username            string          `json:"username"`
	CredentialProfileID string          `json:"credential_profile_id"`
	Enabled             *bool           `json:"enabled,omitempty"`
	Config              json.RawMessage `json:"config"`
}

// HandleCreateProtocolConfig handles POST /assets/{id}/protocols.
func (d *Deps) HandleCreateProtocolConfig(w http.ResponseWriter, r *http.Request, assetID string) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req protocolConfigRequest
	if err := d.decodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid request payload")
		return
	}

	req.Protocol = strings.TrimSpace(req.Protocol)
	req.Host = strings.TrimSpace(req.Host)
	req.Username = strings.TrimSpace(req.Username)
	req.CredentialProfileID = strings.TrimSpace(req.CredentialProfileID)

	if err := protocols.ValidateProtocol(req.Protocol); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Host != "" {
		if err := protocols.ValidateManualDeviceHost(req.Host); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	port := req.Port
	if port == 0 {
		port = protocols.DefaultPort(req.Protocol)
	}
	if err := protocols.ValidatePort(port); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := protocols.ValidateProtocolConfig(req.Protocol, req.Config); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.CredentialProfileID != "" && d.CredentialStore != nil {
		_, ok, err := d.CredentialStore.GetCredentialProfile(req.CredentialProfileID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to validate credential profile")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusBadRequest, "credential_profile_id does not reference an existing profile")
			return
		}
	}

	existing, err := d.DB.GetProtocolConfig(r.Context(), assetID, req.Protocol)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to check for existing protocol config")
		return
	}
	if existing != nil {
		servicehttp.WriteError(w, http.StatusConflict, "protocol config already exists for this asset")
		return
	}

	pc := protocols.ProtocolConfig{
		ID:                  idgen.New("proto"),
		AssetID:             assetID,
		Protocol:            req.Protocol,
		Host:                req.Host,
		Port:                port,
		Username:            req.Username,
		CredentialProfileID: req.CredentialProfileID,
		Enabled:             true,
		Config:              req.Config,
	}

	if err := d.DB.SaveProtocolConfig(r.Context(), &pc); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to save protocol config")
		return
	}

	ev := audit.NewEvent("protocol.config.created")
	ev.ActorID = d.principalActorID(r.Context())
	ev.Target = assetID
	ev.Details = map[string]any{
		"protocol": pc.Protocol,
	}
	d.appendAuditEventBestEffort(ev, "api warning: failed to append protocol config create audit event")

	servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"protocol": pc})
}

// HandleUpdateProtocolConfig handles PUT /assets/{id}/protocols/{protocol}.
func (d *Deps) HandleUpdateProtocolConfig(w http.ResponseWriter, r *http.Request, assetID, protocol string) {
	if r.Method != http.MethodPut {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	protocol = strings.TrimSpace(protocol)
	if err := protocols.ValidateProtocol(protocol); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req protocolConfigRequest
	if err := d.decodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid request payload")
		return
	}

	req.Host = strings.TrimSpace(req.Host)
	req.Username = strings.TrimSpace(req.Username)
	req.CredentialProfileID = strings.TrimSpace(req.CredentialProfileID)

	if req.Host != "" {
		if err := protocols.ValidateManualDeviceHost(req.Host); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	port := req.Port
	if port == 0 {
		port = protocols.DefaultPort(protocol)
	}
	if err := protocols.ValidatePort(port); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := protocols.ValidateProtocolConfig(protocol, req.Config); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.CredentialProfileID != "" && d.CredentialStore != nil {
		_, ok, err := d.CredentialStore.GetCredentialProfile(req.CredentialProfileID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to validate credential profile")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusBadRequest, "credential_profile_id does not reference an existing profile")
			return
		}
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	pc := protocols.ProtocolConfig{
		AssetID:             assetID,
		Protocol:            protocol,
		Host:                req.Host,
		Port:                port,
		Username:            req.Username,
		CredentialProfileID: req.CredentialProfileID,
		Enabled:             enabled,
		Config:              req.Config,
	}

	if err := d.DB.SaveProtocolConfig(r.Context(), &pc); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to save protocol config")
		return
	}

	ev := audit.NewEvent("protocol.config.updated")
	ev.ActorID = d.principalActorID(r.Context())
	ev.Target = assetID
	ev.Details = map[string]any{
		"protocol": protocol,
	}
	d.appendAuditEventBestEffort(ev, "api warning: failed to append protocol config update audit event")

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"protocol": pc})
}

// HandleDeleteProtocolConfig handles DELETE /assets/{id}/protocols/{protocol}.
func (d *Deps) HandleDeleteProtocolConfig(w http.ResponseWriter, r *http.Request, assetID, protocol string) {
	if r.Method != http.MethodDelete {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	protocol = strings.TrimSpace(protocol)
	if err := protocols.ValidateProtocol(protocol); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := d.DB.DeleteProtocolConfig(r.Context(), assetID, protocol); err != nil {
		if strings.Contains(err.Error(), "protocol config not found") {
			servicehttp.WriteError(w, http.StatusNotFound, "protocol config not found")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete protocol config")
		return
	}

	ev := audit.NewEvent("protocol.config.deleted")
	ev.ActorID = d.principalActorID(r.Context())
	ev.Target = assetID
	ev.Details = map[string]any{
		"protocol": protocol,
	}
	d.appendAuditEventBestEffort(ev, "api warning: failed to append protocol config delete audit event")

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// HandleTestProtocolConnection handles POST /assets/{id}/protocols/{protocol}/test.
func (d *Deps) HandleTestProtocolConnection(w http.ResponseWriter, r *http.Request, assetID, protocol string) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
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
	if pc.CredentialProfileID != "" && d.CredentialStore != nil && d.SecretsManager != nil {
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
			case credentials.KindSSHPassword, credentials.KindTelnetPassword,
				credentials.KindRDPPassword, credentials.KindVNCPassword:
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
		var hostKeyCallback ssh.HostKeyCallback
		var sshCfg protocols.SSHConfig
		if len(pc.Config) > 0 {
			_ = json.Unmarshal(pc.Config, &sshCfg)
		}
		if sshCfg.StrictHostKey && sshCfg.HostKey != "" {
			hostPub, _, _, _, parseErr := ssh.ParseAuthorizedKey([]byte(sshCfg.HostKey))
			if parseErr == nil {
				hostKeyCallback = ssh.FixedHostKey(hostPub)
			}
		}
		result = protocols.TestSSH(r.Context(), host, pc.Port, username, password, privateKey, hostKeyCallback)
	case protocols.ProtocolTelnet:
		result = protocols.TestTelnet(r.Context(), host, pc.Port)
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

// HandlePushHubKey handles POST /assets/{id}/protocols/ssh/push-hub-key.
// It connects to the remote host using the password credential on the SSH
// protocol config, installs the hub's ED25519 public key into
// ~/.ssh/authorized_keys, verifies key-based auth, and updates the protocol
// config credential to the hub identity profile.
func (d *Deps) HandlePushHubKey(w http.ResponseWriter, r *http.Request, assetID string) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	pc, err := d.DB.GetProtocolConfig(r.Context(), assetID, protocols.ProtocolSSH)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load SSH protocol config")
		return
	}
	if pc == nil {
		servicehttp.WriteError(w, http.StatusNotFound, "SSH protocol config not found")
		return
	}

	username := strings.TrimSpace(pc.Username)
	if username == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "SSH username is required to push hub key")
		return
	}

	// Ensure hub identity is available.
	hubIdentity := d.HubIdentity
	if hubIdentity == nil {
		if d.EnsureHubIdentity == nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "hub SSH identity management is not configured")
			return
		}
		var identErr error
		hubIdentity, identErr = d.EnsureHubIdentity(d)
		if identErr != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to initialise hub SSH identity")
			return
		}
		d.HubIdentity = hubIdentity
	}

	// Resolve host.
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
		servicehttp.WriteError(w, http.StatusBadRequest, "no host configured for SSH protocol or asset")
		return
	}

	// Decrypt password credential for initial auth.
	var password string
	if pc.CredentialProfileID != "" && d.CredentialStore != nil && d.SecretsManager != nil {
		profile, ok, credErr := d.CredentialStore.GetCredentialProfile(pc.CredentialProfileID)
		if credErr != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load credential profile")
			return
		}
		if ok && (profile.Kind == credentials.KindSSHPassword) {
			secret, decErr := d.SecretsManager.DecryptString(profile.SecretCiphertext, profile.ID)
			if decErr != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to decrypt credential")
				return
			}
			password = secret
		}
	}

	if password == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "password credential is required for hub key push")
		return
	}

	addr := fmt.Sprintf("%s:%d", host, pc.Port)
	hostKeyCallback, hostKeyErr := buildHubKeyPushHostKeyCallback(pc.Config)
	if hostKeyErr != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, hostKeyErr.Error())
		return
	}

	sshCfg := &ssh.ClientConfig{
		User:            username,
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: hostKeyCallback,
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", addr, sshCfg)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, fmt.Sprintf("SSH connection failed: %v", err))
		return
	}
	defer client.Close()

	// Detect remote platform.
	platform, err := runSSHCommand(client, "uname -s")
	if err != nil {
		// Default to Linux/POSIX path if uname is not available.
		platform = "Linux"
	}
	platform = strings.TrimSpace(platform)

	// Install hub public key.
	pubKey := strings.TrimSpace(hubIdentity.PublicKey)
	if !validSSHPubKeyRe.MatchString(pubKey) {
		servicehttp.WriteError(w, http.StatusInternalServerError, "hub public key has unexpected format")
		return
	}
	var installScript string
	switch strings.ToLower(platform) {
	case "darwin", "linux", "freebsd", "openbsd", "netbsd":
		installScript = fmt.Sprintf(`
set -e
mkdir -p ~/.ssh
chmod 700 ~/.ssh
touch ~/.ssh/authorized_keys
chmod 600 ~/.ssh/authorized_keys
grep -qxF %q ~/.ssh/authorized_keys || echo %q >> ~/.ssh/authorized_keys
`, pubKey, pubKey)
	default:
		// Windows — use PowerShell.
		installScript = fmt.Sprintf(`
$sshDir = "$env:USERPROFILE\.ssh"
if (-not (Test-Path $sshDir)) { New-Item -ItemType Directory -Path $sshDir | Out-Null }
$authKeys = Join-Path $sshDir "authorized_keys"
if (-not (Test-Path $authKeys)) { New-Item -ItemType File -Path $authKeys | Out-Null }
$key = '%s'
$existing = Get-Content $authKeys -ErrorAction SilentlyContinue
if ($existing -notcontains $key) { Add-Content -Path $authKeys -Value $key }
`, pubKey)
	}

	if _, err := runSSHCommand(client, installScript); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("failed to install hub key: %v", err))
		return
	}
	if err := client.Close(); err != nil {
		log.Printf("protocol config: close password-auth SSH client for %s: %v", assetID, err)
	}

	// Verify: reconnect using the hub private key.
	if d.LoadHubPrivateKeyPEM == nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "hub private key loader is not configured")
		return
	}
	hubPrivPEM, privErr := d.LoadHubPrivateKeyPEM(hubIdentity)
	if privErr != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("failed to load hub private key for verification: %v", privErr))
		return
	}

	signer, err := ssh.ParsePrivateKey([]byte(hubPrivPEM))
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("failed to parse hub private key: %v", err))
		return
	}

	verifyCfg := &ssh.ClientConfig{
		User:            username,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: hostKeyCallback,
		Timeout:         10 * time.Second,
	}

	verifyClient, err := ssh.Dial("tcp", addr, verifyCfg)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, fmt.Sprintf("hub key verification failed — key may not have been installed correctly: %v", err))
		return
	}
	if err := verifyClient.Close(); err != nil {
		log.Printf("protocol config: close verification SSH client for %s: %v", assetID, err)
	}

	// Update credential to hub identity profile.
	if dbErr := d.DB.UpdateProtocolConfigCredential(r.Context(), assetID, protocols.ProtocolSSH, hubIdentity.ProfileID); dbErr != nil {
		// Non-fatal: log and continue — the key is installed.
		_ = dbErr
	}

	// Mark hub_key_installed in the SSH protocol config JSONB.
	if sshPC, readErr := d.DB.GetProtocolConfig(r.Context(), assetID, protocols.ProtocolSSH); readErr == nil && sshPC != nil {
		var sshCfg protocols.SSHConfig
		if len(sshPC.Config) > 0 {
			_ = json.Unmarshal(sshPC.Config, &sshCfg)
		}
		sshCfg.HubKeyInstalled = true
		if updated, marshalErr := json.Marshal(sshCfg); marshalErr == nil {
			sshPC.Config = updated
			_ = d.DB.SaveProtocolConfig(r.Context(), sshPC)
		}
	}

	ev := audit.NewEvent("protocol.ssh.hub_key_pushed")
	ev.ActorID = d.principalActorID(r.Context())
	ev.Target = assetID
	ev.Details = map[string]any{
		"platform": platform,
		"key_type": "ed25519",
	}
	d.appendAuditEventBestEffort(ev, "api warning: failed to append hub key push audit event")

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"key_type": "ed25519",
	})
}

// runSSHCommand opens a session on the given client, runs cmd, and returns
// combined stdout output.
func runSSHCommand(client *ssh.Client, cmd string) (string, error) {
	sess, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to open SSH session: %w", err)
	}
	defer sess.Close()

	out, err := sess.CombinedOutput(cmd)
	return string(out), err
}

func buildHubKeyPushHostKeyCallback(rawConfig []byte) (ssh.HostKeyCallback, error) {
	var sshCfg protocols.SSHConfig
	if len(rawConfig) > 0 {
		_ = json.Unmarshal(rawConfig, &sshCfg)
	}
	if hostKey := strings.TrimSpace(sshCfg.HostKey); hostKey != "" {
		hostPub, _, _, _, parseErr := ssh.ParseAuthorizedKey([]byte(hostKey))
		if parseErr != nil {
			return nil, fmt.Errorf("configured SSH host key is invalid")
		}
		return ssh.FixedHostKey(hostPub), nil
	}
	knownHostsCallback, err := shared.BuildKnownHostsHostKeyCallback()
	if err != nil {
		return nil, fmt.Errorf("SSH host key verification is required for hub key push; configure the asset host key or install a known_hosts entry")
	}
	return knownHostsCallback, nil
}
