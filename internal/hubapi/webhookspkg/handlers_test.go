package webhookspkg

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/webhooks"
)

// newDeps returns a Deps wired with in-memory stores and nil audit (no-op).
func newDeps() *Deps {
	return &Deps{
		WebhookStore: persistence.NewMemoryWebhookStore(),
		AuditStore:   nil, // shared.AppendAuditEventBestEffort handles nil gracefully
	}
}

// newReq builds an HTTP request with scopes injected into its context.
func newReq(method, path string, body any, scopes []string) *http.Request {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	r := httptest.NewRequest(method, path, &buf)
	r.Header.Set("Content-Type", "application/json")
	ctx := apiv2.ContextWithScopes(context.Background(), scopes)
	return r.WithContext(ctx)
}

// decodeData unwraps the {"request_id":"...","data":<T>} envelope from apiv2.WriteJSON.
func decodeData(t *testing.T, body *bytes.Buffer, dst any) {
	t.Helper()
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(body).Decode(&env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if err := json.Unmarshal(env.Data, dst); err != nil {
		t.Fatalf("decode data field: %v", err)
	}
}

// ── Create ───────────────────────────────────────────────────────────────────

func TestHandleV2WebhookCreate_Success(t *testing.T) {
	d := newDeps()
	payload := webhooks.CreateRequest{
		Name:   "My Hook",
		URL:    "https://example.com/hook",
		Events: []string{"alert.fired"},
	}
	r := newReq(http.MethodPost, "/api/v2/webhooks", payload, nil)
	w := httptest.NewRecorder()

	d.HandleV2WebhookCreate(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	var wh webhooks.Webhook
	decodeData(t, w.Body, &wh)
	if wh.ID == "" {
		t.Error("created webhook should have a non-empty ID")
	}
	if wh.Name != "My Hook" {
		t.Errorf("Name = %q, want %q", wh.Name, "My Hook")
	}
	if !wh.Enabled {
		t.Error("new webhook should default to enabled")
	}
}

func TestHandleV2WebhookCreate_MissingName(t *testing.T) {
	d := newDeps()
	r := newReq(http.MethodPost, "/api/v2/webhooks", map[string]any{
		"url": "https://example.com/hook",
	}, nil)
	w := httptest.NewRecorder()
	d.HandleV2WebhookCreate(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleV2WebhookCreate_BadURL(t *testing.T) {
	d := newDeps()
	r := newReq(http.MethodPost, "/api/v2/webhooks", map[string]any{
		"name": "test",
		"url":  "ftp://invalid.example.com",
	}, nil)
	w := httptest.NewRecorder()
	d.HandleV2WebhookCreate(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleV2WebhookCreate_ScopeForbidden(t *testing.T) {
	d := newDeps()
	// Provide read-only scope — write is required
	r := newReq(http.MethodPost, "/api/v2/webhooks", map[string]any{
		"name": "hook",
		"url":  "https://example.com/h",
	}, []string{"webhooks:read"})
	w := httptest.NewRecorder()
	d.HandleV2Webhooks(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

// ── List ─────────────────────────────────────────────────────────────────────

func TestHandleV2WebhookList_Empty(t *testing.T) {
	d := newDeps()
	r := newReq(http.MethodGet, "/api/v2/webhooks", nil, nil)
	w := httptest.NewRecorder()
	d.HandleV2WebhookList(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var list []webhooks.Webhook
	decodeData(t, w.Body, &list)
	if list == nil {
		t.Error("response should be an empty array, not null")
	}
}

func TestHandleV2WebhookList_AfterCreate(t *testing.T) {
	d := newDeps()

	// Create two webhooks
	for _, name := range []string{"hook-a", "hook-b"} {
		r := newReq(http.MethodPost, "/api/v2/webhooks", map[string]any{
			"name": name,
			"url":  "https://example.com/" + name,
		}, nil)
		d.HandleV2WebhookCreate(httptest.NewRecorder(), r)
	}

	r := newReq(http.MethodGet, "/api/v2/webhooks", nil, nil)
	w := httptest.NewRecorder()
	d.HandleV2WebhookList(w, r)

	var list []webhooks.Webhook
	decodeData(t, w.Body, &list)
	if len(list) != 2 {
		t.Errorf("list length = %d, want 2", len(list))
	}
}

// ── Get ──────────────────────────────────────────────────────────────────────

func TestHandleV2WebhookGet_NotFound(t *testing.T) {
	d := newDeps()
	r := newReq(http.MethodGet, "/api/v2/webhooks/notreal", nil, nil)
	w := httptest.NewRecorder()
	d.HandleV2WebhookGet(w, r, "notreal")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestHandleV2WebhookGet_Found(t *testing.T) {
	d := newDeps()
	createW := httptest.NewRecorder()
	d.HandleV2WebhookCreate(createW, newReq(http.MethodPost, "/api/v2/webhooks", map[string]any{
		"name": "findme",
		"url":  "https://example.com/findme",
	}, nil))
	var created webhooks.Webhook
	decodeData(t, createW.Body, &created)

	r := newReq(http.MethodGet, "/api/v2/webhooks/"+created.ID, nil, nil)
	w := httptest.NewRecorder()
	d.HandleV2WebhookGet(w, r, created.ID)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got webhooks.Webhook
	decodeData(t, w.Body, &got)
	if got.ID != created.ID {
		t.Errorf("ID = %q, want %q", got.ID, created.ID)
	}
}

// ── Delete ───────────────────────────────────────────────────────────────────

func TestHandleV2WebhookDelete(t *testing.T) {
	d := newDeps()
	createW := httptest.NewRecorder()
	d.HandleV2WebhookCreate(createW, newReq(http.MethodPost, "/api/v2/webhooks", map[string]any{
		"name": "deleteme",
		"url":  "https://example.com/del",
	}, nil))
	var created webhooks.Webhook
	decodeData(t, createW.Body, &created)

	delW := httptest.NewRecorder()
	d.HandleV2WebhookDelete(delW, newReq(http.MethodDelete, "/api/v2/webhooks/"+created.ID, nil, nil), created.ID)
	if delW.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want 200", delW.Code)
	}

	// Verify it's gone
	getW := httptest.NewRecorder()
	d.HandleV2WebhookGet(getW, newReq(http.MethodGet, "/api/v2/webhooks/"+created.ID, nil, nil), created.ID)
	if getW.Code != http.StatusNotFound {
		t.Errorf("after delete, get status = %d, want 404", getW.Code)
	}
}

// ── Method routing ───────────────────────────────────────────────────────────

func TestHandleV2Webhooks_MethodNotAllowed(t *testing.T) {
	d := newDeps()
	r := newReq(http.MethodPut, "/api/v2/webhooks", nil, nil)
	w := httptest.NewRecorder()
	d.HandleV2Webhooks(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}
