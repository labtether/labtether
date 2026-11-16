package resources_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/hubapi/testutil/hubdeps"
)

func TestHandleProcessListNormalizesEmptyAgentReply(t *testing.T) {
	sut := hubdeps.New(t)
	sut.AgentMgr = agentmgr.NewManager()
	serverConn, clientConn, cleanup := testutil.CreateWSPairForNetworkTest(t)
	defer cleanup()
	sut.AgentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-empty", "linux"))
	defer sut.AgentMgr.Unregister("node-empty")

	done := make(chan struct{})
	go func() {
		defer close(done)
		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read process request: %v", err)
			return
		}
		var request agentmgr.ProcessListData
		if err := json.Unmarshal(outbound.Data, &request); err != nil {
			t.Errorf("decode process request: %v", err)
			return
		}
		data, err := json.Marshal(agentmgr.ProcessListedData{RequestID: request.RequestID})
		if err != nil {
			t.Errorf("encode empty process reply: %v", err)
			return
		}
		sut.ResourcesDeps().ProcessAgentProcessListed(&agentmgr.AgentConn{AssetID: "node-empty"}, agentmgr.Message{
			Type: agentmgr.MsgProcessListed,
			ID:   request.RequestID,
			Data: data,
		})
	}()

	rec := httptest.NewRecorder()
	sut.ResourcesDeps().HandleProcesses(rec, httptest.NewRequest(http.MethodGet, "/processes/node-empty", nil))
	<-done
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Processes json.RawMessage `json:"processes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode process response: %v", err)
	}
	if string(payload.Processes) != "[]" {
		t.Fatalf("empty processes = %s, want []", payload.Processes)
	}
}
