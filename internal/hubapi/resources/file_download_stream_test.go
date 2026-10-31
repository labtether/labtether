package resources

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

// Staging the whole file before the headers made large downloads through the
// console fail after 5 minutes. The browser must get headers and data while
// the agent is still sending, and a failure after that must break the
// download rather than end it as if it were complete.
func TestFileDownloadStreamsBeforeAgentFinishesAndAbortsOnLaterFailure(t *testing.T) {
	for _, failLater := range []bool{false, true} {
		deps, conn, client := fileRequestTestAgent(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			deps.HandleFileDownload(w, r, "file-agent")
		}))
		defer server.Close()

		type result struct {
			resp *http.Response
			err  error
		}
		responses := make(chan result, 1)
		go func() {
			resp, err := http.Get(server.URL + "/files/file-agent/download?path=/big.bin")
			responses <- result{resp, err}
		}()

		var read agentmgr.FileReadData
		if err := json.Unmarshal(readFileRequest(t, client).Data, &read); err != nil {
			t.Fatal(err)
		}
		send := func(chunk agentmgr.FileDataPayload) {
			chunk.RequestID = read.RequestID
			data, _ := json.Marshal(chunk)
			deps.ProcessAgentFileData(conn, agentmgr.Message{Type: agentmgr.MsgFileData, ID: read.RequestID, Data: data})
		}
		send(agentmgr.FileDataPayload{Data: base64.StdEncoding.EncodeToString([]byte("first "))})

		var got result
		select {
		case got = <-responses:
		case <-time.After(2 * time.Second):
			t.Fatal("no headers while the agent was still sending")
		}
		if got.err != nil {
			t.Fatalf("headers did not arrive before the agent finished: %v", got.err)
		}
		body := bufio.NewReader(got.resp.Body)
		head := make([]byte, len("first "))
		if _, err := io.ReadFull(body, head); err != nil || string(head) != "first " {
			t.Fatalf("first bytes = %q, %v", head, err)
		}

		if failLater {
			send(agentmgr.FileDataPayload{Error: "disk read failed", Done: true})
		} else {
			send(agentmgr.FileDataPayload{Data: base64.StdEncoding.EncodeToString([]byte("last")), Offset: 6, Done: true})
		}
		rest, err := io.ReadAll(body)
		_ = got.resp.Body.Close()
		if failLater && err == nil {
			t.Fatalf("failed download ended cleanly with %q", rest)
		}
		if !failLater && (err != nil || string(rest) != "last") {
			t.Fatalf("rest = %q, %v", rest, err)
		}
	}
}
