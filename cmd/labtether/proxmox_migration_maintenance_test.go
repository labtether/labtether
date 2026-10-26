package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groupmaintenance"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/persistence"
)

type migrationMaintenanceStore struct {
	persistence.GroupMaintenanceStore
}

func (migrationMaintenanceStore) ListGroupMaintenanceWindows(group string, _ *time.Time, _ int) ([]groupmaintenance.MaintenanceWindow, error) {
	if group == "locked" {
		return []groupmaintenance.MaintenanceWindow{{ID: "window", GroupID: group, BlockActions: true}}, nil
	}
	return nil, nil
}

func TestProxmoxHAMigrationChecksMaintenanceBeforeProvider(t *testing.T) {
	for _, kind := range []string{"qemu", "lxc"} {
		for _, tc := range []struct {
			name, sourceGroup, destinationGroup string
			restricted                          bool
			want                                int
		}{
			{name: "destination locked", destinationGroup: "locked", want: http.StatusLocked},
			{name: "source locked", sourceGroup: "locked", want: http.StatusLocked},
			{name: "unlocked", want: http.StatusOK},
			{name: "source-only key", restricted: true, want: http.StatusForbidden},
		} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if r.Method != http.MethodPost || r.URL.Path != "/api2/json/nodes/node-a/"+kind+"/100/migrate" {
						t.Errorf("unexpected provider request: %s %s", r.Method, r.URL.Path)
					}
					if err := r.ParseForm(); err != nil || r.Form.Get("target") != "NODE-B" {
						t.Errorf("wrong provider destination: %v %v", r.Form, err)
					}
					_, _ = w.Write([]byte(`{"data":"UPID:task-migration"}`))
				}))
				defer upstream.Close()
				sut := newTestAPIServer(t)
				sut.groupMaintenanceStore = migrationMaintenanceStore{}
				createProxmoxCredentialProfile(t, sut, "migration-credential", "fixture@pve!test", "fixture-secret", upstream.URL)
				sut.hubCollectorStore = &stubHubCollectorStore{collectors: []hubcollector.Collector{{
					ID: "pve-a", CollectorType: "proxmox", Enabled: true,
					Config: map[string]any{"base_url": upstream.URL, "credential_id": "migration-credential"},
				}}}
				assetKind, native := "vm", "proxmox-vm-100"
				if kind == "lxc" {
					assetKind, native = "container", "proxmox-ct-100"
				}
				scoped := func(id string) string { return assetid.ScopeCollectorAssetID(id, "pve-a") }
				for _, asset := range []assets.HeartbeatRequest{
					{AssetID: scoped(native), Source: "proxmox", Type: assetKind, Metadata: map[string]string{"collector_id": "pve-a", "node": "node-a", "vmid": "100", "proxmox_type": kind}},
					{AssetID: scoped("proxmox-node-node-a"), Source: "proxmox", Type: "hypervisor-node", GroupID: tc.sourceGroup, Metadata: map[string]string{"collector_id": "pve-a", "node": "node-a"}},
					{AssetID: scoped("proxmox-node-node-b"), Source: "proxmox", Type: "hypervisor-node", GroupID: tc.destinationGroup, Metadata: map[string]string{"collector_id": "pve-a", "node": "NODE-B"}},
				} {
					if _, err := sut.assetStore.UpsertAssetHeartbeat(asset); err != nil {
						t.Fatal(err)
					}
				}
				deps := sut.ensureProxmoxDeps()
				deps.RequireAdminAuth = func(http.ResponseWriter, *http.Request) bool { return true }
				req := httptest.NewRequest(http.MethodPost, "/proxmox/assets/"+scoped(native)+"/ha/migrate", strings.NewReader(`{"target_node":" NODE-B "}`))
				if tc.restricted {
					req = req.WithContext(apiv2.ContextWithAllowedAssets(req.Context(), []string{scoped(native)}))
				}
				recorder := httptest.NewRecorder()
				deps.HandleProxmoxAssets(recorder, req)
				wantCalls := int32(0)
				if tc.want == http.StatusOK {
					wantCalls = 1
				}
				if recorder.Code != tc.want || calls.Load() != wantCalls {
					t.Fatalf("status=%d provider calls=%d body=%s", recorder.Code, calls.Load(), recorder.Body.String())
				}
			})
		}
	}
}
