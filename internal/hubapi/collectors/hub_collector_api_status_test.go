package collectors

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/hubcollector"
)

// A second collector on one device hit the one-collector-per-device rule and
// answered "An internal error occurred.".
func TestHandleHubCollectorsSaysTheDeviceAlreadyHasOne(t *testing.T) {
	deps := newHubCollectorHandlerDeps(&hubCollectorHandlerStore{createErr: hubcollector.ErrCollectorExists})
	req := httptest.NewRequest(http.MethodPost, "/hub-collectors",
		bytes.NewReader([]byte(`{"asset_id":"asset-1","collector_type":"ssh"}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	deps.HandleHubCollectors(rec, req)

	var body struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	want := "This device already has a collector. Edit that one, or add the new one to another device."
	if rec.Code != http.StatusConflict || body.Error != want {
		t.Fatalf("got %d %s, want 409 %q", rec.Code, rec.Body.String(), want)
	}
}

func TestAPICollectorRecordsWhatThePageAnswered(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	}))
	defer server.Close()
	store := &hubCollectorHandlerStore{}
	deps := newHubCollectorHandlerDeps(store)

	deps.executeAPICollector(context.Background(), hubcollector.Collector{
		ID: "collector-1", AssetID: "asset-1", CollectorType: hubcollector.CollectorTypeAPI,
		Config: map[string]any{"url": server.URL + "/no-such-page"},
	})

	if store.lastStatus != "error" || store.lastError != "The page answered 404 Not Found." {
		t.Fatalf("got status %q error %q", store.lastStatus, store.lastError)
	}
}
