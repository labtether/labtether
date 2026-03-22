package proxmox

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/connectorsdk"
)

func TestConnectorIdentityAndCapabilities(t *testing.T) {
	allowInsecureTransportForProxmoxTests(t)
	connector := &Connector{}

	if connector.ID() != "proxmox" {
		t.Fatalf("unexpected connector id: %s", connector.ID())
	}
	if connector.DisplayName() != "Proxmox VE" {
		t.Fatalf("unexpected connector display name: %s", connector.DisplayName())
	}

	capabilities := connector.Capabilities()
	if !capabilities.DiscoverAssets || !capabilities.CollectMetrics || !capabilities.CollectEvents || !capabilities.ExecuteActions {
		t.Fatalf("unexpected connector capabilities: %+v", capabilities)
	}

	actions := connector.Actions()
	if len(actions) == 0 {
		t.Fatalf("expected proxmox connector actions to be declared")
	}
	if actions[0].ID == "" {
		t.Fatalf("expected first action descriptor to have an ID")
	}
}

func TestConnectorNewFromEnvironment(t *testing.T) {
	allowInsecureTransportForProxmoxTests(t)
	t.Setenv("PROXMOX_BASE_URL", "https://pve.local:8006")
	t.Setenv("PROXMOX_TOKEN_ID", "labtether@pve!agent")
	t.Setenv("PROXMOX_TOKEN_SECRET", "secret")
	t.Setenv("PROXMOX_DEFAULT_NODE", "pve01")
	t.Setenv("PROXMOX_SKIP_VERIFY", "true")
	t.Setenv("PROXMOX_HTTP_TIMEOUT", "11s")

	connector := New()
	if connector.defaultNode != "pve01" {
		t.Fatalf("expected default node pve01, got %q", connector.defaultNode)
	}
	if !connector.isConfigured() {
		t.Fatalf("expected configured connector from env")
	}

	t.Setenv("PROXMOX_CA_CERT_PEM", "not-a-valid-pem")
	connector = New()
	if connector.clientErr == nil {
		t.Fatalf("expected invalid CA PEM to produce clientErr")
	}
}

func TestConnectorDiscoverAndHealth(t *testing.T) {
	allowInsecureTransportForProxmoxTests(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/cluster/resources":
			_, _ = w.Write([]byte(`{"data":[
				{"type":"node","node":"pve01","status":"online","cpu":0.2,"maxmem":8192,"maxdisk":1000000},
				{"type":"qemu","node":"pve01","vmid":100,"name":"web-01","status":"running","template":0,"hastate":"started"},
				{"type":"lxc","node":"pve01","vmid":200,"name":"ct-01","status":"running","template":0,"hastate":"started"},
				{"type":"storage","id":"storage/pve01/local-zfs","name":"local-zfs","node":"pve01","status":"available","plugintype":"zfspool","content":"images,backup"}
			]}`))
		case "/api2/json/version":
			_, _ = w.Write([]byte(`{"data":{"release":"8.3"}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL:     server.URL,
		TokenID:     "id",
		TokenSecret: "secret",
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	connector := &Connector{client: client, defaultNode: "pve01"}

	assets, err := connector.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}
	if len(assets) != 4 {
		t.Fatalf("expected 4 discovered assets, got %d", len(assets))
	}
	if assets[0].ID == "" || assets[0].Source != "proxmox" {
		t.Fatalf("unexpected discovered asset payload: %+v", assets[0])
	}

	health, err := connector.TestConnection(context.Background())
	if err != nil {
		t.Fatalf("TestConnection returned error: %v", err)
	}
	if health.Status != "ok" || !strings.Contains(health.Message, "8.3") {
		t.Fatalf("unexpected health response: %+v", health)
	}

	stubConnector := &Connector{}
	stubAssets, err := stubConnector.Discover(context.Background())
	if err != nil {
		t.Fatalf("stub discover failed: %v", err)
	}
	if len(stubAssets) == 0 {
		t.Fatalf("expected stub assets")
	}
	stubHealth, err := stubConnector.TestConnection(context.Background())
	if err != nil {
		t.Fatalf("stub TestConnection returned error: %v", err)
	}
	if stubHealth.Status != "ok" || !strings.Contains(stubHealth.Message, "stub mode") {
		t.Fatalf("unexpected stub health: %+v", stubHealth)
	}

	errConnector := &Connector{clientErr: errors.New("broken config")}
	errHealth, err := errConnector.TestConnection(context.Background())
	if err != nil {
		t.Fatalf("error connector TestConnection returned error: %v", err)
	}
	if errHealth.Status != "failed" || !strings.Contains(errHealth.Message, "broken config") {
		t.Fatalf("unexpected failed health response: %+v", errHealth)
	}
}

func TestConnectorExecuteActionFlows(t *testing.T) {
	allowInsecureTransportForProxmoxTests(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/qemu/100/status/start":
			_, _ = w.Write([]byte(`{"data":"UPID-1"}`))
		case "/api2/json/nodes/pve01/tasks/UPID-1/status":
			_, _ = w.Write([]byte(`{"data":{"status":"stopped","exitstatus":"OK"}}`))
		case "/api2/json/nodes/pve01/qemu/100/status/stop":
			_, _ = w.Write([]byte(`{"data":"UPID-2"}`))
		case "/api2/json/nodes/pve01/tasks/UPID-2/status":
			_, _ = w.Write([]byte(`{"data":{"status":"stopped","exitstatus":"ERROR"}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL:     server.URL,
		TokenID:     "id",
		TokenSecret: "secret",
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	connector := &Connector{client: client, defaultNode: "pve01"}

	dryRun, err := connector.ExecuteAction(context.Background(), "vm.start", connectorsdk.ActionRequest{
		TargetID: "pve01/100",
		DryRun:   true,
	})
	if err != nil {
		t.Fatalf("ExecuteAction dry-run returned error: %v", err)
	}
	if dryRun.Status != "succeeded" || !strings.Contains(dryRun.Output, "would execute vm.start") {
		t.Fatalf("unexpected dry-run result: %+v", dryRun)
	}

	startResult, err := connector.ExecuteAction(context.Background(), "vm.start", connectorsdk.ActionRequest{
		TargetID: "pve01/100",
	})
	if err != nil {
		t.Fatalf("ExecuteAction vm.start returned error: %v", err)
	}
	if startResult.Status != "succeeded" || startResult.Metadata["exitstatus"] != "OK" {
		t.Fatalf("unexpected vm.start result: %+v", startResult)
	}

	stopResult, err := connector.ExecuteAction(context.Background(), "vm.stop", connectorsdk.ActionRequest{
		TargetID: "pve01/100",
	})
	if err != nil {
		t.Fatalf("ExecuteAction vm.stop returned error: %v", err)
	}
	if stopResult.Status != "failed" || !strings.Contains(stopResult.Message, "exitstatus ERROR") {
		t.Fatalf("unexpected vm.stop result: %+v", stopResult)
	}

	migrateResult, err := connector.ExecuteAction(context.Background(), "vm.migrate", connectorsdk.ActionRequest{
		TargetID: "pve01/100",
	})
	if err != nil {
		t.Fatalf("ExecuteAction vm.migrate returned error: %v", err)
	}
	if migrateResult.Status != "failed" || !strings.Contains(migrateResult.Message, "target_node is required") {
		t.Fatalf("unexpected vm.migrate validation result: %+v", migrateResult)
	}

	unsupportedResult, err := connector.ExecuteAction(context.Background(), "unsupported.action", connectorsdk.ActionRequest{
		TargetID: "pve01/100",
	})
	if err != nil {
		t.Fatalf("ExecuteAction unsupported action returned error: %v", err)
	}
	if unsupportedResult.Status != "failed" || !strings.Contains(unsupportedResult.Message, "unsupported action") {
		t.Fatalf("unexpected unsupported action result: %+v", unsupportedResult)
	}
}

func TestParseComputeTargetAndHelpers(t *testing.T) {
	allowInsecureTransportForProxmoxTests(t)
	node, vmid, err := parseComputeTarget(connectorsdk.ActionRequest{TargetID: "pve01/100"}, "")
	if err != nil || node != "pve01" || vmid != "100" {
		t.Fatalf("expected target pve01/100, got node=%q vmid=%q err=%v", node, vmid, err)
	}

	node, vmid, err = parseComputeTarget(connectorsdk.ActionRequest{TargetID: "proxmox-vm-101"}, "pve01")
	if err != nil || node != "pve01" || vmid != "101" {
		t.Fatalf("expected proxmox-vm-101 to use default node, got node=%q vmid=%q err=%v", node, vmid, err)
	}

	node, vmid, err = parseComputeTarget(connectorsdk.ActionRequest{
		Params: map[string]string{"node": " pve02 ", "vmid": " 202 "},
	}, "")
	if err != nil || node != "pve02" || vmid != "202" {
		t.Fatalf("expected params node/vmid parse, got node=%q vmid=%q err=%v", node, vmid, err)
	}

	if _, _, err := parseComputeTarget(connectorsdk.ActionRequest{TargetID: "pve01/not-a-number"}, ""); err == nil {
		t.Fatalf("expected non-numeric vmid parse to fail")
	}

	if got := vmidString(100); got != "100" {
		t.Fatalf("expected vmidString(100)=100, got %q", got)
	}
	if got := vmidString(0); got != "" {
		t.Fatalf("expected vmidString(0) to be empty, got %q", got)
	}

	if got := normalizeID("Storage/PVE01 Local"); got != "storage-pve01-local" {
		t.Fatalf("unexpected normalizeID result: %q", got)
	}
	if got := formatFloat(1.5); got != "1.5" {
		t.Fatalf("unexpected formatFloat result: %q", got)
	}
	if got := anyToString(true); got != "true" {
		t.Fatalf("unexpected anyToString(true) result: %q", got)
	}
	if got := anyToString(false); got != "false" {
		t.Fatalf("unexpected anyToString(false) result: %q", got)
	}
	if got := anyToString("  value "); got != "value" {
		t.Fatalf("unexpected anyToString(string) result: %q", got)
	}
	if got := anyToString(42); got != "42" {
		t.Fatalf("unexpected anyToString(int) result: %q", got)
	}
}

func TestConnectorExecuteTaskActionMappings(t *testing.T) {
	allowInsecureTransportForProxmoxTests(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/resize") {
			_, _ = w.Write([]byte(`{"data":null}`))
			return
		}
		if r.Method == http.MethodDelete || r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"data":"UPID:ok"}`))
			return
		}
		t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL:     server.URL,
		TokenID:     "id",
		TokenSecret: "secret",
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	connector := &Connector{client: client}

	cases := []struct {
		actionID string
		vmid     string
		params   map[string]string
		wantUPID bool
	}{
		{actionID: "vm.start", vmid: "100", wantUPID: true},
		{actionID: "vm.stop", vmid: "100", wantUPID: true},
		{actionID: "vm.shutdown", vmid: "100", wantUPID: true},
		{actionID: "vm.reboot", vmid: "100", wantUPID: true},
		{actionID: "vm.snapshot", vmid: "100", params: map[string]string{"snapshot_name": "snap-1"}, wantUPID: true},
		{actionID: "vm.migrate", vmid: "100", params: map[string]string{"target_node": "pve02"}, wantUPID: true},
		{actionID: "ct.start", vmid: "200", wantUPID: true},
		{actionID: "ct.stop", vmid: "200", wantUPID: true},
		{actionID: "ct.shutdown", vmid: "200", wantUPID: true},
		{actionID: "ct.reboot", vmid: "200", wantUPID: true},
		{actionID: "ct.snapshot", vmid: "200", params: map[string]string{"snapshot_name": "snap-2"}, wantUPID: true},
		{actionID: "vm.suspend", vmid: "100", wantUPID: true},
		{actionID: "vm.resume", vmid: "100", wantUPID: true},
		{actionID: "vm.force_stop", vmid: "100", wantUPID: true},
		{actionID: "ct.force_stop", vmid: "200", wantUPID: true},
		{actionID: "vm.snapshot.delete", vmid: "100", params: map[string]string{"snapshot_name": "snap-1"}, wantUPID: true},
		{actionID: "vm.snapshot.rollback", vmid: "100", params: map[string]string{"snapshot_name": "snap-1"}, wantUPID: true},
		{actionID: "ct.snapshot.delete", vmid: "200", params: map[string]string{"snapshot_name": "snap-2"}, wantUPID: true},
		{actionID: "ct.snapshot.rollback", vmid: "200", params: map[string]string{"snapshot_name": "snap-2"}, wantUPID: true},
		{actionID: "ct.migrate", vmid: "200", params: map[string]string{"target_node": "pve02"}, wantUPID: true},
		{actionID: "vm.backup", vmid: "100", params: map[string]string{"storage": "local", "mode": "snapshot"}, wantUPID: true},
		{actionID: "ct.backup", vmid: "200", params: map[string]string{"storage": "local", "mode": "snapshot"}, wantUPID: true},
		{actionID: "vm.clone", vmid: "100", params: map[string]string{"new_id": "101", "new_name": "clone-vm"}, wantUPID: true},
		{actionID: "ct.clone", vmid: "200", params: map[string]string{"new_id": "201", "new_name": "clone-ct"}, wantUPID: true},
		{actionID: "vm.clone_from_template", vmid: "100", params: map[string]string{"new_id": "102", "new_name": "template-vm"}, wantUPID: true},
		{actionID: "ct.clone_from_template", vmid: "200", params: map[string]string{"new_id": "202", "new_name": "template-ct"}, wantUPID: true},
		{actionID: "vm.disk_resize", vmid: "100", params: map[string]string{"disk": "scsi0", "size": "+10G"}, wantUPID: false},
	}

	for _, tc := range cases {
		upid, err := connector.executeTaskAction(context.Background(), tc.actionID, "pve01", tc.vmid, tc.params)
		if err != nil {
			t.Fatalf("executeTaskAction(%s) failed: %v", tc.actionID, err)
		}
		if tc.wantUPID && strings.TrimSpace(upid) == "" {
			t.Fatalf("expected action %s to return UPID", tc.actionID)
		}
		if !tc.wantUPID && strings.TrimSpace(upid) != "" {
			t.Fatalf("expected action %s to return empty UPID, got %q", tc.actionID, upid)
		}
	}

	if _, err := connector.executeTaskAction(context.Background(), "vm.migrate", "pve01", "100", nil); err == nil {
		t.Fatalf("expected vm.migrate without target_node to fail")
	}
	if _, err := connector.executeTaskAction(context.Background(), "vm.clone", "pve01", "100", map[string]string{"new_id": "not-a-number"}); err == nil {
		t.Fatalf("expected vm.clone with non-numeric new_id to fail")
	}
	if _, err := connector.executeTaskAction(context.Background(), "vm.disk_resize", "pve01", "100", map[string]string{"disk": "scsi0"}); err == nil {
		t.Fatalf("expected vm.disk_resize without size to fail")
	}
	if _, err := connector.executeTaskAction(context.Background(), "unknown.action", "pve01", "100", nil); err == nil {
		t.Fatalf("expected unsupported action to fail")
	}
}
