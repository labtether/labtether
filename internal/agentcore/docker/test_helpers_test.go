package docker

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

type recordingCollectorTransport struct {
	mu        sync.Mutex
	connected bool
	messages  []agentmgr.Message
	ch        chan agentmgr.Message
	onSend    func(agentmgr.Message)
}

func newRecordingCollectorTransport(connected bool) *recordingCollectorTransport {
	return &recordingCollectorTransport{
		connected: connected,
		ch:        make(chan agentmgr.Message, 32),
	}
}

func (r *recordingCollectorTransport) Send(msg agentmgr.Message) error {
	r.mu.Lock()
	r.messages = append(r.messages, msg)
	r.mu.Unlock()

	select {
	case r.ch <- msg:
	default:
	}
	if r.onSend != nil {
		r.onSend(msg)
	}
	return nil
}

func (r *recordingCollectorTransport) Connect(context.Context) error { return nil }

func (r *recordingCollectorTransport) Receive() (agentmgr.Message, error) {
	return agentmgr.Message{}, nil
}

func (r *recordingCollectorTransport) Close() {}

func (r *recordingCollectorTransport) Connected() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.connected
}

func (r *recordingCollectorTransport) MessageCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.messages)
}

func waitForCollectorMessage(t *testing.T, transport *recordingCollectorTransport, timeout time.Duration) agentmgr.Message {
	t.Helper()

	select {
	case msg := <-transport.ch:
		return msg
	case <-time.After(timeout):
		t.Fatal("timed out waiting for collector transport message")
		return agentmgr.Message{}
	}
}
