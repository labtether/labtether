// Package testutil provides shared test factories for hubapi sub-packages.
// This is a regular (non-test) package so that _test.go files in any
// hubapi sub-package can import it.
//
// IMPORTANT: This package must NOT import any hubapi sub-package (to avoid
// import cycles). It provides store factories and no-op helpers that test
// files combine into package-local Deps structs.
package testutil

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/secrets"
)

// --- No-op middleware and helpers ---

// NoopAuth is a passthrough auth wrapper for tests.
func NoopAuth(h http.HandlerFunc) http.HandlerFunc { return h }

// NoopRateLimit always allows requests in tests.
func NoopRateLimit(_ http.ResponseWriter, _ *http.Request, _ string, _ int, _ time.Duration) bool {
	return true
}

// NoopAdminAuth always returns false (no admin enforcement) in tests.
func NoopAdminAuth(_ http.ResponseWriter, _ *http.Request) bool {
	return false
}

// TestActorID returns "test-actor" for all contexts.
func TestActorID(_ context.Context) string { return "test-actor" }

// TestUserID returns "test-user" for all contexts.
func TestUserID(_ context.Context) string { return "test-user" }

// NoopAudit is a no-op audit event appender.
func NoopAudit(_ audit.Event, _ string) {}

// NoopLogEvent is a no-op log event appender.
func NoopLogEvent(_ logs.Event, _ string) {}

// DecodeJSONBody delegates to shared.DecodeJSONBody for tests.
func DecodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) error {
	return shared.DecodeJSONBody(w, r, dst)
}

// --- Store and secrets factories ---

// TestSecretsManager creates a secrets.Manager for test use.
func TestSecretsManager(t *testing.T) *secrets.Manager {
	t.Helper()
	sm, err := secrets.NewManagerFromEncodedKey("MDEyMzQ1Njc4OUFCQ0RFRjAxMjM0NTY3ODlBQkNERUY=")
	if err != nil {
		t.Fatalf("failed to create test secrets manager: %v", err)
	}
	return sm
}

// TestPolicyState implements any PolicyStateProvider interface.
type TestPolicyState struct {
	Config policy.EvaluatorConfig
}

func (s *TestPolicyState) Current() policy.EvaluatorConfig { return s.Config }

// NewTestPolicyState creates a default policy state for tests.
func NewTestPolicyState() *TestPolicyState {
	return &TestPolicyState{Config: policy.DefaultEvaluatorConfig()}
}

// --- Store constructors (re-exported for convenience) ---

func NewAssetStore() persistence.AssetStore         { return persistence.NewMemoryAssetStore() }
func NewGroupStore() persistence.GroupStore         { return persistence.NewMemoryGroupStore() }
func NewTelemetryStore() persistence.TelemetryStore { return persistence.NewMemoryTelemetryStore() }
func NewLogStore() persistence.LogStore             { return persistence.NewMemoryLogStore() }
func NewAuditStore() persistence.AuditStore         { return persistence.NewMemoryAuditStore() }
func NewTerminalStore() persistence.TerminalStore   { return persistence.NewMemoryTerminalStore() }
func NewCredentialStore() persistence.CredentialStore {
	return persistence.NewMemoryCredentialStore()
}
func NewRetentionStore() persistence.RetentionStore {
	return persistence.NewMemoryRetentionStore()
}
func NewSyntheticStore() persistence.SyntheticStore {
	return persistence.NewMemorySyntheticStore()
}
func NewLinkSuggestionStore() persistence.LinkSuggestionStore {
	return persistence.NewMemoryLinkSuggestionStore()
}
func NewAlertStore() persistence.AlertStore {
	return persistence.NewMemoryAlertStore()
}
func NewAlertInstanceStore() persistence.AlertInstanceStore {
	return persistence.NewMemoryAlertInstanceStore()
}
func NewIncidentStore() persistence.IncidentStore {
	return persistence.NewMemoryIncidentStore()
}
func NewCanonicalStore() persistence.CanonicalModelStore {
	return persistence.NewMemoryCanonicalModelStore()
}
func NewActionStore() persistence.ActionStore {
	return persistence.NewMemoryActionStore()
}
func NewUpdateStore() persistence.UpdateStore {
	return persistence.NewMemoryUpdateStore()
}
func NewEnrollmentStore() persistence.EnrollmentStore {
	return persistence.NewMemoryEnrollmentStore()
}
func NewRuntimeSettingsStore() persistence.RuntimeSettingsStore {
	return persistence.NewMemoryRuntimeSettingsStore()
}

// --- Asset creation helper ---

// CreateTestAsset is a helper to create an asset via the persistence layer.
func CreateTestAsset(t *testing.T, store persistence.AssetStore, id, assetType, name string) {
	t.Helper()
	_, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: id,
		Type:    assetType,
		Name:    name,
		Source:  "test",
	})
	if err != nil {
		t.Fatalf("create test asset %s: %v", id, err)
	}
}
