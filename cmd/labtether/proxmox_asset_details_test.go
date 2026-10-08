package main

import (
	"context"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectors/proxmox"
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLoadProxmoxAssetDetails(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/version":
			_, _ = w.Write([]byte(`{"data":{"release":"8.2"}}`))
		case "/api2/json/nodes/pve01/qemu/101/config":
			_, _ = w.Write([]byte(`{"data":{"name":"web-01","cores":4}}`))
		case "/api2/json/nodes/pve01/qemu/101/snapshot":
			_, _ = w.Write([]byte(`{"data":[{"name":"current"},{"name":"snap-new","snaptime":20},{"name":"snap-old","snaptime":10}]}`))
		case "/api2/json/nodes/pve01/tasks":
			_, _ = w.Write([]byte(`{"data":[{"upid":"UPID:pve01:001:001:001:qmstart:101:root@pam:","node":"pve01","id":"101","type":"qmstart","status":"stopped","exitstatus":"OK","starttime":100}]}`))
		case "/api2/json/cluster/ha/resources":
			_, _ = w.Write([]byte(`{"data":[{"sid":"vm:101","node":"pve01","state":"started","group":"prod"}]}`))
		case "/api2/json/nodes/pve01/qemu/101/firewall/rules":
			_, _ = w.Write([]byte(`{"data":[{"pos":0,"type":"in","action":"ACCEPT","proto":"tcp","dport":"22","enable":1,"comment":"SSH"}]}`))
		case "/api2/json/cluster/backup":
			_, _ = w.Write([]byte(`{"data":[{"id":"backup-0001","schedule":"sat 02:00","storage":"local","mode":"snapshot","compress":"zstd","enabled":1,"vmid":"101","comment":"Weekly backup"}]}`))
		case "/api2/json/cluster/ceph/status":
			// Simulate no Ceph — return 500 (non-fatal, silently skipped).
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"node":"no ceph"}}`))
		case "/api2/json/cluster/ceph/osd":
			// Simulate no Ceph — return 500 (non-fatal, silently skipped).
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"node":"no ceph"}}`))
		default:
			t.Fatalf("unexpected proxmox path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := proxmox.NewClient(proxmox.Config{
		BaseURL:     server.URL,
		TokenID:     "id",
		TokenSecret: "secret",
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	srv := &apiServer{}
	target := proxmoxSessionTarget{
		Kind:        "qemu",
		Node:        "pve01",
		VMID:        "101",
		CollectorID: "collector-1",
	}
	runtime := proxmoxpkg.NewProxmoxRuntimeWithCollector(client, "collector-1")

	details, err := srv.loadProxmoxAssetDetails(context.Background(), "proxmox-vm-101", target, runtime)
	if err != nil {
		t.Fatalf("loadProxmoxAssetDetails failed: %v", err)
	}
	if details.Kind != "qemu" || details.Node != "pve01" || details.VMID != "101" {
		t.Fatalf("unexpected detail identity payload: %+v", details)
	}
	if details.Config["name"] != "web-01" {
		t.Fatalf("unexpected config payload: %+v", details.Config)
	}
	if len(details.Snapshots) != 2 || details.Snapshots[0].Name != "snap-new" {
		t.Fatalf("unexpected snapshots payload: %+v", details.Snapshots)
	}
	if len(details.Tasks) != 1 || details.Tasks[0].ID != "101" {
		t.Fatalf("unexpected tasks payload: %+v", details.Tasks)
	}
	if details.HA.Match == nil || details.HA.Match.SID != "vm:101" {
		t.Fatalf("unexpected ha payload: %+v", details.HA)
	}
	if len(details.FirewallRules) != 1 || details.FirewallRules[0].Action != "ACCEPT" || details.FirewallRules[0].Dport != "22" {
		t.Fatalf("unexpected firewall rules payload: %+v", details.FirewallRules)
	}
	if len(details.BackupSchedules) != 1 || details.BackupSchedules[0].ID != "backup-0001" || details.BackupSchedules[0].Schedule != "sat 02:00" {
		t.Fatalf("unexpected backup schedules payload: %+v", details.BackupSchedules)
	}
	if len(details.Warnings) != 0 {
		t.Fatalf("did not expect warnings, got: %+v", details.Warnings)
	}
}

func TestHandleProxmoxAssetsDetailsSuccess(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/version":
			_, _ = w.Write([]byte(`{"data":{"release":"8.3"}}`))
		case "/api2/json/nodes/pve01/status":
			_, _ = w.Write([]byte(`{"data":{"status":"online","cpu":0.2}}`))
		case "/api2/json/nodes/pve01/tasks":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api2/json/cluster/ha/resources":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api2/json/nodes/pve01/firewall/rules":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api2/json/cluster/backup":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api2/json/cluster/ceph/status", "/api2/json/cluster/ceph/osd":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"node":"no ceph"}}`))
		case "/api2/json/nodes/pve01/disks/zfs":
			_, _ = w.Write([]byte(`{"data":[]}`))
		default:
			t.Fatalf("unexpected proxmox path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	sut := newTestAPIServer(t)
	configureSingleProxmoxCollector(t, sut, server.URL, "collector-proxmox-1")

	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-node-pve01",
		Type:    "hypervisor-node",
		Name:    "pve01",
		Source:  "proxmox",
		Status:  "online",
		Metadata: map[string]string{
			"proxmox_type": "node",
			"node":         "pve01",
			"collector_id": "collector-proxmox-1",
		},
	})
	if err != nil {
		t.Fatalf("failed to seed proxmox node asset: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/proxmox/assets/proxmox-node-pve01/details", nil)
	rec := httptest.NewRecorder()
	sut.handleProxmoxAssets(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if !strings.Contains(rec.Body.String(), `"asset_id":"proxmox-node-pve01"`) {
		t.Fatalf("expected proxmox details payload, got %s", rec.Body.String())
	}
}

func TestLoadProxmoxAssetDetailsFatalAndWarningBranches(t *testing.T) {
	configErrorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/version":
			_, _ = w.Write([]byte(`{"data":{"release":"8.3"}}`))
		case "/api2/json/nodes/pve01/qemu/101/config":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"config":"failed"}}`))
		case "/api2/json/nodes/pve01/qemu/101/snapshot",
			"/api2/json/nodes/pve01/tasks",
			"/api2/json/cluster/ha/resources",
			"/api2/json/nodes/pve01/qemu/101/firewall/rules",
			"/api2/json/cluster/backup",
			"/api2/json/cluster/ceph/status",
			"/api2/json/cluster/ceph/osd":
			_, _ = w.Write([]byte(`{"data":[]}`))
		default:
			t.Fatalf("unexpected proxmox path: %s", r.URL.Path)
		}
	}))
	defer configErrorServer.Close()

	client, err := proxmox.NewClient(proxmox.Config{
		BaseURL:     configErrorServer.URL,
		TokenID:     "id",
		TokenSecret: "secret",
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	sut := newTestAPIServer(t)

	_, err = sut.loadProxmoxAssetDetails(
		context.Background(),
		"proxmox-vm-101",
		proxmoxSessionTarget{Kind: "qemu", Node: "pve01", VMID: "101"},
		proxmoxpkg.NewProxmoxRuntimeWithCollector(client, "collector-1"),
	)
	if err == nil {
		t.Fatalf("expected config failure to bubble up as fatal error")
	}

	warningServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/version":
			_, _ = w.Write([]byte(`{"data":{"release":"8.3"}}`))
		case "/api2/json/nodes/pve01/tasks",
			"/api2/json/cluster/ha/resources",
			"/api2/json/cluster/backup",
			"/api2/json/cluster/ceph/status",
			"/api2/json/cluster/ceph/osd":
			_, _ = w.Write([]byte(`{"data":[]}`))
		default:
			t.Fatalf("unexpected proxmox path: %s", r.URL.Path)
		}
	}))
	defer warningServer.Close()

	client, err = proxmox.NewClient(proxmox.Config{
		BaseURL:     warningServer.URL,
		TokenID:     "id",
		TokenSecret: "secret",
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	details, err := sut.loadProxmoxAssetDetails(
		context.Background(),
		"proxmox-storage-local-zfs",
		proxmoxSessionTarget{Kind: "storage", Node: "pve01"},
		proxmoxpkg.NewProxmoxRuntimeWithCollector(client, "collector-1"),
	)
	if err != nil {
		t.Fatalf("did not expect error for storage details with missing storage name warning: %v", err)
	}
	if len(details.Warnings) == 0 || !strings.Contains(strings.Join(details.Warnings, " "), "storage name unavailable") {
		t.Fatalf("expected storage-name warning, got %+v", details.Warnings)
	}
}

func TestLoadProxmoxAssetDetailsLXCWarningsAndCephData(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/version":
			_, _ = w.Write([]byte(`{"data":{"release":"8.4"}}`))
		case "/api2/json/nodes/pve01/lxc/200/config":
			_, _ = w.Write([]byte(`{"data":{"hostname":"ct-200","cores":2}}`))
		case "/api2/json/nodes/pve01/lxc/200/snapshot":
			_, _ = w.Write([]byte(`{"data":[{"name":"snap-1","snaptime":20}]}`))
		case "/api2/json/nodes/pve01/tasks":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"errors":"tasks failed"}`))
		case "/api2/json/cluster/ha/resources":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"errors":"ha failed"}`))
		case "/api2/json/nodes/pve01/lxc/200/firewall/rules":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"errors":"firewall failed"}`))
		case "/api2/json/cluster/backup":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"errors":"backup failed"}`))
		case "/api2/json/cluster/ceph/status":
			_, _ = w.Write([]byte(`{"data":{"health":{"status":"HEALTH_WARN"}}}`))
		case "/api2/json/cluster/ceph/osd":
			_, _ = w.Write([]byte(`{"data":[{"name":"osd.0","in":1,"up":1}]}`))
		default:
			t.Fatalf("unexpected proxmox path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := proxmox.NewClient(proxmox.Config{
		BaseURL:     server.URL,
		TokenID:     "id",
		TokenSecret: "secret",
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	sut := newTestAPIServer(t)
	details, err := sut.loadProxmoxAssetDetails(
		context.Background(),
		"proxmox-ct-200",
		proxmoxSessionTarget{Kind: "lxc", Node: "pve01", VMID: "200"},
		proxmoxpkg.NewProxmoxRuntimeWithCollector(client, "collector-1"),
	)
	if err != nil {
		t.Fatalf("loadProxmoxAssetDetails lxc branch failed: %v", err)
	}
	if len(details.Snapshots) != 1 || details.Snapshots[0].Name != "snap-1" {
		t.Fatalf("expected lxc snapshots to be loaded, got %+v", details.Snapshots)
	}
	if details.CephStatus == nil {
		t.Fatalf("expected ceph status to be included")
	}
	if len(details.CephOSDs) != 1 || details.CephOSDs[0].Name != "osd.0" {
		t.Fatalf("expected ceph osds to be included, got %+v", details.CephOSDs)
	}
	joinedWarnings := strings.Join(details.Warnings, " | ")
	for _, needle := range []string{"tasks unavailable", "ha resources unavailable", "firewall rules unavailable", "backup schedules unavailable"} {
		if !strings.Contains(joinedWarnings, needle) {
			t.Fatalf("expected warning %q in %q", needle, joinedWarnings)
		}
	}
}

func TestLoadProxmoxAssetDetailsSnapshotWarningBranch(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/version":
			_, _ = w.Write([]byte(`{"data":{"release":"8.3"}}`))
		case "/api2/json/nodes/pve01/qemu/101/config":
			_, _ = w.Write([]byte(`{"data":{"name":"vm-101"}}`))
		case "/api2/json/nodes/pve01/qemu/101/snapshot":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"errors":"snapshots failed"}`))
		case "/api2/json/nodes/pve01/tasks", "/api2/json/cluster/ha/resources", "/api2/json/nodes/pve01/qemu/101/firewall/rules", "/api2/json/cluster/backup":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api2/json/cluster/ceph/status", "/api2/json/cluster/ceph/osd":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"node":"no ceph"}}`))
		default:
			t.Fatalf("unexpected proxmox path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := proxmox.NewClient(proxmox.Config{
		BaseURL:     server.URL,
		TokenID:     "id",
		TokenSecret: "secret",
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	sut := newTestAPIServer(t)
	details, err := sut.loadProxmoxAssetDetails(
		context.Background(),
		"proxmox-vm-101",
		proxmoxSessionTarget{Kind: "qemu", Node: "pve01", VMID: "101"},
		proxmoxpkg.NewProxmoxRuntimeWithCollector(client, "collector-1"),
	)
	if err != nil {
		t.Fatalf("loadProxmoxAssetDetails snapshot-warning branch failed: %v", err)
	}
	if !strings.Contains(strings.Join(details.Warnings, " | "), "snapshots unavailable") {
		t.Fatalf("expected snapshots warning, got %+v", details.Warnings)
	}
}

func TestLoadProxmoxAssetDetailsVersionWarningBranch(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/version":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"errors":"version unavailable"}`))
		case "/api2/json/nodes/pve01/qemu/101/config":
			_, _ = w.Write([]byte(`{"data":{"name":"vm-101"}}`))
		case "/api2/json/nodes/pve01/qemu/101/snapshot",
			"/api2/json/nodes/pve01/tasks",
			"/api2/json/cluster/ha/resources",
			"/api2/json/nodes/pve01/qemu/101/firewall/rules",
			"/api2/json/cluster/backup":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api2/json/cluster/ceph/status", "/api2/json/cluster/ceph/osd":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"node":"no ceph"}}`))
		default:
			t.Fatalf("unexpected proxmox path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := proxmox.NewClient(proxmox.Config{
		BaseURL:     server.URL,
		TokenID:     "id",
		TokenSecret: "secret",
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	sut := newTestAPIServer(t)
	details, err := sut.loadProxmoxAssetDetails(
		context.Background(),
		"proxmox-vm-101",
		proxmoxSessionTarget{Kind: "qemu", Node: "pve01", VMID: "101"},
		proxmoxpkg.NewProxmoxRuntimeWithCollector(client, "collector-1"),
	)
	if err != nil {
		t.Fatalf("loadProxmoxAssetDetails failed: %v", err)
	}
	if !strings.Contains(strings.Join(details.Warnings, " | "), "version unavailable") {
		t.Fatalf("expected version warning, got %+v", details.Warnings)
	}
}
