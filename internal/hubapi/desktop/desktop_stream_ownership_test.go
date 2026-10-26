package desktop

import (
	"encoding/base64"
	"encoding/json"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/agentmgr"
)

func desktopWireMessage(t *testing.T, value any) agentmgr.Message {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return agentmgr.Message{Data: data}
}

func TestDesktopLateFramesAndFinalizerKeepReplacement(t *testing.T) {
	var wires sync.Map
	d := &Deps{DesktopBridges: &wires, StreamTraceLogValue: func(v string) string { return v }}
	conn := &agentmgr.AgentConn{AssetID: "node"}
	otherConn := &agentmgr.AgentConn{AssetID: "node"}
	makeBridge := func(wire string) *DesktopBridge {
		return &DesktopBridge{SessionID: "public", WireSessionID: wire, AgentConn: conn,
			ExpectedAgentID: "node", OutputCh: make(chan []byte, 4), ClosedCh: make(chan struct{})}
	}
	old, current := makeBridge("old"), makeBridge("current")
	d.registerDesktopStream(old)
	d.registerDesktopStream(current)
	d.ProcessAgentDesktopStarted(conn, desktopWireMessage(t, agentmgr.DesktopStartedData{SessionID: "old"}))
	d.ProcessAgentDesktopData(conn, desktopWireMessage(t, agentmgr.DesktopDataPayload{SessionID: "old", Data: "b2xk"}))
	d.ProcessAgentDesktopClosed(conn, desktopWireMessage(t, agentmgr.DesktopCloseData{SessionID: "old"}))
	d.ProcessAgentDesktopClosed(otherConn, desktopWireMessage(t, agentmgr.DesktopCloseData{SessionID: "current"}))
	d.ProcessAgentDesktopData(otherConn, desktopWireMessage(t, agentmgr.DesktopDataPayload{SessionID: "current", Data: "YmFk"}))
	var stopped []string
	stop := func(got *agentmgr.AgentConn, wire string) {
		if got != conn {
			t.Fatal("finalizer changed agent connection")
		}
		stopped = append(stopped, wire)
	}
	d.FinalizeAgentDesktopSession("public", old, otherConn, true, stop)
	d.FinalizeAgentDesktopSession("public", old, otherConn, true, stop)
	if len(stopped) != 1 || stopped[0] != "old" {
		t.Fatalf("stopped %v", stopped)
	}
	if got, _ := d.loadDesktopOwner("public"); got != current {
		t.Fatal("lost public owner")
	}
	if got, _ := wires.Load("current"); got != current {
		t.Fatal("lost replacement route")
	}
	d.CloseDesktopStreamsForConnection(otherConn)
	select {
	case <-current.ClosedCh:
		t.Fatal("wrong connection closed replacement")
	default:
	}
	d.ProcessAgentDesktopData(conn, desktopWireMessage(t, agentmgr.DesktopDataPayload{SessionID: "current", Data: "bmV3"}))
	if len(current.OutputCh) != 1 || string(<-current.OutputCh) != "new" {
		t.Fatal("old or wrong-connection output reached replacement")
	}
	d.CloseDesktopStreamsForConnection(conn)
	select {
	case <-current.ClosedCh:
	default:
		t.Fatal("owner disconnect did not close stream")
	}
}

func TestAudioAttachmentHasIndependentWireAndQueue(t *testing.T) {
	var wires sync.Map
	d := &Deps{DesktopBridges: &wires}
	conn := &agentmgr.AgentConn{AssetID: "node"}
	parent := &DesktopBridge{SessionID: "public", WireSessionID: "desktop", AgentConn: conn, ExpectedAgentID: "node", ClosedCh: make(chan struct{})}
	d.registerDesktopStream(parent)
	old := d.registerDesktopAudioStream(parent)
	d.ProcessAgentDesktopAudioData(conn, desktopWireMessage(t, agentmgr.DesktopAudioDataPayload{SessionID: old.WireSessionID, Data: "b2xk"}))
	d.finalizeDesktopAudioStream(old)
	current := d.registerDesktopAudioStream(parent)
	defer d.finalizeDesktopAudioStream(current)
	if old.WireSessionID == current.WireSessionID || current.WireSessionID == parent.WireSessionID {
		t.Fatal("audio reused a stream ID")
	}
	for _, wire := range []string{old.WireSessionID, parent.SessionID, parent.WireSessionID} {
		d.ProcessAgentDesktopAudioData(conn, desktopWireMessage(t, agentmgr.DesktopAudioDataPayload{SessionID: wire, Data: "YmFk"}))
	}
	d.ProcessAgentDesktopAudioState(&agentmgr.AgentConn{AssetID: "node"}, desktopWireMessage(t, agentmgr.DesktopAudioStateData{SessionID: current.WireSessionID, State: "stopped"}))
	d.ProcessAgentDesktopAudioData(conn, desktopWireMessage(t, agentmgr.DesktopAudioDataPayload{SessionID: current.WireSessionID, Data: base64.StdEncoding.EncodeToString([]byte("new"))}))
	if len(current.AudioCh) != 1 {
		t.Fatal("late audio reached replacement")
	}
	if frame := <-current.AudioCh; frame.MessageType != websocket.BinaryMessage || string(frame.Payload) != "new" {
		t.Fatalf("wrong frame: %+v", frame)
	}
	d.ProcessAgentDesktopAudioState(conn, desktopWireMessage(t, agentmgr.DesktopAudioStateData{SessionID: current.WireSessionID, State: "started"}))
	frame := <-current.AudioCh
	var state agentmgr.DesktopAudioStateData
	if err := json.Unmarshal(frame.Payload, &state); err != nil {
		t.Fatal(err)
	}
	if state.SessionID != "public" {
		t.Fatalf("wire ID leaked to browser: %+v", state)
	}
}

func TestClosedDesktopStreamsCannotStartAfterFinalization(t *testing.T) {
	var desktops, webrtcs sync.Map
	d := &Deps{DesktopBridges: &desktops, WebRTCBridges: &webrtcs}
	desktop := &DesktopBridge{SessionID: "public", WireSessionID: "vnc", ClosedCh: make(chan struct{})}
	d.registerDesktopStream(desktop)
	d.FinalizeAgentDesktopSession("public", desktop, nil, false, nil)
	if err := desktop.start(agentmgr.Message{}); err == nil {
		t.Fatal("finalized desktop started")
	}
	if _, _, err := desktop.StartRecordingLocked(func() (*ActiveRecording, error) {
		t.Fatal("closed desktop started recording")
		return nil, nil
	}); err == nil {
		t.Fatal("closed recording start did not fail")
	}
	webrtc := &WebRTCSignalingBridge{SessionID: "public", WireSessionID: "webrtc", ClosedCh: make(chan struct{})}
	d.registerWebRTCStream(webrtc)
	d.finalizeWebRTCStream(webrtc)
	if err := webrtc.start(agentmgr.Message{}); err == nil {
		t.Fatal("finalized WebRTC started")
	}
}

func TestStaleAgentCannotReplaceCurrentDesktopOwner(t *testing.T) {
	var desktops, webrtcs sync.Map
	currentConn := &agentmgr.AgentConn{AssetID: "node"}
	staleConn := &agentmgr.AgentConn{AssetID: "node"}
	manager := agentmgr.NewManager()
	manager.Register(currentConn)
	d := &Deps{AgentMgr: manager, DesktopBridges: &desktops, WebRTCBridges: &webrtcs}
	current := &DesktopBridge{SessionID: "public", WireSessionID: "new-vnc", AgentConn: currentConn, ClosedCh: make(chan struct{})}
	staleRTC := &WebRTCSignalingBridge{SessionID: "public", WireSessionID: "old-rtc", AgentConn: staleConn, ClosedCh: make(chan struct{})}
	d.registerDesktopStream(current)
	d.registerWebRTCStream(staleRTC)
	select {
	case <-current.ClosedCh:
		t.Fatal("stale WebRTC replaced current VNC")
	default:
	}
	if got, _ := d.loadDesktopOwner("public"); got != current {
		t.Fatal("stale WebRTC took public owner")
	}
	currentRTC := &WebRTCSignalingBridge{SessionID: "public", WireSessionID: "new-rtc", AgentConn: currentConn, ClosedCh: make(chan struct{})}
	stale := &DesktopBridge{SessionID: "public", WireSessionID: "old-vnc", AgentConn: staleConn, ClosedCh: make(chan struct{})}
	d.registerWebRTCStream(currentRTC)
	d.registerDesktopStream(stale)
	select {
	case <-currentRTC.ClosedCh:
		t.Fatal("stale VNC replaced current WebRTC")
	default:
	}
	if got, _ := d.loadDesktopOwner("public"); got != currentRTC {
		t.Fatal("stale VNC took public owner")
	}
	select {
	case <-current.ClosedCh:
	default:
		t.Fatal("protocol switch did not close previous owner")
	}
}
