package resources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

func TestFileUploadRequiresExactAgentByteCount(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		ack  int64
		want int
	}{
		{"complete", strings.Repeat("x", fileChunkSizeHub+1), fileChunkSizeHub + 1, http.StatusOK},
		{"empty", "", 0, http.StatusOK},
		{"truncated", "payload", 1, http.StatusBadGateway},
		{"overcount", "payload", 8, http.StatusBadGateway},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, client, cleanup := createWoLWebSocketPair(t)
			defer cleanup()
			manager := agentmgr.NewManager()
			conn := agentmgr.NewAgentConn(server, "node", "linux")
			manager.Register(conn)
			defer manager.Unregister("node")
			deps := &Deps{AgentMgr: manager, FileBridges: &sync.Map{}}
			done := make(chan struct{})
			go func() {
				defer close(done)
				_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
				for {
					var msg agentmgr.Message
					if err := client.ReadJSON(&msg); err != nil {
						t.Errorf("read upload: %v", err)
						return
					}
					var chunk agentmgr.FileWriteData
					if err := json.Unmarshal(msg.Data, &chunk); err != nil {
						t.Errorf("decode upload: %v", err)
						return
					}
					if chunk.Done {
						data, _ := json.Marshal(agentmgr.FileWrittenData{RequestID: chunk.RequestID, BytesWritten: tc.ack})
						deps.ProcessAgentFileWritten(conn, agentmgr.Message{Type: agentmgr.MsgFileWritten, Data: data})
						return
					}
				}
			}()
			rec := httptest.NewRecorder()
			deps.HandleFileUpload(rec, httptest.NewRequest(http.MethodPost, "/files/node/upload?path=/target", strings.NewReader(tc.body)), "node")
			<-done
			if rec.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestFileDownloadPreservesWindowsUnicodeFilename(t *testing.T) {
	path := `C:\Users\Michael\Desktop\résumé.txt `
	server, client, cleanup := createWoLWebSocketPair(t)
	defer cleanup()
	manager := agentmgr.NewManager()
	conn := agentmgr.NewAgentConn(server, "node", "windows")
	manager.Register(conn)
	defer manager.Unregister("node")
	deps := &Deps{AgentMgr: manager, FileBridges: &sync.Map{}}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
		var msg agentmgr.Message
		if err := client.ReadJSON(&msg); err != nil {
			t.Errorf("read download request: %v", err)
			return
		}
		var request agentmgr.FileReadData
		if err := json.Unmarshal(msg.Data, &request); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if request.Path != path {
			t.Errorf("download path changed: got %q want %q", request.Path, path)
		}
		data, _ := json.Marshal(agentmgr.FileDataPayload{
			RequestID: request.RequestID, Data: base64.StdEncoding.EncodeToString([]byte("contents")), Done: true,
		})
		deps.ProcessAgentFileData(conn, agentmgr.Message{Type: agentmgr.MsgFileData, Data: data})
	}()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/files/node/download?path="+url.QueryEscape(path), nil)
	deps.HandleFileDownload(rec, req, "node")
	<-done
	_, params, err := mime.ParseMediaType(rec.Header().Get("Content-Disposition"))
	if err != nil || params["filename"] != "résumé.txt " || rec.Code != http.StatusOK || rec.Body.String() != "contents" {
		t.Fatalf("download status=%d params=%v err=%v body=%q", rec.Code, params, err, rec.Body.String())
	}
}

func TestFileDownloadCancellationStopsWaitingAndDropsBridge(t *testing.T) {
	server, client, cleanup := createWoLWebSocketPair(t)
	defer cleanup()
	manager := agentmgr.NewManager()
	conn := agentmgr.NewAgentConn(server, "node", "linux")
	manager.Register(conn)
	defer manager.Unregister("node")
	deps := &Deps{AgentMgr: manager, FileBridges: &sync.Map{}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	req := httptest.NewRequest(http.MethodGet, "/files/node/download?path=/target", nil).WithContext(ctx)
	go func() {
		defer close(done)
		deps.HandleFileDownload(httptest.NewRecorder(), req, "node")
	}()
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	var msg agentmgr.Message
	if err := client.ReadJSON(&msg); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancelled download is still waiting for the agent")
	}
	deps.FileBridges.Range(func(_, _ any) bool {
		t.Error("cancelled download retained a response bridge")
		return false
	})
}
