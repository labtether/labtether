package proxmox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func allowInsecureTransportForProxmoxTests(t *testing.T) {
	t.Helper()
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
}

func TestNewClientTransportPooling(t *testing.T) {
	allowInsecureTransportForProxmoxTests(t)
	client, err := NewClient(Config{
		BaseURL:     "https://pve.local:8006",
		TokenID:     "id",
		TokenSecret: "secret",
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	transport, ok := client.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.httpClient.Transport)
	}
	if transport.MaxIdleConns != 20 {
		t.Fatalf("expected MaxIdleConns=20, got %d", transport.MaxIdleConns)
	}
	if transport.MaxIdleConnsPerHost != 10 {
		t.Fatalf("expected MaxIdleConnsPerHost=10, got %d", transport.MaxIdleConnsPerHost)
	}
	if transport.MaxConnsPerHost != 20 {
		t.Fatalf("expected MaxConnsPerHost=20, got %d", transport.MaxConnsPerHost)
	}
	if transport.IdleConnTimeout != 90*time.Second {
		t.Fatalf("expected IdleConnTimeout=90s, got %v", transport.IdleConnTimeout)
	}
}

func TestClientGetClusterResources(t *testing.T) {
	allowInsecureTransportForProxmoxTests(t)
	const tokenHeader = "PVEAPIToken=labtether@pve!agent=secret123"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/cluster/resources" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != tokenHeader {
			t.Fatalf("unexpected authorization header: %s", got)
		}
		_, _ = w.Write([]byte(`{"data":[{"type":"qemu","node":"pve01","vmid":100,"name":"web-01","status":"running","cpu":0.2}]}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL:     server.URL,
		TokenID:     "labtether@pve!agent",
		TokenSecret: "secret123",
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	resources, err := client.GetClusterResources(context.Background())
	if err != nil {
		t.Fatalf("GetClusterResources failed: %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	if resources[0].Type != "qemu" || resources[0].Node != "pve01" {
		t.Fatalf("unexpected resource payload: %+v", resources[0])
	}
}

func TestClientStartVMReturnsUPID(t *testing.T) {
	allowInsecureTransportForProxmoxTests(t)
	const expectedUPID = "UPID:pve01:001A1234:01122334:67ABCDEF:qmstart:100:root@pam:"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/nodes/pve01/qemu/100/status/start" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":"` + expectedUPID + `"}`))
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

	upid, err := client.StartVM(context.Background(), "pve01", "100")
	if err != nil {
		t.Fatalf("StartVM failed: %v", err)
	}
	if upid != expectedUPID {
		t.Fatalf("unexpected upid: %s", upid)
	}
}

func TestClientWaitForTask(t *testing.T) {
	allowInsecureTransportForProxmoxTests(t)
	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/api2/json/nodes/pve01/tasks/UPID-1/status") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		call := calls.Add(1)
		if call == 1 {
			_, _ = w.Write([]byte(`{"data":{"status":"running"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"status":"stopped","exitstatus":"OK"}}`))
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

	status, err := client.WaitForTask(context.Background(), "pve01", "UPID-1", 10*time.Millisecond, time.Second)
	if err != nil {
		t.Fatalf("WaitForTask failed: %v", err)
	}
	if !strings.EqualFold(status.ExitStatus, "OK") {
		t.Fatalf("unexpected exit status: %s", status.ExitStatus)
	}
	if calls.Load() < 2 {
		t.Fatalf("expected at least 2 polling calls, got %d", calls.Load())
	}
}

func TestClientBuildVNCWebSocketURL(t *testing.T) {
	allowInsecureTransportForProxmoxTests(t)
	client, err := NewClient(Config{
		BaseURL:     "https://pve.local:8006",
		TokenID:     "id",
		TokenSecret: "secret",
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// Test node-level WebSocket URL (kind="node", no vmid).
	url, err := client.BuildVNCWebSocketURL("pve01", "node", "", 5902, "PVEVNC:ticket")
	if err != nil {
		t.Fatalf("BuildVNCWebSocketURL failed: %v", err)
	}
	if !strings.HasPrefix(url, "wss://pve.local:8006/api2/json/nodes/pve01/vncwebsocket?") {
		t.Fatalf("unexpected node URL prefix: %s", url)
	}
	if !strings.Contains(url, "port=5902") || !strings.Contains(url, "vncticket=") {
		t.Fatalf("unexpected node URL query: %s", url)
	}

	// Test QEMU VM WebSocket URL.
	qemuURL, err := client.BuildVNCWebSocketURL("pve01", "qemu", "100", 5902, "PVEVNC:ticket")
	if err != nil {
		t.Fatalf("BuildVNCWebSocketURL qemu failed: %v", err)
	}
	if !strings.HasPrefix(qemuURL, "wss://pve.local:8006/api2/json/nodes/pve01/qemu/100/vncwebsocket?") {
		t.Fatalf("unexpected qemu URL prefix: %s", qemuURL)
	}

	// Test LXC container WebSocket URL.
	lxcURL, err := client.BuildVNCWebSocketURL("pve01", "lxc", "101", 5902, "PVEVNC:ticket")
	if err != nil {
		t.Fatalf("BuildVNCWebSocketURL lxc failed: %v", err)
	}
	if !strings.HasPrefix(lxcURL, "wss://pve.local:8006/api2/json/nodes/pve01/lxc/101/vncwebsocket?") {
		t.Fatalf("unexpected lxc URL prefix: %s", lxcURL)
	}
}

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

func TestClientSuspendResumeVM(t *testing.T) {
	allowInsecureTransportForProxmoxTests(t)
	const expectedUPID = "UPID:pve01:001A:01:67AB:qmsuspend:100:root@pam:"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/qemu/100/status/suspend":
			_, _ = w.Write([]byte(`{"data":"` + expectedUPID + `"}`))
		case "/api2/json/nodes/pve01/qemu/100/status/resume":
			_, _ = w.Write([]byte(`{"data":"UPID:resume"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, TokenID: "id", TokenSecret: "secret"})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	upid, err := client.SuspendVM(context.Background(), "pve01", "100")
	if err != nil {
		t.Fatalf("SuspendVM failed: %v", err)
	}
	if upid != expectedUPID {
		t.Fatalf("unexpected upid: %s", upid)
	}

	upid, err = client.ResumeVM(context.Background(), "pve01", "100")
	if err != nil {
		t.Fatalf("ResumeVM failed: %v", err)
	}
	if upid != "UPID:resume" {
		t.Fatalf("unexpected resume upid: %s", upid)
	}
}

func TestClientSnapshotDeleteRollback(t *testing.T) {
	allowInsecureTransportForProxmoxTests(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/qemu/100/snapshot/snap1"):
			_, _ = w.Write([]byte(`{"data":"UPID:delete"}`))
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/qemu/100/snapshot/snap1/rollback"):
			_, _ = w.Write([]byte(`{"data":"UPID:rollback"}`))
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/lxc/200/snapshot/snap2"):
			_, _ = w.Write([]byte(`{"data":"UPID:lxc-delete"}`))
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/lxc/200/snapshot/snap2/rollback"):
			_, _ = w.Write([]byte(`{"data":"UPID:lxc-rollback"}`))
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, TokenID: "id", TokenSecret: "secret"})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	upid, err := client.DeleteQemuSnapshot(context.Background(), "pve01", "100", "snap1")
	if err != nil {
		t.Fatalf("DeleteQemuSnapshot failed: %v", err)
	}
	if upid != "UPID:delete" {
		t.Fatalf("unexpected upid: %s", upid)
	}

	upid, err = client.RollbackQemuSnapshot(context.Background(), "pve01", "100", "snap1")
	if err != nil {
		t.Fatalf("RollbackQemuSnapshot failed: %v", err)
	}
	if upid != "UPID:rollback" {
		t.Fatalf("unexpected upid: %s", upid)
	}

	upid, err = client.DeleteLXCSnapshot(context.Background(), "pve01", "200", "snap2")
	if err != nil {
		t.Fatalf("DeleteLXCSnapshot failed: %v", err)
	}
	if upid != "UPID:lxc-delete" {
		t.Fatalf("unexpected upid: %s", upid)
	}

	upid, err = client.RollbackLXCSnapshot(context.Background(), "pve01", "200", "snap2")
	if err != nil {
		t.Fatalf("RollbackLXCSnapshot failed: %v", err)
	}
	if upid != "UPID:lxc-rollback" {
		t.Fatalf("unexpected upid: %s", upid)
	}
}

func TestClientTaskLogAndStop(t *testing.T) {
	allowInsecureTransportForProxmoxTests(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/log"):
			_, _ = w.Write([]byte(`{"data":[{"n":1,"t":"starting task"},{"n":2,"t":"task complete"}]}`))
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/tasks/UPID-1"):
			_, _ = w.Write([]byte(`{"data":null}`))
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, TokenID: "id", TokenSecret: "secret"})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	logText, err := client.GetTaskLog(context.Background(), "pve01", "UPID-1", 500)
	if err != nil {
		t.Fatalf("GetTaskLog failed: %v", err)
	}
	if !strings.Contains(logText, "starting task") || !strings.Contains(logText, "task complete") {
		t.Fatalf("unexpected log text: %s", logText)
	}

	err = client.StopTask(context.Background(), "pve01", "UPID-1")
	if err != nil {
		t.Fatalf("StopTask failed: %v", err)
	}
}

func TestClientCloneAndResize(t *testing.T) {
	allowInsecureTransportForProxmoxTests(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/qemu/100/clone"):
			_, _ = w.Write([]byte(`{"data":"UPID:clone-vm"}`))
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/lxc/200/clone"):
			_, _ = w.Write([]byte(`{"data":"UPID:clone-ct"}`))
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/qemu/100/resize"):
			_, _ = w.Write([]byte(`{"data":null}`))
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, TokenID: "id", TokenSecret: "secret"})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	upid, err := client.CloneVM(context.Background(), "pve01", "100", "clone-test", 999)
	if err != nil {
		t.Fatalf("CloneVM failed: %v", err)
	}
	if upid != "UPID:clone-vm" {
		t.Fatalf("unexpected upid: %s", upid)
	}

	upid, err = client.CloneCT(context.Background(), "pve01", "200", "clone-ct-test", 998)
	if err != nil {
		t.Fatalf("CloneCT failed: %v", err)
	}
	if upid != "UPID:clone-ct" {
		t.Fatalf("unexpected upid: %s", upid)
	}

	err = client.ResizeVMDisk(context.Background(), "pve01", "100", "scsi0", "+10G")
	if err != nil {
		t.Fatalf("ResizeVMDisk failed: %v", err)
	}
}

func TestClientClusterStatusAndNetwork(t *testing.T) {
	allowInsecureTransportForProxmoxTests(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/cluster/status":
			_, _ = w.Write([]byte(`{"data":[{"name":"pve01","type":"node","online":1,"nodeid":1},{"name":"cluster","type":"cluster","quorate":1,"nodes":3}]}`))
		case "/api2/json/nodes/pve01/network":
			_, _ = w.Write([]byte(`{"data":[{"iface":"vmbr0","type":"bridge","address":"10.0.0.1","active":1}]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, TokenID: "id", TokenSecret: "secret"})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	entries, err := client.GetClusterStatus(context.Background())
	if err != nil {
		t.Fatalf("GetClusterStatus failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Name != "pve01" || entries[0].Online != 1 {
		t.Fatalf("unexpected entry: %+v", entries[0])
	}

	ifaces, err := client.GetNodeNetwork(context.Background(), "pve01")
	if err != nil {
		t.Fatalf("GetNodeNetwork failed: %v", err)
	}
	if len(ifaces) != 1 || ifaces[0]["iface"] != "vmbr0" {
		t.Fatalf("unexpected network: %+v", ifaces)
	}
}

func TestClientBackupTrigger(t *testing.T) {
	allowInsecureTransportForProxmoxTests(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/vzdump") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":"UPID:backup"}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, TokenID: "id", TokenSecret: "secret"})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	upid, err := client.TriggerBackup(context.Background(), "pve01", "100", "local", "snapshot")
	if err != nil {
		t.Fatalf("TriggerBackup failed: %v", err)
	}
	if upid != "UPID:backup" {
		t.Fatalf("unexpected upid: %s", upid)
	}
}

func TestClientMigrateCT(t *testing.T) {
	allowInsecureTransportForProxmoxTests(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/lxc/200/migrate") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":"UPID:migrate-ct"}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, TokenID: "id", TokenSecret: "secret"})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	upid, err := client.MigrateCT(context.Background(), "pve01", "200", "pve02")
	if err != nil {
		t.Fatalf("MigrateCT failed: %v", err)
	}
	if upid != "UPID:migrate-ct" {
		t.Fatalf("unexpected upid: %s", upid)
	}
}

func TestFlexIntUnmarshal(t *testing.T) {
	allowInsecureTransportForProxmoxTests(t)
	var asInt flexInt
	if err := json.Unmarshal([]byte(`5900`), &asInt); err != nil {
		t.Fatalf("expected int flexInt decode to succeed: %v", err)
	}
	if asInt.Int() != 5900 {
		t.Fatalf("expected 5900, got %d", asInt.Int())
	}

	var asString flexInt
	if err := json.Unmarshal([]byte(`"5901"`), &asString); err != nil {
		t.Fatalf("expected string flexInt decode to succeed: %v", err)
	}
	if asString.Int() != 5901 {
		t.Fatalf("expected 5901, got %d", asString.Int())
	}

	var invalid flexInt
	if err := json.Unmarshal([]byte(`"bad"`), &invalid); err == nil {
		t.Fatalf("expected invalid flexInt decode to fail")
	}
}

func TestClientPasswordAuthTicketCaching(t *testing.T) {
	allowInsecureTransportForProxmoxTests(t)
	var ticketCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/access/ticket":
			ticketCalls.Add(1)
			_, _ = w.Write([]byte(`{"data":{"ticket":"PVE:ticket-1","CSRFPreventionToken":"csrf-1"}}`))
		case "/api2/json/version":
			if cookie := r.Header.Get("Cookie"); !strings.Contains(cookie, "PVEAuthCookie=PVE:ticket-1") {
				t.Fatalf("expected auth cookie on GET, got %q", cookie)
			}
			_, _ = w.Write([]byte(`{"data":{"release":"8.3"}}`))
		case "/api2/json/nodes/pve01/qemu/100/status/stop":
			if cookie := r.Header.Get("Cookie"); !strings.Contains(cookie, "PVEAuthCookie=PVE:ticket-1") {
				t.Fatalf("expected auth cookie on POST, got %q", cookie)
			}
			if csrf := r.Header.Get("CSRFPreventionToken"); csrf != "csrf-1" {
				t.Fatalf("expected CSRFPreventionToken=csrf-1, got %q", csrf)
			}
			_, _ = w.Write([]byte(`{"data":"UPID:stop"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL:  server.URL,
		AuthMode: AuthModePassword,
		Username: "root@pam",
		Password: "secret",
		Timeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if !client.IsConfigured() {
		t.Fatalf("expected password-mode client to be configured")
	}
	if client.GetAuthMode() != AuthModePassword {
		t.Fatalf("expected auth mode password, got %s", client.GetAuthMode())
	}

	release, err := client.GetVersion(context.Background())
	if err != nil {
		t.Fatalf("GetVersion failed: %v", err)
	}
	if release != "8.3" {
		t.Fatalf("unexpected release: %s", release)
	}
	upid, err := client.StopVM(context.Background(), "pve01", "100")
	if err != nil {
		t.Fatalf("StopVM failed: %v", err)
	}
	if upid != "UPID:stop" {
		t.Fatalf("unexpected upid: %s", upid)
	}
	if ticketCalls.Load() != 1 {
		t.Fatalf("expected ticket endpoint to be called once, got %d", ticketCalls.Load())
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
