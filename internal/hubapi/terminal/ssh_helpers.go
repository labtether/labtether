package terminal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/protocols"
	"github.com/labtether/labtether/internal/terminal"
)

// jumpChainHopDialTimeout is the SSH dial timeout used for each hop in a jump chain.
const jumpChainHopDialTimeout = 6 * time.Second

// ResolveSessionSSHConfig resolves the SSH configuration for a terminal session.
// Priority order:
//  1. Inline SSH config (Quick Connect sessions)
//  2. asset_protocol_configs row for SSH
//  3. Environment variable fallback
func (d *Deps) ResolveSessionSSHConfig(session terminal.Session) (*terminal.SSHConfig, error) {
	if session.InlineSSHConfig != nil {
		return session.InlineSSHConfig, nil
	}

	if d.GetProtocolConfig != nil {
		pc, err := d.GetProtocolConfig(context.Background(), session.Target, protocols.ProtocolSSH)
		if err != nil {
			return nil, fmt.Errorf("failed to look up ssh protocol config: %w", err)
		}
		if pc != nil {
			return d.resolveProtocolConfigSSH(pc, session.Target)
		}
	}

	return resolveEnvSSHConfig(session.Target)
}

// resolveProtocolConfigSSH builds a terminal.SSHConfig from an asset_protocol_configs row.
func (d *Deps) resolveProtocolConfigSSH(pc *protocols.ProtocolConfig, assetID string) (*terminal.SSHConfig, error) {
	host := strings.TrimSpace(pc.Host)
	if host == "" && d.AssetStore != nil {
		asset, ok, err := d.AssetStore.GetAsset(assetID)
		if err != nil {
			return nil, fmt.Errorf("failed to load asset for ssh host resolution: %w", err)
		}
		if ok {
			host = strings.TrimSpace(asset.Host)
		}
	}
	if host == "" {
		return nil, errors.New("no host configured for SSH protocol or asset")
	}

	port := pc.Port
	if port <= 0 {
		port = 22
	}

	resolved := &terminal.SSHConfig{
		Host:          host,
		Port:          port,
		User:          strings.TrimSpace(pc.Username),
		StrictHostKey: false,
	}

	if len(pc.Config) > 0 && string(pc.Config) != "null" && string(pc.Config) != "{}" {
		var sshCfg protocols.SSHConfig
		if err := unmarshalProtocolConfigJSON(pc.Config, &sshCfg); err == nil {
			resolved.StrictHostKey = sshCfg.StrictHostKey
			resolved.HostKey = strings.TrimSpace(sshCfg.HostKey)
		}
	}

	if pc.CredentialProfileID == "" {
		if strings.TrimSpace(resolved.User) == "" {
			return nil, errors.New("ssh username is required")
		}
		return nil, errors.New("no credential configured for SSH protocol config")
	}

	if d.CredentialStore == nil {
		return nil, errors.New("credential store not configured")
	}
	profile, ok, err := d.CredentialStore.GetCredentialProfile(pc.CredentialProfileID)
	if err != nil {
		return nil, fmt.Errorf("failed to load credential profile: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("credential profile %s not found", pc.CredentialProfileID)
	}

	if strings.TrimSpace(resolved.User) == "" {
		resolved.User = strings.TrimSpace(profile.Username)
	}

	if d.SecretsManager == nil {
		return nil, errors.New("credential encryption not configured")
	}
	secret, err := d.SecretsManager.DecryptString(profile.SecretCiphertext, profile.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt credential profile secret: %w", err)
	}
	passphrase := ""
	if strings.TrimSpace(profile.PassphraseCiphertext) != "" {
		passphrase, err = d.SecretsManager.DecryptString(profile.PassphraseCiphertext, profile.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt credential profile passphrase: %w", err)
		}
	}

	switch strings.TrimSpace(profile.Kind) {
	case credentials.KindSSHPassword:
		resolved.Password = secret
	case credentials.KindSSHPrivateKey, credentials.KindHubSSHIdentity:
		resolved.PrivateKey = secret
		resolved.PrivateKeyPassphrase = passphrase
	default:
		return nil, fmt.Errorf("unsupported credential profile kind for SSH: %s", profile.Kind)
	}

	if strings.TrimSpace(resolved.User) == "" {
		return nil, errors.New("ssh username is required")
	}
	return resolved, nil
}

// ResolveAssetTerminalConfig resolves a terminal.SSHConfig from a credentials.AssetTerminalConfig.
func (d *Deps) ResolveAssetTerminalConfig(cfg credentials.AssetTerminalConfig) (*terminal.SSHConfig, error) {
	if strings.TrimSpace(cfg.Host) == "" {
		return nil, errors.New("asset terminal config host is required")
	}
	if cfg.Port <= 0 {
		cfg.Port = 22
	}

	resolved := &terminal.SSHConfig{
		Host:          strings.TrimSpace(cfg.Host),
		Port:          cfg.Port,
		User:          strings.TrimSpace(cfg.Username),
		StrictHostKey: cfg.StrictHostKey,
		HostKey:       strings.TrimSpace(cfg.HostKey),
	}

	if cfg.CredentialProfileID != "" {
		if d.CredentialStore == nil {
			return nil, errors.New("credential store not configured")
		}
		profile, ok, err := d.CredentialStore.GetCredentialProfile(cfg.CredentialProfileID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("credential profile %s not found", cfg.CredentialProfileID)
		}

		if strings.TrimSpace(resolved.User) == "" {
			resolved.User = strings.TrimSpace(profile.Username)
		}

		if d.SecretsManager == nil {
			return nil, errors.New("credential encryption not configured")
		}

		secret, err := d.SecretsManager.DecryptString(profile.SecretCiphertext, profile.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt credential profile secret: %w", err)
		}
		passphrase := ""
		if strings.TrimSpace(profile.PassphraseCiphertext) != "" {
			passphrase, err = d.SecretsManager.DecryptString(profile.PassphraseCiphertext, profile.ID)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt credential profile passphrase: %w", err)
			}
		}

		switch strings.TrimSpace(profile.Kind) {
		case credentials.KindSSHPassword:
			resolved.Password = secret
		case credentials.KindSSHPrivateKey, credentials.KindHubSSHIdentity:
			resolved.PrivateKey = secret
			resolved.PrivateKeyPassphrase = passphrase
		default:
			return nil, fmt.Errorf("unsupported credential profile kind: %s", profile.Kind)
		}

		_ = d.CredentialStore.MarkCredentialProfileUsed(profile.ID, time.Now().UTC())
	}

	if strings.TrimSpace(resolved.User) == "" {
		return nil, errors.New("ssh username is required")
	}
	return resolved, nil
}

// CreateSSHClientConfig builds an ssh.ClientConfig from a resolved SSHConfig.
func (d *Deps) CreateSSHClientConfig(resolved *terminal.SSHConfig, timeout time.Duration) (*ssh.ClientConfig, error) {
	if resolved == nil {
		return nil, errors.New("ssh config is required")
	}
	authMethods, err := d.resolveSSHAuthMethods(resolved)
	if err != nil {
		return nil, err
	}
	hostKeyCallback, err := d.buildSSHHostKeyCallback(resolved)
	if err != nil {
		return nil, err
	}
	return &ssh.ClientConfig{
		User:            strings.TrimSpace(resolved.User),
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         timeout,
	}, nil
}

func (d *Deps) resolveSSHAuthMethods(cfg *terminal.SSHConfig) ([]ssh.AuthMethod, error) {
	auth := make([]ssh.AuthMethod, 0, 3)
	if cfg == nil {
		return nil, errors.New("ssh config is required")
	}

	if password := strings.TrimSpace(cfg.Password); password != "" {
		auth = append(auth, ssh.Password(password))
	}

	if keyRaw := shared.NormalizePrivateKey(strings.TrimSpace(cfg.PrivateKey)); keyRaw != "" {
		var (
			signer ssh.Signer
			err    error
		)
		passphrase := strings.TrimSpace(cfg.PrivateKeyPassphrase)
		if passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(keyRaw), []byte(passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey([]byte(keyRaw))
		}
		if err != nil {
			return nil, fmt.Errorf("invalid ssh private key: %w", err)
		}
		auth = append(auth, ssh.PublicKeys(signer))
	}

	if len(auth) == 0 {
		return nil, errors.New("no ssh auth method configured")
	}
	return auth, nil
}

func (d *Deps) buildSSHHostKeyCallback(cfg *terminal.SSHConfig) (ssh.HostKeyCallback, error) {
	if cfg == nil || !cfg.StrictHostKey {
		// #nosec G106 -- explicit non-strict host-key mode for local/dev operator flows.
		return ssh.InsecureIgnoreHostKey(), nil
	}

	expected := strings.TrimSpace(cfg.HostKey)
	if expected == "" {
		knownHostsCallback, err := shared.BuildKnownHostsHostKeyCallback()
		if err != nil {
			return nil, errors.New("strict host key enabled but no SSH_HOST_KEY provided and no known_hosts file is available")
		}
		return knownHostsCallback, nil
	}

	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		fingerprint := strings.TrimSpace(ssh.FingerprintSHA256(key))
		if strings.EqualFold(fingerprint, expected) {
			return nil
		}

		encoded := strings.TrimSpace(base64.StdEncoding.EncodeToString(key.Marshal()))
		if strings.EqualFold(encoded, expected) {
			return nil
		}

		return errors.New("host key mismatch")
	}, nil
}

// ResolveJumpChainHops resolves each hop in a JumpChain into a ResolvedHop
// by looking up credential profiles and decrypting secrets.
func (d *Deps) ResolveJumpChainHops(chain terminal.JumpChain) ([]terminal.ResolvedHop, error) {
	if len(chain.Hops) == 0 {
		return nil, nil
	}

	resolved := make([]terminal.ResolvedHop, 0, len(chain.Hops))
	for i, hop := range chain.Hops {
		host := strings.TrimSpace(hop.Host)
		if host == "" {
			return nil, fmt.Errorf("jump chain hop %d: host is required", i)
		}
		if err := ValidateQuickConnectHost(host); err != nil {
			return nil, fmt.Errorf("jump chain hop %d: %w", i, err)
		}
		port := hop.Port
		if port <= 0 {
			port = 22
		}
		username := strings.TrimSpace(hop.Username)
		if username == "" {
			return nil, fmt.Errorf("jump chain hop %d: username is required", i)
		}

		sshCfg := &terminal.SSHConfig{
			Host:          host,
			Port:          port,
			User:          username,
			StrictHostKey: false,
		}

		profileID := strings.TrimSpace(hop.CredentialProfileID)
		if profileID != "" {
			if d.CredentialStore == nil {
				return nil, fmt.Errorf("jump chain hop %d: credential store not configured", i)
			}
			profile, ok, err := d.CredentialStore.GetCredentialProfile(profileID)
			if err != nil {
				return nil, fmt.Errorf("jump chain hop %d: credential lookup failed: %w", i, err)
			}
			if !ok {
				return nil, fmt.Errorf("jump chain hop %d: credential profile %s not found", i, profileID)
			}
			if d.SecretsManager == nil {
				return nil, fmt.Errorf("jump chain hop %d: credential encryption not configured", i)
			}

			secret, err := d.SecretsManager.DecryptString(profile.SecretCiphertext, profile.ID)
			if err != nil {
				return nil, fmt.Errorf("jump chain hop %d: failed to decrypt credential: %w", i, err)
			}
			passphrase := ""
			if strings.TrimSpace(profile.PassphraseCiphertext) != "" {
				passphrase, err = d.SecretsManager.DecryptString(profile.PassphraseCiphertext, profile.ID)
				if err != nil {
					return nil, fmt.Errorf("jump chain hop %d: failed to decrypt passphrase: %w", i, err)
				}
			}

			switch strings.TrimSpace(profile.Kind) {
			case credentials.KindSSHPassword:
				sshCfg.Password = secret
			case credentials.KindSSHPrivateKey, credentials.KindHubSSHIdentity:
				sshCfg.PrivateKey = secret
				sshCfg.PrivateKeyPassphrase = passphrase
			default:
				return nil, fmt.Errorf("jump chain hop %d: unsupported credential kind: %s", i, profile.Kind)
			}

			_ = d.CredentialStore.MarkCredentialProfileUsed(profile.ID, time.Now().UTC())
		}

		clientConfig, err := d.CreateSSHClientConfig(sshCfg, jumpChainHopDialTimeout)
		if err != nil {
			return nil, fmt.Errorf("jump chain hop %d: invalid ssh config: %w", i, err)
		}

		resolved = append(resolved, terminal.ResolvedHop{
			Addr:         net.JoinHostPort(host, strconv.Itoa(port)),
			ClientConfig: clientConfig,
		})
	}
	return resolved, nil
}

// ResolveJumpChain looks up a group by ID and resolves its jump chain hops.
// Returns nil if the group has no jump chain configured.
func (d *Deps) ResolveJumpChain(groupID string) ([]terminal.ResolvedHop, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil, nil
	}
	if d.GroupStore == nil {
		return nil, nil
	}

	group, ok, err := d.GroupStore.GetGroup(groupID)
	if err != nil {
		return nil, fmt.Errorf("jump chain: group lookup failed: %w", err)
	}
	if !ok || len(group.JumpChain) == 0 || string(group.JumpChain) == "null" {
		return nil, nil
	}

	var chain terminal.JumpChain
	if err := json.Unmarshal(group.JumpChain, &chain); err != nil {
		return nil, fmt.Errorf("jump chain: invalid config: %w", err)
	}
	if len(chain.Hops) == 0 {
		return nil, nil
	}

	return d.ResolveJumpChainHops(chain)
}

// GetAssetGroupID returns the group ID for the given asset.
// Returns empty string if the asset is not found or has no group.
func (d *Deps) GetAssetGroupID(assetID string) (string, error) {
	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return "", nil
	}
	if d.AssetStore == nil {
		return "", nil
	}

	asset, ok, err := d.AssetStore.GetAsset(assetID)
	if err != nil {
		return "", fmt.Errorf("asset group lookup failed: %w", err)
	}
	if !ok {
		return "", nil
	}
	return strings.TrimSpace(asset.GroupID), nil
}

// resolveEnvSSHConfig builds an SSHConfig from environment variables.
// Returns nil (no error) when no SSH_USERNAME is set, indicating SSH is not
// configured via the environment.
func resolveEnvSSHConfig(target string) (*terminal.SSHConfig, error) {
	user := strings.TrimSpace(os.Getenv("SSH_USERNAME"))
	if user == "" {
		return nil, nil
	}

	resolved := &terminal.SSHConfig{
		Host:          target,
		Port:          shared.EnvOrDefaultInt("SSH_PORT", 22),
		User:          user,
		StrictHostKey: shared.EnvOrDefaultBool("SSH_STRICT_HOST_KEY", true),
		HostKey:       strings.TrimSpace(os.Getenv("SSH_HOST_KEY")),
	}

	if password := strings.TrimSpace(os.Getenv("SSH_PASSWORD")); password != "" {
		resolved.Password = password
	}

	if keyB64 := strings.TrimSpace(os.Getenv("SSH_PRIVATE_KEY_B64")); keyB64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(keyB64)
		if err == nil {
			resolved.PrivateKey = string(decoded)
		}
	} else if keyRaw := strings.TrimSpace(os.Getenv("SSH_PRIVATE_KEY")); keyRaw != "" {
		resolved.PrivateKey = shared.NormalizePrivateKey(keyRaw)
	} else if keyPath := strings.TrimSpace(os.Getenv("SSH_PRIVATE_KEY_PATH")); keyPath != "" {
		// #nosec G304,G703 -- operator-managed local path from trusted runtime env.
		data, err := os.ReadFile(keyPath)
		if err == nil {
			resolved.PrivateKey = string(data)
		}
	}

	resolved.PrivateKeyPassphrase = strings.TrimSpace(os.Getenv("SSH_PRIVATE_KEY_PASSPHRASE"))

	if resolved.Password == "" && resolved.PrivateKey == "" {
		return nil, nil
	}
	return resolved, nil
}

// unmarshalProtocolConfigJSON decodes raw JSON into target.
// Returns nil for empty/null payloads.
func unmarshalProtocolConfigJSON(raw []byte, target any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, target)
}
