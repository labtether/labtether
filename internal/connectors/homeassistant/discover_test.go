package homeassistant

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Discovery must return every entity, not an arbitrary first few, and must
// never copy camera access tokens into metadata that viewers can read.
func TestConnectorDiscoverReturnsAllEntitiesWithoutCredentials(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")

	const cameraToken = "a1b2c3d4e5f6"
	states := []map[string]any{{
		"entity_id": "camera.front_door",
		"state":     "idle",
		"attributes": map[string]any{
			"friendly_name":  "Front Door",
			"access_token":   cameraToken,
			"entity_picture": "/api/camera_proxy/camera.front_door?token=" + cameraToken,
			"brand":          "Reolink",
		},
	}}
	for i := 0; i < 120; i++ {
		states = append(states, map[string]any{"entity_id": fmt.Sprintf("sensor.room_%d", i), "state": "21"})
	}
	payload, err := json.Marshal(states)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	connector := NewWithConfig(Config{BaseURL: server.URL, Token: "token-1", SkipVerify: true, Timeout: 2 * time.Second})
	assets, err := connector.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != len(states) {
		t.Fatalf("Discover() returned %d entities, want %d", len(assets), len(states))
	}
	camera := assets[0].Metadata
	if camera["brand"] != "Reolink" {
		t.Fatalf("ordinary attribute dropped: %v", camera)
	}
	for key, value := range camera {
		if strings.Contains(value, cameraToken) {
			t.Fatalf("camera token copied into metadata key %q", key)
		}
	}
}
