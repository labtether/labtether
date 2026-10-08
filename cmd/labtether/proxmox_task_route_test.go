package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestHandleProxmoxTaskLogUsesCollectorQueryParam(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	var collectorOneCalls atomic.Int32

	collectorOne := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		collectorOneCalls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"errors":"wrong collector selected"}`))
	}))
	defer collectorOne.Close()

	collectorTwo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve02/tasks/UPID-2/log":
			if got := r.URL.Query().Get("limit"); got != "500" {
				t.Fatalf("expected limit=500 query, got %q", got)
			}
			_, _ = w.Write([]byte(`{"data":[{"n":1,"t":"line-one"},{"n":2,"t":"line-two"}]}`))
		default:
			t.Fatalf("unexpected proxmox request path: %s", r.URL.Path)
		}
	}))
	defer collectorTwo.Close()

	sut := newTestAPIServer(t)
	configureDualProxmoxCollectors(t, sut, collectorOne.URL, collectorTwo.URL)

	req := httptest.NewRequest(http.MethodGet, "/proxmox/tasks/pve02/UPID-2/log?collector_id=collector-proxmox-2", nil)
	rec := httptest.NewRecorder()
	sut.handleProxmoxTaskLog(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if !strings.Contains(rec.Body.String(), "line-one\\nline-two\\n") {
		t.Fatalf("expected proxmox task log payload, got %s", rec.Body.String())
	}
	if collectorOneCalls.Load() != 0 {
		t.Fatalf("expected collector one to receive no requests, got %d", collectorOneCalls.Load())
	}
}

func TestHandleProxmoxTaskStopUsesCollectorQueryParam(t *testing.T) {
	var collectorOneCalls atomic.Int32

	collectorOne := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		collectorOneCalls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"errors":"wrong collector selected"}`))
	}))
	defer collectorOne.Close()

	collectorTwo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("expected DELETE, got %s", r.Method)
		}
		switch r.URL.Path {
		case "/api2/json/nodes/pve02/tasks/UPID-2":
			_, _ = w.Write([]byte(`{"data":"OK"}`))
		default:
			t.Fatalf("unexpected proxmox request path: %s", r.URL.Path)
		}
	}))
	defer collectorTwo.Close()

	sut := newTestAPIServer(t)
	configureDualProxmoxCollectors(t, sut, collectorOne.URL, collectorTwo.URL)

	req := httptest.NewRequest(http.MethodPost, "/proxmox/tasks/pve02/UPID-2/stop?collector_id=collector-proxmox-2", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "owner", "owner"))
	rec := httptest.NewRecorder()
	sut.handleProxmoxTaskStop(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if !strings.Contains(rec.Body.String(), `"status":"stopped"`) {
		t.Fatalf("expected stopped status payload, got %s", rec.Body.String())
	}
	if collectorOneCalls.Load() != 0 {
		t.Fatalf("expected collector one to receive no requests, got %d", collectorOneCalls.Load())
	}
}

func TestHandleProxmoxClusterStatusUsesCollectorQueryParam(t *testing.T) {
	var collectorOneCalls atomic.Int32

	collectorOne := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		collectorOneCalls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"errors":"wrong collector selected"}`))
	}))
	defer collectorOne.Close()

	collectorTwo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/cluster/status":
			_, _ = w.Write([]byte(`{"data":[{"type":"node","name":"pve02","online":1}]}`))
		default:
			t.Fatalf("unexpected proxmox request path: %s", r.URL.Path)
		}
	}))
	defer collectorTwo.Close()

	sut := newTestAPIServer(t)
	configureDualProxmoxCollectors(t, sut, collectorOne.URL, collectorTwo.URL)

	req := httptest.NewRequest(http.MethodGet, "/proxmox/cluster/status?collector_id=collector-proxmox-2", nil)
	rec := httptest.NewRecorder()
	sut.handleProxmoxClusterStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if !strings.Contains(rec.Body.String(), `"pve02"`) {
		t.Fatalf("expected cluster status payload from collector two, got %s", rec.Body.String())
	}
	if collectorOneCalls.Load() != 0 {
		t.Fatalf("expected collector one to receive no requests, got %d", collectorOneCalls.Load())
	}
}

func TestHandleProxmoxNodeNetworkUsesCollectorQueryParam(t *testing.T) {
	var collectorOneCalls atomic.Int32

	collectorOne := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		collectorOneCalls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"errors":"wrong collector selected"}`))
	}))
	defer collectorOne.Close()

	collectorTwo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve02/network":
			_, _ = w.Write([]byte(`{"data":[{"iface":"vmbr0","active":1}]}`))
		default:
			t.Fatalf("unexpected proxmox request path: %s", r.URL.Path)
		}
	}))
	defer collectorTwo.Close()

	sut := newTestAPIServer(t)
	configureDualProxmoxCollectors(t, sut, collectorOne.URL, collectorTwo.URL)

	req := httptest.NewRequest(http.MethodGet, "/proxmox/nodes/pve02/network?collector_id=collector-proxmox-2", nil)
	rec := httptest.NewRecorder()
	sut.handleProxmoxNodeNetwork(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if !strings.Contains(rec.Body.String(), `"vmbr0"`) {
		t.Fatalf("expected proxmox node network payload from collector two, got %s", rec.Body.String())
	}
	if collectorOneCalls.Load() != 0 {
		t.Fatalf("expected collector one to receive no requests, got %d", collectorOneCalls.Load())
	}
}

func TestHandleProxmoxRouteAndGuardBranches(t *testing.T) {
	sut := newTestAPIServer(t)

	taskReq := httptest.NewRequest(http.MethodGet, "/proxmox/tasks/", nil)
	taskRec := httptest.NewRecorder()
	sut.handleProxmoxTaskRoutes(taskRec, taskReq)
	if taskRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing task path, got %d", taskRec.Code)
	}

	taskReq = httptest.NewRequest(http.MethodPost, "/proxmox/tasks/pve01/UPID-1/log", nil)
	taskRec = httptest.NewRecorder()
	sut.handleProxmoxTaskRoutes(taskRec, taskReq)
	if taskRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected log route dispatch to enforce method guard (405), got %d", taskRec.Code)
	}

	taskReq = httptest.NewRequest(http.MethodGet, "/proxmox/tasks/pve01/UPID-1/stop", nil)
	taskRec = httptest.NewRecorder()
	sut.handleProxmoxTaskRoutes(taskRec, taskReq)
	if taskRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected stop route dispatch to enforce method guard (405), got %d", taskRec.Code)
	}

	nodeReq := httptest.NewRequest(http.MethodGet, "/proxmox/nodes/", nil)
	nodeRec := httptest.NewRecorder()
	sut.handleProxmoxNodeRoutes(nodeRec, nodeReq)
	if nodeRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing node path, got %d", nodeRec.Code)
	}

	// POST to network is now allowed (create interface) — without a configured
	// collector, it returns 502 (bad gateway) instead of the old 405.
	nodeReq = httptest.NewRequest(http.MethodPost, "/proxmox/nodes/pve01/network", nil)
	nodeRec = httptest.NewRecorder()
	sut.handleProxmoxNodeRoutes(nodeRec, nodeReq)
	if nodeRec.Code != http.StatusBadGateway {
		t.Fatalf("expected network POST to return 502 (no collector), got %d", nodeRec.Code)
	}
}

func TestHandleProxmoxTaskAndNodeHandlersGuardBranches(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodPost, "/proxmox/tasks/pve01/UPID-1/log", nil)
	rec := httptest.NewRecorder()
	sut.handleProxmoxTaskLog(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for task log method guard, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/proxmox/tasks/pve01/UPID-1", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxTaskLog(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for malformed task log path, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/proxmox/tasks//UPID-1/log", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxTaskLog(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing node in task log path, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/proxmox/tasks/pve01/UPID-1/log", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxTaskLog(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when task log runtime is unavailable, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/proxmox/tasks/pve01/UPID-1/stop", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxTaskStop(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for task stop method guard, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/proxmox/tasks/pve01/UPID-1", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "owner", "owner"))
	rec = httptest.NewRecorder()
	sut.handleProxmoxTaskStop(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for malformed task stop path, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/proxmox/tasks//UPID-1/stop", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "owner", "owner"))
	rec = httptest.NewRecorder()
	sut.handleProxmoxTaskStop(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing node in task stop path, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/proxmox/tasks/pve01/UPID-1/stop", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "owner", "owner"))
	rec = httptest.NewRecorder()
	sut.handleProxmoxTaskStop(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when task stop runtime is unavailable, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/proxmox/cluster/status", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxClusterStatus(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for cluster status method guard, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/proxmox/cluster/status", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxClusterStatus(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when cluster runtime is unavailable, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/proxmox/nodes/pve01/network", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxNodeNetwork(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for node network method guard, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/proxmox/nodes/pve01/iface", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxNodeNetwork(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for malformed node network path, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/proxmox/nodes//network", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxNodeNetwork(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing node on network path, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/proxmox/nodes/pve01/network", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxNodeNetwork(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when node network runtime is unavailable, got %d", rec.Code)
	}
}

func TestProxmoxTaskAndNodeHandlersAdditionalErrorBranches(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/proxmox/tasks/", nil)
	rec := httptest.NewRecorder()
	sut.handleProxmoxTaskLog(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing task-log path, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/proxmox/tasks/", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "owner", "owner"))
	rec = httptest.NewRecorder()
	sut.handleProxmoxTaskStop(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing task-stop path, got %d", rec.Code)
	}

	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/tasks/UPID-1/log":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"errors":"task log unavailable"}`))
		case "/api2/json/nodes/pve01/tasks/UPID-1":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"errors":"stop failed"}`))
		case "/api2/json/cluster/status":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"errors":"cluster unavailable"}`))
		case "/api2/json/nodes/pve01/network":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"errors":"network unavailable"}`))
		default:
			t.Fatalf("unexpected proxmox path: %s", r.URL.Path)
		}
	}))
	defer errorServer.Close()
	configureSingleProxmoxCollector(t, sut, errorServer.URL, "collector-proxmox-1")

	req = httptest.NewRequest(http.MethodGet, "/proxmox/tasks/pve01/UPID-1/log", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxTaskLog(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when proxmox task-log fetch fails, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/proxmox/tasks/pve01/UPID-1/stop", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "owner", "owner"))
	rec = httptest.NewRecorder()
	sut.handleProxmoxTaskStop(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when proxmox task stop fails, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/proxmox/cluster/status", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxClusterStatus(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when cluster status fetch fails, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/proxmox/nodes/pve01/network", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxNodeNetwork(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when node network fetch fails, got %d", rec.Code)
	}
}
