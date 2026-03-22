package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"fmt"
	"log"
	"net/http"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/servicehttp"
)

const hubIdentityProfileName = "hub-identity"

type hubSSHIdentity = shared.HubSSHIdentity

// generateHubSSHKeypair creates a new SSH keypair of the requested type.
// keyType must be "rsa" for RSA 4096 or anything else (including "") for Ed25519.
// Returns the OpenSSH PEM-encoded private key, the authorized_keys public key
// string, and the canonical key type name.
func generateHubSSHKeypair(keyType string) (privPEM []byte, pubKeyStr string, err error) {
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
	default: // "ed25519"
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
}

// ensureHubSSHIdentity generates or loads the hub's SSH keypair.
// Key type is controlled by LABTETHER_SSH_KEY_TYPE (default: ed25519).
// The private key is encrypted and stored as a credential profile.
func ensureHubSSHIdentity(s *apiServer) (*hubSSHIdentity, error) {
	if s.secretsManager == nil {
		return nil, fmt.Errorf("encryption not configured; cannot manage hub SSH identity")
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
				// Encryption key changed — delete the broken profile and regenerate.
				log.Printf("hub-identity: existing profile %s cannot be decrypted (key mismatch?), regenerating", p.ID)
				if delErr := s.credentialStore.DeleteCredentialProfile(p.ID); delErr != nil {
					log.Printf("hub-identity: failed to delete broken profile %s: %v", p.ID, delErr)
				}
				break
			}
			signer, parseErr := ssh.ParsePrivateKey([]byte(privPEM))
			if parseErr != nil {
				return nil, fmt.Errorf("failed to parse hub SSH private key: %w", parseErr)
			}
			pubKey := string(ssh.MarshalAuthorizedKey(signer.PublicKey()))
			log.Printf("hub-identity: loaded existing SSH keypair (profile %s)", p.ID)
			return &hubSSHIdentity{
				ProfileID: p.ID,
				PublicKey: pubKey,
			}, nil
		}
	}

	// Generate new keypair using the configured key type.
	keyType := envOrDefault("LABTETHER_SSH_KEY_TYPE", "ed25519")
	privPEM, pubKeyStr, err := generateHubSSHKeypair(keyType)
	if err != nil {
		return nil, err
	}
	// Normalize keyType to canonical name (empty string -> "ed25519").
	if keyType != "rsa" {
		keyType = "ed25519"
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
	if s.hubIdentity == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "hub SSH identity not configured")
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]string{
		"public_key": s.hubIdentity.PublicKey,
	})
}
