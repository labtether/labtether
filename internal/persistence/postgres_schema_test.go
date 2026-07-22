package persistence

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNormalizedSchemaMigrationsSortsByVersion(t *testing.T) {
	input := []schemaMigration{
		{Version: 3, Name: "third"},
		{Version: 1, Name: "first"},
		{Version: 2, Name: "second"},
	}

	got, err := normalizedSchemaMigrations(input)
	if err != nil {
		t.Fatalf("normalizedSchemaMigrations returned error: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 migrations, got %d", len(got))
	}
	if got[0].Version != 1 || got[1].Version != 2 || got[2].Version != 3 {
		t.Fatalf("expected sorted versions [1 2 3], got [%d %d %d]", got[0].Version, got[1].Version, got[2].Version)
	}
}

func TestNormalizedSchemaMigrationsRejectsDuplicates(t *testing.T) {
	input := []schemaMigration{
		{Version: 7, Name: "one"},
		{Version: 7, Name: "two"},
	}

	if _, err := normalizedSchemaMigrations(input); err == nil {
		t.Fatalf("expected duplicate version error, got nil")
	}
}

func TestPostgresSchemaMigrationsHaveUniqueVersions(t *testing.T) {
	migrations, err := normalizedSchemaMigrations(postgresSchemaMigrations())
	if err != nil {
		t.Fatalf("postgresSchemaMigrations returned invalid migration set: %v", err)
	}
	if len(migrations) == 0 {
		t.Fatalf("expected at least one migration")
	}
}

func TestAppliedPushDeviceTimezoneMigrationChecksumRemainsStable(t *testing.T) {
	const (
		version      = 84
		wantChecksum = "b7c6503a8f3bc3caf14ff5e8c97e0f9617da0f25b44e2175135bbe2080ab7ff8"
	)

	for _, migration := range postgresSchemaMigrations() {
		if migration.Version != version {
			continue
		}
		if got := schemaMigrationChecksum(migration); got != wantChecksum {
			t.Fatalf("applied migration v%d checksum = %s, want %s; append a new migration instead of editing applied SQL", version, got, wantChecksum)
		}
		return
	}
	t.Fatalf("missing applied migration v%d", version)
}

func TestAppliedHubMetricSamplesMigrationChecksumRemainsStable(t *testing.T) {
	const (
		version      = 87
		wantChecksum = "8d4a072d0dd050bb8c5122ba87424414feae292a270830d98224b5ad58323036"
	)

	for _, migration := range postgresSchemaMigrations() {
		if migration.Version != version {
			continue
		}
		if got := schemaMigrationChecksum(migration); got != wantChecksum {
			t.Fatalf("applied migration v%d checksum = %s, want %s; append a new migration instead of editing applied SQL", version, got, wantChecksum)
		}
		return
	}
	t.Fatalf("missing applied migration v%d", version)
}

func TestAppliedFinalQAMigrationChecksumsRemainStable(t *testing.T) {
	wantChecksums := map[int]string{
		88: "5a077dbb7fa3ba1bdc2fb871b6d3ddb96a42c8b277a36201d4b5c3e1f15c0ed8",
		89: "b6dbb6371999d629c093f3c8318442a63bebe940162a9c4ea255208be8de604d",
		90: "483d5732002acced4181e6f9411b2bb2f7081ecad1a766b5487b5a45eee29b8d",
		91: "7f3182a20f76004ce3f3571aa5cf5f1c0de28b2877c7871ae97ec571c8e44ca7",
	}

	for _, migration := range postgresSchemaMigrations() {
		wantChecksum, tracked := wantChecksums[migration.Version]
		if !tracked {
			continue
		}
		if got := schemaMigrationChecksum(migration); got != wantChecksum {
			t.Fatalf("applied migration v%d checksum = %s, want %s; append a new migration instead of editing applied SQL", migration.Version, got, wantChecksum)
		}
		delete(wantChecksums, migration.Version)
	}
	if len(wantChecksums) != 0 {
		t.Fatalf("missing applied final-QA migrations: %v", wantChecksums)
	}
}

func TestSchemaMigrationErrorDoesNotExposeDriverCause(t *testing.T) {
	secretCause := errors.New("postgres://operator:credential-bearing-secret@database.invalid/labtether")
	err := newSchemaMigrationError(secretCause)

	if got := err.Error(); strings.Contains(got, "credential-bearing-secret") || strings.Contains(got, "postgres://") {
		t.Fatalf("schema migration error exposed its driver cause: %q", got)
	}
	if !errors.Is(err, secretCause) {
		t.Fatal("schema migration error must preserve its cause for trusted classification")
	}
}

func TestPushDeviceTimezoneUnknownDefaultUsesNewMigration(t *testing.T) {
	const version = 86
	for _, migration := range postgresSchemaMigrations() {
		if migration.Version != version {
			continue
		}
		statements := strings.Join(migration.Statements, "\n")
		if !strings.Contains(statements, "ALTER COLUMN time_zone SET DEFAULT ''") {
			t.Fatalf("migration v%d does not set the unknown timezone default: %s", version, statements)
		}
		return
	}
	t.Fatalf("missing timezone-default migration v%d", version)
}

func TestHubMetricSamplesMigrationUsesBoundedNonAssetScopes(t *testing.T) {
	const version = 87
	for _, migration := range postgresSchemaMigrations() {
		if migration.Version != version {
			continue
		}
		statements := strings.Join(migration.Statements, "\n")
		for _, required := range []string{
			"CREATE TABLE IF NOT EXISTS hub_metric_samples",
			"scope IN ('hub-alerts', 'hub-reliability')",
			"idx_hub_metric_samples_scope_metric_time",
			"idx_hub_metric_samples_time",
		} {
			if !strings.Contains(statements, required) {
				t.Fatalf("migration v%d is missing %q", version, required)
			}
		}
		if strings.Contains(statements, "REFERENCES assets") {
			t.Fatalf("hub metric scope must not create a fake asset FK: %s", statements)
		}
		return
	}
	t.Fatalf("missing hub metric samples migration v%d", version)
}

func TestScheduledTaskExecutionMigrationAddsDurableOccurrenceState(t *testing.T) {
	const version = 90
	for _, migration := range postgresSchemaMigrations() {
		if migration.Version != version {
			continue
		}
		statements := strings.Join(migration.Statements, "\n")
		for _, required := range []string{
			"last_run_status",
			"last_error",
			"last_run_job_id",
			"idx_scheduled_tasks_enabled_next_run",
			"next_run_at ASC NULLS FIRST, id ASC",
		} {
			if !strings.Contains(statements, required) {
				t.Fatalf("migration v%d missing %q: %s", version, required, statements)
			}
		}
		return
	}
	t.Fatalf("missing scheduled-task execution migration v%d", version)
}

func TestLiveActivityPushTokenMigrationKeepsTerminalRecoveryState(t *testing.T) {
	const version = 89
	for _, migration := range postgresSchemaMigrations() {
		if migration.Version != version {
			continue
		}
		statements := strings.Join(migration.Statements, "\n")
		for _, required := range []string{
			"CREATE TABLE IF NOT EXISTS live_activity_push_tokens",
			"user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE",
			"retry_count BETWEEN 0 AND 255",
			"delivery_generation BIGINT NOT NULL DEFAULT 0",
			"pending_state_ciphertext TEXT NOT NULL DEFAULT ''",
			"last_delivered_incident_updated_at TIMESTAMPTZ",
			"UNIQUE(user_id, device_id, activity_id)",
			"UNIQUE(token_hash, bundle_id, environment)",
			"idx_live_activity_tokens_incident_expiry",
			"idx_live_activity_tokens_retry",
			"idx_live_activity_tokens_expiry",
		} {
			if !strings.Contains(statements, required) {
				t.Fatalf("migration v%d is missing %q", version, required)
			}
		}
		if strings.Contains(statements, "incident_id TEXT NOT NULL REFERENCES incidents") {
			t.Fatal("live activity registrations need to outlive hard-deleted incidents so reconciliation can send a terminal update")
		}
		return
	}
	t.Fatalf("missing live activity push token migration v%d", version)
}

func TestLabeledMetricSnapshotMigrationAddsBoundedTimeIndex(t *testing.T) {
	const version = 88
	for _, migration := range postgresSchemaMigrations() {
		if migration.Version != version {
			continue
		}
		statements := strings.Join(migration.Statements, "\n")
		for _, required := range []string{
			"idx_metric_samples_asset_time_labeled_snapshot",
			"asset_id, collected_at DESC, id DESC",
		} {
			if !strings.Contains(statements, required) {
				t.Fatalf("migration v%d is missing %q", version, required)
			}
		}
		return
	}
	t.Fatalf("missing labeled metric snapshot migration v%d", version)
}

func TestPrometheusRemoteWriteReplayStateMigrationIsDurableAndNonSecret(t *testing.T) {
	const version = 94
	for _, migration := range postgresSchemaMigrations() {
		if migration.Version != version {
			continue
		}
		statements := strings.Join(migration.Statements, "\n")
		for _, required := range []string{
			"CREATE TABLE IF NOT EXISTS prometheus_remote_write_state",
			"endpoint_fingerprint",
			"asset_sample_id",
			"hub_sample_id",
			"last_advanced_at",
			"CHECK (asset_sample_id >= 0)",
			"CHECK (hub_sample_id >= 0)",
		} {
			if !strings.Contains(statements, required) {
				t.Fatalf("migration v%d missing %q: %s", version, required, statements)
			}
		}
		for _, forbidden := range []string{"remote_write_url", "username", "password", "secret"} {
			if strings.Contains(strings.ToLower(statements), forbidden) {
				t.Fatalf("migration v%d stores forbidden endpoint/credential field %q", version, forbidden)
			}
		}
		return
	}
	t.Fatalf("missing prometheus remote write migration v%d", version)
}

func TestOIDCIssuerMigrationReplacesSubjectOnlyUniqueness(t *testing.T) {
	var migration *schemaMigration
	for _, candidate := range postgresSchemaMigrations() {
		if candidate.Version == 85 {
			copy := candidate
			migration = &copy
			break
		}
	}
	if migration == nil {
		t.Fatal("missing OIDC issuer-scoping migration")
	}
	statements := strings.Join(migration.Statements, "\n")
	for _, required := range []string{
		"ADD COLUMN IF NOT EXISTS oidc_issuer",
		"DROP INDEX IF EXISTS idx_users_auth_provider_subject",
		"idx_users_auth_provider_issuer_subject",
		"idx_users_auth_provider_legacy_subject",
	} {
		if !strings.Contains(statements, required) {
			t.Fatalf("OIDC issuer migration is missing %q", required)
		}
	}
}

func TestDBStatementTimeoutDefaultWhenUnset(t *testing.T) {
	t.Setenv("LABTETHER_DB_STATEMENT_TIMEOUT", "")

	if got := dbStatementTimeout(); got != defaultDBStatementTimeout {
		t.Fatalf("expected default timeout %s, got %s", defaultDBStatementTimeout, got)
	}
}

func TestDBStatementTimeoutUsesConfiguredValue(t *testing.T) {
	t.Setenv("LABTETHER_DB_STATEMENT_TIMEOUT", "42s")

	if got := dbStatementTimeout(); got != 42*time.Second {
		t.Fatalf("expected timeout 42s, got %s", got)
	}
}

func TestDBStatementTimeoutInvalidFallsBackToDefault(t *testing.T) {
	t.Setenv("LABTETHER_DB_STATEMENT_TIMEOUT", "definitely-not-a-duration")

	if got := dbStatementTimeout(); got != defaultDBStatementTimeout {
		t.Fatalf("expected default timeout %s on invalid input, got %s", defaultDBStatementTimeout, got)
	}
}

func TestEnvIntInRangeOrDefaultRejectsOutOfRangeValue(t *testing.T) {
	t.Setenv("LABTETHER_TEST_INT_RANGE", "153722868")

	if got := envIntInRangeOrDefault("LABTETHER_TEST_INT_RANGE", 5, maxDBPoolDurationMinutes); got != 5 {
		t.Fatalf("envIntInRangeOrDefault() = %d, want fallback 5", got)
	}
}

func TestSchemaMigrationChecksumIsStable(t *testing.T) {
	m := schemaMigration{
		Version:    1,
		Name:       "test",
		Statements: []string{"CREATE TABLE foo (id TEXT PRIMARY KEY)", "CREATE INDEX idx_foo ON foo(id)"},
	}

	first := schemaMigrationChecksum(m)
	second := schemaMigrationChecksum(m)
	if first != second {
		t.Fatalf("checksum is not deterministic: %s vs %s", first, second)
	}
	if len(first) != 64 {
		t.Fatalf("expected 64-char hex SHA-256, got %d chars: %s", len(first), first)
	}
}

func TestSchemaMigrationChecksumDiffersOnStatementChange(t *testing.T) {
	base := schemaMigration{
		Version:    1,
		Name:       "test",
		Statements: []string{"CREATE TABLE foo (id TEXT PRIMARY KEY)"},
	}
	modified := schemaMigration{
		Version:    1,
		Name:       "test",
		Statements: []string{"CREATE TABLE foo (id TEXT PRIMARY KEY, extra TEXT)"},
	}

	if schemaMigrationChecksum(base) == schemaMigrationChecksum(modified) {
		t.Fatalf("expected different checksums for different statements")
	}
}

func TestSchemaMigrationChecksumDiffersOnStatementOrder(t *testing.T) {
	a := schemaMigration{
		Version:    1,
		Name:       "test",
		Statements: []string{"CREATE TABLE a (id TEXT)", "CREATE TABLE b (id TEXT)"},
	}
	b := schemaMigration{
		Version:    1,
		Name:       "test",
		Statements: []string{"CREATE TABLE b (id TEXT)", "CREATE TABLE a (id TEXT)"},
	}

	if schemaMigrationChecksum(a) == schemaMigrationChecksum(b) {
		t.Fatalf("expected different checksums for different statement order")
	}
}

func TestSchemaMigrationChecksumDiffersOnAdjacentStatements(t *testing.T) {
	// Ensure ["AB", "C"] and ["A", "BC"] produce different checksums (null-byte separator).
	ab := schemaMigration{Version: 1, Name: "t", Statements: []string{"AB", "C"}}
	a := schemaMigration{Version: 1, Name: "t", Statements: []string{"A", "BC"}}

	if schemaMigrationChecksum(ab) == schemaMigrationChecksum(a) {
		t.Fatalf("null-byte separator not working: adjacent statement concatenation collision")
	}
}

func TestSchemaMigrationChecksumIsHexString(t *testing.T) {
	m := schemaMigration{Version: 99, Name: "hex_check", Statements: []string{"SELECT 1"}}
	cs := schemaMigrationChecksum(m)
	for _, ch := range cs {
		if !strings.ContainsRune("0123456789abcdef", ch) {
			t.Fatalf("checksum contains non-hex character %q: %s", ch, cs)
		}
	}
}

func TestPostgresSchemaMigrationsChecksumsAreReproducible(t *testing.T) {
	// Ensure every migration in the real set produces the same checksum twice —
	// guards against any non-determinism in the migrations slice construction.
	migrations := postgresSchemaMigrations()
	for _, m := range migrations {
		if schemaMigrationChecksum(m) != schemaMigrationChecksum(m) {
			t.Fatalf("non-deterministic checksum for migration v%d (%s)", m.Version, m.Name)
		}
	}
}
