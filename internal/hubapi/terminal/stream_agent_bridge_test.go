package terminal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	terminalmodel "github.com/labtether/labtether/internal/terminal"
)

func TestDefaultTerminalTmuxSessionNameIsStableAndSessionIsolated(t *testing.T) {
	firstID := "sess_1784077900279951758_2204"
	secondID := "sess_1784077900279951758_2205"

	first := DefaultTerminalTmuxSessionName(firstID)
	if first != DefaultTerminalTmuxSessionName(firstID) {
		t.Fatal("expected the same session id to produce a stable tmux name")
	}
	if second := DefaultTerminalTmuxSessionName(secondID); second == first {
		t.Fatalf("distinct session ids with a shared prefix produced the same tmux name %q", first)
	}
	if !regexp.MustCompile(`^lt-[0-9a-f]{32}$`).MatchString(first) {
		t.Fatalf("tmux name %q is not bounded to the expected safe format", first)
	}
}

type agentScrollbackTestStore struct {
	mu      sync.Mutex
	buffer  []byte
	upserts int
}

func (s *agentScrollbackTestStore) UpsertScrollback(_ string, buffer []byte, _, _ int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buffer = append([]byte(nil), buffer...)
	s.upserts++
	return nil
}

func (s *agentScrollbackTestStore) GetScrollback(string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.buffer...), nil
}

func (s *agentScrollbackTestStore) DeleteScrollback(string) error { return nil }

func TestScrollbackFlushLoopPersistsFinalSnapshotOnCancel(t *testing.T) {
	store := &agentScrollbackTestStore{}
	d := &Deps{TerminalScrollbackStore: store}
	ring := terminalmodel.NewRingBuffer(100)
	ring.Write([]byte("output immediately before disconnect\n"))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		d.scrollbackFlushLoop(ctx, "persistent-1", ring)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scrollback flush loop did not stop")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.upserts != 1 || string(store.buffer) != "output immediately before disconnect\n" {
		t.Fatalf("final persisted scrollback upserts=%d buffer=%q", store.upserts, store.buffer)
	}
}

func TestProcessAgentTerminalDataCapturesPersistentScrollback(t *testing.T) {
	var bridges sync.Map
	ring := terminalmodel.NewRingBuffer(100)
	bridge := &TerminalBridge{
		OutputCh:        make(chan []byte, 1),
		ClosedCh:        make(chan struct{}),
		ExpectedAgentID: "node-1",
		SessionID:       "session-1",
		Target:          "node-1",
		Scrollback:      ring,
	}
	bridges.Store("session-1", bridge)
	d := &Deps{TerminalBridges: &bridges}
	payload, err := json.Marshal(agentmgr.TerminalDataPayload{
		SessionID: "session-1",
		Data:      base64.StdEncoding.EncodeToString([]byte("persistent output\n")),
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	d.ProcessAgentTerminalData(&agentmgr.AgentConn{AssetID: "node-1"}, agentmgr.Message{Data: payload})

	if got := string(ring.Snapshot()); got != "persistent output\n" {
		t.Fatalf("captured scrollback = %q", got)
	}
	select {
	case got := <-bridge.OutputCh:
		if string(got) != "persistent output\n" {
			t.Fatalf("live output = %q", got)
		}
	default:
		t.Fatal("expected output to remain available to the live stream")
	}
}
