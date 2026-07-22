package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/persistence"
)

func TestCredentialProfileLifecycleAuditIsAttributedAndRedacted(t *testing.T) {
	const (
		actorID        = "usr_security_admin"
		createSecret   = "CREDENTIAL_CREATE_SECRET_CANARY"
		createPhrase   = "CREDENTIAL_CREATE_PASSPHRASE_CANARY"
		rotationSecret = "CREDENTIAL_ROTATION_SECRET_CANARY"
		rotationPhrase = "CREDENTIAL_ROTATION_PASSPHRASE_CANARY"
		rotationReason = "CREDENTIAL_ROTATION_REASON_CANARY"
		credentialKind = credentials.KindSSHPrivateKey
	)

	var events []audit.Event
	var warnings []string
	deps := newCredentialProfileAuditDeps(t, func(event audit.Event, warning string) {
		events = append(events, event)
		warnings = append(warnings, warning)
	})
	ctx := apiv2.ContextWithPrincipal(context.Background(), actorID, "admin")

	createBody := fmt.Sprintf(
		`{"name":"QA credential","kind":%q,"secret":%q,"passphrase":%q}`,
		credentialKind,
		createSecret,
		createPhrase,
	)
	createRecorder := runCredentialProfileAuditRequest(
		deps.HandleCredentialProfiles,
		http.MethodPost,
		"/credentials/profiles",
		createBody,
		ctx,
	)
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d; body=%s", createRecorder.Code, http.StatusCreated, createRecorder.Body.String())
	}

	var createdEnvelope struct {
		Profile credentials.Profile `json:"profile"`
	}
	if err := json.Unmarshal(createRecorder.Body.Bytes(), &createdEnvelope); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	profileID := createdEnvelope.Profile.ID
	if profileID == "" {
		t.Fatal("create response omitted credential profile id")
	}

	rotationBody := fmt.Sprintf(
		`{"secret":%q,"passphrase":%q,"reason":%q}`,
		rotationSecret,
		rotationPhrase,
		rotationReason,
	)
	rotationRecorder := runCredentialProfileAuditRequest(
		deps.HandleCredentialProfileActions,
		http.MethodPost,
		"/credentials/profiles/"+profileID+"/rotate",
		rotationBody,
		ctx,
	)
	if rotationRecorder.Code != http.StatusOK {
		t.Fatalf("rotate status = %d, want %d; body=%s", rotationRecorder.Code, http.StatusOK, rotationRecorder.Body.String())
	}

	deleteRecorder := runCredentialProfileAuditRequest(
		deps.HandleCredentialProfileActions,
		http.MethodDelete,
		"/credentials/profiles/"+profileID,
		"",
		ctx,
	)
	if deleteRecorder.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d; body=%s", deleteRecorder.Code, http.StatusOK, deleteRecorder.Body.String())
	}

	want := []struct {
		eventType string
		action    string
	}{
		{eventType: credentialProfileCreatedAuditType, action: "create"},
		{eventType: credentialProfileRotatedAuditType, action: "rotate"},
		{eventType: credentialProfileDeletedAuditType, action: "delete"},
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
		if event.Target != profileID {
			t.Errorf("event[%d].Target = %q, want %q", index, event.Target, profileID)
		}
		if event.Decision != "applied" {
			t.Errorf("event[%d].Decision = %q, want applied", index, event.Decision)
		}
		wantDetails := map[string]any{
			"resource_type": "credential_profile",
			"action":        expected.action,
			"kind":          credentialKind,
		}
		if !reflect.DeepEqual(event.Details, wantDetails) {
			t.Errorf("event[%d].Details = %#v, want %#v", index, event.Details, wantDetails)
		}
		if event.Reason != "" {
			t.Errorf("event[%d].Reason unexpectedly retained request data: %q", index, event.Reason)
		}
		if warnings[index] != credentialProfileAuditWarning {
			t.Errorf("warning[%d] = %q, want %q", index, warnings[index], credentialProfileAuditWarning)
		}
	}

	assertAuditCaptureExcludes(t, events, warnings,
		createSecret,
		createPhrase,
		rotationSecret,
		rotationPhrase,
		rotationReason,
		"v2:",
	)
}

func TestCredentialProfileAuditRedactsUnexpectedPersistedKind(t *testing.T) {
	const unexpectedKind = "CREDENTIAL_KIND_SECRET_CANARY"

	store := persistence.NewMemoryCredentialStore()
	if _, err := store.CreateCredentialProfile(credentials.Profile{
		ID:   "cred_unexpected_kind",
		Name: "Unexpected kind fixture",
		Kind: unexpectedKind,
	}); err != nil {
		t.Fatalf("create credential profile fixture: %v", err)
	}

	var events []audit.Event
	deps := &Deps{
		CredentialStore:   store,
		UserIDFromContext: apiv2.PrincipalActorID,
		AppendAuditEventBestEffort: func(event audit.Event, _ string) {
			events = append(events, event)
		},
	}
	ctx := apiv2.ContextWithPrincipal(context.Background(), "usr_kind_auditor", "admin")
	recorder := runCredentialProfileAuditRequest(
		deps.HandleCredentialProfileActions,
		http.MethodDelete,
		"/credentials/profiles/cred_unexpected_kind",
		"",
		ctx,
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if len(events) != 1 {
		t.Fatalf("audit event count = %d, want 1", len(events))
	}
	if events[0].Details["kind"] != "unknown" {
		t.Fatalf("audit kind = %q, want unknown", events[0].Details["kind"])
	}
	assertAuditCaptureExcludes(t, events, nil, unexpectedKind)
}

func TestCredentialProfileAuditAttributesCookieAndAPIKeyActors(t *testing.T) {
	tests := []struct {
		name    string
		actorID string
		ctx     context.Context
	}{
		{
			name:    "cookie user",
			actorID: "usr_cookie_admin",
			ctx:     apiv2.ContextWithPrincipal(context.Background(), "usr_cookie_admin", "admin"),
		},
		{
			name:    "api key",
			actorID: "apikey:key_audit_admin",
			ctx: apiv2.ContextWithAPIKeyID(
				apiv2.ContextWithPrincipal(context.Background(), "apikey:key_audit_admin", "admin"),
				"key_audit_admin",
			),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var events []audit.Event
			deps := newCredentialProfileAuditDeps(t, func(event audit.Event, _ string) {
				events = append(events, event)
			})
			recorder := runCredentialProfileAuditRequest(
				deps.HandleCredentialProfiles,
				http.MethodPost,
				"/credentials/profiles",
				`{"name":"Actor attribution","kind":"ssh_password","secret":"actor-attribution-secret"}`,
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

func newCredentialProfileAuditDeps(
	t *testing.T,
	appendAudit func(audit.Event, string),
) *Deps {
	t.Helper()
	return &Deps{
		CredentialStore:            persistence.NewMemoryCredentialStore(),
		SecretsManager:             testutil.TestSecretsManager(t),
		AppendAuditEventBestEffort: appendAudit,
		EnforceRateLimit:           testutil.NoopRateLimit,
		UserIDFromContext:          apiv2.PrincipalActorID,
	}
}

func runCredentialProfileAuditRequest(
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

func assertAuditCaptureExcludes(t *testing.T, events []audit.Event, warnings []string, forbidden ...string) {
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
