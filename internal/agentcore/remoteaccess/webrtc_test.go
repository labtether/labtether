package remoteaccess

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

func TestResolveWebRTCDisplay(t *testing.T) {
	caps := agentmgr.WebRTCCapabilitiesData{
		Displays: []string{":2", ":0"},
	}

	if got := ResolveWebRTCDisplay("Display 1", caps); got != ":2" {
		t.Fatalf("expected invalid monitor-picker label to fall back to first advertised display, got %q", got)
	}

	if got := ResolveWebRTCDisplay(":0", caps); got != ":0" {
		t.Fatalf("expected advertised X display to be preserved, got %q", got)
	}

	if got := ResolveWebRTCDisplay(":9", agentmgr.WebRTCCapabilitiesData{}); got != ":9" {
		t.Fatalf("expected explicit X display to be preserved when no capabilities are advertised, got %q", got)
	}

	if got := ResolveWebRTCDisplay("", agentmgr.WebRTCCapabilitiesData{}); got != ":0" {
		t.Fatalf("expected empty display to fall back to :0, got %q", got)
	}
}

func TestResolveWebRTCDisplayWaylandIgnoresX11Selection(t *testing.T) {
	caps := agentmgr.WebRTCCapabilitiesData{
		DesktopSessionType: DesktopSessionTypeWayland,
		DesktopBackend:     DesktopBackendWaylandPipeWire,
		Displays:           []string{":2"},
	}
	if got := ResolveWebRTCDisplay(":2", caps); got != "" {
		t.Fatalf("expected Wayland session to ignore X11 display selection, got %q", got)
	}
}

func TestWebRTCVideoBitrateForQuality(t *testing.T) {
	tests := []struct {
		quality string
		want    int
	}{
		{quality: "", want: 5000},
		{quality: "medium", want: 5000},
		{quality: "low", want: 1000},
		{quality: "high", want: 20000},
		{quality: "HIGH", want: 20000},
		{quality: "custom", want: 5000},
	}

	for _, tc := range tests {
		if got := WebRTCVideoBitrateForQuality(tc.quality); got != tc.want {
			t.Fatalf("quality=%q bitrate=%d, want %d", tc.quality, got, tc.want)
		}
	}
}

func TestWebRTCManagerHandleWebRTCStartReportsUnavailable(t *testing.T) {
	transport, messages, cleanup := newDesktopRuntimeTransport(t)
	defer cleanup()

	manager := NewWebRTCManager(agentmgr.WebRTCCapabilitiesData{}, nil, nil, nil)
	manager.HandleWebRTCStart(transport, agentmgr.Message{
		Type: agentmgr.MsgWebRTCStart,
		Data: mustMarshalDesktopRuntime(t, agentmgr.WebRTCSessionData{SessionID: "sess-unavailable"}),
	})

	stopped := readWebRTCStopped(t, messages)
	if stopped.Reason != "webrtc unavailable" {
		t.Fatalf("reason=%q, want webrtc unavailable", stopped.Reason)
	}
}

func TestWebRTCManagerHandleWebRTCStartHonorsDisabledSetting(t *testing.T) {
	transport, messages, cleanup := newDesktopRuntimeTransport(t)
	defer cleanup()

	settings := newDisabledWebRTCSettings()
	manager := NewWebRTCManager(agentmgr.WebRTCCapabilitiesData{Available: true}, settings, nil, nil)
	manager.HandleWebRTCStart(transport, agentmgr.Message{
		Type: agentmgr.MsgWebRTCStart,
		Data: mustMarshalDesktopRuntime(t, agentmgr.WebRTCSessionData{SessionID: "sess-disabled"}),
	})

	stopped := readWebRTCStopped(t, messages)
	if stopped.Reason != "webrtc disabled" {
		t.Fatalf("reason=%q, want webrtc disabled", stopped.Reason)
	}
}

func TestWebRTCManagerHandleWebRTCStartRequiresSupportedEncoder(t *testing.T) {
	transport, messages, cleanup := newDesktopRuntimeTransport(t)
	defer cleanup()

	manager := NewWebRTCManager(agentmgr.WebRTCCapabilitiesData{Available: true}, nil, nil, nil)
	manager.HandleWebRTCStart(transport, agentmgr.Message{
		Type: agentmgr.MsgWebRTCStart,
		Data: mustMarshalDesktopRuntime(t, agentmgr.WebRTCSessionData{SessionID: "sess-no-encoder"}),
	})

	stopped := readWebRTCStopped(t, messages)
	if stopped.Reason != "no supported encoder" {
		t.Fatalf("reason=%q, want no supported encoder", stopped.Reason)
	}
}

func TestWebRTCManagerHandleWebRTCStartSetsManagedDisplayX11Env(t *testing.T) {
	transport, messages, cleanup := newDesktopRuntimeTransport(t)
	defer cleanup()

	originalCommand := NewWebRTCSecurityCommand
	originalStartXvfb := StartDesktopXvfb
	originalFindDisplay := FindDesktopFreeDisplay
	t.Cleanup(func() {
		NewWebRTCSecurityCommand = originalCommand
		StartDesktopXvfb = originalStartXvfb
		FindDesktopFreeDisplay = originalFindDisplay
	})

	var startedCommands []*exec.Cmd
	NewWebRTCSecurityCommand = func(name string, args ...string) (*exec.Cmd, error) {
		cmd := exec.Command("sh", "-c", "sleep 60")
		startedCommands = append(startedCommands, cmd)
		return cmd, nil
	}
	StartDesktopXvfb = func(display, width, height int) (*exec.Cmd, string, error) {
		if display != 77 {
			t.Fatalf("display=%d, want 77", display)
		}
		if width != 1920 || height != 1080 {
			t.Fatalf("unexpected Xvfb size %dx%d", width, height)
		}
		return exec.Command("sh", "-c", "sleep 60"), "/tmp/labtether-webrtc.xauth", nil
	}
	FindDesktopFreeDisplay = func() int { return 77 }

	manager := NewWebRTCManager(
		agentmgr.WebRTCCapabilitiesData{Available: true, VideoEncoders: []string{"x264"}},
		nil,
		nil,
		NewDisplayManager(),
	)
	manager.HandleWebRTCStart(transport, agentmgr.Message{
		Type: agentmgr.MsgWebRTCStart,
		Data: mustMarshalDesktopRuntime(t, agentmgr.WebRTCSessionData{
			SessionID: "sess-managed-display",
			Display:   ":777",
		}),
	})
	t.Cleanup(func() {
		manager.Cleanup("sess-managed-display")
	})

	msg := readDesktopRuntimeMessage(t, messages)
	if msg.Type != agentmgr.MsgWebRTCStarted {
		t.Fatalf("message type=%q, want %q", msg.Type, agentmgr.MsgWebRTCStarted)
	}
	if len(startedCommands) == 0 {
		t.Fatal("expected gst-launch command to be started")
	}
	env := startedCommands[0].Env
	if !ContainsEnvValue(env, "DISPLAY=:77") {
		t.Fatalf("expected managed display env, got %v", env)
	}
	if !ContainsEnvValue(env, "XAUTHORITY=/tmp/labtether-webrtc.xauth") {
		t.Fatalf("expected managed display xauth env, got %v", env)
	}
}

func TestWebRTCManagerHandleWebRTCStopCleansSessionAndNotifiesHub(t *testing.T) {
	transport, messages, cleanup := newDesktopRuntimeTransport(t)
	defer cleanup()

	cancelled := make(chan struct{})
	manager := NewWebRTCManager(agentmgr.WebRTCCapabilitiesData{Available: true}, nil, nil, nil)
	sess := &WebRTCSession{
		sessionID: "sess-stop",
		inputCh:   make(chan WebRTCInputEvent, 1),
		cancel: func() {
			select {
			case <-cancelled:
			default:
				close(cancelled)
			}
		},
		done: make(chan struct{}),
	}
	manager.Sessions["sess-stop"] = sess

	manager.HandleWebRTCStop(agentmgr.Message{
		Type: agentmgr.MsgWebRTCStop,
		Data: mustMarshalDesktopRuntime(t, agentmgr.WebRTCStoppedData{SessionID: "sess-stop"}),
	}, transport)

	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("expected session cancel to be invoked")
	}
	select {
	case <-sess.done:
	case <-time.After(2 * time.Second):
		t.Fatal("expected session done channel to be closed")
	}
	if _, ok := manager.Sessions["sess-stop"]; ok {
		t.Fatal("expected session to be removed from manager")
	}

	stopped := readWebRTCStopped(t, messages)
	if stopped.SessionID != "sess-stop" {
		t.Fatalf("session_id=%q, want sess-stop", stopped.SessionID)
	}
	if stopped.Reason != "stopped by hub" {
		t.Fatalf("reason=%q, want stopped by hub", stopped.Reason)
	}
}

func TestWebRTCManagerHandleWebRTCInputQueuesEvent(t *testing.T) {
	manager := NewWebRTCManager(agentmgr.WebRTCCapabilitiesData{Available: true}, nil, nil, nil)
	sess := &WebRTCSession{
		sessionID: "sess-input",
		inputCh:   make(chan WebRTCInputEvent, 1),
		done:      make(chan struct{}),
	}
	manager.Sessions["sess-input"] = sess

	manager.HandleWebRTCInput(agentmgr.Message{
		Type: agentmgr.MsgWebRTCInput,
		Data: mustMarshalDesktopRuntime(t, agentmgr.WebRTCInputData{
			SessionID: "sess-input",
			Type:      " mousemove ",
			X:         11,
			Y:         22,
		}),
	})

	select {
	case evt := <-sess.inputCh:
		if evt.Type != "mousemove" {
			t.Fatalf("type=%q, want mousemove", evt.Type)
		}
		if evt.X != 11 || evt.Y != 22 {
			t.Fatalf("unexpected event %+v", evt)
		}
	default:
		t.Fatal("expected input event to be queued")
	}
}

func TestDecodeWebRTCInputEventAcceptsFallbackPayload(t *testing.T) {
	raw, err := json.Marshal(agentmgr.WebRTCInputData{
		SessionID: "sess-fallback",
		Type:      "keydown",
		KeyCode:   13,
	})
	if err != nil {
		t.Fatalf("marshal fallback payload: %v", err)
	}

	evt, err := DecodeWebRTCInputEvent(raw)
	if err != nil {
		t.Fatalf("decode fallback payload: %v", err)
	}
	if evt.Type != "keydown" || evt.KeyCode != 13 {
		t.Fatalf("unexpected event %+v", evt)
	}
}

func TestX11KeyArgumentPrefersDOMCodeMapping(t *testing.T) {
	keyArg, ok := X11KeyArgument(WebRTCInputEvent{
		Type:    "keydown",
		KeyCode: 20,
		Code:    "CapsLock",
		Key:     "CapsLock",
	})
	if !ok {
		t.Fatal("expected key argument to resolve")
	}
	if keyArg != "Caps_Lock" {
		t.Fatalf("keyArg=%q, want Caps_Lock", keyArg)
	}
}

func TestX11KeyArgumentFallsBackToKeysymCode(t *testing.T) {
	keyArg, ok := X11KeyArgument(WebRTCInputEvent{
		Type:    "keydown",
		KeyCode: 0xffe3,
	})
	if !ok {
		t.Fatal("expected key argument to resolve")
	}
	if keyArg != "0xffe3" {
		t.Fatalf("keyArg=%q, want 0xffe3", keyArg)
	}
}

func TestICECandidateSendDelay(t *testing.T) {
	if got := ICECandidateSendDelay("candidate:1 1 UDP 123 10.0.0.10 10000 typ relay"); got != 300*time.Millisecond {
		t.Fatalf("relay delay=%v, want 300ms", got)
	}
	if got := ICECandidateSendDelay("candidate:1 1 UDP 123 10.0.0.10 10000 typ srflx"); got != 150*time.Millisecond {
		t.Fatalf("srflx delay=%v, want 150ms", got)
	}
	if got := ICECandidateSendDelay("candidate:1 1 UDP 123 10.0.0.10 10000 typ host"); got != 0 {
		t.Fatalf("host delay=%v, want 0", got)
	}
}

func TestWebRTCManagerWatchPipelineExitKeepsSessionOnAudioFailure(t *testing.T) {
	manager := NewWebRTCManager(agentmgr.WebRTCCapabilitiesData{Available: true}, nil, nil, nil)
	sess := &WebRTCSession{
		sessionID:    "sess-audio-exit",
		inputCh:      make(chan WebRTCInputEvent, 1),
		done:         make(chan struct{}),
		audioPort:    4321,
		audioLogPath: "sentinel",
	}
	manager.Sessions[sess.sessionID] = sess

	logFile, err := os.CreateTemp("", "labtether-webrtc-audio-exit-*.log")
	if err != nil {
		t.Fatalf("create temp log: %v", err)
	}
	logPath := logFile.Name()
	if _, err := logFile.WriteString("forced audio pipeline failure"); err != nil {
		t.Fatalf("seed temp log: %v", err)
	}
	if err := logFile.Close(); err != nil {
		t.Fatalf("close temp log: %v", err)
	}

	cmd := exec.Command("sh", "-c", "exit 1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start command: %v", err)
	}

	manager.WatchPipelineExit(context.Background(), sess.sessionID, "audio", cmd, logPath, nil)

	if _, ok := manager.Sessions[sess.sessionID]; !ok {
		t.Fatal("expected audio pipeline failure to keep session alive")
	}
	if sess.audioPort != 0 {
		t.Fatalf("audio_port=%d, want 0 after audio pipeline exit", sess.audioPort)
	}
	if sess.audioLogPath != "" {
		t.Fatalf("audio_log_path=%q, want cleared after audio pipeline exit", sess.audioLogPath)
	}
	select {
	case <-sess.done:
		t.Fatal("expected session to remain open after audio pipeline exit")
	default:
	}
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatalf("expected temp log to be removed, stat err=%v", err)
	}
}

// readWebRTCStopped moved to test_helpers_test.go
