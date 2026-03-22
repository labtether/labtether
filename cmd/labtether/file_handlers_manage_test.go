package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

func TestHandleFileListBridgesAgentSuccess(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()

	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-list", "linux"))
	defer sut.agentMgr.Unregister("node-list")

	done := make(chan struct{})
	go func() {
		defer close(done)

		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read outbound: %v", err)
			return
		}
		if outbound.Type != agentmgr.MsgFileList {
			t.Errorf("outbound type=%q, want %q", outbound.Type, agentmgr.MsgFileList)
			return
		}

		var listReq agentmgr.FileListData
		if err := json.Unmarshal(outbound.Data, &listReq); err != nil {
			t.Errorf("decode file list payload: %v", err)
			return
		}
		if listReq.Path != "/tmp/data" {
			t.Errorf("path=%q, want /tmp/data", listReq.Path)
		}
		if !listReq.ShowHidden {
			t.Errorf("expected show_hidden=true")
		}

		raw, _ := json.Marshal(agentmgr.FileListedData{
			RequestID: listReq.RequestID,
			Path:      "/tmp/data",
			Entries: []agentmgr.FileEntry{
				{Name: "visible.txt", Size: 12, Mode: "-rw-r--r--"},
			},
		})
		sut.processAgentFileListed(&agentmgr.AgentConn{AssetID: "node-list"}, agentmgr.Message{
			Type: agentmgr.MsgFileListed,
			ID:   listReq.RequestID,
			Data: raw,
		})
	}()

	req := httptest.NewRequest(http.MethodGet, "/files/node-list/list?path=%2Ftmp%2Fdata&show_hidden=true", nil)
	rec := httptest.NewRecorder()
	sut.handleFileList(rec, req, "node-list")

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for agent list bridge")
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var response agentmgr.FileListedData
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Path != "/tmp/data" {
		t.Fatalf("path=%q, want /tmp/data", response.Path)
	}
	if len(response.Entries) != 1 || response.Entries[0].Name != "visible.txt" {
		t.Fatalf("unexpected entries: %+v", response.Entries)
	}
}

func TestHandleFileMkdirBridgesAgentSuccess(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()

	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-mkdir", "linux"))
	defer sut.agentMgr.Unregister("node-mkdir")

	done := make(chan struct{})
	go func() {
		defer close(done)

		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read outbound: %v", err)
			return
		}
		if outbound.Type != agentmgr.MsgFileMkdir {
			t.Errorf("outbound type=%q, want %q", outbound.Type, agentmgr.MsgFileMkdir)
			return
		}

		var mkdirReq agentmgr.FileMkdirData
		if err := json.Unmarshal(outbound.Data, &mkdirReq); err != nil {
			t.Errorf("decode file mkdir payload: %v", err)
			return
		}
		if mkdirReq.Path != "/tmp/new-dir" {
			t.Errorf("path=%q, want /tmp/new-dir", mkdirReq.Path)
		}

		raw, _ := json.Marshal(agentmgr.FileResultData{
			RequestID: mkdirReq.RequestID,
			OK:        true,
		})
		sut.processAgentFileResult(&agentmgr.AgentConn{AssetID: "node-mkdir"}, agentmgr.Message{
			Type: agentmgr.MsgFileResult,
			ID:   mkdirReq.RequestID,
			Data: raw,
		})
	}()

	req := httptest.NewRequest(http.MethodPost, "/files/node-mkdir/mkdir?path=%2Ftmp%2Fnew-dir", nil)
	rec := httptest.NewRecorder()
	sut.handleFileMkdir(rec, req, "node-mkdir")

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for agent mkdir bridge")
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var response agentmgr.FileResultData
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.OK {
		t.Fatalf("expected ok=true, got %+v", response)
	}
}

func TestHandleFileRenamePropagatesAgentError(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()

	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-rename", "linux"))
	defer sut.agentMgr.Unregister("node-rename")

	done := make(chan struct{})
	go func() {
		defer close(done)

		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read outbound: %v", err)
			return
		}
		if outbound.Type != agentmgr.MsgFileRename {
			t.Errorf("outbound type=%q, want %q", outbound.Type, agentmgr.MsgFileRename)
			return
		}

		var renameReq agentmgr.FileRenameData
		if err := json.Unmarshal(outbound.Data, &renameReq); err != nil {
			t.Errorf("decode file rename payload: %v", err)
			return
		}
		if renameReq.OldPath != "/tmp/old.txt" || renameReq.NewPath != "/tmp/new.txt" {
			t.Errorf("unexpected rename payload: %+v", renameReq)
		}

		raw, _ := json.Marshal(agentmgr.FileResultData{
			RequestID: renameReq.RequestID,
			OK:        false,
			Error:     "destination already exists",
		})
		sut.processAgentFileResult(&agentmgr.AgentConn{AssetID: "node-rename"}, agentmgr.Message{
			Type: agentmgr.MsgFileResult,
			ID:   renameReq.RequestID,
			Data: raw,
		})
	}()

	req := httptest.NewRequest(http.MethodPost, "/files/node-rename/rename?old_path=%2Ftmp%2Fold.txt&new_path=%2Ftmp%2Fnew.txt", nil)
	rec := httptest.NewRecorder()
	sut.handleFileRename(rec, req, "node-rename")

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for agent rename bridge")
	}

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "destination already exists") {
		t.Fatalf("expected rename error, got %s", rec.Body.String())
	}
}
