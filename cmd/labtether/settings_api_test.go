package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/installstate"
)

func TestRetentionSettingsGetAndUpdate(t *testing.T) {
	sut := newTestAPIServer(t)

	getReq := httptest.NewRequest(http.MethodGet, "/settings/retention", nil)
	getRec := httptest.NewRecorder()
	sut.handleRetentionSettings(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getRec.Code)
	}

	updatePayload := []byte(`{"preset":"compact"}`)
	updateReq := httptest.NewRequest(http.MethodPost, "/settings/retention", bytes.NewReader(updatePayload))
	updateRec := httptest.NewRecorder()
	sut.handleRetentionSettings(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", updateRec.Code)
	}
}

func TestRuntimeSettingsGetUpdateAndReset(t *testing.T) {
	t.Setenv("LABTETHER_POLL_INTERVAL_SECONDS", "9")
	sut := newTestAPIServer(t)

	getReq := httptest.NewRequest(http.MethodGet, "/settings/runtime", nil)
	getRec := httptest.NewRecorder()
	sut.handleRuntimeSettings(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getRec.Code)
	}

	var getPayload runtimeSettingsPayload
	if err := json.Unmarshal(getRec.Body.Bytes(), &getPayload); err != nil {
		t.Fatalf("failed to decode runtime settings payload: %v", err)
	}

	var pollEntry runtimeSettingEntry
	foundPoll := false
	var discoveryEntry runtimeSettingEntry
	foundDiscovery := false
	var dockerDiscoveryEntry runtimeSettingEntry
	foundDockerDiscovery := false
	var outboundEntry runtimeSettingEntry
	foundOutbound := false
	var remoteAccessEntry runtimeSettingEntry
	foundRemoteAccess := false
	var tailscaleTargetEntry runtimeSettingEntry
	foundTailscaleTarget := false
	for _, entry := range getPayload.Settings {
		if entry.Key == "console.poll_interval_seconds" {
			pollEntry = entry
			foundPoll = true
		}
		if entry.Key == "services.discovery_default_proxy_enabled" {
			discoveryEntry = entry
			foundDiscovery = true
		}
		if entry.Key == "services.discovery_default_docker_enabled" {
			dockerDiscoveryEntry = entry
			foundDockerDiscovery = true
		}
		if entry.Key == "security.outbound_allow_private" {
			outboundEntry = entry
			foundOutbound = true
		}
		if entry.Key == "remote_access.mode" {
			remoteAccessEntry = entry
			foundRemoteAccess = true
		}
		if entry.Key == "remote_access.tailscale_serve_target" {
			tailscaleTargetEntry = entry
			foundTailscaleTarget = true
		}
	}
	if !foundPoll {
		t.Fatalf("expected poll interval setting entry")
	}
	if !foundDiscovery {
		t.Fatalf("expected services discovery default proxy setting entry")
	}
	if !foundDockerDiscovery {
		t.Fatalf("expected services discovery default docker setting entry")
	}
	if !foundOutbound {
		t.Fatalf("expected outbound private-target setting entry")
	}
	if !foundRemoteAccess {
		t.Fatalf("expected remote access mode setting entry")
	}
	if !foundTailscaleTarget {
		t.Fatalf("expected tailscale target setting entry")
	}
	if pollEntry.EffectiveValue != "9" || pollEntry.Source != "docker" {
		t.Fatalf("expected env-derived poll interval (9/docker), got (%s/%s)", pollEntry.EffectiveValue, pollEntry.Source)
	}
	if discoveryEntry.EffectiveValue != "true" || discoveryEntry.Source != "default" {
		t.Fatalf("expected default discovery proxy setting (true/default), got (%s/%s)", discoveryEntry.EffectiveValue, discoveryEntry.Source)
	}
	if dockerDiscoveryEntry.EffectiveValue != "true" || dockerDiscoveryEntry.Source != "default" {
		t.Fatalf("expected default docker discovery setting (true/default), got (%s/%s)", dockerDiscoveryEntry.EffectiveValue, dockerDiscoveryEntry.Source)
	}
	if outboundEntry.EffectiveValue != "auto" || outboundEntry.Source != "default" {
		t.Fatalf("expected default outbound private setting (auto/default), got (%s/%s)", outboundEntry.EffectiveValue, outboundEntry.Source)
	}
	if remoteAccessEntry.EffectiveValue != "serve" || remoteAccessEntry.Source != "default" {
		t.Fatalf("expected default remote access mode (serve/default), got (%s/%s)", remoteAccessEntry.EffectiveValue, remoteAccessEntry.Source)
	}
	if tailscaleTargetEntry.EffectiveValue != "" || tailscaleTargetEntry.Source != "default" {
		t.Fatalf("expected default tailscale target (empty/default), got (%q/%s)", tailscaleTargetEntry.EffectiveValue, tailscaleTargetEntry.Source)
	}

	// Verify that old merge-related settings are no longer in the runtime settings list.
	for _, entry := range getPayload.Settings {
		if entry.Key == "services.merge_mode" || entry.Key == "services.merge_confidence_threshold" ||
			entry.Key == "services.merge_dry_run" || entry.Key == "services.merge_alias_rules" ||
			entry.Key == "services.force_merge_rules" || entry.Key == "services.never_merge_rules" {
			t.Fatalf("unexpected legacy merge setting %q still in runtime settings list", entry.Key)
		}
	}

	updateReq := httptest.NewRequest(
		http.MethodPatch,
		"/settings/runtime",
		bytes.NewReader([]byte(`{"values":{"console.poll_interval_seconds":"12","services.discovery_default_docker_enabled":"false","security.outbound_allow_private":"false","remote_access.mode":"manual","remote_access.tailscale_serve_target":"https://127.0.0.1:9443"}}`)),
	)
	sut.ensureCollectorsDeps().WebServiceURLGroupingCfgAt = time.Now().UTC()
	updateRec := httptest.NewRecorder()
	sut.handleRuntimeSettings(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", updateRec.Code)
	}
	if !sut.ensureCollectorsDeps().WebServiceURLGroupingCfgAt.IsZero() {
		t.Fatalf("expected runtime update to invalidate web service url grouping cache timestamp")
	}

	var updatePayload runtimeSettingsPayload
	if err := json.Unmarshal(updateRec.Body.Bytes(), &updatePayload); err != nil {
		t.Fatalf("failed to decode updated runtime settings payload: %v", err)
	}
	if updatePayload.Overrides["console.poll_interval_seconds"] != "12" {
		t.Fatalf("expected override value 12, got %q", updatePayload.Overrides["console.poll_interval_seconds"])
	}
	if updatePayload.Overrides["services.discovery_default_docker_enabled"] != "false" {
		t.Fatalf("expected docker discovery override false, got %q", updatePayload.Overrides["services.discovery_default_docker_enabled"])
	}
	if updatePayload.Overrides["security.outbound_allow_private"] != "false" {
		t.Fatalf("expected outbound private override false, got %q", updatePayload.Overrides["security.outbound_allow_private"])
	}
	if updatePayload.Overrides["remote_access.mode"] != "manual" {
		t.Fatalf("expected remote access mode override manual, got %q", updatePayload.Overrides["remote_access.mode"])
	}
	if updatePayload.Overrides["remote_access.tailscale_serve_target"] != "https://127.0.0.1:9443" {
		t.Fatalf("expected tailscale target override, got %q", updatePayload.Overrides["remote_access.tailscale_serve_target"])
	}

	resetReq := httptest.NewRequest(
		http.MethodPost,
		"/settings/runtime/reset",
		bytes.NewReader([]byte(`{"keys":["console.poll_interval_seconds","services.discovery_default_docker_enabled","security.outbound_allow_private","remote_access.mode","remote_access.tailscale_serve_target"]}`)),
	)
	sut.ensureCollectorsDeps().WebServiceURLGroupingCfgAt = time.Now().UTC()
	resetRec := httptest.NewRecorder()
	sut.handleRuntimeSettingsReset(resetRec, resetReq)
	if resetRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resetRec.Code)
	}
	if !sut.ensureCollectorsDeps().WebServiceURLGroupingCfgAt.IsZero() {
		t.Fatalf("expected runtime reset to invalidate web service url grouping cache timestamp")
	}

	var resetPayload runtimeSettingsPayload
	if err := json.Unmarshal(resetRec.Body.Bytes(), &resetPayload); err != nil {
		t.Fatalf("failed to decode reset runtime settings payload: %v", err)
	}
	if _, ok := resetPayload.Overrides["console.poll_interval_seconds"]; ok {
		t.Fatalf("expected override key to be removed")
	}
	if _, ok := resetPayload.Overrides["services.discovery_default_docker_enabled"]; ok {
		t.Fatalf("expected docker discovery override key to be removed")
	}
	if _, ok := resetPayload.Overrides["security.outbound_allow_private"]; ok {
		t.Fatalf("expected outbound private override key to be removed")
	}
	if _, ok := resetPayload.Overrides["remote_access.mode"]; ok {
		t.Fatalf("expected remote access mode override key to be removed")
	}
	if _, ok := resetPayload.Overrides["remote_access.tailscale_serve_target"]; ok {
		t.Fatalf("expected tailscale target override key to be removed")
	}

	auditEvents, err := sut.auditStore.List(20, 0)
	if err != nil {
		t.Fatalf("failed to list audit events: %v", err)
	}
	foundUpdated := false
	foundReset := false
	for _, event := range auditEvents {
		if event.Type == "settings.runtime.updated" && event.ActorID == "system" {
			foundUpdated = true
		}
		if event.Type == "settings.runtime.reset" && event.ActorID == "system" {
			foundReset = true
		}
	}
	if !foundUpdated {
		t.Fatalf("expected settings.runtime.updated audit event")
	}
	if !foundReset {
		t.Fatalf("expected settings.runtime.reset audit event")
	}
}

func TestRuntimeSettingsRejectUnknownKey(t *testing.T) {
	sut := newTestAPIServer(t)

	updateReq := httptest.NewRequest(http.MethodPatch, "/settings/runtime", bytes.NewReader([]byte(`{"values":{"unknown.key":"123"}}`)))
	updateRec := httptest.NewRecorder()
	sut.handleRuntimeSettings(updateRec, updateReq)
	if updateRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", updateRec.Code)
	}
}

func TestRuntimeSettingsUpdateAppliesPolicyStateImmediately(t *testing.T) {
	sut := newTestAPIServer(t)

	if !sut.policyState.Current().InteractiveEnabled {
		t.Fatalf("expected interactive policy enabled by default")
	}

	updateReq := httptest.NewRequest(
		http.MethodPatch,
		"/settings/runtime",
		bytes.NewReader([]byte(`{"values":{"policy.interactive_enabled":"false"}}`)),
	)
	updateRec := httptest.NewRecorder()
	sut.handleRuntimeSettings(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", updateRec.Code)
	}
	if sut.policyState.Current().InteractiveEnabled {
		t.Fatalf("expected interactive policy override to apply immediately")
	}

	resetReq := httptest.NewRequest(
		http.MethodPost,
		"/settings/runtime/reset",
		bytes.NewReader([]byte(`{"keys":["policy.interactive_enabled"]}`)),
	)
	resetRec := httptest.NewRecorder()
	sut.handleRuntimeSettingsReset(resetRec, resetReq)
	if resetRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resetRec.Code)
	}
	if !sut.policyState.Current().InteractiveEnabled {
		t.Fatalf("expected interactive policy reset to restore enabled default immediately")
	}
}

func TestManagedDatabaseSettingsAndReveal(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://labtether:secret@postgres:5432/labtether?sslmode=disable")
	t.Setenv("LABTETHER_MANAGED_POSTGRES", "true")
	sut := newTestAPIServer(t)
	if err := sut.installStateStore.Save(installstate.Metadata{}, installstate.Secrets{
		PostgresPassword: "generated-db-password",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/settings/managed-database", nil)
	getRec := httptest.NewRecorder()
	sut.handleManagedDatabaseSettings(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getRec.Code)
	}

	var getPayload managedDatabaseSettingsPayload
	if err := json.Unmarshal(getRec.Body.Bytes(), &getPayload); err != nil {
		t.Fatalf("failed to decode managed database payload: %v", err)
	}
	if !getPayload.Managed || !getPayload.PasswordAvailable {
		t.Fatalf("expected managed database password to be available, got %+v", getPayload)
	}
	if getPayload.Username != "labtether" || getPayload.Database != "labtether" || getPayload.Host != "postgres" {
		t.Fatalf("unexpected database connection info: %+v", getPayload)
	}
	if strings.Contains(getPayload.PasswordHint, "generated-db-password") {
		t.Fatalf("password hint should be masked, got %q", getPayload.PasswordHint)
	}

	revealReq := httptest.NewRequest(http.MethodPost, "/settings/managed-database/reveal", nil)
	revealReq = revealReq.WithContext(contextWithPrincipal(revealReq.Context(), "owner", "owner"))
	revealRec := httptest.NewRecorder()
	sut.handleManagedDatabasePasswordReveal(revealRec, revealReq)
	if revealRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", revealRec.Code)
	}

	var revealPayload managedDatabaseRevealPayload
	if err := json.Unmarshal(revealRec.Body.Bytes(), &revealPayload); err != nil {
		t.Fatalf("failed to decode managed database reveal payload: %v", err)
	}
	if revealPayload.Password != "generated-db-password" {
		t.Fatalf("Password = %q, want generated-db-password", revealPayload.Password)
	}

	auditEvents, err := sut.auditStore.List(20, 0)
	if err != nil {
		t.Fatalf("failed to list audit events: %v", err)
	}
	foundReveal := false
	for _, event := range auditEvents {
		if event.Type == managedDatabaseRevealAuditType && event.ActorID == "owner" {
			foundReveal = true
			break
		}
	}
	if !foundReveal {
		t.Fatalf("expected managed database password reveal audit event")
	}
}

func TestManagedDatabaseSettingsHidePasswordForExternalPostgres(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://labtether:external-secret@db.example:5432/labtether?sslmode=disable")
	sut := newTestAPIServer(t)
	if err := sut.installStateStore.Save(installstate.Metadata{}, installstate.Secrets{
		PostgresPassword: "persisted-but-not-managed",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/settings/managed-database", nil)
	getRec := httptest.NewRecorder()
	sut.handleManagedDatabaseSettings(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getRec.Code)
	}

	var getPayload managedDatabaseSettingsPayload
	if err := json.Unmarshal(getRec.Body.Bytes(), &getPayload); err != nil {
		t.Fatalf("failed to decode managed database payload: %v", err)
	}
	if getPayload.Managed || getPayload.PasswordAvailable {
		t.Fatalf("expected external postgres to stay hidden, got %+v", getPayload)
	}

	revealReq := httptest.NewRequest(http.MethodPost, "/settings/managed-database/reveal", nil)
	revealReq = revealReq.WithContext(contextWithPrincipal(revealReq.Context(), "owner", "owner"))
	revealRec := httptest.NewRecorder()
	sut.handleManagedDatabasePasswordReveal(revealRec, revealReq)
	if revealRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when postgres is not marked managed, got %d", revealRec.Code)
	}
}

func TestRuntimeSettingsResetRejectsUnknownKey(t *testing.T) {
	sut := newTestAPIServer(t)

	resetReq := httptest.NewRequest(http.MethodPost, "/settings/runtime/reset", bytes.NewReader([]byte(`{"keys":["unknown.key"]}`)))
	resetRec := httptest.NewRecorder()
	sut.handleRuntimeSettingsReset(resetRec, resetReq)
	if resetRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resetRec.Code)
	}
}

func TestRetentionSettingsReturnServiceUnavailableWithoutRetentionStore(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.retentionStore = nil

	getReq := httptest.NewRequest(http.MethodGet, "/settings/retention", nil)
	getRec := httptest.NewRecorder()
	sut.handleRetentionSettings(getRec, getReq)
	if getRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", getRec.Code)
	}
	if !strings.Contains(getRec.Body.String(), "An internal error occurred.") {
		t.Fatalf("expected sanitized error message, got %s", getRec.Body.String())
	}

	updateReq := httptest.NewRequest(http.MethodPost, "/settings/retention", bytes.NewReader([]byte(`{"preset":"compact"}`)))
	updateRec := httptest.NewRecorder()
	sut.handleRetentionSettings(updateRec, updateReq)
	if updateRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", updateRec.Code)
	}
}
