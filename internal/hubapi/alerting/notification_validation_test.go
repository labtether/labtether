package alerting

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/notifications"
)

func TestNotificationChannelAPIRejectsInvalidCreateWithoutPersistence(t *testing.T) {
	store := newNotificationSecurityStore()
	deps := newTestAlertingDeps(t)
	deps.NotificationStore = store
	deps.NotificationSecrets = testutil.TestSecretsManager(t)

	response := runNotificationHandlerRequest(t, deps.HandleNotificationChannels, http.MethodPost, "/notifications/channels", `{
		"name":"Broken webhook",
		"type":"webhook",
		"config":{"headers":{"X-Api-Key":"secret"}}
	}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid create status = %d, want %d: %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
	channels, err := store.ListNotificationChannels(10)
	if err != nil {
		t.Fatalf("list channels: %v", err)
	}
	if len(channels) != 0 {
		t.Fatalf("invalid create persisted %d channels", len(channels))
	}
}

func TestNotificationChannelAPIRejectsInvalidMergedUpdateAndPreservesChannel(t *testing.T) {
	store := newNotificationSecurityStore()
	deps := newTestAlertingDeps(t)
	deps.NotificationStore = store
	deps.NotificationSecrets = testutil.TestSecretsManager(t)

	created := runNotificationHandlerRequest(t, deps.HandleNotificationChannels, http.MethodPost, "/notifications/channels", `{
		"name":"Operations webhook",
		"type":"webhook",
		"config":{"url":"https://hooks.example.invalid/labtether","headers":{"X-Api-Key":"secret"}}
	}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", created.Code, created.Body.String())
	}
	var envelope struct {
		Channel notifications.Channel `json:"channel"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	updated := runNotificationHandlerRequest(t, deps.HandleNotificationChannelActions, http.MethodPatch, "/notifications/channels/"+envelope.Channel.ID, `{
		"name":"   ",
		"config":{"url":""}
	}`)
	if updated.Code != http.StatusBadRequest {
		t.Fatalf("invalid update status = %d, want %d: %s", updated.Code, http.StatusBadRequest, updated.Body.String())
	}

	runtime, ok, err := deps.getNotificationChannelForRuntime(envelope.Channel.ID)
	if err != nil || !ok {
		t.Fatalf("load channel after invalid update: ok=%t err=%v", ok, err)
	}
	if runtime.Name != "Operations webhook" || runtime.Config["url"] != "https://hooks.example.invalid/labtether" {
		t.Fatalf("invalid update changed stored channel: name=%q config=%v", runtime.Name, runtime.Config)
	}
}

func TestValidateNotificationChannelConfigRejectsMalformedPerTypeConfig(t *testing.T) {
	tests := []struct {
		name        string
		channelType string
		config      map[string]any
	}{
		{name: "webhook credentials in URL", channelType: "webhook", config: map[string]any{"url": "https://user:pass@example.invalid/hook"}},
		{name: "webhook reserved header", channelType: "webhook", config: map[string]any{"url": "https://example.invalid/hook", "headers": map[string]any{"Host": "attacker.invalid"}}},
		{name: "slack missing endpoint", channelType: "slack", config: map[string]any{}},
		{name: "email mismatched credentials", channelType: "email", config: map[string]any{"smtp_host": "smtp.example.invalid", "from": "from@example.invalid", "to": "to@example.invalid", "smtp_user": "user"}},
		{name: "apns relative key path", channelType: "apns", config: map[string]any{"auth_key_path": "AuthKey.p8", "key_id": "KEYID00001", "team_id": "TEAMID0001", "bundle_id": "com.labtether.mobile"}},
		{name: "ntfy topic traversal", channelType: "ntfy", config: map[string]any{"server_url": "https://ntfy.example.invalid", "topic": "ops/../../admin"}},
		{name: "ntfy conflicting auth", channelType: "ntfy", config: map[string]any{"server_url": "https://ntfy.example.invalid", "topic": "ops", "token": "token", "username": "user", "password": "pass"}},
		{name: "gotify query in base URL", channelType: "gotify", config: map[string]any{"server_url": "https://gotify.example.invalid?token=leak", "app_token": "secret"}},
		{name: "gotify missing token", channelType: "gotify", config: map[string]any{"server_url": "https://gotify.example.invalid"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateNotificationChannelConfig(test.channelType, test.config); err == nil {
				t.Fatalf("malformed %s config unexpectedly accepted: %v", test.channelType, test.config)
			}
		})
	}
}

func TestValidateNotificationChannelConfigAcceptsEverySupportedType(t *testing.T) {
	tests := []struct {
		channelType string
		config      map[string]any
	}{
		{channelType: "webhook", config: map[string]any{"url": "https://hooks.example.invalid/labtether", "headers": map[string]any{"X-Api-Key": "secret"}}},
		{channelType: "slack", config: map[string]any{"webhook_url": "https://hooks.slack.invalid/services/test"}},
		{channelType: "email", config: map[string]any{"smtp_host": "smtp.example.invalid", "smtp_port": 587, "smtp_tls_mode": "starttls", "from": "LabTether <alerts@example.invalid>", "to": "ops@example.invalid, oncall@example.invalid"}},
		{channelType: "apns", config: map[string]any{"auth_key_path": "/run/secrets/AuthKey.p8", "key_id": "KEYID00001", "team_id": "TEAMID0001", "bundle_id": "com.labtether.mobile", "production": true}},
		{channelType: "ntfy", config: map[string]any{"server_url": "https://ntfy.example.invalid", "topic": "operations", "token": "secret", "priority": 4}},
		{channelType: "gotify", config: map[string]any{"server_url": "https://gotify.example.invalid", "app_token": "secret", "priority": "5"}},
	}
	for _, test := range tests {
		t.Run(test.channelType, func(t *testing.T) {
			if err := ValidateNotificationChannelConfig(test.channelType, test.config); err != nil {
				t.Fatalf("valid %s config rejected: %v", test.channelType, err)
			}
		})
	}
}

func TestValidateNotificationChannelConfigEnforcesStructuralBounds(t *testing.T) {
	deep := map[string]any{"url": "https://example.invalid/hook"}
	cursor := deep
	for index := 0; index <= maxNotificationConfigDepth; index++ {
		nested := map[string]any{}
		cursor["nested"] = nested
		cursor = nested
	}
	if err := ValidateNotificationChannelConfig("webhook", deep); err == nil || !strings.Contains(err.Error(), "nesting") {
		t.Fatalf("deep config was not rejected: %v", err)
	}

	oversized := map[string]any{
		"url":   "https://example.invalid/hook",
		"value": strings.Repeat("x", maxNotificationConfigStringLen+1),
	}
	if err := ValidateNotificationChannelConfig("webhook", oversized); err == nil {
		t.Fatal("oversized config string was not rejected")
	}
}
