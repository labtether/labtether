package agentcaps

import (
	"errors"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
)

type recordingManager struct {
	conn *agentmgr.AgentConn
	sent []string
}

func (m *recordingManager) Get(string) (*agentmgr.AgentConn, bool) { return m.conn, true }
func (m *recordingManager) SendToAgent(_ string, msg agentmgr.Message) error {
	m.sent = append(m.sent, msg.Type)
	return nil
}

// Fault: Docker container and stack actions are sent to an explicit agent
// that never advertised Docker, and wait for a result that never comes.
func TestGatedSenderSendsOnlyWhatIsOffered(t *testing.T) {
	manager := &recordingManager{conn: agentmgr.NewAgentConn(nil, "node01", "linux")}
	sender := GatedSender{Manager: manager}
	if err := sender.SendToAgent("node01", agentmgr.Message{Type: agentmgr.MsgDockerAction}); err != nil {
		t.Fatalf("agent without the signal: %v", err)
	}
	Record(manager.conn, map[string]string{MetaMode: ModeExplicit})
	if err := sender.SendToAgent("node01", agentmgr.Message{Type: agentmgr.MsgDockerAction}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("explicit agent: error %v, want ErrUnsupported", err)
	}
	if len(manager.sent) != 1 {
		t.Fatalf("sent %v, want one docker.action", manager.sent)
	}
}
