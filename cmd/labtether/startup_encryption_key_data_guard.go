package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/labtether/labtether/internal/installstate"
	"github.com/labtether/labtether/internal/persistence"
)

// errEncryptionKeyMissing marks a start refused because the database holds
// encrypted secrets but no encryption key is stored or configured, so the Hub
// would otherwise generate a new key that cannot read them.
var errEncryptionKeyMissing = errors.New("the database holds encrypted secrets but no encryption key is stored or configured")

type encryptedSecretsStateLoader interface {
	LoadEncryptedSecretsState(context.Context) (persistence.EncryptedSecretsState, error)
}

// checkEncryptionKeyPresentForData refuses to start when the database holds
// encrypted secrets but neither secrets.json nor LABTETHER_ENCRYPTION_KEY has
// a usable key. That happens when a database is kept or restored without the
// data volume it was written with. A stored well-known example key counts as
// missing, because resolveRuntimeInstallSecrets replaces it. Before this check
// the Hub generated a new key, and every saved credential, webhook secret and
// 2FA enrolment became unreadable without any warning. It runs before the
// install state is resolved, so a refused start writes no new key.
// LABTETHER_ENCRYPTION_KEY_REPLACE=true lets the start continue with a new
// key, discarding those secrets on purpose.
func checkEncryptionKeyPresentForData(ctx context.Context, store *installstate.Store, loader encryptedSecretsStateLoader) error {
	if store == nil || loader == nil {
		return nil
	}
	_, persisted, _, err := store.Load()
	if err != nil {
		// resolveRuntimeInstallSecrets reports unreadable install state.
		return nil
	}
	storedKey := strings.TrimSpace(persisted.EncryptionKey)
	storedPlaceholder := isWellKnownPlaceholderKey(storedKey)
	if (storedKey != "" && !storedPlaceholder) || strings.TrimSpace(os.Getenv("LABTETHER_ENCRYPTION_KEY")) != "" {
		return nil
	}

	state, err := loader.LoadEncryptedSecretsState(ctx)
	if err != nil {
		return err
	}
	totpDependsOnKey := strings.TrimSpace(os.Getenv("LABTETHER_TOTP_KEY")) == ""
	if !state.Credentials && !(state.TOTP && totpDependsOnKey) {
		return nil
	}
	if envOrDefaultBool(encryptionKeyReplaceEnv, false) {
		log.Printf("labtether: WARNING: %s=true — starting with a new encryption key although the database holds secrets "+
			"encrypted with a key that is not in %s/secrets.json. Saved credentials, webhook secrets and 2FA enrolments "+
			"can no longer be decrypted and must be entered again. Remove %s after this start.",
			encryptionKeyReplaceEnv, store.Root(), encryptionKeyReplaceEnv)
		return nil
	}
	stored := "has no encryption key"
	remedy := "Restore secrets.json from the backup of this Hub, or set LABTETHER_ENCRYPTION_KEY to the encryption_key value from it, then restart."
	if storedPlaceholder {
		stored = "holds only a well-known example encryption key, which the Hub replaces with a new one,"
		remedy = "Re-encrypt them with a new key and set LABTETHER_ENCRYPTION_KEY to it (see the encryption key steps in docs/internal/SECRET_ROTATION.md), then restart."
	}
	// Startup failures never log their cause, so state the remedy here.
	log.Printf("labtether: refusing to start: the database holds saved credentials, webhook secrets or 2FA enrolments, "+
		"but %s/secrets.json %s and LABTETHER_ENCRYPTION_KEY is not set. A new key could not read them. %s "+
		"To discard those secrets and start with a new key, start once with %s=true.", store.Root(), stored, remedy, encryptionKeyReplaceEnv)
	return fmt.Errorf("%w (install state in %s)", errEncryptionKeyMissing, store.Root())
}
