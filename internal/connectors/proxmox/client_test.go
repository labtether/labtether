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

func TestSPICETicketParsesHostSubject(t *testing.T) {
	var ticket SPICETicket
	if err := json.Unmarshal([]byte(`{"host":"pvespiceproxy:68b8d480:101:pve01::aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","tls-port":61000,"host-subject":"OU=PVE Cluster Node,O=Proxmox Virtual Environment,CN=pve01"}`), &ticket); err != nil {
		t.Fatalf("decode SPICE ticket: %v", err)
	}
	if ticket.HostSubject != "OU=PVE Cluster Node,O=Proxmox Virtual Environment,CN=pve01" {
		t.Fatalf("host-subject=%q", ticket.HostSubject)
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
