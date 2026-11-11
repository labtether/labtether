package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/assets"
)

// writeProxmoxStatusLine answers the way Proxmox does: its own message in
// the HTTP status line, with {"data":null} as the body.
func writeProxmoxStatusLine(t *testing.T, w http.ResponseWriter, code int, reason string) {
	t.Helper()
	conn, buf, err := w.(http.Hijacker).Hijack()
	if err != nil {
		t.Fatalf("hijack: %v", err)
	}
	defer conn.Close()
	body := `{"data":null}`
	_, _ = buf.WriteString("HTTP/1.1 " + strconv.Itoa(code) + " " + reason + "\r\n" +
		"Content-Type: application/json\r\nConnection: close\r\n" +
		"Content-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body)
	_ = buf.Flush()
}

func proxmoxErrorText(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return body.Error
}

// The Ceph pools route asked Proxmox for /cluster/ceph/pools, which Proxmox
// does not have, so it always failed as "An internal error occurred.".
func TestProxmoxCephPoolsReadsANodesPools(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/cluster/resources":
			_, _ = w.Write([]byte(`{"data":[{"type":"node","node":"pve01","status":"offline"},{"type":"node","node":"pve02","status":"online"}]}`))
		case "/api2/json/nodes/pve02/ceph/pool":
			_, _ = w.Write([]byte(`{"data":[{"pool_name":"rbd","size":3}]}`))
		default:
			w.WriteHeader(http.StatusNotImplemented)
			_, _ = w.Write([]byte(`{"data":null}`))
		}
	}))
	defer server.Close()
	sut := newTestAPIServer(t)
	configureSingleProxmoxCollector(t, sut, server.URL, "collector-proxmox-1")

	rec := httptest.NewRecorder()
	sut.ensureProxmoxDeps().HandleProxmoxCeph(rec, httptest.NewRequest(http.MethodGet, "/proxmox/ceph/pools", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"pool_name":"rbd"`) {
		t.Fatalf("got %d %s, want the node's pools", rec.Code, rec.Body.String())
	}
}

func TestProxmoxCephSaysWhenCephIsNotSetUp(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/cluster/resources":
			_, _ = w.Write([]byte(`{"data":[{"type":"node","node":"pve01","status":"online"}]}`))
		case "/api2/json/cluster/ceph/status", "/api2/json/nodes/pve01/ceph/pool":
			writeProxmoxStatusLine(t, w, http.StatusInternalServerError, "binary not installed: /usr/bin/ceph-mon")
		default:
			t.Errorf("unexpected proxmox path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	sut := newTestAPIServer(t)
	configureSingleProxmoxCollector(t, sut, server.URL, "collector-proxmox-1")

	for name, call := range map[string]func(*httptest.ResponseRecorder){
		"pools": func(rec *httptest.ResponseRecorder) {
			sut.ensureProxmoxDeps().HandleProxmoxCeph(rec, httptest.NewRequest(http.MethodGet, "/proxmox/ceph/pools", nil))
		},
		"status": func(rec *httptest.ResponseRecorder) {
			sut.handleProxmoxCephStatus(rec, httptest.NewRequest(http.MethodGet, "/proxmox/ceph/status", nil))
		},
	} {
		rec := httptest.NewRecorder()
		call(rec)
		if rec.Code != http.StatusNotFound || proxmoxErrorText(t, rec) != "Ceph is not set up on this Proxmox cluster." {
			t.Fatalf("%s: got %d %s, want 404 saying Ceph is not set up", name, rec.Code, rec.Body.String())
		}
	}
}

func TestProxmoxCephPoolsWithoutAVisibleNodeSaysSo(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/cluster/resources" {
			t.Errorf("unexpected proxmox path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"type":"qemu","node":"pve01","vmid":100,"status":"running"}]}`))
	}))
	defer server.Close()
	sut := newTestAPIServer(t)
	configureSingleProxmoxCollector(t, sut, server.URL, "collector-proxmox-1")

	rec := httptest.NewRecorder()
	sut.ensureProxmoxDeps().HandleProxmoxCeph(rec, httptest.NewRequest(http.MethodGet, "/proxmox/ceph/pools", nil))
	if rec.Code != http.StatusConflict || !strings.Contains(proxmoxErrorText(t, rec), "cannot see an online Proxmox node") {
		t.Fatalf("got %d %s, want 409 in words", rec.Code, rec.Body.String())
	}
}

// A login that may read the guests but not the node status lost the whole
// device page with "Could not reach Proxmox".
func TestProxmoxDetailsKeepWhatTheLoginMayRead(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/version":
			_, _ = w.Write([]byte(`{"data":{"release":"9.2"}}`))
		case "/api2/json/nodes/pve01/status", "/api2/json/nodes/pve01/firewall/rules", "/api2/json/nodes/pve01/disks/zfs":
			writeProxmoxStatusLine(t, w, http.StatusForbidden, "Permission check failed (/nodes/pve01, Sys.Audit)")
		case "/api2/json/nodes/pve01/tasks", "/api2/json/cluster/ha/resources", "/api2/json/cluster/backup":
			_, _ = w.Write([]byte(`{"data":[]}`))
		default:
			w.WriteHeader(http.StatusNotImplemented)
			_, _ = w.Write([]byte(`{"data":null}`))
		}
	}))
	defer server.Close()
	sut := newTestAPIServer(t)
	configureSingleProxmoxCollector(t, sut, server.URL, "collector-proxmox-1")
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-node-pve01", Type: "hypervisor-node", Name: "pve01", Source: "proxmox", Status: "online",
		Metadata: map[string]string{"proxmox_type": "node", "node": "pve01", "collector_id": "collector-proxmox-1"},
	}); err != nil {
		t.Fatalf("seed node: %v", err)
	}

	rec := httptest.NewRecorder()
	sut.handleProxmoxAssets(rec, httptest.NewRequest(http.MethodGet, "/proxmox/assets/proxmox-node-pve01/details", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d %s, want the page with what the login may read", rec.Code, rec.Body.String())
	}
	var details struct {
		Version  string   `json:"version"`
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &details); err != nil {
		t.Fatalf("decode: %v", err)
	}
	joined := strings.Join(details.Warnings, " | ")
	if details.Version != "9.2" || !strings.Contains(joined, "Proxmox refused the node status for the saved login.") ||
		!strings.Contains(joined, "Proxmox refused the firewall rules for the saved login.") {
		t.Fatalf("got version %q warnings %q", details.Version, joined)
	}
}
