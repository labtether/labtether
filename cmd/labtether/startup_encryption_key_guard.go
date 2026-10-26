package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"strings"
)

// errEncryptionKeyMismatch marks a start refused because LABTETHER_ENCRYPTION_KEY
// would silently replace the key that encrypted this Hub's stored secrets.
var errEncryptionKeyMismatch = errors.New("LABTETHER_ENCRYPTION_KEY does not match the stored encryption key")

const encryptionKeyReplaceEnv = "LABTETHER_ENCRYPTION_KEY_REPLACE"

// checkEncryptionKeyChange refuses a LABTETHER_ENCRYPTION_KEY that differs
// from the key persisted in the install state. The persisted key is the one
// that encrypted credential profiles, webhook and notification secrets, file
// connection passwords and 2FA enrolments, so it is the verifier: a different
// key cannot decrypt any of them. Replacing it is allowed only when the
// operator explicitly confirms with LABTETHER_ENCRYPTION_KEY_REPLACE=true.
func checkEncryptionKeyChange(storedKey, envKey, stateRoot string) error {
	storedKey = strings.TrimSpace(storedKey)
	envKey = strings.TrimSpace(envKey)
	if storedKey == "" || envKey == "" || sameEncryptionKey(storedKey, envKey) {
		return nil
	}
	if envOrDefaultBool(encryptionKeyReplaceEnv, false) {
		log.Printf("labtether: WARNING: %s=true — replacing the stored encryption key in %s. "+
			"Secrets encrypted with the previous key (saved credentials, webhook and notification secrets, "+
			"file connection passwords, 2FA enrolments) can no longer be decrypted and must be re-entered. "+
			"Remove %s after this start.", encryptionKeyReplaceEnv, stateRoot, encryptionKeyReplaceEnv)
		return nil
	}
	// Startup failures never log their cause, so state the remedy here. The
	// message names the file but never prints either key.
	log.Printf("labtether: refusing to start: LABTETHER_ENCRYPTION_KEY differs from the encryption key stored in %s/secrets.json. "+
		"This Hub's saved credentials and secrets are encrypted with the stored key and cannot be read with the new one. "+
		"To keep them, remove LABTETHER_ENCRYPTION_KEY or set it to the stored encryption_key value, then restart. "+
		"To deliberately discard them and use the new key, start once with %s=true.", stateRoot, encryptionKeyReplaceEnv)
	return fmt.Errorf("%w (stored in %s)", errEncryptionKeyMismatch, stateRoot)
}

// sameEncryptionKey compares decoded key bytes so equivalent encodings of the
// same key are not treated as a change.
func sameEncryptionKey(a, b string) bool {
	if a == b {
		return true
	}
	decodedA, errA := base64.StdEncoding.DecodeString(a)
	decodedB, errB := base64.StdEncoding.DecodeString(b)
	if errA != nil || errB != nil {
		return false
	}
	return bytes.Equal(decodedA, decodedB)
}
