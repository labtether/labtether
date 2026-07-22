package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/servicehttp"
)

const hubIdentityProfileName = "hub-identity"

type hubSSHIdentity = shared.HubSSHIdentity

func normalizeHubSSHKeyType(keyType string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(keyType)) {
	case "", "ed25519":
		return "ed25519", nil
	case "rsa":
		return "rsa", nil
	default:
		return "", fmt.Errorf("unsupported hub SSH key type")
	}
}

func hubSSHKeyTypeFromPublicKey(publicKey ssh.PublicKey) (string, error) {
	if publicKey == nil {
		return "", fmt.Errorf("hub SSH public key is unavailable")
	}
	switch publicKey.Type() {
	case ssh.KeyAlgoED25519:
		return "ed25519", nil
	case ssh.KeyAlgoRSA, ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSASHA512:
		return "rsa", nil
	default:
		return "", fmt.Errorf("unsupported persisted hub SSH key algorithm %q", publicKey.Type())
	}
}

type sshHubKeyInfo struct {
	PublicKey         string `json:"public_key"`
	KeyType           string `json:"key_type"`
	FingerprintSHA256 string `json:"fingerprint_sha256"`
}

func hubSSHKeyInfoForIdentity(identity *hubSSHIdentity) (sshHubKeyInfo, error) {
	if identity == nil {
		return sshHubKeyInfo{}, fmt.Errorf("hub SSH identity is unavailable")
	}
	publicKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(identity.PublicKey))
	if err != nil {
		return sshHubKeyInfo{}, fmt.Errorf("failed to parse hub SSH public key: %w", err)
	}
	keyType, err := hubSSHKeyTypeFromPublicKey(publicKey)
	if err != nil {
		return sshHubKeyInfo{}, err
	}
	if configuredType := strings.TrimSpace(identity.KeyType); configuredType != "" && configuredType != keyType {
		return sshHubKeyInfo{}, fmt.Errorf("hub SSH identity key type is inconsistent")
	}
	return sshHubKeyInfo{
		PublicKey:         identity.PublicKey,
		KeyType:           keyType,
		FingerprintSHA256: ssh.FingerprintSHA256(publicKey),
	}, nil
}

func (s *apiServer) currentHubSSHIdentity() *hubSSHIdentity {
	if s == nil {
		return nil
	}
	s.hubIdentityMu.RLock()
	defer s.hubIdentityMu.RUnlock()
	if s.hubIdentity == nil {
		return nil
	}
	identity := *s.hubIdentity
	return &identity
}

func (s *apiServer) setHubSSHIdentity(identity *hubSSHIdentity) {
	if s == nil {
		return
	}
	s.hubIdentityMu.Lock()
	defer s.hubIdentityMu.Unlock()
	if identity == nil {
		s.hubIdentity = nil
		return
	}
	copy := *identity
	s.hubIdentity = &copy
}

func (s *apiServer) ensureCurrentHubSSHIdentity() (*hubSSHIdentity, error) {
	s.hubIdentityOperationMu.Lock()
	defer s.hubIdentityOperationMu.Unlock()
	if identity := s.currentHubSSHIdentity(); identity != nil {
		return identity, nil
	}
	identity, err := ensureHubSSHIdentity(s)
	if err != nil {
		return nil, err
	}
	s.setHubSSHIdentity(identity)
	return s.currentHubSSHIdentity(), nil
}

// generateHubSSHKeypair creates a new SSH keypair of the requested type.
// Supported key types are Ed25519 (the default) and RSA 4096.
// Returns the OpenSSH PEM-encoded private key and authorized_keys public key.
func generateHubSSHKeypair(keyType string) (privPEM []byte, pubKeyStr string, err error) {
	keyType, err = normalizeHubSSHKeyType(keyType)
	if err != nil {
		return nil, "", err
	}
	switch keyType {
	case "rsa":
		rsaKey, err := rsa.GenerateKey(rand.Reader, 4096)
		if err != nil {
			return nil, "", fmt.Errorf("failed to generate RSA 4096 keypair: %w", err)
		}
		block, err := ssh.MarshalPrivateKey(rsaKey, "labtether-hub")
		if err != nil {
			return nil, "", fmt.Errorf("failed to marshal RSA private key: %w", err)
		}
		signer, err := ssh.NewSignerFromKey(rsaKey)
		if err != nil {
			return nil, "", fmt.Errorf("failed to create RSA signer: %w", err)
		}
		return pem.EncodeToMemory(block), string(ssh.MarshalAuthorizedKey(signer.PublicKey())), nil
	case "ed25519":
		_, privKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, "", fmt.Errorf("failed to generate ED25519 keypair: %w", err)
		}
		block, err := ssh.MarshalPrivateKey(privKey, "labtether-hub")
		if err != nil {
			return nil, "", fmt.Errorf("failed to marshal ED25519 private key: %w", err)
		}
		signer, err := ssh.NewSignerFromKey(privKey)
		if err != nil {
			return nil, "", fmt.Errorf("failed to create ED25519 signer: %w", err)
		}
		return pem.EncodeToMemory(block), string(ssh.MarshalAuthorizedKey(signer.PublicKey())), nil
	}
	return nil, "", fmt.Errorf("unsupported hub SSH key type")
}

// ensureHubSSHIdentity generates or loads the hub's SSH keypair.
// Key type is controlled by LABTETHER_SSH_KEY_TYPE (default: ed25519).
// The private key is encrypted and stored as a credential profile.
func ensureHubSSHIdentity(s *apiServer) (*hubSSHIdentity, error) {
	if s.secretsManager == nil {
		return nil, fmt.Errorf("encryption not configured; cannot manage hub SSH identity")
	}
	if s.credentialStore == nil {
		return nil, fmt.Errorf("credential store not configured; cannot manage hub SSH identity")
	}

	// Check for existing hub identity profile.
	profiles, err := s.credentialStore.ListCredentialProfiles(100)
	if err != nil {
		return nil, fmt.Errorf("failed to list credential profiles: %w", err)
	}
	for _, p := range profiles {
		if p.Kind == credentials.KindHubSSHIdentity && p.Name == hubIdentityProfileName {
			privPEM, decErr := s.secretsManager.DecryptString(p.SecretCiphertext, p.ID)
			if decErr != nil {
				// Never delete and silently regenerate an identity whose private key
				// cannot be decrypted. Agents may still trust it, so replacement must
				// be an explicit recovery decision rather than a startup side effect.
				return nil, fmt.Errorf("failed to decrypt existing hub SSH identity: %w", decErr)
			}
			signer, parseErr := ssh.ParsePrivateKey([]byte(privPEM))
			if parseErr != nil {
				return nil, fmt.Errorf("failed to parse hub SSH private key: %w", parseErr)
			}
			pubKey := string(ssh.MarshalAuthorizedKey(signer.PublicKey()))
			keyType, typeErr := hubSSHKeyTypeFromPublicKey(signer.PublicKey())
			if typeErr != nil {
				return nil, typeErr
			}
			log.Printf("hub-identity: loaded existing SSH keypair (profile %s)", p.ID)
			return &hubSSHIdentity{
				ProfileID: p.ID,
				PublicKey: pubKey,
				KeyType:   keyType,
			}, nil
		}
	}

	// Generate new keypair using the configured key type.
	keyType, err := normalizeHubSSHKeyType(envOrDefault("LABTETHER_SSH_KEY_TYPE", "ed25519"))
	if err != nil {
		return nil, fmt.Errorf("invalid LABTETHER_SSH_KEY_TYPE: %w", err)
	}
	privPEM, pubKeyStr, err := generateHubSSHKeypair(keyType)
	if err != nil {
		return nil, err
	}

	// Encrypt private key for storage (AAD-bound to profile ID).
	profileID := idgen.New("cred")
	ciphertext, err := s.secretsManager.EncryptString(string(privPEM), profileID)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt hub SSH private key: %w", err)
	}

	now := time.Now().UTC()
	profile := credentials.Profile{
		ID:               profileID,
		Name:             hubIdentityProfileName,
		Kind:             credentials.KindHubSSHIdentity,
		Description:      "Auto-generated hub SSH identity for agent key provisioning",
		Status:           "active",
		SecretCiphertext: ciphertext,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	saved, err := s.credentialStore.CreateCredentialProfile(profile)
	if err != nil {
		return nil, fmt.Errorf("failed to save hub SSH identity: %w", err)
	}

	log.Printf("hub-identity: generated new %s SSH keypair (profile %s)", keyType, saved.ID)
	return &hubSSHIdentity{
		ProfileID: saved.ID,
		PublicKey: pubKeyStr,
		KeyType:   keyType,
	}, nil
}

// loadHubPrivateKeyPEM decrypts and returns the hub SSH private key PEM for
// the given identity. Called from admin_bridge.go and hub key push handler.
func (s *apiServer) loadHubPrivateKeyPEM(identity *hubSSHIdentity) (string, error) {
	if s.credentialStore == nil || s.secretsManager == nil {
		return "", fmt.Errorf("credential store or secrets manager not configured")
	}
	profile, ok, err := s.credentialStore.GetCredentialProfile(identity.ProfileID)
	if err != nil {
		return "", fmt.Errorf("failed to load hub identity profile: %w", err)
	}
	if !ok {
		return "", fmt.Errorf("hub identity profile %s not found", identity.ProfileID)
	}
	privPEM, err := s.secretsManager.DecryptString(profile.SecretCiphertext, profile.ID)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt hub private key: %w", err)
	}
	return privPEM, nil
}

// handleHubSSHPublicKey returns the hub's SSH public key for manual use.
func (s *apiServer) handleHubSSHPublicKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	identity := s.currentHubSSHIdentity()
	if identity == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "hub SSH identity not configured")
		return
	}
	info, err := hubSSHKeyInfoForIdentity(identity)
	if err != nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "hub SSH identity metadata unavailable")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	servicehttp.WriteJSON(w, http.StatusOK, info)
}
