package testutil

import (
	"testing"
	"time"

	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/secrets"
)

// CreatePBSCredentialProfile stores an encrypted PBS API token profile, as
// the PBS settings page does.
func CreatePBSCredentialProfile(t *testing.T, store persistence.CredentialStore, manager *secrets.Manager, credentialID, username, secret, baseURL string) {
	t.Helper()
	AllowInsecureTransportForConnectorTests(t)

	ciphertext, err := manager.EncryptString(secret, credentialID)
	if err != nil {
		t.Fatalf("encrypt pbs secret: %v", err)
	}
	_, err = store.CreateCredentialProfile(credentials.Profile{
		ID:               credentialID,
		Name:             "pbs " + credentialID,
		Kind:             credentials.KindPBSAPIToken,
		Username:         username,
		Status:           "active",
		SecretCiphertext: ciphertext,
		Metadata: map[string]string{
			"base_url": baseURL,
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create pbs credential profile: %v", err)
	}
}
