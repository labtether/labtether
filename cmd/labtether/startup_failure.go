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

// startupFailureMeaning says in words what each stage code means, so the log
// line is readable without the code table. The words are fixed per stage and
// never include the cause, which can carry credentials.
var startupFailureMeaning = map[startupFailureCode]string{
	startupFailureDatabaseInitialize: "PostgreSQL could not be reached or opened; the Hub starts again when it answers",
	startupFailureDatabaseSchema:     "the database schema migration failed",
	startupFailureRuntimeOwnership:   "the Hub lost its PostgreSQL connection or its single-instance lock; it starts again on its own",
	startupFailureInstallState:       "the install state in the data volume could not be read or written",
	startupFailureAuthConfiguration:  "the sign-in settings are not valid",
	startupFailureEncryptionConfig:   "the encryption key could not be loaded",
	startupFailureDemoConfiguration:  "the demo mode settings are not valid",
	startupFailureAdminBootstrap:     "the admin account from LABTETHER_ADMIN_USERNAME and LABTETHER_ADMIN_PASSWORD could not be created",
	startupFailureDemoBootstrap:      "the demo data could not be created",
	startupFailureTOTPKey:            "the two-factor key could not be loaded",
	startupFailureTLSConfiguration:   "the TLS settings are not valid (LABTETHER_TLS_MODE, LABTETHER_TLS_CERT, LABTETHER_TLS_KEY or a port)",
	startupFailureServerRuntime:      "the HTTP server stopped with an error",
}

func logStartupFailure(err error) {
	fields := collectStartupFailureLogFields(err)
	meaning := ""
	if text := startupFailureMeaning[fields.code]; text != "" {
		meaning = ": " + text
	}
	securityruntime.Logf(
		"labtether exited with fatal error: code=%s cause_type=%s fingerprint=%s%s",
		string(fields.code),
		fields.causeType,
		fields.fingerprint,
		meaning,
	)
}
