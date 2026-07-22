package agents

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/agentsettings"
	"github.com/labtether/labtether/internal/hubapi/testutil"
)

func TestAgentSettingsPatchEncryptsTURNPasswordAndRedactsViews(t *testing.T) {
	store := testutil.NewRuntimeSettingsStore()
	deps := &Deps{
		RuntimeStore:     store,
		SecretsManager:   testutil.TestSecretsManager(t),
		EnforceRateLimit: testutil.NoopRateLimit,
	}
	secret := "turn-password-super-secret"
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/agents/asset-1/settings", bytes.NewBufferString(`{"values":{"webrtc_turn_pass":"`+secret+`"}}`))
	rec := httptest.NewRecorder()
	deps.HandleAgentSettingsPatch(rec, req, "asset-1")
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status = %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), secret) {
		t.Fatal("settings response exposed TURN password")
	}

	stored, err := store.ListRuntimeSettingOverrides()
	if err != nil {
		t.Fatal(err)
	}
	storeKey := AgentSettingStoreKey("asset-1", agentsettings.SettingKeyWebRTCTURNPass)
	if stored[storeKey] == secret || strings.Contains(stored[storeKey], secret) || !strings.HasPrefix(stored[storeKey], encryptedAgentSettingPrefix) {
		t.Fatalf("TURN password was not encrypted at rest: %q", stored[storeKey])
	}

	deps.SetAgentSettingsRuntimeState("asset-1", AgentSettingsRuntimeState{Values: map[string]string{
		agentsettings.SettingKeyWebRTCTURNPass: secret,
	}})
	payload, err := deps.BuildAgentSettingsPayload("asset-1")
	if err != nil {
		t.Fatal(err)
	}
	if payload.State == nil {
		t.Fatal("expected state payload")
	}
	if _, exposed := payload.State.Values[agentsettings.SettingKeyWebRTCTURNPass]; exposed {
		t.Fatal("state values exposed TURN password")
	}
	found := false
	for _, entry := range payload.Settings {
		if entry.Key != agentsettings.SettingKeyWebRTCTURNPass {
			continue
		}
		found = true
		if !entry.Sensitive || !entry.Configured {
			t.Fatalf("sensitive presence metadata missing: %#v", entry)
		}
		if entry.GlobalValue != "" || entry.OverrideValue != "" || entry.StateValue != "" || entry.EffectiveValue != "" {
			t.Fatalf("sensitive values were not redacted: %#v", entry)
		}
	}
	if !found {
		t.Fatal("TURN password setting missing from payload")
	}
}

func TestCollectEffectiveAgentSettingsMigratesLegacyTURNPassword(t *testing.T) {
	store := testutil.NewRuntimeSettingsStore()
	storeKey := AgentSettingStoreKey("asset-legacy", agentsettings.SettingKeyWebRTCTURNPass)
	if _, err := store.SaveRuntimeSettingOverrides(map[string]string{storeKey: "legacy-turn-secret"}); err != nil {
		t.Fatal(err)
	}
	deps := &Deps{RuntimeStore: store, SecretsManager: testutil.TestSecretsManager(t)}
	values, err := deps.CollectEffectiveAgentSettingValues("asset-legacy")
	if err != nil {
		t.Fatal(err)
	}
	if values[agentsettings.SettingKeyWebRTCTURNPass] != "legacy-turn-secret" {
		t.Fatal("legacy secret was not available to the private agent apply path")
	}
	stored, err := store.ListRuntimeSettingOverrides()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored[storeKey], "legacy-turn-secret") || !strings.HasPrefix(stored[storeKey], encryptedAgentSettingPrefix) {
		t.Fatalf("legacy TURN password was not migrated: %q", stored[storeKey])
	}
}

func TestAgentSettingsPatchFailsClosedWithoutSecretManager(t *testing.T) {
	store := testutil.NewRuntimeSettingsStore()
	deps := &Deps{RuntimeStore: store, EnforceRateLimit: testutil.NoopRateLimit}
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/agents/asset-1/settings", bytes.NewBufferString(`{"values":{"webrtc_turn_pass":"must-not-store"}}`))
	rec := httptest.NewRecorder()
	deps.HandleAgentSettingsPatch(rec, req, "asset-1")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected fail-closed 500, got %d: %s", rec.Code, rec.Body.String())
	}
	stored, err := store.ListRuntimeSettingOverrides()
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 0 {
		t.Fatalf("secret was stored without encryption manager: %#v", stored)
	}
}
