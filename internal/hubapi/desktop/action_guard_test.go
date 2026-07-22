package desktop

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/terminal"
)

func TestDesktopRuntimeMutationsHonorMaintenanceBlockActions(t *testing.T) {
	evaluations := 0
	deps := &Deps{
		EvaluateAssetGuardrails: func(assetID string, _ time.Time) (groupfeatures.GroupMaintenanceGuardrails, error) {
			evaluations++
			if assetID != "asset-1" {
				t.Fatalf("evaluated asset = %q, want asset-1", assetID)
			}
			return groupfeatures.GroupMaintenanceGuardrails{GroupID: "group-1", BlockActions: true}, nil
		},
	}

	tests := []struct {
		name string
		run  func(http.ResponseWriter, *http.Request)
	}{
		{name: "desktop stream", run: func(w http.ResponseWriter, r *http.Request) {
			deps.HandleDesktopStream(w, r, terminal.Session{ID: "session-1", Target: "asset-1"})
		}},
		{name: "clipboard set", run: func(w http.ResponseWriter, r *http.Request) {
			deps.handleClipboardSet(w, r, "asset-1")
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/desktop/test", nil)
			test.run(recorder, request)
			if recorder.Code != http.StatusLocked {
				t.Fatalf("status = %d, want 423: %s", recorder.Code, recorder.Body.String())
			}
		})
	}
	if evaluations != len(tests) {
		t.Fatalf("guard evaluations = %d, want %d", evaluations, len(tests))
	}
}
