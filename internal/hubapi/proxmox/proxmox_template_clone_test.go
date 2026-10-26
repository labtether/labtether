package proxmox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	proxmoxconnector "github.com/labtether/labtether/internal/connectors/proxmox"
	"github.com/labtether/labtether/internal/connectorsdk"
)

func TestTemplateCloneDestinationMatchesHubAndConnector(t *testing.T) {
	for _, key := range []string{"LABTETHER_ALLOW_INSECURE_TRANSPORT", "LABTETHER_OUTBOUND_ALLOW_PRIVATE", "LABTETHER_OUTBOUND_ALLOW_LOOPBACK"} {
		t.Setenv(key, "true")
	}
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "false")
	for _, kind := range []string{"vm", "ct"} {
		for _, destination := range []string{"", " node-b "} {
			t.Run(kind+"/"+destination, func(t *testing.T) {
				var posts atomic.Int32
				providerKind, nameKey := "qemu", "name"
				if kind == "ct" {
					providerKind, nameKey = "lxc", "hostname"
				}
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/node-a/tasks/UPID:clone/status" {
						_, _ = w.Write([]byte(`{"data":{"status":"stopped","exitstatus":"OK"}}`))
						return
					}
					posts.Add(1)
					if r.Method != http.MethodPost || r.URL.Path != "/api2/json/nodes/node-a/"+providerKind+"/100/clone" {
						t.Errorf("unexpected provider request: %s %s", r.Method, r.URL.Path)
					}
					if err := r.ParseForm(); err != nil || r.Form.Get("target") != strings.TrimSpace(destination) || r.Form.Get("newid") != "101" || r.Form.Get(nameKey) != "new-guest" || r.Form.Get("full") != "1" {
						t.Errorf("wrong clone form: %v, err=%v", r.Form, err)
					}
					if destination == "" && r.Form.Has("target") {
						t.Error("same-node clone sent a destination override")
					}
					_, _ = w.Write([]byte(`{"data":"UPID:clone"}`))
				}))
				defer upstream.Close()
				client, err := proxmoxconnector.NewClient(proxmoxconnector.Config{BaseURL: upstream.URL, TokenID: "fixture@pve!test", TokenSecret: "fixture"})
				if err != nil {
					t.Fatal(err)
				}
				params := map[string]string{"new_name": " new-guest ", "new_id": "101", "target_node": destination}
				upid, err := InvokeProxmoxAction(context.Background(), &ProxmoxRuntime{client: client}, kind+".clone_from_template", "node-a", "100", params)
				if err != nil || upid != "UPID:clone" {
					t.Fatalf("Hub clone: upid=%q err=%v", upid, err)
				}
				t.Setenv("PROXMOX_BASE_URL", upstream.URL)
				t.Setenv("PROXMOX_TOKEN_ID", "fixture@pve!test")
				t.Setenv("PROXMOX_TOKEN_SECRET", "fixture")
				result, err := proxmoxconnector.New().ExecuteAction(context.Background(), kind+".clone_from_template", connectorsdk.ActionRequest{TargetID: "node-a/100", Params: params})
				if err != nil || result.Status != "succeeded" || posts.Load() != 2 {
					t.Fatalf("connector clone: result=%+v err=%v requests=%d", result, err, posts.Load())
				}
			})
		}
	}
}

func TestHubRejectsInvalidProxmoxActionBeforeRuntimeOrDryRun(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		for _, tc := range []struct {
			action, want string
			params       map[string]string
		}{
			{action: "vm.clone_from_template", want: "new_name is required", params: map[string]string{"new_id": "101"}},
			{action: "ct.clone_from_template", want: "new_id must be a positive integer", params: map[string]string{"new_id": "0", "new_name": "guest"}},
			{action: "vm.migrate", want: "target_node is required"},
			{action: "vm.backup", want: "mode must be", params: map[string]string{"mode": "unknown"}},
		} {
			_, err := (&Deps{}).ExecuteProxmoxAction(context.Background(), tc.action, "node-a/100", tc.params, dryRun)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("%s dry=%v accepted invalid action or reached runtime: %v", tc.action, dryRun, err)
			}
		}
	}
}
