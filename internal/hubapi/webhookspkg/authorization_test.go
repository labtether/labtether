package webhookspkg

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
)

func TestRestrictedWebhookConfigurationFailsClosed(t *testing.T) {
	ctx := apiv2.ContextWithAllowedAssets(context.Background(), []string{"asset-a"})
	req := httptest.NewRequest(http.MethodGet, "/api/v2/webhooks", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	(&Deps{}).HandleV2Webhooks(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}
