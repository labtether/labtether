package resources

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A revoked or replaced agent must release every pending file request, rather
// than making the browser wait 30 seconds for a socket that no longer exists.
func TestFileDisconnectWakesPendingRequests(t *testing.T) {
	for _, tc := range []struct {
		name, method, query string
		handler             func(*Deps, http.ResponseWriter, *http.Request, string)
	}{
		{"list", http.MethodGet, "path=/qa", (*Deps).HandleFileList},
		{"download", http.MethodGet, "path=/qa.bin", (*Deps).HandleFileDownload},
		{"upload", http.MethodPost, "path=/empty.bin", (*Deps).HandleFileUpload},
		{"mkdir", http.MethodPost, "path=/qa", (*Deps).HandleFileMkdir},
		{"delete", http.MethodDelete, "path=/qa", (*Deps).HandleFileDelete},
		{"rename", http.MethodPost, "old_path=/qa&new_path=/renamed", (*Deps).HandleFileRename},
		{"copy", http.MethodPost, "src_path=/qa&dst_path=/copy", (*Deps).HandleFileCopy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps, _, client := fileRequestTestAgent(t)
			req := httptest.NewRequest(tc.method, "/files/file-agent/"+tc.name+"?"+tc.query, strings.NewReader(""))
			rec := httptest.NewRecorder()
			done := make(chan struct{})
			go func() { defer close(done); tc.handler(deps, rec, req, "file-agent") }()
			msg := readFileRequest(t, client)
			if msg.ID == "" {
				t.Fatal("file request missing correlation ID")
			}
			unrelated := NewFileBridge(1, "another-agent")
			deps.FileBridges.Store("other-request", unrelated)
			deps.FailAgentFileOperations("file-agent")
			if _, ok := deps.FileBridges.Load(msg.ID); ok {
				t.Error("disconnected request remained routable")
			}
			if _, ok := deps.FileBridges.Load("other-request"); !ok {
				t.Error("another agent's request was removed")
			}
			select {
			case <-unrelated.Done:
				t.Error("another agent's request was closed")
			default:
			}
			deps.FileBridges.Delete("other-request")
			awaitFileHandler(t, deps, done)
			var response struct {
				Error string `json:"error"`
			}
			// servicehttp intentionally sanitizes all 5xx details. Require a
			// failed response without coupling cleanup to its public wording.
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil || rec.Code != http.StatusBadGateway || response.Error == "" {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}
