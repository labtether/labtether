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
// never copy credentials (camera tokens, signed URLs, nested Wi-Fi keys, URL
// passwords, password-mode text) into metadata that viewers can read.
func TestConnectorDiscoverReturnsAllEntitiesWithoutCredentials(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")

	const cameraToken = "zzzzzzzzzzzz" // low-entropy fake, not a real token
	states := []map[string]any{{
		"entity_id": "camera.front_door",
		"state":     "idle",
		"attributes": map[string]any{
			"friendly_name":  "Front Door",
			"access_token":   cameraToken,
			"entity_picture": "/api/camera_proxy/camera.front_door?token=" + cameraToken,
			"snapshot_url":   "/api/image/serve/front?authSig=" + cameraToken,
			"stream_source":  "rtsp://viewer:" + cameraToken + "@nas01.example.com/live",
			"network":        map[string]any{"ssid": "lab", "wifi": map[string]any{"psk": cameraToken}},
			"brand":          "Reolink",
		},
	}, {
		"entity_id":  "input_text.alarm_code",
		"state":      cameraToken,
		"attributes": map[string]any{"mode": "password", "friendly_name": "Alarm code"},
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
	if camera := assets[0].Metadata; camera["brand"] != "Reolink" {
		t.Fatalf("ordinary attribute dropped: %v", camera)
	}
	for _, asset := range assets[:2] {
		for key, value := range asset.Metadata {
			if strings.Contains(value, cameraToken) {
				t.Fatalf("%s copied a credential into metadata key %q", asset.ID, key)
			}
		}
	}
}
