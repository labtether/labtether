package resources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/agentmgr"
)

func fileRequestTestAgent(t *testing.T) (*Deps, *agentmgr.AgentConn, *websocket.Conn) {
	t.Helper()
	server, client, cleanup := createWoLWebSocketPair(t)
	t.Cleanup(cleanup)
	manager := agentmgr.NewManager()
	conn := agentmgr.NewAgentConn(server, "file-agent", "linux")
	manager.Register(conn)
	t.Cleanup(func() { manager.Unregister("file-agent") })
	if err := client.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	return &Deps{AgentMgr: manager, FileBridges: &sync.Map{}}, conn, client
}

func readFileRequest(t *testing.T, client *websocket.Conn) agentmgr.Message {
	t.Helper()
	var msg agentmgr.Message
	if err := client.ReadJSON(&msg); err != nil {
		t.Fatal(err)
	}
	return msg
}

func awaitFileHandler(t *testing.T, deps *Deps, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		// Unblock the original broken handler too, so a failed regression leaves
		// no 30-second waiter in the package process.
		deps.FileBridges.Range(func(_, raw any) bool {
			bridge := raw.(*FileBridge)
			select {
			case bridge.Ch <- agentmgr.Message{Data: json.RawMessage(`{"error":"test cleanup"}`)}:
			default:
			}
			return true
		})
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("handler did not stop after cleanup response")
		}
		t.Fatal("cancelled file handler kept waiting for the agent")
	}
	deps.FileBridges.Range(func(_, _ any) bool {
		t.Error("completed request left a response bridge")
		return true
	})
}

func TestFileUploadCancellationReleasesConfirmationWait(t *testing.T) {
	deps, _, client := fileRequestTestAgent(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/files/file-agent/upload?path=/empty.bin", strings.NewReader("")).WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { defer close(done); deps.HandleFileUpload(rec, req, "file-agent") }()
	msg := readFileRequest(t, client)
	var chunk agentmgr.FileWriteData
	if err := json.Unmarshal(msg.Data, &chunk); err != nil || !chunk.Done {
		t.Fatalf("missing EOF write: %s; error=%v", msg.Data, err)
	}
	cancel()
	awaitFileHandler(t, deps, done)
	if rec.Body.Len() != 0 {
		t.Fatalf("cancelled request wrote a response: %s", rec.Body.String())
	}
}

func TestFileDownloadCancellationReleasesWaitAndSpool(t *testing.T) {
	for _, started := range []bool{false, true} {
		name := "before-first-chunk"
		if started {
			name = "while-spooling"
		}
		t.Run(name, func(t *testing.T) {
			temp := t.TempDir()
			for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
				t.Setenv(key, temp)
			}
			deps, conn, client := fileRequestTestAgent(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			req := httptest.NewRequest(http.MethodGet, "/files/file-agent/download?path=/qa.bin", nil).WithContext(ctx)
			rec := httptest.NewRecorder()
			done := make(chan struct{})
			go func() { defer close(done); deps.HandleFileDownloadWithTimeout(rec, req, "file-agent", 3*time.Second) }()
			msg := readFileRequest(t, client)
			var read agentmgr.FileReadData
			if err := json.Unmarshal(msg.Data, &read); err != nil {
				t.Fatal(err)
			}
			if started {
				data, err := json.Marshal(agentmgr.FileDataPayload{RequestID: read.RequestID, Data: base64.StdEncoding.EncodeToString([]byte("partial"))})
				if err != nil {
					t.Fatal(err)
				}
				deps.ProcessAgentFileData(conn, agentmgr.Message{Type: agentmgr.MsgFileData, ID: read.RequestID, Data: data})
				deadline := time.Now().Add(time.Second)
				for {
					paths, err := filepath.Glob(filepath.Join(temp, "labtether-download-*"))
					if err != nil {
						t.Fatal(err)
					}
					if len(paths) == 1 {
						info, err := os.Stat(paths[0])
						if err == nil && info.Size() == 7 {
							break
						}
					}
					if time.Now().After(deadline) {
						t.Fatal("first chunk did not reach spool")
					}
					time.Sleep(time.Millisecond)
				}
			}
			cancel()
			awaitFileHandler(t, deps, done)
			entries, err := os.ReadDir(temp)
			if err != nil || len(entries) != 0 {
				t.Fatalf("download spool remained: entries=%v err=%v", entries, err)
			}
			// Once the first chunk arrived the download has started streaming,
			// so only a request cancelled before it must have no response.
			if !started && rec.Body.Len() != 0 {
				t.Fatalf("cancelled request wrote a response: %s", rec.Body.String())
			}
		})
	}
}
