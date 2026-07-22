package main

import (
	"bytes"
	"errors"
	"log"
	"regexp"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/persistence"
)

func TestStartupFailureFieldsDoNotExposeCredentialBearingCause(t *testing.T) {
	const secret = "postgres://operator:credential-bearing-secret@database.invalid/labtether"
	err := newStartupFailure(startupFailureInstallState, errors.New(secret))
	fields := collectStartupFailureLogFields(err)

	logged := strings.Join([]string{string(fields.code), fields.causeType, fields.fingerprint}, " ")
	if strings.Contains(logged, secret) || strings.Contains(logged, "credential-bearing-secret") || strings.Contains(logged, "postgres://") {
		t.Fatalf("startup diagnostic exposed credential-bearing cause: %q", logged)
	}
	if fields.code != startupFailureInstallState {
		t.Fatalf("startup code = %q, want %q", fields.code, startupFailureInstallState)
	}
	if fields.causeType != "*errors.errorString" {
		t.Fatalf("underlying cause type = %q, want *errors.errorString", fields.causeType)
	}
	if !regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(fields.fingerprint) {
		t.Fatalf("fingerprint = %q, want 16 lowercase hex characters", fields.fingerprint)
	}
}

func TestLogStartupFailureNeverEmitsCredentialBearingCause(t *testing.T) {
	const secret = "postgres://operator:credential-bearing-secret@database.invalid/labtether"
	var output bytes.Buffer
	originalWriter := log.Writer()
	originalFlags := log.Flags()
	originalPrefix := log.Prefix()
	log.SetOutput(&output)
	log.SetFlags(0)
	log.SetPrefix("")
	t.Cleanup(func() {
		log.SetOutput(originalWriter)
		log.SetFlags(originalFlags)
		log.SetPrefix(originalPrefix)
	})

	logStartupFailure(newStartupFailure(startupFailureDatabaseInitialize, errors.New(secret)))
	logged := output.String()
	if strings.Contains(logged, secret) || strings.Contains(logged, "credential-bearing-secret") || strings.Contains(logged, "postgres://") {
		t.Fatalf("fatal startup log exposed credential-bearing cause: %q", logged)
	}
	for _, required := range []string{"code=database_initialize", "cause_type=*errors.errorString", "fingerprint="} {
		if !strings.Contains(logged, required) {
			t.Fatalf("fatal startup log %q is missing %q", logged, required)
		}
	}
}

func TestStartupFailureFingerprintNeverDependsOnCauseMessage(t *testing.T) {
	left := collectStartupFailureLogFields(newStartupFailure(startupFailureTLSConfiguration, errors.New("first-secret")))
	right := collectStartupFailureLogFields(newStartupFailure(startupFailureTLSConfiguration, errors.New("different-secret")))
	if left.fingerprint != right.fingerprint {
		t.Fatalf("same code and error types produced message-dependent fingerprints: %q != %q", left.fingerprint, right.fingerprint)
	}
}

func TestDatabaseSchemaMigrationFailureHasSpecificStartupCode(t *testing.T) {
	err := newDatabaseStartupFailure(&persistence.SchemaMigrationError{})
	fields := collectStartupFailureLogFields(err)
	if fields.code != startupFailureDatabaseSchema {
		t.Fatalf("schema migration startup code = %q, want %q", fields.code, startupFailureDatabaseSchema)
	}
	if !strings.Contains(fields.causeType, "SchemaMigrationError") {
		t.Fatalf("schema migration cause type = %q, want SchemaMigrationError", fields.causeType)
	}
}

func TestRuntimeOwnershipFailureHasSpecificNonSecretStartupCode(t *testing.T) {
	err := newStartupFailure(startupFailureRuntimeOwnership, persistence.ErrHubRuntimeLeaseHeld)
	fields := collectStartupFailureLogFields(err)
	if fields.code != startupFailureRuntimeOwnership {
		t.Fatalf("runtime ownership startup code = %q, want %q", fields.code, startupFailureRuntimeOwnership)
	}
	if strings.Contains(fields.fingerprint, persistence.ErrHubRuntimeLeaseHeld.Error()) {
		t.Fatal("runtime ownership fingerprint exposed the error message")
	}
}

func TestUnclassifiedStartupFailureUsesNonSecretFallback(t *testing.T) {
	const secret = "owner-token-do-not-log"
	fields := collectStartupFailureLogFields(errors.New(secret))
	if fields.code != startupFailureUnknown {
		t.Fatalf("unclassified startup code = %q, want %q", fields.code, startupFailureUnknown)
	}
	if strings.Contains(fields.fingerprint, secret) || strings.Contains(fields.causeType, secret) {
		t.Fatal("unclassified startup diagnostics exposed the error message")
	}
}
