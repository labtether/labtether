package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
)

func TestHandleProcessListBridgesAgentInventory(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()

	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-proc", "linux"))
	defer sut.agentMgr.Unregister("node-proc")

	done := make(chan struct{})
	go func() {
		defer close(done)

		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read outbound: %v", err)
			return
		}
		if outbound.Type != agentmgr.MsgProcessList {
			t.Errorf("outbound type=%q, want %q", outbound.Type, agentmgr.MsgProcessList)
			return
		}

		var req agentmgr.ProcessListData
		if err := json.Unmarshal(outbound.Data, &req); err != nil {
			t.Errorf("decode process list payload: %v", err)
			return
		}
		if req.SortBy != "memory" || req.Limit != 2 {
			t.Errorf("unexpected request %+v", req)
		}

		raw, _ := json.Marshal(agentmgr.ProcessListedData{
			RequestID: req.RequestID,
			Processes: []agentmgr.ProcessInfo{{
				PID:     4242,
				Name:    "postgres",
				User:    "postgres",
				MemPct:  12.5,
				Command: "/usr/bin/postgres",
			}},
		})
		sut.processAgentProcessListed(&agentmgr.AgentConn{AssetID: "node-proc"}, agentmgr.Message{
			Type: agentmgr.MsgProcessListed,
			ID:   req.RequestID,
			Data: raw,
		})
	}()

	req := httptest.NewRequest(http.MethodGet, "/processes/node-proc?sort=memory&limit=2", nil)
	rec := httptest.NewRecorder()
	sut.handleProcesses(rec, req)

	<-done

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var response agentmgr.ProcessListedData
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Processes) != 1 || response.Processes[0].PID != 4242 {
		t.Fatalf("unexpected response %+v", response)
	}
}

func TestHandleProcessKillBridgesAgentResult(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()

	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-proc", "linux"))
	defer sut.agentMgr.Unregister("node-proc")

	done := make(chan struct{})
	go func() {
		defer close(done)

		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read outbound: %v", err)
			return
		}
		if outbound.Type != agentmgr.MsgProcessKill {
			t.Errorf("outbound type=%q, want %q", outbound.Type, agentmgr.MsgProcessKill)
			return
		}
		if strings.TrimSpace(outbound.ID) == "" {
			t.Error("expected process.kill request id")
			return
		}

		var req agentmgr.ProcessKillData
		if err := json.Unmarshal(outbound.Data, &req); err != nil {
			t.Errorf("decode process kill payload: %v", err)
			return
		}
		if req.PID != 4242 || req.Signal != "SIGTERM" {
			t.Errorf("unexpected request %+v", req)
		}

		raw, _ := json.Marshal(agentmgr.ProcessKillResultData{
			PID:     4242,
			Success: true,
		})
		sut.processAgentProcessKillResult(&agentmgr.AgentConn{AssetID: "node-proc"}, agentmgr.Message{
			Type: agentmgr.MsgProcessKillResult,
			ID:   outbound.ID,
			Data: raw,
		})
	}()

	req := httptest.NewRequest(http.MethodPost, "/processes/node-proc/kill", strings.NewReader(`{"pid":4242,"signal":"term"}`))
	rec := httptest.NewRecorder()
	sut.handleProcesses(rec, req)

	<-done

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var response agentmgr.ProcessKillResultData
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Success || response.PID != 4242 {
		t.Fatalf("unexpected response %+v", response)
	}
}

func TestHandleServiceListAndActionBridgeAgentResponses(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()

	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-svc", "linux"))
	defer sut.agentMgr.Unregister("node-svc")

	t.Run("service list", func(t *testing.T) {
		done := make(chan struct{})
		go func() {
			defer close(done)

			var outbound agentmgr.Message
			if err := clientConn.ReadJSON(&outbound); err != nil {
				t.Errorf("read outbound: %v", err)
				return
			}
			if outbound.Type != agentmgr.MsgServiceList {
				t.Errorf("outbound type=%q, want %q", outbound.Type, agentmgr.MsgServiceList)
				return
			}

			var req agentmgr.ServiceListData
			if err := json.Unmarshal(outbound.Data, &req); err != nil {
				t.Errorf("decode service list payload: %v", err)
				return
			}

			raw, _ := json.Marshal(agentmgr.ServiceListedData{
				RequestID: req.RequestID,
				Services: []agentmgr.ServiceInfo{{
					Name:        "sshd",
					Description: "OpenSSH Daemon",
					ActiveState: "active",
					SubState:    "running",
					Enabled:     "enabled",
					LoadState:   "loaded",
				}},
			})
			sut.processAgentServiceListed(&agentmgr.AgentConn{AssetID: "node-svc"}, agentmgr.Message{
				Type: agentmgr.MsgServiceListed,
				ID:   req.RequestID,
				Data: raw,
			})
		}()

		req := httptest.NewRequest(http.MethodGet, "/services/node-svc", nil)
		rec := httptest.NewRecorder()
		sut.handleServices(rec, req)

		<-done

		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}

		var response agentmgr.ServiceListedData
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(response.Services) != 1 || response.Services[0].Name != "sshd" {
			t.Fatalf("unexpected response %+v", response)
		}
	})

	t.Run("service action", func(t *testing.T) {
		done := make(chan struct{})
		go func() {
			defer close(done)

			var outbound agentmgr.Message
			if err := clientConn.ReadJSON(&outbound); err != nil {
				t.Errorf("read outbound: %v", err)
				return
			}
			if outbound.Type != agentmgr.MsgServiceAction {
				t.Errorf("outbound type=%q, want %q", outbound.Type, agentmgr.MsgServiceAction)
				return
			}

			var req agentmgr.ServiceActionData
			if err := json.Unmarshal(outbound.Data, &req); err != nil {
				t.Errorf("decode service action payload: %v", err)
				return
			}
			if req.Action != "restart" || req.Service != "sshd" {
				t.Errorf("unexpected request %+v", req)
			}

			raw, _ := json.Marshal(agentmgr.ServiceResultData{
				RequestID: req.RequestID,
				OK:        true,
				Output:    "restarted",
			})
			sut.processAgentServiceResult(&agentmgr.AgentConn{AssetID: "node-svc"}, agentmgr.Message{
				Type: agentmgr.MsgServiceResult,
				ID:   req.RequestID,
				Data: raw,
			})
		}()

		req := httptest.NewRequest(http.MethodPost, "/services/node-svc/restart", strings.NewReader(`{"service":"sshd"}`))
		rec := httptest.NewRecorder()
		sut.handleServices(rec, req)

		<-done

		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}

		var response agentmgr.ServiceResultData
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if !response.OK || response.Output != "restarted" {
			t.Fatalf("unexpected response %+v", response)
		}
	})
}

func TestHandlePackageListAndActionBridgeAgentResponses(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()

	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-pkg", "linux"))
	defer sut.agentMgr.Unregister("node-pkg")

	t.Run("package list", func(t *testing.T) {
		done := make(chan struct{})
		go func() {
			defer close(done)

			var outbound agentmgr.Message
			if err := clientConn.ReadJSON(&outbound); err != nil {
				t.Errorf("read outbound: %v", err)
				return
			}
			if outbound.Type != agentmgr.MsgPackageList {
				t.Errorf("outbound type=%q, want %q", outbound.Type, agentmgr.MsgPackageList)
				return
			}

			var req agentmgr.PackageListData
			if err := json.Unmarshal(outbound.Data, &req); err != nil {
				t.Errorf("decode package list payload: %v", err)
				return
			}

			raw, _ := json.Marshal(agentmgr.PackageListedData{
				RequestID: req.RequestID,
				Packages: []agentmgr.PackageInfo{{
					Name:    "jq",
					Version: "1.7",
					Status:  "installed",
				}},
			})
			sut.processAgentPackageListed(&agentmgr.AgentConn{AssetID: "node-pkg"}, agentmgr.Message{
				Type: agentmgr.MsgPackageListed,
				ID:   req.RequestID,
				Data: raw,
			})
		}()

		req := httptest.NewRequest(http.MethodGet, "/packages/node-pkg", nil)
		rec := httptest.NewRecorder()
		sut.handlePackages(rec, req)

		<-done

		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}

		var response agentmgr.PackageListedData
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(response.Packages) != 1 || response.Packages[0].Name != "jq" {
			t.Fatalf("unexpected response %+v", response)
		}
	})

	t.Run("package action", func(t *testing.T) {
		done := make(chan struct{})
		go func() {
			defer close(done)

			var outbound agentmgr.Message
			if err := clientConn.ReadJSON(&outbound); err != nil {
				t.Errorf("read outbound: %v", err)
				return
			}
			if outbound.Type != agentmgr.MsgPackageAction {
				t.Errorf("outbound type=%q, want %q", outbound.Type, agentmgr.MsgPackageAction)
				return
			}

			var req agentmgr.PackageActionData
			if err := json.Unmarshal(outbound.Data, &req); err != nil {
				t.Errorf("decode package action payload: %v", err)
				return
			}
			if req.Action != "install" {
				t.Errorf("action=%q, want install", req.Action)
			}
			if got := strings.Join(req.Packages, ","); got != "jq,curl" {
				t.Errorf("packages=%q, want jq,curl", got)
			}

			raw, _ := json.Marshal(agentmgr.PackageResultData{
				RequestID:      req.RequestID,
				OK:             true,
				Output:         "installed",
				RebootRequired: true,
			})
			sut.processAgentPackageResult(&agentmgr.AgentConn{AssetID: "node-pkg"}, agentmgr.Message{
				Type: agentmgr.MsgPackageResult,
				ID:   req.RequestID,
				Data: raw,
			})
		}()

		req := httptest.NewRequest(http.MethodPost, "/packages/node-pkg/install", strings.NewReader(`{"packages":[" jq ","jq"],"package":"curl"}`))
		rec := httptest.NewRecorder()
		sut.handlePackages(rec, req)

		<-done

		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}

		var response agentmgr.PackageResultData
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if !response.OK || !response.RebootRequired || response.Output != "installed" {
			t.Fatalf("unexpected response %+v", response)
		}
	})
}

func TestHandleJournalLogsBridgesAgentEntries(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()

	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-journal", "linux"))
	defer sut.agentMgr.Unregister("node-journal")

	done := make(chan struct{})
	go func() {
		defer close(done)

		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read outbound: %v", err)
			return
		}
		if outbound.Type != agentmgr.MsgJournalQuery {
			t.Errorf("outbound type=%q, want %q", outbound.Type, agentmgr.MsgJournalQuery)
			return
		}

		var req agentmgr.JournalQueryData
		if err := json.Unmarshal(outbound.Data, &req); err != nil {
			t.Errorf("decode journal query payload: %v", err)
			return
		}
		if req.Since != "1h ago" || req.Until != "now" || req.Unit != "sshd.service" || req.Priority != "err" || req.Search != "denied" || req.Limit != 5 {
			t.Errorf("unexpected request %+v", req)
		}

		raw, _ := json.Marshal(agentmgr.JournalEntriesData{
			RequestID: req.RequestID,
			Entries: []agentmgr.LogStreamData{{
				Timestamp: "2026-03-08T12:00:00Z",
				Level:     "error",
				Message:   "denied password",
				Source:    "sshd",
			}},
		})
		sut.processAgentJournalEntries(&agentmgr.AgentConn{AssetID: "node-journal"}, agentmgr.Message{
			Type: agentmgr.MsgJournalEntries,
			ID:   req.RequestID,
			Data: raw,
		})
	}()

	req := httptest.NewRequest(http.MethodGet, "/logs/journal/node-journal?since=1h%20ago&until=now&unit=sshd.service&priority=err&q=denied&limit=5", nil)
	rec := httptest.NewRecorder()
	sut.handleJournalLogs(rec, req)

	<-done

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var response struct {
		Entries []agentmgr.LogStreamData `json:"entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Entries) != 1 || response.Entries[0].Source != "sshd" {
		t.Fatalf("unexpected response %+v", response)
	}
}
