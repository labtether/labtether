package alerting

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/notifications"
)

type notificationSecurityStore struct {
	mu       sync.Mutex
	channels map[string]notifications.Channel
	records  []notifications.Record
	casCount int
	casRace  map[string]any
}

func newNotificationSecurityStore() *notificationSecurityStore {
	return &notificationSecurityStore{channels: make(map[string]notifications.Channel)}
}

func (s *notificationSecurityStore) CreateNotificationChannel(req notifications.CreateChannelRequest) (notifications.Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	channel := notifications.Channel{
		ID:        req.ID,
		Name:      req.Name,
		Type:      req.Type,
		Config:    deepCloneNotificationMap(req.Config),
		Enabled:   enabled,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.channels[channel.ID] = channel
	return cloneNotificationSecurityChannel(channel), nil
}

func (s *notificationSecurityStore) GetNotificationChannel(id string) (notifications.Channel, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	channel, ok := s.channels[id]
	return cloneNotificationSecurityChannel(channel), ok, nil
}

func (s *notificationSecurityStore) ListNotificationChannels(_ int) ([]notifications.Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	channels := make([]notifications.Channel, 0, len(s.channels))
	for _, channel := range s.channels {
		channels = append(channels, cloneNotificationSecurityChannel(channel))
	}
	return channels, nil
}

func (s *notificationSecurityStore) UpdateNotificationChannel(id string, req notifications.UpdateChannelRequest) (notifications.Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	channel, ok := s.channels[id]
	if !ok {
		return notifications.Channel{}, notifications.ErrChannelNotFound
	}
	if req.Name != nil {
		channel.Name = *req.Name
	}
	if req.Config != nil {
		channel.Config = deepCloneNotificationMap(*req.Config)
	}
	if req.Enabled != nil {
		channel.Enabled = *req.Enabled
	}
	channel.UpdatedAt = time.Now().UTC()
	s.channels[id] = channel
	return cloneNotificationSecurityChannel(channel), nil
}

func (s *notificationSecurityStore) DeleteNotificationChannel(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.channels[id]; !ok {
		return notifications.ErrChannelNotFound
	}
	delete(s.channels, id)
	return nil
}

func (s *notificationSecurityStore) CompareAndSwapNotificationChannelConfig(id string, expected, replacement map[string]any) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	channel, ok := s.channels[id]
	if !ok {
		return false, nil
	}
	if s.casRace != nil {
		channel.Config = deepCloneNotificationMap(s.casRace)
		s.channels[id] = channel
		s.casRace = nil
		return false, nil
	}
	if !reflect.DeepEqual(channel.Config, expected) {
		return false, nil
	}
	channel.Config = deepCloneNotificationMap(replacement)
	channel.UpdatedAt = time.Now().UTC()
	s.channels[id] = channel
	s.casCount++
	return true, nil
}

func (s *notificationSecurityStore) CreateAlertRoute(notifications.CreateRouteRequest) (notifications.Route, error) {
	return notifications.Route{}, errors.New("unused notification route store method")
}

func (s *notificationSecurityStore) GetAlertRoute(string) (notifications.Route, bool, error) {
	return notifications.Route{}, false, errors.New("unused notification route store method")
}

func (s *notificationSecurityStore) ListAlertRoutes(int) ([]notifications.Route, error) {
	return nil, errors.New("unused notification route store method")
}

func (s *notificationSecurityStore) UpdateAlertRoute(string, notifications.UpdateRouteRequest) (notifications.Route, error) {
	return notifications.Route{}, errors.New("unused notification route store method")
}

func (s *notificationSecurityStore) DeleteAlertRoute(string) error {
	return errors.New("unused notification route store method")
}

func (s *notificationSecurityStore) CreateNotificationRecord(req notifications.CreateRecordRequest) (notifications.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record := notifications.Record{
		ID:              fmt.Sprintf("record-%d", len(s.records)+1),
		ChannelID:       req.ChannelID,
		AlertInstanceID: req.AlertInstanceID,
		RouteID:         req.RouteID,
		Payload:         deepCloneNotificationMap(req.Payload),
		Status:          req.Status,
		Error:           req.Error,
		MaxRetries:      notifications.DefaultMaxRetries,
		CreatedAt:       time.Now().UTC(),
	}
	s.records = append(s.records, record)
	return record, nil
}

func (s *notificationSecurityStore) ListNotificationHistory(_ int, channelID string) ([]notifications.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	records := make([]notifications.Record, 0, len(s.records))
	for _, record := range s.records {
		if channelID == "" || record.ChannelID == channelID {
			records = append(records, record)
		}
	}
	return records, nil
}

func (s *notificationSecurityStore) ListPendingRetries(context.Context, time.Time, int) ([]notifications.Record, error) {
	return nil, nil
}

func (s *notificationSecurityStore) UpdateRetryState(_ context.Context, id string, retryCount int, nextRetryAt *time.Time, status, errorMessage string, payload map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.records {
		if s.records[i].ID == id {
			s.records[i].RetryCount = retryCount
			s.records[i].NextRetryAt = nextRetryAt
			s.records[i].Status = status
			s.records[i].Error = errorMessage
			if payload != nil {
				s.records[i].Payload = deepCloneNotificationMap(payload)
			}
		}
	}
	return nil
}

func (s *notificationSecurityStore) seed(channel notifications.Channel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.channels[channel.ID] = cloneNotificationSecurityChannel(channel)
}

func (s *notificationSecurityStore) replaceConfig(id string, config map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	channel := s.channels[id]
	channel.Config = deepCloneNotificationMap(config)
	s.channels[id] = channel
}

func (s *notificationSecurityStore) migrations() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.casCount
}

func (s *notificationSecurityStore) forceCASRace(config map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.casRace = deepCloneNotificationMap(config)
}

func cloneNotificationSecurityChannel(channel notifications.Channel) notifications.Channel {
	channel.Config = deepCloneNotificationMap(channel.Config)
	return channel
}

type notificationSecretEchoAdapter struct{ typ string }

func (a *notificationSecretEchoAdapter) Type() string { return a.typ }

func (a *notificationSecretEchoAdapter) Send(_ context.Context, config map[string]any, _ map[string]any) error {
	token, _ := config["api_token"].(string)
	return fmt.Errorf("provider rejected endpoint %v authorization=%v encoded=%s", config["webhook_url"], token, base64.StdEncoding.EncodeToString([]byte(token)))
}

func TestNotificationChannelAPIEncryptsAtRestAndRedactsEveryResponse(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "false")
	store := newNotificationSecurityStore()
	deps := newTestAlertingDeps(t)
	deps.NotificationStore = store
	deps.NotificationSecrets = testutil.TestSecretsManager(t)

	secretURL := "https://hooks.example.invalid/services/test/secret-fragment"
	apiToken := "synthetic-notification-api-token"
	createBody := fmt.Sprintf(`{"name":"Operations","type":"slack","config":{"webhook_url":%q,"api_token":%q,"channel":"infra"}}`, secretURL, apiToken)
	createRec := runNotificationHandlerRequest(t, deps.HandleNotificationChannels, http.MethodPost, "/notifications/channels", createBody)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createRec.Code, http.StatusCreated)
	}
	assertNotificationResponseRedacted(t, createRec.Body.String(), secretURL, apiToken)

	var createdEnvelope struct {
		Channel notifications.Channel `json:"channel"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &createdEnvelope); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	channelID := createdEnvelope.Channel.ID
	if channelID == "" {
		t.Fatal("create response omitted channel id")
	}
	if _, ok := createdEnvelope.Channel.Config["webhook_url"]; ok {
		t.Fatal("create response retained a secret field")
	}

	stored, ok, err := store.GetNotificationChannel(channelID)
	if err != nil || !ok {
		t.Fatalf("load stored channel: ok=%t err=%v", ok, err)
	}
	assertNotificationConfigEncrypted(t, stored.Config, secretURL, apiToken)

	getRec := runNotificationHandlerRequest(t, deps.HandleNotificationChannelActions, http.MethodGet, "/notifications/channels/"+channelID, "")
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", getRec.Code, http.StatusOK)
	}
	assertNotificationResponseRedacted(t, getRec.Body.String(), secretURL, apiToken)

	listRec := runNotificationHandlerRequest(t, deps.HandleNotificationChannels, http.MethodGet, "/notifications/channels", "")
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listRec.Code, http.StatusOK)
	}
	assertNotificationResponseRedacted(t, listRec.Body.String(), secretURL, apiToken)
	var listEnvelope struct {
		Capabilities struct {
			SMTPInsecureTransportAllowed bool `json:"smtp_insecure_transport_allowed"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listEnvelope); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if listEnvelope.Capabilities.SMTPInsecureTransportAllowed {
		t.Fatal("notification API enabled insecure SMTP contrary to backend policy")
	}

	updateRec := runNotificationHandlerRequest(t, deps.HandleNotificationChannelActions, http.MethodPatch, "/notifications/channels/"+channelID, `{"config":{"channel":"platform"}}`)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d", updateRec.Code, http.StatusOK)
	}
	assertNotificationResponseRedacted(t, updateRec.Body.String(), secretURL, apiToken)

	placeholderRec := runNotificationHandlerRequest(t, deps.HandleNotificationChannelActions, http.MethodPatch, "/notifications/channels/"+channelID, `{"config":{"channel":"security","webhook_url":"[REDACTED]","api_token":"********"}}`)
	if placeholderRec.Code != http.StatusOK {
		t.Fatalf("placeholder update status = %d, want %d", placeholderRec.Code, http.StatusOK)
	}
	assertNotificationResponseRedacted(t, placeholderRec.Body.String(), secretURL, apiToken)

	runtime, ok, err := deps.getNotificationChannelForRuntime(channelID)
	if err != nil || !ok {
		t.Fatalf("load runtime channel: ok=%t err=%v", ok, err)
	}
	if runtime.Config["webhook_url"] != secretURL || runtime.Config["api_token"] != apiToken {
		t.Fatal("omitted or redacted update did not preserve existing secrets")
	}
	if runtime.Config["channel"] != "security" {
		t.Fatal("non-secret update was not applied")
	}
	stored, _, _ = store.GetNotificationChannel(channelID)
	assertNotificationConfigEncrypted(t, stored.Config, secretURL, apiToken)
}

func TestNotificationChannelAPICamelCaseSecretsAreEncryptedAndRedacted(t *testing.T) {
	store := newNotificationSecurityStore()
	deps := newTestAlertingDeps(t)
	deps.NotificationStore = store
	deps.NotificationSecrets = testutil.TestSecretsManager(t)

	secretValues := map[string]string{
		"webhookUrl":   "https://hooks.example.invalid/services/camel/private",
		"apiToken":     "camel-api-token-secret",
		"clientSecret": "camel-client-secret",
		"APIKey":       "camel-api-key-secret",
	}
	config := map[string]any{"nonSecretLabel": "operations"}
	for key, value := range secretValues {
		config[key] = value
	}
	body, err := json.Marshal(map[string]any{
		"name":   "Camel case credentials",
		"type":   notifications.ChannelTypeSlack,
		"config": config,
	})
	if err != nil {
		t.Fatalf("marshal create request: %v", err)
	}

	createRec := runNotificationHandlerRequest(t, deps.HandleNotificationChannels, http.MethodPost, "/notifications/channels", string(body))
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createRec.Code, http.StatusCreated)
	}
	plaintext := make([]string, 0, len(secretValues))
	for _, value := range secretValues {
		plaintext = append(plaintext, value)
	}
	assertNotificationResponseRedacted(t, createRec.Body.String(), plaintext...)

	var envelope struct {
		Channel notifications.Channel `json:"channel"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if envelope.Channel.Config["nonSecretLabel"] != "operations" {
		t.Fatal("create response removed non-secret camelCase config")
	}
	for key := range secretValues {
		if _, present := envelope.Channel.Config[key]; present {
			t.Fatalf("create response retained camelCase secret key %q", key)
		}
	}

	stored, ok, err := store.GetNotificationChannel(envelope.Channel.ID)
	if err != nil || !ok {
		t.Fatalf("load stored channel: ok=%t err=%v", ok, err)
	}
	assertNotificationConfigEncrypted(t, stored.Config, plaintext...)
	for key := range secretValues {
		ciphertext, ok := stored.Config[key].(string)
		if !ok || !strings.HasPrefix(ciphertext, "v2:") {
			t.Fatalf("stored camelCase secret %q was not encrypted", key)
		}
	}

	getRec := runNotificationHandlerRequest(t, deps.HandleNotificationChannelActions, http.MethodGet, "/notifications/channels/"+envelope.Channel.ID, "")
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", getRec.Code, http.StatusOK)
	}
	assertNotificationResponseRedacted(t, getRec.Body.String(), plaintext...)

	runtime, ok, err := deps.getNotificationChannelForRuntime(envelope.Channel.ID)
	if err != nil || !ok {
		t.Fatalf("load runtime channel: ok=%t err=%v", ok, err)
	}
	for key, value := range secretValues {
		if runtime.Config[key] != value {
			t.Fatalf("runtime camelCase secret %q did not decrypt", key)
		}
	}
}

func TestAPNsAuthKeyPathIsEncryptedAndNeverReturnedByChannelAPI(t *testing.T) {
	store := newNotificationSecurityStore()
	deps := newTestAlertingDeps(t)
	deps.NotificationStore = store
	deps.NotificationSecrets = testutil.TestSecretsManager(t)

	secretPath := "/private/labtether/apns/AuthKey_TEST123.p8"
	body, err := json.Marshal(map[string]any{
		"name": "LabTether Mobile",
		"type": notifications.ChannelTypeAPNs,
		"config": map[string]any{
			"auth_key_path": secretPath,
			"key_id":        "KEYID00001",
			"team_id":       "TEAMID0001",
			"bundle_id":     "com.labtether.mobile",
			"production":    false,
		},
	})
	if err != nil {
		t.Fatalf("marshal create request: %v", err)
	}

	created := runNotificationHandlerRequest(t, deps.HandleNotificationChannels, http.MethodPost, "/notifications/channels", string(body))
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d: %s", created.Code, http.StatusCreated, created.Body.String())
	}
	assertNotificationResponseRedacted(t, created.Body.String(), secretPath)

	var envelope struct {
		Channel notifications.Channel `json:"channel"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if _, present := envelope.Channel.Config["auth_key_path"]; present {
		t.Fatal("create response exposed APNs auth_key_path")
	}

	stored, ok, err := store.GetNotificationChannel(envelope.Channel.ID)
	if err != nil || !ok {
		t.Fatalf("load stored APNs channel: ok=%t err=%v", ok, err)
	}
	ciphertext, ok := stored.Config["auth_key_path"].(string)
	if !ok || !strings.HasPrefix(ciphertext, "v2:") || strings.Contains(ciphertext, secretPath) {
		t.Fatalf("APNs auth key path was not encrypted at rest: %q", ciphertext)
	}

	listed := runNotificationHandlerRequest(t, deps.HandleNotificationChannels, http.MethodGet, "/notifications/channels", "")
	if listed.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", listed.Code, listed.Body.String())
	}
	assertNotificationResponseRedacted(t, listed.Body.String(), secretPath)

	runtime, ok, err := deps.getNotificationChannelForRuntime(envelope.Channel.ID)
	if err != nil || !ok {
		t.Fatalf("load runtime APNs channel: ok=%t err=%v", ok, err)
	}
	if runtime.Config["auth_key_path"] != secretPath {
		t.Fatal("runtime APNs channel did not decrypt auth_key_path")
	}
}

func TestNotificationChannelListPublishesInsecureSMTPCapabilityOnlyWhenPolicyAllows(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	deps := newTestAlertingDeps(t)
	deps.NotificationStore = newNotificationSecurityStore()

	recorder := runNotificationHandlerRequest(t, deps.HandleNotificationChannels, http.MethodGet, "/notifications/channels", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var envelope struct {
		Capabilities struct {
			SMTPInsecureTransportAllowed bool `json:"smtp_insecure_transport_allowed"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if !envelope.Capabilities.SMTPInsecureTransportAllowed {
		t.Fatal("notification API omitted backend-approved insecure SMTP capability")
	}
}

func TestNotificationChannelCreateFailsClosedWithoutSecretManager(t *testing.T) {
	store := newNotificationSecurityStore()
	deps := newTestAlertingDeps(t)
	deps.NotificationStore = store

	recorder := runNotificationHandlerRequest(
		t,
		deps.HandleNotificationChannels,
		http.MethodPost,
		"/notifications/channels",
		`{"name":"No key","type":"slack","config":{"webhook_url":"https://hooks.example.invalid/private"}}`,
	)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("create without secret manager status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	channels, err := store.ListNotificationChannels(10)
	if err != nil {
		t.Fatalf("list channels after rejected create: %v", err)
	}
	if len(channels) != 0 {
		t.Fatal("create without secret manager wrote a notification channel")
	}
}

func TestLegacyNotificationConfigMigratesAndCiphertextIsBoundToChannel(t *testing.T) {
	store := newNotificationSecurityStore()
	deps := newTestAlertingDeps(t)
	deps.NotificationStore = store
	deps.NotificationSecrets = testutil.TestSecretsManager(t)

	legacyURL := "https://legacy.example.invalid/hooks/private-segment"
	legacyHeader := "Bearer synthetic-legacy-value"
	store.seed(notifications.Channel{
		ID:      "legacy-channel",
		Name:    "Legacy",
		Type:    notifications.ChannelTypeWebhook,
		Enabled: true,
		Config: map[string]any{
			"url":     legacyURL,
			"headers": map[string]any{"Authorization": legacyHeader},
			"timeout": 12,
		},
	})

	runtime, ok, err := deps.getNotificationChannelForRuntime("legacy-channel")
	if err != nil || !ok {
		t.Fatalf("load legacy channel: ok=%t err=%v", ok, err)
	}
	if runtime.Config["url"] != legacyURL {
		t.Fatal("legacy secret was not available to the runtime during migration")
	}
	if store.migrations() != 1 {
		t.Fatalf("migration count = %d, want 1", store.migrations())
	}
	migrated, _, _ := store.GetNotificationChannel("legacy-channel")
	assertNotificationConfigEncrypted(t, migrated.Config, legacyURL, legacyHeader)

	first, err := deps.createSecureNotificationChannel(notifications.CreateChannelRequest{
		Name:   "First",
		Type:   notifications.ChannelTypeSlack,
		Config: map[string]any{"webhook_url": "https://hooks.example.invalid/first/private"},
	})
	if err != nil {
		t.Fatalf("create first channel: %v", err)
	}
	second, err := deps.createSecureNotificationChannel(notifications.CreateChannelRequest{
		Name:   "Second",
		Type:   notifications.ChannelTypeSlack,
		Config: map[string]any{"webhook_url": "https://hooks.example.invalid/second/private"},
	})
	if err != nil {
		t.Fatalf("create second channel: %v", err)
	}
	firstStored, _, _ := store.GetNotificationChannel(first.ID)
	transplanted := firstStored.Config["webhook_url"]
	store.replaceConfig(second.ID, map[string]any{"webhook_url": transplanted})
	if _, _, err := deps.getNotificationChannelForRuntime(second.ID); err == nil {
		t.Fatal("ciphertext transplanted between notification channels unexpectedly decrypted")
	}
}

func TestLegacyMigrationCASRaceReloadsCurrentChannelConfig(t *testing.T) {
	store := newNotificationSecurityStore()
	deps := newTestAlertingDeps(t)
	deps.NotificationStore = store
	deps.NotificationSecrets = testutil.TestSecretsManager(t)

	store.seed(notifications.Channel{
		ID:      "racing-channel",
		Name:    "Racing",
		Type:    notifications.ChannelTypeSlack,
		Enabled: true,
		Config:  map[string]any{"webhook_url": "https://hooks.example.invalid/stale/private"},
	})
	currentURL := "https://hooks.example.invalid/current/private"
	currentConfig, err := deps.encryptNotificationConfig("racing-channel", notifications.ChannelTypeSlack, map[string]any{"webhook_url": currentURL})
	if err != nil {
		t.Fatalf("encrypt concurrent config: %v", err)
	}
	store.forceCASRace(currentConfig)

	runtime, ok, err := deps.getNotificationChannelForRuntime("racing-channel")
	if err != nil || !ok {
		t.Fatalf("load channel after migration race: ok=%t err=%v", ok, err)
	}
	if runtime.Config["webhook_url"] != currentURL {
		t.Fatal("migration race dispatched stale notification configuration")
	}
}

func TestNotificationAdapterErrorsAreRedactedBeforeAPIAndPersistence(t *testing.T) {
	store := newNotificationSecurityStore()
	deps := newTestAlertingDeps(t)
	deps.NotificationStore = store
	deps.NotificationSecrets = testutil.TestSecretsManager(t)
	deps.NotificationAdapters[notifications.ChannelTypeSlack] = &notificationSecretEchoAdapter{typ: notifications.ChannelTypeSlack}

	secretURL := "https://hooks.example.invalid/services/private/error-path"
	apiToken := "synthetic-error-api-token"
	created, err := deps.createSecureNotificationChannel(notifications.CreateChannelRequest{
		Name:   "Error channel",
		Type:   notifications.ChannelTypeSlack,
		Config: map[string]any{"webhook_url": secretURL, "api_token": apiToken},
	})
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}

	testRec := runNotificationHandlerRequest(t, deps.HandleNotificationChannelTest, http.MethodPost, "/notifications/channels/"+created.ID+"/test", "")
	if testRec.Code != http.StatusOK {
		t.Fatalf("test status = %d, want %d", testRec.Code, http.StatusOK)
	}
	assertNotificationResponseRedacted(t, testRec.Body.String(), secretURL, apiToken)

	runtime, _, err := deps.getNotificationChannelForRuntime(created.ID)
	if err != nil {
		t.Fatalf("load runtime channel: %v", err)
	}
	sendErr := deps.sendNotification(context.Background(), runtime, map[string]any{"text": "test"})
	if sendErr == nil {
		t.Fatal("expected adversarial adapter error")
	}
	safeError := sanitizeNotificationDeliveryError(runtime, sendErr)
	deps.recordNotificationHistoryWithRetry(runtime.ID, "instance", "route", notifications.RecordStatusFailed, safeError, nil)
	records, err := store.ListNotificationHistory(10, runtime.ID)
	if err != nil || len(records) != 1 {
		t.Fatalf("stored history count = %d, err=%v", len(records), err)
	}
	assertNotificationResponseRedacted(t, records[0].Error, secretURL, apiToken)

	if _, err := store.CreateNotificationRecord(notifications.CreateRecordRequest{
		ChannelID: runtime.ID,
		Status:    notifications.RecordStatusFailed,
		Error:     "legacy provider echoed " + apiToken,
	}); err != nil {
		t.Fatalf("seed legacy notification history: %v", err)
	}
	historyRec := runNotificationHandlerRequest(t, deps.HandleNotificationHistory, http.MethodGet, "/notifications/history", "")
	if historyRec.Code != http.StatusOK {
		t.Fatalf("history status = %d, want %d", historyRec.Code, http.StatusOK)
	}
	assertNotificationResponseRedacted(t, historyRec.Body.String(), secretURL, apiToken)
}

func runNotificationHandlerRequest(t *testing.T, handler http.HandlerFunc, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	handler(recorder, request)
	return recorder
}

func assertNotificationResponseRedacted(t *testing.T, response string, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if secret != "" && strings.Contains(response, secret) {
			t.Fatal("notification response or persisted error disclosed secret material")
		}
		if encoded := base64.StdEncoding.EncodeToString([]byte(secret)); secret != "" && strings.Contains(response, encoded) {
			t.Fatal("notification response or persisted error disclosed encoded secret material")
		}
	}
	if strings.Contains(response, "v2:") {
		t.Fatal("notification response or persisted error disclosed encrypted secret material")
	}
}

func assertNotificationConfigEncrypted(t *testing.T, config map[string]any, plaintext ...string) {
	t.Helper()
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal stored notification config: %v", err)
	}
	for _, value := range plaintext {
		if value != "" && strings.Contains(string(encoded), value) {
			t.Fatal("stored notification config retained plaintext secret material")
		}
	}
	if !strings.Contains(string(encoded), "v2:") {
		t.Fatal("stored notification config did not contain encrypted secret material")
	}
}
