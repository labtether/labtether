package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubcollector"
)

// A stale appliance reference must not send even a read to a replacement
// appliance. The positive control uses the same real handler and live client.
func TestPBSAndPortainerAssetActionsKeepRuntimeOwner(t *testing.T) {
	const containerID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	for _, provider := range []string{"pbs", "portainer"} {
		for _, scenario := range []string{
			"missing owner", "disabled owner", "broken owner config", "unowned ambiguous",
			"namespace disagrees with metadata", "namespace without matching owner",
			"explicit owner among two", "legacy unique owner", "namespace matches unique owner",
			"explicit owner beyond unrelated page", "unique owner beyond unrelated page", "ambiguous owner beyond unrelated page",
			"wrong provider owner",
		} {
			t.Run(provider+"/"+scenario, func(t *testing.T) {
				sut := newTestAPIServer(t)
				allowInsecureTransportForConnectorTests(t)
				var callsA, callsB atomic.Int32
				serve := func(calls *atomic.Int32) *httptest.Server {
					return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						if provider == "pbs" {
							if r.Method != http.MethodDelete || r.URL.Path != "/api2/json/admin/datastore/backup/groups" ||
								r.URL.Query().Get("backup-type") != "vm" || r.URL.Query().Get("backup-id") != "900" {
								t.Errorf("unexpected PBS action: %s %s", r.Method, r.URL.Path)
							}
							_, _ = w.Write([]byte(`{"data":null}`))
						} else if r.Method == http.MethodGet && r.URL.Path == "/api/endpoints/7/docker/containers/container-fixture/json" {
							_, _ = fmt.Fprintf(w, `{"Id":%q,"Name":"/container-fixture"}`, containerID)
						} else {
							if r.Method != http.MethodPost || r.URL.Path != "/api/endpoints/7/docker/containers/"+containerID+"/stop" {
								t.Errorf("unexpected Portainer action: %s %s", r.Method, r.URL.Path)
							}
							w.WriteHeader(http.StatusNoContent)
						}
					}))
				}
				serverA, serverB := serve(&callsA), serve(&callsB)
				defer serverA.Close()
				defer serverB.Close()
				credentialKind := credentials.KindPBSAPIToken
				if provider == "portainer" {
					credentialKind = credentials.KindPortainerAPIKey
				}
				ciphertext, err := sut.secretsManager.EncryptString("synthetic-secret", "fixture-credential")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := sut.credentialStore.CreateCredentialProfile(credentials.Profile{
					ID: "fixture-credential", Name: "fixture", Kind: credentialKind,
					Username: "fixture@pam!token", SecretCiphertext: ciphertext, Status: "active",
				}); err != nil {
					t.Fatal(err)
				}
				collector := func(id, baseURL string) hubcollector.Collector {
					return hubcollector.Collector{ID: id, CollectorType: provider, Enabled: true,
						Config: map[string]any{"base_url": baseURL, "credential_id": "fixture-credential",
							"token_id": "fixture@pam!token", "auth_method": "api_key"}}
				}
				a, b := collector("collector-a", serverA.URL), collector("collector-b", serverB.URL)
				store := &runtimeOwnerCollectorStore{stubHubCollectorStore: &stubHubCollectorStore{collectors: []hubcollector.Collector{b}}}
				sut.hubCollectorStore = store
				metadata := map[string]string{"collector_id": a.ID, "store": "backup", "endpoint_id": "7"}
				targetID := provider + "-fixture"
				wantCode, wantA, wantB := http.StatusBadGateway, int32(0), int32(0)
				switch scenario {
				case "disabled owner":
					a.Enabled = false
					store.collectors = append(store.collectors, a)
				case "wrong provider owner":
					a.CollectorType = hubcollector.CollectorTypeDocker
					store.collectors = append(store.collectors, a)
				case "broken owner config":
					a.Config["credential_id"] = "missing-credential"
					store.collectors = append(store.collectors, a)
				case "unowned ambiguous":
					delete(metadata, "collector_id")
					store.collectors = append(store.collectors, a)
				case "namespace disagrees with metadata":
					metadata["collector_id"] = b.ID
					targetID = assetid.ScopeCollectorAssetID(targetID, a.ID)
				case "namespace without matching owner":
					delete(metadata, "collector_id")
					targetID = assetid.ScopeCollectorAssetID(targetID, a.ID)
				case "explicit owner among two":
					metadata["collector_id"] = " " + a.ID + " "
					store.collectors = append(store.collectors, a)
					wantCode, wantA = http.StatusOK, 1
				case "explicit owner beyond unrelated page", "unique owner beyond unrelated page", "ambiguous owner beyond unrelated page":
					store.collectors = nil
					for i := 0; i < 201; i++ {
						store.collectors = append(store.collectors, hubcollector.Collector{
							ID: fmt.Sprintf("unrelated-%d", i), CollectorType: hubcollector.CollectorTypeDocker, Enabled: true})
					}
					if scenario == "explicit owner beyond unrelated page" {
						store.collectors = append(store.collectors, a, b)
						wantCode, wantA = http.StatusOK, 1
					} else {
						delete(metadata, "collector_id")
						store.collectors = append(store.collectors, b)
						if scenario == "ambiguous owner beyond unrelated page" {
							store.collectors = append(store.collectors, a)
						} else {
							wantCode, wantB = http.StatusOK, 1
						}
					}
				case "legacy unique owner", "namespace matches unique owner":
					delete(metadata, "collector_id")
					if scenario == "namespace matches unique owner" {
						targetID = assetid.ScopeCollectorAssetID(targetID, b.ID)
					}
					wantCode, wantB = http.StatusOK, 1
				}
				// Portainer says plainly when the saved connection is gone, off,
				// or of another kind, instead of a 502 that looks like an
				// appliance failure. No request reaches either appliance.
				if provider == "portainer" {
					switch scenario {
					case "missing owner", "disabled owner":
						wantCode = http.StatusNotFound
					case "wrong provider owner":
						wantCode = http.StatusConflict
					}
				}
				assetType := "storage-pool"
				if provider == "portainer" {
					assetType = "container-host"
				}
				seed := func(id string, meta map[string]string) {
					t.Helper()
					if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
						AssetID: id, Type: assetType, Name: id, Source: provider,
						Status: "online", Metadata: meta,
					}); err != nil {
						t.Fatal(err)
					}
				}
				call := func(id string) *httptest.ResponseRecorder {
					method, path := http.MethodDelete, "/pbs/assets/"+id+"/groups/forget?store=backup&backup-type=vm&backup-id=900"
					if provider == "portainer" {
						method, path = http.MethodPost, "/portainer/assets/"+id+"/containers/container-fixture/stop"
					}
					req := httptest.NewRequest(method, path, nil)
					req = req.WithContext(contextWithPrincipal(req.Context(), "owner", "owner"))
					rec := httptest.NewRecorder()
					if provider == "pbs" {
						sut.handlePBSAssets(rec, req)
					} else {
						sut.handlePortainerAssets(rec, req)
					}
					return rec
				}
				requestsPerAction := int32(1)
				if provider == "portainer" {
					requestsPerAction = 2 // Selected provider inspect, then exact full-ID mutation.
				}
				wantA *= requestsPerAction
				wantB *= requestsPerAction
				seed(targetID, metadata)
				rec := call(targetID)
				if rec.Code != wantCode || callsA.Load() != wantA || callsB.Load() != wantB {
					t.Fatalf("status=%d calls A=%d B=%d, want %d/%d/%d; body=%s",
						rec.Code, callsA.Load(), callsB.Load(), wantCode, wantA, wantB, rec.Body.String())
				}
				if store.listCalls != 0 {
					t.Fatal("runtime selection used a global inventory page")
				}
				// B is genuinely usable; rejected cases did not pass just because
				// the alternative client/transport was broken too.
				if err := sut.assetStore.DeleteAsset(targetID); err != nil {
					t.Fatal(err)
				}
				seed(provider+"-control", map[string]string{"collector_id": b.ID, "store": "backup", "endpoint_id": "7"})
				rec = call(provider + "-control")
				if rec.Code != http.StatusOK || callsA.Load() != wantA || callsB.Load() != wantB+requestsPerAction {
					t.Fatalf("positive B control: status=%d calls A=%d B=%d body=%s",
						rec.Code, callsA.Load(), callsB.Load(), rec.Body.String())
				}
			})
		}
	}
}

// Existing cmd fixtures use these stores. Keep their optional bounded lookup
// behavior aligned with PostgreSQL without changing the broad store interface.
func (s *stubHubCollectorStore) FindEnabledHubCollectorsByType(kind string) ([]hubcollector.Collector, error) {
	return enabledRuntimeFixtureCollectors(s.collectors, kind), nil
}

func (s *errorHubCollectorStore) FindEnabledHubCollectorsByType(kind string) ([]hubcollector.Collector, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return enabledRuntimeFixtureCollectors(s.collectors, kind), nil
}

func enabledRuntimeFixtureCollectors(collectors []hubcollector.Collector, kind string) []hubcollector.Collector {
	var found []hubcollector.Collector
	for _, collector := range collectors {
		if collector.Enabled && collector.CollectorType == kind {
			found = append(found, collector)
			if len(found) == 2 {
				break
			}
		}
	}
	return found
}

type runtimeOwnerCollectorStore struct {
	*stubHubCollectorStore
	listCalls int
}

func (s *runtimeOwnerCollectorStore) ListHubCollectors(limit int, enabledOnly bool) ([]hubcollector.Collector, error) {
	s.listCalls++
	rows, err := s.stubHubCollectorStore.ListHubCollectors(limit, enabledOnly)
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, err
}
