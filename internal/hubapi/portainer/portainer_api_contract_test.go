package portainer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/assets"
)

func TestWritePortainerJSONSetsContentTypeBeforeWritingBody(t *testing.T) {
	recorder := httptest.NewRecorder()

	WritePortainerJSON(recorder, map[string]string{"status": "ok"}, []string{"warning", "warning"})

	response := recorder.Result()
	defer response.Body.Close()
	if got := response.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	var payload PortainerResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Warnings) != 1 || payload.Warnings[0] != "warning" {
		t.Fatalf("warnings = %#v, want one deduplicated warning", payload.Warnings)
	}
}

func TestHandlePortainerStacksRequiresAdminBeforeUpstreamWork(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		subActions []string
	}{
		{name: "start", method: http.MethodPost, subActions: []string{"7", "start"}},
		{name: "stop", method: http.MethodPost, subActions: []string{"7", "stop"}},
		{name: "redeploy", method: http.MethodPost, subActions: []string{"7", "redeploy"}},
		{name: "remove", method: http.MethodPost, subActions: []string{"7", "remove"}},
		{name: "update compose", method: http.MethodPut, subActions: []string{"7", "compose"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adminChecks := 0
			deps := &Deps{
				RequireAdminAuth: func(w http.ResponseWriter, _ *http.Request) bool {
					adminChecks++
					http.Error(w, "forbidden", http.StatusForbidden)
					return false
				},
			}
			request := httptest.NewRequest(test.method, "/portainer/assets/asset-1/stacks/7/"+test.subActions[1], nil)
			recorder := httptest.NewRecorder()

			// A nil runtime deliberately proves the authorization decision happens
			// before the handler can contact or inspect the upstream environment.
			deps.HandlePortainerStacks(context.Background(), recorder, request, assets.Asset{}, nil, test.subActions)

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
			}
			if adminChecks != 1 {
				t.Fatalf("admin checks = %d, want 1", adminChecks)
			}
		})
	}
}
