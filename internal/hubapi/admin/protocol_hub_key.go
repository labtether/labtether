package admin

import (
	"encoding/json"
	"fmt"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/protocols"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
	"golang.org/x/crypto/ssh"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var validSSHPubKeyRe = regexp.MustCompile(`^ssh-\S+ [A-Za-z0-9+/=]+(?: \S+)?$`)

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
	if !apiv2.RequireAssetAccess(w, r, assetID) {
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
	if strings.TrimSpace(pc.CredentialProfileID) != "" && !apiv2.RequireScope(w, r, "credentials:use") {
		return
	}

	username := strings.TrimSpace(pc.Username)
	if username == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "SSH username is required to push hub key")
		return
	}

	// Ensure hub identity is available.
	hubIdentity := d.currentHubIdentity()
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
		if d.CurrentHubIdentity == nil {
			d.HubIdentity = hubIdentity
		}
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

	client, err := securityruntime.DialOutboundSSHContext(r.Context(), host, pc.Port, sshCfg, 10*time.Second)
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

	verifyClient, err := securityruntime.DialOutboundSSHContext(r.Context(), host, pc.Port, verifyCfg, 10*time.Second)
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
