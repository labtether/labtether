package agentcore

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

// mockMessageSender implements remoteaccess.MessageSender for root tests.
type mockMessageSender struct {
	messages chan agentmgr.Message
}

func (m *mockMessageSender) Send(msg agentmgr.Message) error {
	m.messages <- msg
	return nil
}

func (m *mockMessageSender) Connected() bool {
	return true
}

func (m *mockMessageSender) AssetID() string {
	return "test-asset"
}

func newDesktopRuntimeTransport(t *testing.T) (*mockMessageSender, <-chan agentmgr.Message, func()) {
	t.Helper()
	mt := &mockMessageSender{messages: make(chan agentmgr.Message, 64)}
	return mt, mt.messages, func() {}
}

func readDesktopRuntimeMessage(t *testing.T, messages <-chan agentmgr.Message) agentmgr.Message {
	t.Helper()
	select {
	case msg := <-messages:
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for desktop runtime message")
		return agentmgr.Message{}
	}
}

func mustMarshalDesktopRuntime(t *testing.T, payload any) []byte {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return data
}
