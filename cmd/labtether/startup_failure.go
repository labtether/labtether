package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/securityruntime"
)

type startupFailureCode string

const (
	startupFailureUnknown            startupFailureCode = "startup_unknown"
	startupFailureDatabaseInitialize startupFailureCode = "database_initialize"
	startupFailureDatabaseSchema     startupFailureCode = "database_schema_migration"
	startupFailureRuntimeOwnership   startupFailureCode = "runtime_ownership"
	startupFailureInstallState       startupFailureCode = "install_state"
	startupFailureAuthConfiguration  startupFailureCode = "auth_configuration"
	startupFailureEncryptionConfig   startupFailureCode = "encryption_configuration"
	startupFailureDemoConfiguration  startupFailureCode = "demo_configuration"
	startupFailureAdminBootstrap     startupFailureCode = "admin_bootstrap"
	startupFailureDemoBootstrap      startupFailureCode = "demo_bootstrap"
	startupFailureTOTPKey            startupFailureCode = "totp_key"
	startupFailureTLSConfiguration   startupFailureCode = "tls_configuration"
	startupFailureServerRuntime      startupFailureCode = "server_runtime"
)

// startupFailure carries a stable, non-secret stage code while preserving the
// original cause for trusted in-process inspection. Error deliberately omits
// the cause because driver and configuration errors can contain credentials.
type startupFailure struct {
	code  startupFailureCode
	cause error
}

func (e *startupFailure) Error() string {
	code := startupFailureUnknown
	if e != nil && e.code != "" {
		code = e.code
	}
	return "labtether startup failed: " + string(code)
}

func (e *startupFailure) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func newStartupFailure(code startupFailureCode, cause error) error {
	if code == "" {
		code = startupFailureUnknown
	}
	if cause == nil {
		cause = errors.New("startup failure")
	}
	return &startupFailure{code: code, cause: cause}
}

func newDatabaseStartupFailure(cause error) error {
	code := startupFailureDatabaseInitialize
	var schemaError *persistence.SchemaMigrationError
	if errors.As(cause, &schemaError) {
		code = startupFailureDatabaseSchema
	}
	return newStartupFailure(code, cause)
}

type startupFailureLogFields struct {
	code        startupFailureCode
	causeType   string
	fingerprint string
}

func collectStartupFailureLogFields(err error) startupFailureLogFields {
	fields := startupFailureLogFields{
		code:      startupFailureUnknown,
		causeType: "unknown",
	}

	var classified *startupFailure
	if errors.As(err, &classified) && classified.code != "" {
		fields.code = classified.code
	}

	typeChain := make([]string, 0, 4)
	for current, depth := err, 0; current != nil && depth < 16; current, depth = errors.Unwrap(current), depth+1 {
		typeName := fmt.Sprintf("%T", current)
		typeChain = append(typeChain, typeName)
		fields.causeType = typeName
	}

	// The fingerprint deliberately hashes only the stable stage and Go error
	// type chain. It never incorporates Error() text, URLs, SQL, or credentials.
	digest := sha256.Sum256([]byte(string(fields.code) + "\x00" + strings.Join(typeChain, "\x00")))
	fields.fingerprint = hex.EncodeToString(digest[:8])
	return fields
}

func logStartupFailure(err error) {
	fields := collectStartupFailureLogFields(err)
	securityruntime.Logf(
		"labtether exited with fatal error: code=%s cause_type=%s fingerprint=%s",
		string(fields.code),
		fields.causeType,
		fields.fingerprint,
	)
}
