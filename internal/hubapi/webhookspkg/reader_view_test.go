package webhookspkg

import (
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

// Delivery refuses plain HTTP unless the Hub allows insecure transport, so
// saving such a URL would create a webhook that silently never fires.
func TestWebhookCreateRejectsHTTPWhenDeliveryWouldRefuseIt(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "false")
	w := httptest.NewRecorder()
	newDeps().HandleV2WebhookCreate(w, newReq(http.MethodPost, "/api/v2/webhooks", map[string]any{"name": "plain", "url": "http://nas01.example.com/hook"}, nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
}
