package alerting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/notifications"
)

type notificationAuditAdapter struct {
	err error
}

func (a *notificationAuditAdapter) Type() string { return notifications.ChannelTypeSlack }

func (a *notificationAuditAdapter) Send(context.Context, map[string]any, map[string]any) error {
	return a.err
}

func TestNotificationChannelLifecycleAuditIsAttributedAndRedacted(t *testing.T) {
	const (
		actorID       = "apikey:key_notification_audit"
		webhookSecret = "https://hooks.example.invalid/NOTIFICATION_WEBHOOK_SECRET_CANARY"
		createToken   = "NOTIFICATION_CREATE_TOKEN_CANARY"
		updateToken   = "NOTIFICATION_UPDATE_TOKEN_CANARY"
		adapterError  = "NOTIFICATION_ADAPTER_ERROR_CANARY"
	)

	store := newNotificationSecurityStore()
	deps := newTestAlertingDeps(t)
	deps.NotificationStore = store
	deps.NotificationSecrets = testutil.TestSecretsManager(t)
	deps.NotificationAdapters[notifications.ChannelTypeSlack] = &notificationAuditAdapter{
		err: errors.New(adapterError + " webhook=" + webhookSecret + " token=" + updateToken),
	}
	deps.PrincipalActorID = apiv2.PrincipalActorID

	var events []audit.Event
	var warnings []string
	deps.AppendAuditEventBestEffort = func(event audit.Event, warning string) {
		events = append(events, event)
		warnings = append(warnings, warning)
	}
	ctx := apiv2.ContextWithAPIKeyID(
		apiv2.ContextWithPrincipal(context.Background(), actorID, "admin"),
		"key_notification_audit",
	)

	createBody := fmt.Sprintf(
		`{"name":"Security notifications","type":"slack","config":{"webhook_url":%q,"api_token":%q,"channel":"infra"}}`,
		webhookSecret,
		createToken,
	)
	createRecorder := runNotificationAuditRequest(
		deps.HandleNotificationChannels,
		http.MethodPost,
		"/notifications/channels",
		createBody,
		ctx,
	)
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d; body=%s", createRecorder.Code, http.StatusCreated, createRecorder.Body.String())
	}

	var createdEnvelope struct {
		Channel notifications.Channel `json:"channel"`
	}
	if err := json.Unmarshal(createRecorder.Body.Bytes(), &createdEnvelope); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	channelID := createdEnvelope.Channel.ID
	if channelID == "" {
		t.Fatal("create response omitted notification channel id")
	}

	updateBody := fmt.Sprintf(`{"config":{"api_token":%q,"channel":"security"}}`, updateToken)
	updateRecorder := runNotificationAuditRequest(
		deps.HandleNotificationChannelActions,
		http.MethodPatch,
		"/notifications/channels/"+channelID,
		updateBody,
		ctx,
	)
	if updateRecorder.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d; body=%s", updateRecorder.Code, http.StatusOK, updateRecorder.Body.String())
	}

	testRecorder := runNotificationAuditRequest(
		deps.HandleNotificationChannelTest,
		http.MethodPost,
		"/notifications/channels/"+channelID+"/test",
		"",
		ctx,
	)
	if testRecorder.Code != http.StatusOK {
		t.Fatalf("test status = %d, want %d; body=%s", testRecorder.Code, http.StatusOK, testRecorder.Body.String())
	}

	deleteRecorder := runNotificationAuditRequest(
		deps.HandleNotificationChannelActions,
		http.MethodDelete,
		"/notifications/channels/"+channelID,
		"",
		ctx,
	)
	if deleteRecorder.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d; body=%s", deleteRecorder.Code, http.StatusOK, deleteRecorder.Body.String())
	}

	want := []struct {
		eventType string
		decision  string
		details   map[string]any
	}{
		{
			eventType: notificationChannelCreatedAuditType,
			decision:  "applied",
			details: map[string]any{
				"resource_type": "notification_channel",
				"action":        "create",
				"channel_type":  notifications.ChannelTypeSlack,
			},
		},
		{
			eventType: notificationChannelUpdatedAuditType,
			decision:  "applied",
			details: map[string]any{
				"resource_type": "notification_channel",
				"action":        "update",
				"channel_type":  notifications.ChannelTypeSlack,
			},
		},
		{
			eventType: notificationChannelTestedAuditType,
			decision:  "failed",
			details: map[string]any{
				"resource_type": "notification_channel",
				"action":        "test",
				"channel_type":  notifications.ChannelTypeSlack,
				"outcome":       "delivery_failed",
			},
		},
		{
			eventType: notificationChannelDeletedAuditType,
			decision:  "applied",
			details: map[string]any{
				"resource_type": "notification_channel",
				"action":        "delete",
			},
		},
	}
	if len(events) != len(want) {
		t.Fatalf("audit event count = %d, want %d: %#v", len(events), len(want), events)
	}
	if len(warnings) != len(want) {
		t.Fatalf("audit warning count = %d, want %d: %#v", len(warnings), len(want), warnings)
	}
	for index, expected := range want {
		event := events[index]
		if event.Type != expected.eventType {
			t.Errorf("event[%d].Type = %q, want %q", index, event.Type, expected.eventType)
		}
		if event.ActorID != actorID {
			t.Errorf("event[%d].ActorID = %q, want %q", index, event.ActorID, actorID)
		}
		if event.Target != channelID {
			t.Errorf("event[%d].Target = %q, want %q", index, event.Target, channelID)
		}
		if event.Decision != expected.decision {
			t.Errorf("event[%d].Decision = %q, want %q", index, event.Decision, expected.decision)
		}
		if !reflect.DeepEqual(event.Details, expected.details) {
			t.Errorf("event[%d].Details = %#v, want %#v", index, event.Details, expected.details)
		}
		if event.Reason != "" {
			t.Errorf("event[%d].Reason unexpectedly retained provider data: %q", index, event.Reason)
		}
		if warnings[index] != notificationChannelAuditWarning {
			t.Errorf("warning[%d] = %q, want %q", index, warnings[index], notificationChannelAuditWarning)
		}
	}

	assertNotificationAuditCaptureExcludes(t, events, warnings,
		webhookSecret,
		createToken,
		updateToken,
		adapterError,
		"v2:",
	)
}

func TestNotificationChannelTestAuditUsesControlledOutcomes(t *testing.T) {
	tests := []struct {
		name         string
		enabled      bool
		adapterError error
		status       int
		decision     string
		outcome      string
	}{
		{
			name:     "disabled",
			enabled:  false,
			status:   http.StatusUnprocessableEntity,
			decision: "denied",
			outcome:  "channel_disabled",
		},
		{
			name:     "delivered",
			enabled:  true,
			status:   http.StatusOK,
			decision: "succeeded",
			outcome:  "delivered",
		},
		{
			name:         "provider failure",
			enabled:      true,
			adapterError: errors.New("RAW_NOTIFICATION_PROVIDER_ERROR_CANARY"),
			status:       http.StatusOK,
			decision:     "failed",
			outcome:      "delivery_failed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newNotificationSecurityStore()
			deps := newTestAlertingDeps(t)
			deps.NotificationStore = store
			deps.NotificationSecrets = testutil.TestSecretsManager(t)
			deps.NotificationAdapters[notifications.ChannelTypeSlack] = &notificationAuditAdapter{err: test.adapterError}
			deps.PrincipalActorID = apiv2.PrincipalActorID

			var events []audit.Event
			deps.AppendAuditEventBestEffort = func(event audit.Event, _ string) {
				events = append(events, event)
			}
			created, err := deps.createSecureNotificationChannel(notifications.CreateChannelRequest{
				Name:    "Outcome test",
				Type:    notifications.ChannelTypeSlack,
				Config:  map[string]any{"webhook_url": "https://hooks.example.invalid/outcome-secret"},
				Enabled: notificationAuditBoolPointer(test.enabled),
			})
			if err != nil {
				t.Fatalf("create channel fixture: %v", err)
			}

			ctx := apiv2.ContextWithPrincipal(context.Background(), "usr_notification_tester", "admin")
			recorder := runNotificationAuditRequest(
				deps.HandleNotificationChannelTest,
				http.MethodPost,
				"/notifications/channels/"+created.ID+"/test",
				"",
				ctx,
			)
			if recorder.Code != test.status {
				t.Fatalf("test status = %d, want %d; body=%s", recorder.Code, test.status, recorder.Body.String())
			}
			if len(events) != 1 {
				t.Fatalf("audit event count = %d, want 1", len(events))
			}
			event := events[0]
			if event.Type != notificationChannelTestedAuditType {
				t.Errorf("Type = %q, want %q", event.Type, notificationChannelTestedAuditType)
			}
			if event.Decision != test.decision {
				t.Errorf("Decision = %q, want %q", event.Decision, test.decision)
			}
			if event.Details["outcome"] != test.outcome {
				t.Errorf("outcome = %q, want %q", event.Details["outcome"], test.outcome)
			}
			assertNotificationAuditCaptureExcludes(t, events, nil, "RAW_NOTIFICATION_PROVIDER_ERROR_CANARY")
		})
	}
}

func TestNotificationChannelAuditAttributesCookieAndAPIKeyActors(t *testing.T) {
	tests := []struct {
		name    string
		actorID string
		ctx     context.Context
	}{
		{
			name:    "cookie user",
			actorID: "usr_cookie_notification_admin",
			ctx:     apiv2.ContextWithPrincipal(context.Background(), "usr_cookie_notification_admin", "admin"),
		},
		{
			name:    "api key",
			actorID: "apikey:key_notification_admin",
			ctx: apiv2.ContextWithAPIKeyID(
				apiv2.ContextWithPrincipal(context.Background(), "apikey:key_notification_admin", "admin"),
				"key_notification_admin",
			),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deps := newTestAlertingDeps(t)
			deps.NotificationStore = newNotificationSecurityStore()
			deps.NotificationSecrets = testutil.TestSecretsManager(t)
			deps.PrincipalActorID = apiv2.PrincipalActorID

			var events []audit.Event
			deps.AppendAuditEventBestEffort = func(event audit.Event, _ string) {
				events = append(events, event)
			}
			recorder := runNotificationAuditRequest(
				deps.HandleNotificationChannels,
				http.MethodPost,
				"/notifications/channels",
				`{"name":"Actor attribution","type":"slack","config":{"webhook_url":"https://hooks.example.invalid/actor"}}`,
				test.ctx,
			)
			if recorder.Code != http.StatusCreated {
				t.Fatalf("create status = %d, want %d; body=%s", recorder.Code, http.StatusCreated, recorder.Body.String())
			}
			if len(events) != 1 {
				t.Fatalf("audit event count = %d, want 1", len(events))
			}
			if events[0].ActorID != test.actorID {
				t.Fatalf("ActorID = %q, want %q", events[0].ActorID, test.actorID)
			}
		})
	}
}

func runNotificationAuditRequest(
	handler http.HandlerFunc,
	method, target, body string,
	ctx context.Context,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, strings.NewReader(body)).WithContext(ctx)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	handler(recorder, request)
	return recorder
}

func notificationAuditBoolPointer(value bool) *bool {
	return &value
}

func assertNotificationAuditCaptureExcludes(
	t *testing.T,
	events []audit.Event,
	warnings []string,
	forbidden ...string,
) {
	t.Helper()
	capture, err := json.Marshal(struct {
		Events   []audit.Event `json:"events"`
		Warnings []string      `json:"warnings"`
	}{Events: events, Warnings: warnings})
	if err != nil {
		t.Fatalf("marshal audit capture: %v", err)
	}
	for _, value := range forbidden {
		if value != "" && strings.Contains(string(capture), value) {
			t.Fatalf("audit capture disclosed forbidden value %q: %s", value, capture)
		}
	}
}
