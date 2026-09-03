package main

import (
	"context"
	"errors"
	"testing"

	"github.com/labtether/labtether/internal/secrets"
)

type failingOIDCSecretMigrator struct {
	err error
}

func (m failingOIDCSecretMigrator) MigrateLegacyOIDCClientSecret(context.Context, *secrets.Manager) error {
	return m.err
}

func TestOIDCClientSecretMigrationFailureStopsStartup(t *testing.T) {
	cause := errors.New("migration failed")
	err := migrateOIDCClientSecretAtStartup(context.Background(), failingOIDCSecretMigrator{err: cause}, nil)
	if err == nil {
		t.Fatal("migration failure did not stop startup")
	}
	fields := collectStartupFailureLogFields(err)
	if fields.code != startupFailureEncryptionConfig {
		t.Fatalf("startup code = %q, want %q", fields.code, startupFailureEncryptionConfig)
	}
	if !errors.Is(err, cause) {
		t.Fatal("startup error did not preserve the migration cause")
	}
}
