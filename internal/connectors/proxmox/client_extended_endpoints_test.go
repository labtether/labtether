package proxmox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientDetailEndpoints(t *testing.T) {
	allowInsecureTransportForProxmoxTests(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/qemu/100/config":
			_, _ = w.Write([]byte(`{"data":{"name":"web-01","cores":4,"memory":8192}}`))
		case "/api2/json/nodes/pve01/qemu/100/snapshot":
			_, _ = w.Write([]byte(`{"data":[{"name":"snap-a","snaptime":1739941200},{"name":"snap-b","snaptime":1739941800}]}`))
		case "/api2/json/nodes/pve01/tasks":
			if got := r.URL.Query().Get("vmid"); got != "100" {
				t.Fatalf("unexpected vmid query: %s", got)
			}
			if got := r.URL.Query().Get("limit"); got != "5" {
				t.Fatalf("unexpected limit query: %s", got)
			}
			_, _ = w.Write([]byte(`{"data":[{"upid":"UPID:1","type":"qmstart","status":"stopped","exitstatus":"OK"}]}`))
		case "/api2/json/cluster/ha/resources":
			_, _ = w.Write([]byte(`{"data":[{"sid":"vm:100","state":"started","group":"prod"}]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL:     server.URL,
		TokenID:     "id",
		TokenSecret: "secret",
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	config, err := client.GetQemuConfig(context.Background(), "pve01", "100")
	if err != nil {
		t.Fatalf("GetQemuConfig failed: %v", err)
	}
	if got := config["name"]; got != "web-01" {
		t.Fatalf("unexpected config name: %v", got)
	}

	snapshots, err := client.ListQemuSnapshots(context.Background(), "pve01", "100")
	if err != nil {
		t.Fatalf("ListQemuSnapshots failed: %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snapshots))
	}

	tasks, err := client.ListClusterTasks(context.Background(), "pve01", "100", 5)
	if err != nil {
		t.Fatalf("ListClusterTasks failed: %v", err)
	}
	if len(tasks) != 1 || tasks[0].UPID == "" {
		t.Fatalf("unexpected tasks payload: %+v", tasks)
	}

	haResources, err := client.ListHAResources(context.Background())
	if err != nil {
		t.Fatalf("ListHAResources failed: %v", err)
	}
	if len(haResources) != 1 || haResources[0].SID != "vm:100" {
		t.Fatalf("unexpected ha resources payload: %+v", haResources)
	}
}

func TestClientExtendedReadEndpoints(t *testing.T) {
	allowInsecureTransportForProxmoxTests(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/version":
			_, _ = w.Write([]byte(`{"data":{"version":"8.2.4"}}`))
		case "/api2/json/nodes/pve01/storage/local/content":
			if r.URL.Query().Get("content") == "backup" {
				_, _ = w.Write([]byte(`{"data":[{"volid":"local:backup/vzdump-qemu-100.vma.zst","content":"backup","vmid":100,"ctime":1700000000,"size":12345}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":[{"volid":"local:iso/debian.iso","content":"iso","format":"iso","size":987654}]}`))
		case "/api2/json/nodes/pve01/storage/local/status":
			_, _ = w.Write([]byte(`{"data":{"active":1,"total":1000000,"used":650000}}`))
		case "/api2/json/nodes/pve01/status":
			_, _ = w.Write([]byte(`{"data":{"status":"online","cpu":0.2}}`))
		case "/api2/json/nodes/pve01/lxc/200/config":
			_, _ = w.Write([]byte(`{"data":{"hostname":"ct-200","memory":2048}}`))
		case "/api2/json/nodes/pve01/lxc/200/snapshot":
			_, _ = w.Write([]byte(`{"data":[{"name":"ct-snap","snaptime":1700000100}]}`))
		case "/api2/json/cluster/firewall/rules":
			_, _ = w.Write([]byte(`{"data":[{"pos":0,"type":"in","action":"ACCEPT","proto":"tcp","dport":"22","enable":1}]}`))
		case "/api2/json/cluster/backup":
			_, _ = w.Write([]byte(`{"data":[{"id":"backup-1","schedule":"daily","storage":"pbs","mode":"snapshot","enabled":1}]}`))
		case "/api2/json/nodes/pve01/firewall/rules":
			_, _ = w.Write([]byte(`{"data":[{"pos":0,"type":"in","action":"ACCEPT","enable":1}]}`))
		case "/api2/json/nodes/pve01/qemu/100/firewall/rules":
			_, _ = w.Write([]byte(`{"data":[{"pos":0,"type":"in","action":"DROP","enable":1}]}`))
		case "/api2/json/cluster/ceph/status":
			_, _ = w.Write([]byte(`{"data":{"health":{"status":"HEALTH_WARN"}}}`))
		case "/api2/json/cluster/ceph/osd":
			_, _ = w.Write([]byte(`{"data":[{"id":1,"name":"osd.1","status":"up"}]}`))
		case "/api2/json/nodes/pve01/disks/zfs":
			_, _ = w.Write([]byte(`{"data":[{"name":"tank","size":1000000,"alloc":700000,"free":300000,"health":"ONLINE"}]}`))
		case "/api2/json/nodes/pve01/rrddata":
			if got := r.URL.Query().Get("timeframe"); got != "hour" {
				t.Fatalf("unexpected node timeframe query: %s", got)
			}
			_, _ = w.Write([]byte(`{"data":[{"time":1700000000,"cpu":0.1,"maxcpu":2}]}`))
		case "/api2/json/nodes/pve01/qemu/100/rrddata":
			if got := r.URL.Query().Get("timeframe"); got != "day" {
				t.Fatalf("unexpected qemu timeframe query: %s", got)
			}
			_, _ = w.Write([]byte(`{"data":[{"time":1700000000,"cpu":0.2,"maxcpu":4}]}`))
		case "/api2/json/nodes/pve01/lxc/200/rrddata":
			if got := r.URL.Query().Get("timeframe"); got != "week" {
				t.Fatalf("unexpected lxc timeframe query: %s", got)
			}
			_, _ = w.Write([]byte(`{"data":[{"time":1700000000,"cpu":0.3,"maxcpu":2}]}`))
		case "/api2/json/nodes/pve01/qemu/100/agent/get-osinfo":
			_, _ = w.Write([]byte(`{"data":{"name":"Ubuntu","kernel-release":"6.8.0"}}`))
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, TokenID: "id", TokenSecret: "secret"})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	release, err := client.GetVersion(context.Background())
	if err != nil || release != "8.2.4" {
		t.Fatalf("unexpected GetVersion result release=%q err=%v", release, err)
	}
	backups, err := client.ListStorageBackups(context.Background(), "pve01", "local")
	if err != nil || len(backups) != 1 {
		t.Fatalf("unexpected ListStorageBackups result backups=%+v err=%v", backups, err)
	}
	content, err := client.GetStorageContent(context.Background(), "pve01", "local")
	if err != nil || len(content) != 1 || content[0].Content != "iso" {
		t.Fatalf("unexpected GetStorageContent result content=%+v err=%v", content, err)
	}
	status, err := client.GetStorageStatus(context.Background(), "pve01", "local")
	if err != nil || status["active"] == nil {
		t.Fatalf("unexpected GetStorageStatus result status=%+v err=%v", status, err)
	}
	nodeStatus, err := client.GetNodeStatus(context.Background(), "pve01")
	if err != nil || nodeStatus["status"] != "online" {
		t.Fatalf("unexpected GetNodeStatus result status=%+v err=%v", nodeStatus, err)
	}
	lxcConfig, err := client.GetLXCConfig(context.Background(), "pve01", "200")
	if err != nil || lxcConfig["hostname"] != "ct-200" {
		t.Fatalf("unexpected GetLXCConfig result config=%+v err=%v", lxcConfig, err)
	}
	lxcSnapshots, err := client.ListLXCSnapshots(context.Background(), "pve01", "200")
	if err != nil || len(lxcSnapshots) != 1 {
		t.Fatalf("unexpected ListLXCSnapshots result snapshots=%+v err=%v", lxcSnapshots, err)
	}
	clusterRules, err := client.GetClusterFirewallRules(context.Background())
	if err != nil || len(clusterRules) != 1 {
		t.Fatalf("unexpected GetClusterFirewallRules result rules=%+v err=%v", clusterRules, err)
	}
	backupSchedules, err := client.GetBackupSchedules(context.Background())
	if err != nil || len(backupSchedules) != 1 {
		t.Fatalf("unexpected GetBackupSchedules result schedules=%+v err=%v", backupSchedules, err)
	}
	nodeRules, err := client.GetNodeFirewallRules(context.Background(), "pve01")
	if err != nil || len(nodeRules) != 1 {
		t.Fatalf("unexpected GetNodeFirewallRules result rules=%+v err=%v", nodeRules, err)
	}
	vmRules, err := client.GetVMFirewallRules(context.Background(), "pve01", "100", "qemu")
	if err != nil || len(vmRules) != 1 {
		t.Fatalf("unexpected GetVMFirewallRules result rules=%+v err=%v", vmRules, err)
	}
	cephStatus, err := client.GetCephStatus(context.Background())
	if err != nil || cephStatus == nil || cephStatus.Health.Status != "HEALTH_WARN" {
		t.Fatalf("unexpected GetCephStatus result status=%+v err=%v", cephStatus, err)
	}
	cephOSDs, err := client.GetCephOSDs(context.Background())
	if err != nil || len(cephOSDs) != 1 {
		t.Fatalf("unexpected GetCephOSDs result osds=%+v err=%v", cephOSDs, err)
	}
	zfsPools, err := client.GetNodeZFSPools(context.Background(), "pve01")
	if err != nil || len(zfsPools) != 1 {
		t.Fatalf("unexpected GetNodeZFSPools result pools=%+v err=%v", zfsPools, err)
	}
	nodeRRD, err := client.GetNodeRRDData(context.Background(), "pve01", "")
	if err != nil || len(nodeRRD) != 1 {
		t.Fatalf("unexpected GetNodeRRDData result points=%+v err=%v", nodeRRD, err)
	}
	qemuRRD, err := client.GetQemuRRDData(context.Background(), "pve01", "100", "day")
	if err != nil || len(qemuRRD) != 1 {
		t.Fatalf("unexpected GetQemuRRDData result points=%+v err=%v", qemuRRD, err)
	}
	lxcRRD, err := client.GetLXCRRDData(context.Background(), "pve01", "200", "week")
	if err != nil || len(lxcRRD) != 1 {
		t.Fatalf("unexpected GetLXCRRDData result points=%+v err=%v", lxcRRD, err)
	}
	osInfo, err := client.GetQemuAgentOSInfo(context.Background(), "pve01", "100")
	if err != nil || osInfo["name"] != "Ubuntu" {
		t.Fatalf("unexpected GetQemuAgentOSInfo result info=%+v err=%v", osInfo, err)
	}
}

func TestClientExtendedMutationAndProxyEndpoints(t *testing.T) {
	allowInsecureTransportForProxmoxTests(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/qemu/100/status/stop",
			"/api2/json/nodes/pve01/qemu/100/status/shutdown",
			"/api2/json/nodes/pve01/qemu/100/status/reboot",
			"/api2/json/nodes/pve01/qemu/100/snapshot",
			"/api2/json/nodes/pve01/qemu/100/migrate",
			"/api2/json/nodes/pve01/lxc/200/status/start",
			"/api2/json/nodes/pve01/lxc/200/status/stop",
			"/api2/json/nodes/pve01/lxc/200/status/shutdown",
			"/api2/json/nodes/pve01/lxc/200/status/reboot",
			"/api2/json/nodes/pve01/lxc/200/snapshot":
			_, _ = w.Write([]byte(`{"data":"UPID:ok"}`))
		case "/api2/json/nodes/pve01/termproxy",
			"/api2/json/nodes/pve01/qemu/100/termproxy",
			"/api2/json/nodes/pve01/lxc/200/termproxy",
			"/api2/json/nodes/pve01/qemu/100/vncproxy",
			"/api2/json/nodes/pve01/lxc/200/vncproxy":
			_, _ = w.Write([]byte(`{"data":{"port":"5900","ticket":"PVEVNC:ticket","user":"root@pam"}}`))
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, TokenID: "id", TokenSecret: "secret"})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if _, err := client.StopVM(context.Background(), "pve01", "100"); err != nil {
		t.Fatalf("StopVM failed: %v", err)
	}
	if _, err := client.ShutdownVM(context.Background(), "pve01", "100"); err != nil {
		t.Fatalf("ShutdownVM failed: %v", err)
	}
	if _, err := client.RebootVM(context.Background(), "pve01", "100"); err != nil {
		t.Fatalf("RebootVM failed: %v", err)
	}
	if _, err := client.SnapshotVM(context.Background(), "pve01", "100", "snap-a"); err != nil {
		t.Fatalf("SnapshotVM failed: %v", err)
	}
	if _, err := client.MigrateVM(context.Background(), "pve01", "100", "pve02"); err != nil {
		t.Fatalf("MigrateVM failed: %v", err)
	}
	if _, err := client.StartCT(context.Background(), "pve01", "200"); err != nil {
		t.Fatalf("StartCT failed: %v", err)
	}
	if _, err := client.StopCT(context.Background(), "pve01", "200"); err != nil {
		t.Fatalf("StopCT failed: %v", err)
	}
	if _, err := client.ShutdownCT(context.Background(), "pve01", "200"); err != nil {
		t.Fatalf("ShutdownCT failed: %v", err)
	}
	if _, err := client.RebootCT(context.Background(), "pve01", "200"); err != nil {
		t.Fatalf("RebootCT failed: %v", err)
	}
	if _, err := client.SnapshotCT(context.Background(), "pve01", "200", "snap-b"); err != nil {
		t.Fatalf("SnapshotCT failed: %v", err)
	}

	nodeProxy, err := client.OpenNodeTermProxy(context.Background(), "pve01")
	if err != nil || nodeProxy.Port.Int() != 5900 {
		t.Fatalf("unexpected OpenNodeTermProxy result proxy=%+v err=%v", nodeProxy, err)
	}
	qemuProxy, err := client.OpenQemuTermProxy(context.Background(), "pve01", "100")
	if err != nil || qemuProxy.Port.Int() != 5900 {
		t.Fatalf("unexpected OpenQemuTermProxy result proxy=%+v err=%v", qemuProxy, err)
	}
	lxcProxy, err := client.OpenLXCTermProxy(context.Background(), "pve01", "200")
	if err != nil || lxcProxy.Port.Int() != 5900 {
		t.Fatalf("unexpected OpenLXCTermProxy result proxy=%+v err=%v", lxcProxy, err)
	}
	qemuVNCProxy, err := client.OpenQemuVNCProxy(context.Background(), "pve01", "100")
	if err != nil || qemuVNCProxy.Port.Int() != 5900 {
		t.Fatalf("unexpected OpenQemuVNCProxy result proxy=%+v err=%v", qemuVNCProxy, err)
	}
	lxcVNCProxy, err := client.OpenLXCVNCProxy(context.Background(), "pve01", "200")
	if err != nil || lxcVNCProxy.Port.Int() != 5900 {
		t.Fatalf("unexpected OpenLXCVNCProxy result proxy=%+v err=%v", lxcVNCProxy, err)
	}
}
