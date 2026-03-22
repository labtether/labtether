package remoteaccess

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentcore/sysconfig"
	"github.com/labtether/labtether/internal/agentmgr"
)

func TestDesktopManagerHandleDesktopStartRejectsMaxSessions(t *testing.T) {
	dm := NewDesktopManager(nil)
	for i := 0; i < MaxDesktopSessions; i++ {
		dm.Sessions[string(rune('a'+i))] = &DesktopSession{done: make(chan struct{})}
	}

	transport, messages, cleanup := newDesktopRuntimeTransport(t)
	defer cleanup()

	req := mustMarshalDesktopRuntime(t, agentmgr.DesktopStartData{SessionID: "sess-max"})
	dm.HandleDesktopStart(transport, agentmgr.Message{Type: agentmgr.MsgDesktopStart, Data: req})

	msg := readDesktopRuntimeMessage(t, messages)
	if msg.Type != agentmgr.MsgDesktopClosed {
		t.Fatalf("message type=%q, want %q", msg.Type, agentmgr.MsgDesktopClosed)
	}
	var closed agentmgr.DesktopCloseData
	if err := json.Unmarshal(msg.Data, &closed); err != nil {
		t.Fatalf("decode close payload: %v", err)
	}
	if !strings.Contains(closed.Reason, "max desktop sessions reached") {
		t.Fatalf("close reason=%q, want max sessions message", closed.Reason)
	}
}

func TestDesktopManagerHandleDesktopStartStreamsOutputAndCleansUpOnEOF(t *testing.T) {
	originalStart := StartDesktopVNCServer
	originalDial := DialDesktopVNC
	t.Cleanup(func() {
		StartDesktopVNCServer = originalStart
		DialDesktopVNC = originalDial
	})

	serverVNC, peerVNC := net.Pipe()
	StartDesktopVNCServer = func(display, quality, vncPassword string) (*exec.Cmd, *exec.Cmd, *exec.Cmd, int, string, error) {
		if display != ":99" {
			t.Fatalf("display=%q, want :99", display)
		}
		if quality != "high" {
			t.Fatalf("quality=%q, want high", quality)
		}
		if vncPassword != "secret" {
			t.Fatalf("vncPassword=%q, want secret", vncPassword)
		}
		return nil, nil, nil, 5901, "", nil
	}
	DialDesktopVNC = func(network, address string, timeout time.Duration) (net.Conn, error) {
		if network != "tcp" {
			t.Fatalf("network=%q, want tcp", network)
		}
		if address != "127.0.0.1:5901" {
			t.Fatalf("address=%q, want 127.0.0.1:5901", address)
		}
		return serverVNC, nil
	}

	dm := NewDesktopManager(nil)
	transport, messages, cleanup := newDesktopRuntimeTransport(t)
	defer cleanup()
	defer dm.CloseAll()
	defer peerVNC.Close()

	req := mustMarshalDesktopRuntime(t, agentmgr.DesktopStartData{
		SessionID:   "sess-stream",
		Width:       1280,
		Height:      720,
		Quality:     "high",
		Display:     ":99",
		VNCPassword: "secret",
	})
	dm.HandleDesktopStart(transport, agentmgr.Message{Type: agentmgr.MsgDesktopStart, Data: req})

	startedMsg := readDesktopRuntimeMessage(t, messages)
	if startedMsg.Type != agentmgr.MsgDesktopStarted {
		t.Fatalf("message type=%q, want %q", startedMsg.Type, agentmgr.MsgDesktopStarted)
	}
	var started agentmgr.DesktopStartedData
	if err := json.Unmarshal(startedMsg.Data, &started); err != nil {
		t.Fatalf("decode started payload: %v", err)
	}
	if started.SessionID != "sess-stream" || started.Width != 1280 || started.Height != 720 {
		t.Fatalf("unexpected started payload: %+v", started)
	}

	if _, err := peerVNC.Write([]byte("frame-1")); err != nil {
		t.Fatalf("write vnc output: %v", err)
	}

	dataMsg := readDesktopRuntimeMessage(t, messages)
	if dataMsg.Type != agentmgr.MsgDesktopData {
		t.Fatalf("message type=%q, want %q", dataMsg.Type, agentmgr.MsgDesktopData)
	}
	var payload agentmgr.DesktopDataPayload
	if err := json.Unmarshal(dataMsg.Data, &payload); err != nil {
		t.Fatalf("decode desktop data payload: %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(payload.Data)
	if err != nil {
		t.Fatalf("decode VNC payload: %v", err)
	}
	if string(decoded) != "frame-1" {
		t.Fatalf("decoded payload=%q, want frame-1", string(decoded))
	}

	_ = peerVNC.Close()

	closedMsg := readDesktopRuntimeMessage(t, messages)
	if closedMsg.Type != agentmgr.MsgDesktopClosed {
		t.Fatalf("message type=%q, want %q", closedMsg.Type, agentmgr.MsgDesktopClosed)
	}
	var closed agentmgr.DesktopCloseData
	if err := json.Unmarshal(closedMsg.Data, &closed); err != nil {
		t.Fatalf("decode close payload: %v", err)
	}
	if closed.SessionID != "sess-stream" {
		t.Fatalf("close session_id=%q, want sess-stream", closed.SessionID)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		dm.Mu.Lock()
		_, ok := dm.Sessions["sess-stream"]
		dm.Mu.Unlock()
		if !ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("expected desktop session cleanup after EOF")
}

func TestHandleListDisplaysReturnsDisplaysAndErrors(t *testing.T) {
	original := sysconfig.PlatformListDisplaysFn
	t.Cleanup(func() {
		sysconfig.PlatformListDisplaysFn = original
	})

	t.Run("success", func(t *testing.T) {
		sysconfig.PlatformListDisplaysFn = func() ([]agentmgr.DisplayInfo, error) {
			return []agentmgr.DisplayInfo{
				{Name: "DP-1", Width: 1920, Height: 1080, Primary: true},
				{Name: "HDMI-1", Width: 2560, Height: 1440},
			}, nil
		}

		transport, messages, cleanup := newDesktopRuntimeTransport(t)
		defer cleanup()

		sysconfig.HandleListDisplays(transport, agentmgr.Message{
			Type: agentmgr.MsgDesktopListDisplays,
			Data: mustMarshalDesktopRuntime(t, map[string]string{"request_id": "req-success"}),
		})

		msg := readDesktopRuntimeMessage(t, messages)
		if msg.Type != agentmgr.MsgDesktopDisplays {
			t.Fatalf("message type=%q, want %q", msg.Type, agentmgr.MsgDesktopDisplays)
		}
		var resp agentmgr.DisplayListData
		if err := json.Unmarshal(msg.Data, &resp); err != nil {
			t.Fatalf("decode display response: %v", err)
		}
		if resp.RequestID != "req-success" {
			t.Fatalf("request_id=%q, want req-success", resp.RequestID)
		}
		if len(resp.Displays) != 2 || resp.Displays[0].Name != "DP-1" || !resp.Displays[0].Primary {
			t.Fatalf("unexpected displays: %+v", resp.Displays)
		}
		if resp.Error != "" {
			t.Fatalf("unexpected error=%q", resp.Error)
		}
	})

	t.Run("error", func(t *testing.T) {
		sysconfig.PlatformListDisplaysFn = func() ([]agentmgr.DisplayInfo, error) {
			return nil, errors.New("xrandr failed")
		}

		transport, messages, cleanup := newDesktopRuntimeTransport(t)
		defer cleanup()

		sysconfig.HandleListDisplays(transport, agentmgr.Message{
			Type: agentmgr.MsgDesktopListDisplays,
			Data: mustMarshalDesktopRuntime(t, map[string]string{"request_id": "req-error"}),
		})

		msg := readDesktopRuntimeMessage(t, messages)
		var resp agentmgr.DisplayListData
		if err := json.Unmarshal(msg.Data, &resp); err != nil {
			t.Fatalf("decode display response: %v", err)
		}
		if resp.RequestID != "req-error" {
			t.Fatalf("request_id=%q, want req-error", resp.RequestID)
		}
		if resp.Error != "xrandr failed" {
			t.Fatalf("error=%q, want xrandr failed", resp.Error)
		}
	})
}

// Test helper functions moved to test_helpers_test.go
