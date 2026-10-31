package collectors

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/secrets"
)

// A large Home Assistant install must not rewrite every entity every run.
// Unchanged entities only get their last-seen time refreshed; a changed one is
// written in full.
func TestHomeAssistantCollectorWritesOnlyChangedEntities(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")

	var lampState atomic.Value
	lampState.Store("off")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/states":
			_, _ = w.Write([]byte(`[{"entity_id":"light.lamp","state":"` + lampState.Load().(string) + `"},
				{"entity_id":"sensor.door","state":"closed"}]`))
		case "/api/config":
			_, _ = w.Write([]byte(`{"version":"2026.7.0","location_name":"Write Test"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	secretsManager, err := secrets.NewManagerFromEncodedKey("MDEyMzQ1Njc4OUFCQ0RFRjAxMjM0NTY3ODlBQkNERUY=")
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := secretsManager.EncryptString("ha-write-test", "credential-ha-writes")
	if err != nil {
		t.Fatal(err)
	}
	credentialStore := persistence.NewMemoryCredentialStore()
	if _, err := credentialStore.CreateCredentialProfile(credentials.Profile{
		ID: "credential-ha-writes", Name: "HA", Kind: credentials.KindHomeAssistantToken, Status: "active", SecretCiphertext: ciphertext,
	}); err != nil {
		t.Fatal(err)
	}
	collector := hubcollector.Collector{
		ID: "collector-ha-writes", AssetID: "ha-root-writes", CollectorType: hubcollector.CollectorTypeHomeAssistant, Enabled: true,
		Config: map[string]any{"base_url": server.URL, "credential_id": "credential-ha-writes"},
	}
	assetStore := persistence.NewMemoryAssetStore()
	var entityWrites, snapshots atomic.Int32
	deps := &Deps{
		AssetStore:        assetStore,
		HubCollectorStore: &homeAssistantCollectorStatusStore{collector: collector},
		CredentialStore:   credentialStore,
		SecretsManager:    secretsManager,
		ProcessHeartbeatRequest: func(req assets.HeartbeatRequest) (*assets.Asset, error) {
			if strings.Contains(req.AssetID, "ha-entity-") {
				entityWrites.Add(1)
			}
			stored, err := assetStore.UpsertAssetHeartbeat(req)
			return &stored, err
		},
		PersistCanonicalConnectorSnapshot: func(string, string, string, string, connectorsdk.Connector, []connectorsdk.Asset) {
			snapshots.Add(1)
		},
	}

	deps.executeHomeAssistantCollector(context.Background(), collector)
	if entityWrites.Load() != 2 || snapshots.Load() != 1 {
		t.Fatalf("first run: entity writes=%d snapshots=%d, want 2 and 1", entityWrites.Load(), snapshots.Load())
	}
	deps.executeHomeAssistantCollector(context.Background(), collector)
	if entityWrites.Load() != 2 || snapshots.Load() != 1 {
		t.Fatalf("unchanged run rewrote entities: writes=%d snapshots=%d", entityWrites.Load(), snapshots.Load())
	}
	lampState.Store("on")
	deps.executeHomeAssistantCollector(context.Background(), collector)
	if entityWrites.Load() != 3 || snapshots.Load() != 2 {
		t.Fatalf("changed run: entity writes=%d snapshots=%d, want 3 and 2", entityWrites.Load(), snapshots.Load())
	}
}
