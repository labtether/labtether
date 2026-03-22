package resources

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandleRestartSettings(t *testing.T) {
	deps := newTestResourcesDeps(t)

	originalRestartSelf := HubRestartSelf
	called := make(chan struct{}, 1)
	HubRestartSelf = func() error {
		select {
		case called <- struct{}{}:
		default:
		}
		return nil
	}
	t.Cleanup(func() {
		HubRestartSelf = originalRestartSelf
	})

	req := httptest.NewRequest(http.MethodPost, RestartSettingsRoute, nil)
	rec := httptest.NewRecorder()
	deps.HandleRestartSettings(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp RestartSettingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Accepted {
		t.Fatalf("expected accepted=true")
	}
	if resp.Message == "" {
		t.Fatalf("expected restart message")
	}

	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected restart callback to be invoked")
	}
}

func TestHandleRestartSettingsMethodNotAllowed(t *testing.T) {
	deps := newTestResourcesDeps(t)

	req := httptest.NewRequest(http.MethodGet, RestartSettingsRoute, nil)
	rec := httptest.NewRecorder()
	deps.HandleRestartSettings(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d: %s", rec.Code, rec.Body.String())
	}
}
