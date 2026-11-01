package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/labtether/labtether/internal/secrets"
)

// errEncryptionKeyWrongForData marks a start refused because the database was
// written with a different encryption key, for example after restoring a
// backup onto a Hub with its own secrets.json.
var errEncryptionKeyWrongForData = errors.New("the encryption key cannot read this database's saved secrets")

const (
	encryptionKeyCheckSetting = "encryption_key_check"
	encryptionKeyCheckAAD     = "system_settings:encryption_key_check"
	encryptionKeyCheckValue   = "labtether-encryption-key-check-v1"
)

type encryptionKeyCheckStore interface {
	GetSystemSetting(ctx context.Context, key string) (json.RawMessage, bool, error)
	PutSystemSetting(ctx context.Context, key string, value json.RawMessage) error
}

// encryptionKeyCheckRecord is a known value encrypted with the key that writes
// this database, plus that key's ID so a refused start can name the key needed.
type encryptionKeyCheckRecord struct {
	Ciphertext string `json:"ciphertext"`
	KeyID      string `json:"key_id"`
}

// checkEncryptionKeyMatchesData refuses to start when the database's key check
// does not decrypt with the Hub's key. The secrets.json guard in
// checkEncryptionKeyChange cannot see this case: a restored database keeps its
// old ciphertext while the Hub's own secrets.json holds a different key, and
// every saved secret fails later. The first start with this check, and a start
// with LABTETHER_ENCRYPTION_KEY_REPLACE=true, write the check for the current key.
func checkEncryptionKeyMatchesData(ctx context.Context, store encryptionKeyCheckStore, manager *secrets.Manager, encodedKey string) error {
	if store == nil || manager == nil {
		return nil
	}
	currentID := encryptionKeyID(encodedKey)
	raw, found, err := store.GetSystemSetting(ctx, encryptionKeyCheckSetting)
	if err != nil {
		return fmt.Errorf("read encryption key check: %w", err)
	}
	if found {
		var record encryptionKeyCheckRecord
		if json.Unmarshal(raw, &record) == nil {
			plain, decryptErr := manager.DecryptString(record.Ciphertext, encryptionKeyCheckAAD)
			if decryptErr == nil && plain == encryptionKeyCheckValue {
				return nil
			}
		}
		if !envOrDefaultBool(encryptionKeyReplaceEnv, false) {
			// Startup failures never log their cause, so state the remedy here.
			// Key IDs are a short SHA-256 of the key, never the key itself.
			log.Printf("labtether: refusing to start: this database was written with the encryption key with ID %s, "+
				"but this Hub's key has ID %s, so saved credentials, webhook secrets and 2FA enrolments cannot be read. "+
				"Set LABTETHER_ENCRYPTION_KEY to the encryption_key from the secrets.json saved with this database "+
				"(its ID is the first 12 characters of: printf '%%s' KEY | base64 -d | sha256sum), then restart. "+
				"To discard those secrets and keep this key, start once with %s=true.",
				orUnknown(record.KeyID), currentID, encryptionKeyReplaceEnv)
			return fmt.Errorf("%w (database key ID %s, Hub key ID %s)", errEncryptionKeyWrongForData, orUnknown(record.KeyID), currentID)
		}
		log.Printf("labtether: WARNING: %s=true: this database was written with the encryption key with ID %s; "+
			"continuing with key ID %s. Secrets saved with the old key cannot be read and must be entered again. "+
			"Remove %s after this start.", encryptionKeyReplaceEnv, orUnknown(record.KeyID), currentID, encryptionKeyReplaceEnv)
	}
	ciphertext, err := manager.EncryptString(encryptionKeyCheckValue, encryptionKeyCheckAAD)
	if err != nil {
		return fmt.Errorf("write encryption key check: %w", err)
	}
	value, err := json.Marshal(encryptionKeyCheckRecord{Ciphertext: ciphertext, KeyID: currentID})
	if err != nil {
		return fmt.Errorf("write encryption key check: %w", err)
	}
	if err := store.PutSystemSetting(ctx, encryptionKeyCheckSetting, value); err != nil {
		return fmt.Errorf("write encryption key check: %w", err)
	}
	return nil
}

// encryptionKeyID is the first 12 hex characters of SHA-256 over the decoded
// key. It names a key without revealing it.
func encryptionKeyID(encodedKey string) string {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encodedKey))
	if err != nil {
		return "unknown"
	}
	sum := sha256.Sum256(decoded)
	return hex.EncodeToString(sum[:])[:12]
}

func orUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return value
}
