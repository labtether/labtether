package collectors

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/connectors/webservice"
)

func TestRestrictedWebServiceReadDoesNotMutateGlobalGroupingSuggestions(t *testing.T) {
	sentinel := WebServiceGroupingSuggestion{ID: "global-suggestion"}
	d := &Deps{
		WebServiceCoordinator:  webservice.NewCoordinator(),
		URLGroupingSuggestions: []WebServiceGroupingSuggestion{sentinel},
	}
	ctx := apiv2.ContextWithAllowedAssets(context.Background(), []string{"asset-a"})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/web", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	d.HandleWebServices(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(d.URLGroupingSuggestions) != 1 || d.URLGroupingSuggestions[0].ID != sentinel.ID {
		t.Fatalf("restricted read mutated global suggestions: %#v", d.URLGroupingSuggestions)
	}
}
