package remoteaccess

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

// mockTransport implements MessageSender for tests.
type mockTransport struct {
	mu       sync.Mutex
	messages chan agentmgr.Message
}

func newMockTransport() *mockTransport {
	return &mockTransport{
		messages: make(chan agentmgr.Message, 64),
	}
}

func (m *mockTransport) Send(msg agentmgr.Message) error {
	m.messages <- msg
	return nil
}

func newDesktopRuntimeTransport(t *testing.T) (MessageSender, <-chan agentmgr.Message, func()) {
	t.Helper()
	mt := newMockTransport()
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

func readWebRTCStopped(t *testing.T, messages <-chan agentmgr.Message) agentmgr.WebRTCStoppedData {
	t.Helper()

	msg := readDesktopRuntimeMessage(t, messages)
	if msg.Type != agentmgr.MsgWebRTCStopped {
		t.Fatalf("message type=%q, want %q", msg.Type, agentmgr.MsgWebRTCStopped)
	}

	var stopped agentmgr.WebRTCStoppedData
	if err := json.Unmarshal(msg.Data, &stopped); err != nil {
		t.Fatalf("decode webrtc stopped payload: %v", err)
	}
	return stopped
}

func ContainsEnvValue(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}

// mockSettingsProvider implements SettingsProvider for tests.
type mockSettingsProvider struct {
	settings map[string]string
}

func (m *mockSettingsProvider) ReportedAgentSettings() map[string]string {
	if m.settings == nil {
		return map[string]string{}
	}
	return m.settings
}

// newDisabledWebRTCSettings returns a SettingsProvider that reports WebRTC disabled.
func newDisabledWebRTCSettings() SettingsProvider {
	return &mockSettingsProvider{settings: map[string]string{"webrtc_enabled": "false"}}
}
