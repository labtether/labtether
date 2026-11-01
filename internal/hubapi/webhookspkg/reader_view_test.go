package webhookspkg

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/webhooks"
)

// Webhook URLs often carry the receiving service's access token. Viewers may
// see where events go, but only roles that can edit webhooks see the full URL.
func TestWebhookURLPathIsHiddenFromViewers(t *testing.T) {
	d := newDeps()
	const full = "https://hooks.example.com:8443/services/T000/B000/XXXX?key=abc"
	created := httptest.NewRecorder()
	d.HandleV2WebhookCreate(created, newReq(http.MethodPost, "/api/v2/webhooks", map[string]any{"name": "chat", "url": full}, nil))
	var wh webhooks.Webhook
	decodeData(t, created.Body, &wh)

	for role, want := range map[string]string{"viewer": "https://hooks.example.com:8443", "operator": full} {
		r := newReq(http.MethodGet, "/api/v2/webhooks/"+wh.ID, nil, nil)
		r = r.WithContext(apiv2.ContextWithUserRole(r.Context(), role))
		w := httptest.NewRecorder()
		d.HandleV2WebhookGet(w, r, wh.ID)
		var got webhooks.Webhook
		decodeData(t, w.Body, &got)
		if got.URL != want {
			t.Fatalf("%s saw url %q, want %q", role, got.URL, want)
		}
	}
}

// Delivery refuses plain HTTP unless the Hub allows insecure transport, so a
// new http:// URL is rejected. A webhook saved with one earlier must stay
// editable so it can be renamed, turned off or moved to https://.
func TestWebhookHTTPURLRejectedForNewURLsButStoredOnesStayEditable(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "false")
	d := newDeps()
	const stored = "http://nas01.example.com/hook"
	w := httptest.NewRecorder()
	d.HandleV2WebhookCreate(w, newReq(http.MethodPost, "/api/v2/webhooks", map[string]any{"name": "plain", "url": stored}, nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("create status = %d, want 400: %s", w.Code, w.Body.String())
	}
	if err := d.WebhookStore.CreateWebhook(context.Background(), webhooks.Webhook{ID: "wh_old", Name: "old", URL: stored, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		url  string
		want int
	}{
		{stored, http.StatusOK},
		{"http://nas02.example.com/hook", http.StatusBadRequest},
		{"https://nas01.example.com/hook", http.StatusOK},
	} {
		w := httptest.NewRecorder()
		d.HandleV2WebhookUpdate(w, newReq(http.MethodPatch, "/api/v2/webhooks/wh_old", map[string]any{"name": "renamed", "url": tc.url}, nil), "wh_old")
		if w.Code != tc.want {
			t.Fatalf("update to %s: status = %d, want %d: %s", tc.url, w.Code, tc.want, w.Body.String())
		}
	}
}
