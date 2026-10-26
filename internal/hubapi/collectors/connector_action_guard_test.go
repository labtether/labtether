package collectors

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/groupmaintenance"
	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/hubapi/maintenanceguard"
	"github.com/labtether/labtether/internal/persistence"
)

type directActionProvider struct {
	connectorsdk.Connector
	id, action string
	calls      int
	last       connectorsdk.ActionRequest
}

func (p *directActionProvider) ID() string { return p.id }
func (p *directActionProvider) Actions() []connectorsdk.ActionDescriptor {
	return []connectorsdk.ActionDescriptor{{ID: p.action}}
}
func (p *directActionProvider) ExecuteAction(_ context.Context, _ string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	p.calls++
	p.last = req
	return connectorsdk.ActionResult{Status: "succeeded"}, nil
}

type directActionMaintenanceStore struct {
	persistence.GroupMaintenanceStore
}

func (directActionMaintenanceStore) ListGroupMaintenanceWindows(groupID string, _ *time.Time, _ int) ([]groupmaintenance.MaintenanceWindow, error) {
	if groupID == "store-error" {
		return nil, errors.New("maintenance store unavailable")
	}
	if groupID == "locked" {
		return []groupmaintenance.MaintenanceWindow{{ID: "window", GroupID: groupID, BlockActions: true}}, nil
	}
	return nil, nil
}

func directActionDeps(t *testing.T, connector connectorsdk.Connector, asset assets.Asset) *Deps {
	t.Helper()
	store := persistence.NewMemoryAssetStore()
	if _, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: asset.ID, Name: asset.Name, Type: asset.Type, Source: asset.Source,
		GroupID: asset.GroupID, Metadata: asset.Metadata,
	}); err != nil {
		t.Fatal(err)
	}
	groups := &groupfeatures.Deps{AssetStore: store, GroupMaintenanceStore: directActionMaintenanceStore{}}
	registry := connectorsdk.NewRegistry()
	registry.Register(connector)
	return &Deps{
		AssetStore: store, ConnectorRegistry: registry,
		EvaluateAssetGuardrails:    groups.EvaluateAssetGuardrails,
		ExecuteProxmoxActionDirect: connector.ExecuteAction,
	}
}

func directActionRequest(t *testing.T, deps *Deps, connector, action string, req connectorsdk.ActionRequest, restricted bool) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/connectors/"+connector+"/actions/"+action+"/execute", strings.NewReader(string(body)))
	if restricted {
		request = request.WithContext(apiv2.ContextWithAllowedAssets(request.Context(), []string{req.TargetID}))
	}
	recorder := httptest.NewRecorder()
	deps.HandleConnectorActions(recorder, request)
	return recorder
}

func TestDirectConnectorActionHonorsCanonicalMaintenance(t *testing.T) {
	for _, operation := range []struct{ connector, action, target, kind string }{
		{"proxmox", "vm.start", "proxmox-vm-100", "vm"},
		{"proxmox", "ct.stop", "proxmox-ct-101", "container"},
		{"portainer", "container.restart", "portainer-container-1-abc", "container"},
		{"truenas", "vm.start", "truenas-vm-builder", "vm"},
	} {
		for _, group := range []string{"locked", "open"} {
			t.Run(operation.connector+"/"+operation.action+"/"+group, func(t *testing.T) {
				provider := &directActionProvider{id: operation.connector, action: operation.action}
				deps := directActionDeps(t, provider, assets.Asset{ID: operation.target, Source: operation.connector, Type: operation.kind, GroupID: group, Metadata: map[string]string{"vm_id": "42"}})
				recorder := directActionRequest(t, deps, operation.connector, operation.action, connectorsdk.ActionRequest{TargetID: operation.target}, false)
				wantCode, wantCalls := http.StatusLocked, 0
				if group == "open" {
					wantCode, wantCalls = http.StatusOK, 1
				}
				if recorder.Code != wantCode || provider.calls != wantCalls {
					t.Fatalf("status=%d provider calls=%d body=%s", recorder.Code, provider.calls, recorder.Body.String())
				}
				if provider.calls > 0 && provider.last.TargetID != operation.target {
					t.Fatalf("canonical target changed: %q", provider.last.TargetID)
				}
			})
		}
	}
}

func TestDirectConnectorActionMaintenanceFailureAndRestrictedKeysDoNotDispatch(t *testing.T) {
	for _, tc := range []struct {
		connector, group string
		restricted       bool
		want             int
	}{
		{"proxmox", "store-error", false, http.StatusInternalServerError},
		{"proxmox", "open", true, http.StatusForbidden},
		{"portainer", "open", true, http.StatusForbidden},
	} {
		t.Run(tc.connector+"/"+tc.group, func(t *testing.T) {
			provider := &directActionProvider{id: tc.connector, action: "vm.start"}
			deps := directActionDeps(t, provider, assets.Asset{ID: "canonical-target", Source: tc.connector, GroupID: tc.group})
			recorder := directActionRequest(t, deps, tc.connector, "vm.start", connectorsdk.ActionRequest{TargetID: "canonical-target"}, tc.restricted)
			if recorder.Code != tc.want || provider.calls != 0 {
				t.Fatalf("status=%d provider calls=%d body=%s", recorder.Code, provider.calls, recorder.Body.String())
			}
		})
	}
}

func TestDirectTrueNASNamedVMReachesStoredProviderID(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	type rpcCall struct {
		ID     int             `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	calls := make(chan rpcCall, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		var call rpcCall
		if err := conn.ReadJSON(&call); err != nil {
			t.Error(err)
			return
		}
		if call.Method != "auth.login_with_api_key" {
			t.Errorf("unexpected auth method %q", call.Method)
			return
		}
		if err := conn.WriteJSON(map[string]any{"jsonrpc": "2.0", "id": call.ID, "result": true}); err != nil {
			t.Error(err)
			return
		}
		if err := conn.ReadJSON(&call); err != nil {
			t.Error(err)
			return
		}
		calls <- call
		if err := conn.WriteJSON(map[string]any{"jsonrpc": "2.0", "id": call.ID, "result": true}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	provider := truenas.NewWithConfig(truenas.Config{BaseURL: server.URL, APIKey: "fixture-key", Timeout: 3 * time.Second})
	deps := directActionDeps(t, provider, assets.Asset{ID: "truenas-vm-builder", Source: "truenas", Type: "vm", GroupID: "open", Metadata: map[string]string{"vm_id": "42"}})
	deps.EvaluateConnectorGuardrails = (maintenanceguard.ConnectorEvaluator{
		Assets: deps.AssetStore, Registry: deps.ConnectorRegistry, EvaluateAsset: deps.EvaluateAssetGuardrails,
	}).Evaluate
	for _, tc := range []struct{ name, action, target, override, wantMethod, wantParams string }{
		{"named", "vm.start", "truenas-vm-builder", "", "vm.start", "[42]"},
		{"alias", "workload.stop", "truenas-vm-builder", "", "vm.stop", "[42]"},
		{"explicit override", "vm.start", "truenas-vm-builder", "43", "vm.start", "[43]"},
		{"raw provider target", "vm.start", "44", "", "vm.start", "[44]"},
		{"global", "system.reboot", "", "", "system.reboot", "[]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := directActionRequest(t, deps, "truenas", tc.action, connectorsdk.ActionRequest{TargetID: tc.target, Params: map[string]string{"vm_id": tc.override}}, false)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			select {
			case call := <-calls:
				if call.Method != tc.wantMethod || string(call.Params) != tc.wantParams {
					t.Fatalf("provider got %s %s, want %s %s", call.Method, call.Params, tc.wantMethod, tc.wantParams)
				}
			default:
				t.Fatal("action did not reach provider")
			}
		})
	}
}
