package desktop

import (
	"errors"
	"strings"
	"sync"
)

// DesktopAudioOutbound holds an audio message to send to the browser.
type DesktopAudioOutbound struct {
	MessageType int
	Payload     []byte
}

// DesktopBridge holds the channels for a desktop session bridged through an agent.
type DesktopBridge struct {
	OutputCh        chan []byte
	AudioCh         chan DesktopAudioOutbound
	ClosedCh        chan struct{}
	ExpectedAgentID string
	SessionID       string
	Target          string
	TraceID         string
	recordingOpMu   sync.Mutex
	recordingMu     sync.RWMutex
	recording       *ActiveRecording
	closeMu         sync.Once
	reasonMu        sync.RWMutex
	reason          string
	audioAttachMu   sync.Mutex
	audioAttached   bool
}

func (b *DesktopBridge) Close() {
	if b == nil {
		return
	}
	b.closeMu.Do(func() {
		close(b.ClosedCh)
	})
}

func (b *DesktopBridge) SetReason(reason string) {
	if b == nil {
		return
	}
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return
	}
	b.reasonMu.Lock()
	b.reason = trimmed
	b.reasonMu.Unlock()
}

func (b *DesktopBridge) CloseReason() string {
	if b == nil {
		return ""
	}
	b.reasonMu.RLock()
	defer b.reasonMu.RUnlock()
	return b.reason
}

func (b *DesktopBridge) TrySendOutput(payload []byte) {
	if b == nil {
		return
	}
	select {
	case <-b.ClosedCh:
		return
	default:
	}
	// Defensive guard: OutputCh should remain open, but avoid process panic if
	// a future change closes it while transport goroutines are still running.
	defer func() { _ = recover() }()
	select {
	case b.OutputCh <- payload:
	case <-b.ClosedCh:
	}
}

func (b *DesktopBridge) TrySendAudio(messageType int, payload []byte) {
	if b == nil || b.AudioCh == nil {
		return
	}
	select {
	case <-b.ClosedCh:
		return
	default:
	}
	defer func() { _ = recover() }()
	select {
	case b.AudioCh <- DesktopAudioOutbound{MessageType: messageType, Payload: payload}:
	case <-b.ClosedCh:
	default:
	}
}

func (b *DesktopBridge) AttachAudio() bool {
	if b == nil {
		return false
	}
	b.audioAttachMu.Lock()
	defer b.audioAttachMu.Unlock()
	if b.audioAttached {
		return false
	}
	b.audioAttached = true
	return true
}

func (b *DesktopBridge) DetachAudio() {
	if b == nil {
		return
	}
	b.audioAttachMu.Lock()
	b.audioAttached = false
	b.audioAttachMu.Unlock()
}

func (b *DesktopBridge) CurrentRecording() *ActiveRecording {
	if b == nil {
		return nil
	}
	b.recordingMu.RLock()
	defer b.recordingMu.RUnlock()
	return b.recording
}

func (b *DesktopBridge) SetRecording(rec *ActiveRecording) {
	if b == nil {
		return
	}
	b.recordingMu.Lock()
	b.recording = rec
	b.recordingMu.Unlock()
}

func (b *DesktopBridge) TakeRecording() *ActiveRecording {
	if b == nil {
		return nil
	}
	b.recordingMu.Lock()
	defer b.recordingMu.Unlock()
	rec := b.recording
	b.recording = nil
	return rec
}

func (b *DesktopBridge) StartRecordingLocked(startFn func() (*ActiveRecording, error)) (*ActiveRecording, bool, error) {
	if b == nil {
		return nil, false, errors.New("desktop bridge unavailable")
	}
	if startFn == nil {
		return nil, false, errors.New("recording start callback is required")
	}
	b.recordingOpMu.Lock()
	defer b.recordingOpMu.Unlock()
	if rec := b.CurrentRecording(); rec != nil {
		return rec, true, nil
	}
	rec, err := startFn()
	if err != nil {
		return nil, false, err
	}
	b.SetRecording(rec)
	return rec, false, nil
}

func (b *DesktopBridge) StopRecordingLocked(stopFn func(*ActiveRecording)) bool {
	if b == nil {
		return false
	}
	b.recordingOpMu.Lock()
	defer b.recordingOpMu.Unlock()
	rec := b.TakeRecording()
	if rec == nil {
		return false
	}
	if stopFn != nil {
		stopFn(rec)
	}
	return true
}

func (b *DesktopBridge) MatchesAgent(assetID string) bool {
	if b == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(b.ExpectedAgentID), strings.TrimSpace(assetID))
}
