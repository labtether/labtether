package resources

import (
	"context"
	"errors"
	"fmt"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/fileproto"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/persistence"
	"log"
	"strings"
	"time"
)

// --- helpers ---

func (d *Deps) buildConnectionConfig(fc *persistence.FileConnection) (fileproto.ConnectionConfig, error) {
	normalizedExtra, err := NormalizeFileConnectionExtraConfig(fc.Protocol, fc.ExtraConfig)
	if err != nil {
		return fileproto.ConnectionConfig{}, err
	}
	port := 0
	if fc.Port != nil {
		port = *fc.Port
	}
	if port == 0 {
		port = fileproto.DefaultPort(fc.Protocol)
	}

	initialPath := fc.InitialPath
	if initialPath == "" {
		initialPath = "/"
	}

	connectionID := strings.TrimSpace(fc.ID)
	config := fileproto.ConnectionConfig{
		ConnectionID: connectionID,
		Protocol:     fc.Protocol,
		Host:         fc.Host,
		Port:         port,
		InitialPath:  initialPath,
		ExtraConfig:  normalizedExtra,
	}
	expectedUpdatedAt := fc.UpdatedAt
	config.ValidateCurrent = func(ctx context.Context) error {
		current, err := d.FileConnectionStore.GetFileConnection(ctx, connectionID)
		switch {
		case err == nil && current.UpdatedAt.Equal(expectedUpdatedAt):
			return nil
		case err == nil, errors.Is(err, persistence.ErrNotFound):
			return errors.New("saved file connection changed; retry the operation")
		default:
			log.Printf("file-connections: failed to validate current connection %s: %v", connectionID, err) // #nosec G706 -- Connection IDs are hub-generated identifiers.
			return errors.New("failed to validate saved file connection")
		}
	}
	if fc.Protocol == "sftp" {
		if _, pinned := normalizedExtra["host_key"]; !pinned {
			expectedHost := strings.TrimSpace(fc.Host)
			expectedPort := port
			config.PersistHostKey = func(ctx context.Context, presentedKey string) error {
				err := d.FileConnectionStore.PinSFTPHostKey(ctx, connectionID, expectedHost, expectedPort, presentedKey)
				switch {
				case err == nil:
					return nil
				case errors.Is(err, persistence.ErrSFTPHostKeyMismatch):
					return errors.New("SFTP host key changed; connection blocked")
				case errors.Is(err, persistence.ErrFileConnectionChanged), errors.Is(err, persistence.ErrNotFound):
					return errors.New("SFTP connection changed during host key verification; connection blocked")
				default:
					log.Printf("file-connections: failed to pin SFTP host key for %s: %v", connectionID, err) // #nosec G706 -- Connection IDs are hub-generated identifiers.
					return errors.New("failed to save trusted SFTP host key")
				}
			}
		}
	}

	if fc.CredentialID != nil && *fc.CredentialID != "" {
		profile, ok, err := d.CredentialStore.GetCredentialProfile(*fc.CredentialID)
		if err != nil {
			return config, fmt.Errorf("failed to load credential profile: %w", err)
		}
		if !ok {
			return config, fmt.Errorf("credential profile %s not found", *fc.CredentialID)
		}

		secret, err := d.SecretsManager.DecryptString(profile.SecretCiphertext, profile.ID)
		if err != nil {
			return config, fmt.Errorf("failed to decrypt credentials: %w", err)
		}

		config.Username = profile.Username
		config.Secret = secret

		// Derive auth method from credential kind.
		switch profile.Kind {
		case credentials.KindSSHPrivateKey:
			config.AuthMethod = "private_key"
			// Decrypt passphrase if present.
			if profile.PassphraseCiphertext != "" {
				passphrase, err := d.SecretsManager.DecryptString(profile.PassphraseCiphertext, profile.ID)
				if err != nil {
					return config, fmt.Errorf("failed to decrypt passphrase: %w", err)
				}
				config.Passphrase = passphrase
			}
		default:
			config.AuthMethod = "password"
		}

		// Mark the credential as used.
		_ = d.CredentialStore.MarkCredentialProfileUsed(profile.ID, time.Now().UTC())
	}

	return config, nil
}

func validateFileConnectionRequest(req fileConnectionCreateRequest) error {
	return validateFileConnectionFields(req.Name, req.Protocol, req.Host, req.Username, req.Secret, strings.TrimSpace(req.Passphrase), req.AuthMethod, true, true)
}

func credentialKindForFileProtocol(protocol, authMethod string) string {
	switch protocol {
	case "sftp":
		if authMethod == "private_key" {
			return credentials.KindSSHPrivateKey
		}
		return credentials.KindSSHPassword
	case "ftp":
		return credentials.KindFTPPassword
	case "smb":
		return credentials.KindSMBCredentials
	case "webdav":
		return credentials.KindWebDAVCredentials
	default:
		return ""
	}
}

func authMethodForCredentialKind(kind string) string {
	if kind == credentials.KindSSHPrivateKey {
		return "private_key"
	}
	return "password"
}

func credentialKindUsesPrivateKey(kind string) bool {
	return kind == credentials.KindSSHPrivateKey
}

func validateFileConnectionFields(name, protocol, host, username, secret, passphrase, authMethod string, requireName, requireSecret bool) error {
	if requireName && strings.TrimSpace(name) == "" {
		return errors.New("name is required")
	}
	protocol = strings.TrimSpace(protocol)
	if protocol == "" {
		return errors.New("protocol is required")
	}
	if strings.TrimSpace(host) == "" {
		return errors.New("host is required")
	}
	if strings.TrimSpace(username) == "" {
		return errors.New("username is required")
	}
	if requireSecret && strings.TrimSpace(secret) == "" {
		return errors.New("secret is required")
	}

	switch protocol {
	case "sftp":
		if authMethod == "" {
			authMethod = "password"
		}
		if authMethod != "password" && authMethod != "private_key" {
			return errors.New("auth_method must be password or private_key for sftp")
		}
		if authMethod != "private_key" && strings.TrimSpace(passphrase) != "" {
			return errors.New("passphrase is only supported for private_key authentication")
		}
	case "ftp", "smb", "webdav":
		if authMethod == "" {
			authMethod = "password"
		}
		if authMethod != "password" {
			return fmt.Errorf("auth_method must be password for %s", protocol)
		}
		if strings.TrimSpace(passphrase) != "" {
			return errors.New("passphrase is only supported for private_key authentication")
		}
	default:
		return fmt.Errorf("protocol must be one of: sftp, ftp, smb, webdav")
	}

	return nil
}

func fileConnectionCredentialProfileName(connectionName string) string {
	return fmt.Sprintf("File Connection — %s", strings.TrimSpace(connectionName))
}

func fileConnectionCredentialProfileDescription(protocol string) string {
	return fmt.Sprintf("Auto-created for file connection (%s)", strings.TrimSpace(protocol))
}

func effectiveFileConnectionPort(protocol string, port *int) int {
	if port != nil && *port != 0 {
		return *port
	}
	return fileproto.DefaultPort(protocol)
}

func (d *Deps) prepareFileConnectionCredentialProfile(connectionName, protocol, username, kind, secret, passphrase string) (credentials.Profile, error) {
	profileID := idgen.New("cred")
	secretCiphertext, err := d.SecretsManager.EncryptString(strings.TrimSpace(secret), profileID)
	if err != nil {
		return credentials.Profile{}, err
	}

	passphraseCiphertext := ""
	if strings.TrimSpace(passphrase) != "" {
		passphraseCiphertext, err = d.SecretsManager.EncryptString(strings.TrimSpace(passphrase), profileID)
		if err != nil {
			return credentials.Profile{}, err
		}
	}

	return credentials.Profile{
		ID:                   profileID,
		Name:                 fileConnectionCredentialProfileName(connectionName),
		Kind:                 strings.TrimSpace(kind),
		Username:             strings.TrimSpace(username),
		Description:          fileConnectionCredentialProfileDescription(protocol),
		Status:               "active",
		SecretCiphertext:     secretCiphertext,
		PassphraseCiphertext: passphraseCiphertext,
	}, nil
}

func (d *Deps) createFileConnectionCredentialProfile(connectionName, protocol, username, kind, secret, passphrase string) (credentials.Profile, error) {
	profile, err := d.prepareFileConnectionCredentialProfile(connectionName, protocol, username, kind, secret, passphrase)
	if err != nil {
		return credentials.Profile{}, err
	}
	return d.CredentialStore.CreateCredentialProfile(profile)
}
