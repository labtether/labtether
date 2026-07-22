package agents

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
)

func TestRestrictedAgentSettingsAndEnrollmentRoutesFailClosed(t *testing.T) {
	ctx := apiv2.ContextWithAllowedAssets(context.Background(), []string{"asset-allowed"})
	d := &Deps{}

	settingsReq := httptest.NewRequest(http.MethodGet, "/api/v1/agents/asset-secret/settings", nil).WithContext(ctx)
	settingsRec := httptest.NewRecorder()
	d.HandleAgentSettingsRoutes(settingsRec, settingsReq)
	if settingsRec.Code != http.StatusForbidden {
		t.Fatalf("settings: expected 403, got %d body=%s", settingsRec.Code, settingsRec.Body.String())
	}

	pendingReq := httptest.NewRequest(http.MethodGet, "/api/v1/agents/pending", nil).WithContext(ctx)
	pendingRec := httptest.NewRecorder()
	d.HandleListPendingAgents(pendingRec, pendingReq)
	if pendingRec.Code != http.StatusForbidden {
		t.Fatalf("pending: expected 403, got %d body=%s", pendingRec.Code, pendingRec.Body.String())
	}
}
