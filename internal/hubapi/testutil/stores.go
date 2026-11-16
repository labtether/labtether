package testutil

import (
	"path/filepath"
	"testing"

	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/installstate"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/retention"
	"github.com/labtether/labtether/internal/secrets"
)

// Stores is the set of in-memory stores that cmd/labtether's
// newTestAPIServer puts on its test server. A handler package test builds
// its Deps from these, the way cmd's build*Deps builds them from apiServer.
//
// Fields that newTestAPIServer leaves nil (HubCollectorStore,
// GroupMaintenanceStore, GroupProfileStore) stay nil here too, so a test
// that relied on "store unavailable" in cmd sees the same here. Tests set
// them when they need them, as they did on apiServer.
type Stores struct {
	TerminalStore           persistence.TerminalStore
	TerminalPersistentStore persistence.TerminalPersistentSessionStore
	SSHKnownHostStore       persistence.SSHKnownHostStore
	AuditStore              persistence.AuditStore
	AssetStore              persistence.AssetStore
	GroupStore              persistence.GroupStore
	GroupMaintenanceStore   persistence.GroupMaintenanceStore
	GroupProfileStore       persistence.GroupProfileStore
	CredentialStore         persistence.CredentialStore
	CanonicalStore          persistence.CanonicalModelStore
	TelemetryStore          persistence.TelemetryStore
	LogStore                persistence.LogStore
	ActionStore             persistence.ActionStore
	UpdateStore             persistence.UpdateStore
	AlertStore              persistence.AlertStore
	AlertInstanceStore      persistence.AlertInstanceStore
	IncidentStore           persistence.IncidentStore
	RetentionStore          persistence.RetentionStore
	RuntimeStore            persistence.RuntimeSettingsStore
	SyntheticStore          persistence.SyntheticStore
	EnrollmentStore         persistence.EnrollmentStore
	AdminResetStore         persistence.AdminResetStore
	APIKeyStore             persistence.APIKeyStore
	ScheduleStore           persistence.ScheduleStore
	WebhookStore            persistence.WebhookStore
	SavedActionStore        persistence.SavedActionStore
	LinkSuggestionStore     persistence.LinkSuggestionStore
	HubSSHKeyStore          persistence.HubSSHKeyStore
	AuthStore               persistence.AuthStore
	HubCollectorStore       persistence.HubCollectorStore
	InstallStateStore       *installstate.Store
	SecretsManager          *secrets.Manager
	ConnectorRegistry       *connectorsdk.Registry
}

// NewStores returns the same stores newTestAPIServer builds: fresh memory
// stores, default retention settings, the fixed test secrets key and an
// empty connector registry.
//
// newTestAPIServer also registers a t.Cleanup that deletes what the test
// created. These are memory stores dropped with the test, so that cleanup
// changes no result and is not repeated here.
func NewStores(t *testing.T) *Stores {
	t.Helper()

	terminalStore := persistence.NewMemoryTerminalStore()
	assetStore := persistence.NewMemoryAssetStore()
	groupStore := persistence.NewMemoryGroupStore()
	retentionStore := persistence.NewMemoryRetentionStore()
	_, _ = retentionStore.SaveRetentionSettings(retention.DefaultSettings())

	return &Stores{
		TerminalStore:           terminalStore,
		TerminalPersistentStore: terminalStore,
		SSHKnownHostStore:       persistence.NewMemorySSHKnownHostStore(),
		AuditStore:              persistence.NewMemoryAuditStore(),
		AssetStore:              assetStore,
		GroupStore:              groupStore,
		CredentialStore:         persistence.NewMemoryCredentialStore(),
		CanonicalStore:          persistence.NewMemoryCanonicalModelStore(),
		TelemetryStore:          persistence.NewMemoryTelemetryStore(),
		LogStore:                persistence.NewMemoryLogStore(),
		ActionStore:             persistence.NewMemoryActionStore(),
		UpdateStore:             persistence.NewMemoryUpdateStore(),
		AlertStore:              persistence.NewMemoryAlertStore(),
		AlertInstanceStore:      persistence.NewMemoryAlertInstanceStore(),
		IncidentStore:           persistence.NewMemoryIncidentStore(),
		RetentionStore:          retentionStore,
		RuntimeStore:            persistence.NewMemoryRuntimeSettingsStore(),
		SyntheticStore:          persistence.NewMemorySyntheticStore(),
		EnrollmentStore:         persistence.NewMemoryEnrollmentStoreWithGroupStore(assetStore, groupStore),
		AdminResetStore:         persistence.NewMemoryAdminResetStore(),
		APIKeyStore:             persistence.NewMemoryAPIKeyStore(),
		ScheduleStore:           persistence.NewMemoryScheduleStore(),
		WebhookStore:            persistence.NewMemoryWebhookStore(),
		SavedActionStore:        persistence.NewMemorySavedActionStore(),
		LinkSuggestionStore:     persistence.NewMemoryLinkSuggestionStore(),
		HubSSHKeyStore:          persistence.NewMemoryHubSSHKeyStore(),
		AuthStore:               persistence.NewMemoryAuthStore(),
		InstallStateStore:       installstate.New(filepath.Join(t.TempDir(), "install")),
		SecretsManager:          TestSecretsManager(t),
		ConnectorRegistry:       connectorsdk.NewRegistry(),
	}
}
