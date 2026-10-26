package desktop

import (
	"github.com/labtether/labtether/internal/hubapi/shared"
)

func (d *Deps) registerDesktopAudioStream(parent *DesktopBridge) *DesktopBridge {
	audio := &DesktopBridge{
		SessionID:       parent.SessionID,
		WireSessionID:   "desktop-audio:" + shared.GenerateRequestID(),
		AgentConn:       parent.AgentConn,
		ExpectedAgentID: parent.ExpectedAgentID,
		ClosedCh:        make(chan struct{}),
		AudioCh:         make(chan DesktopAudioOutbound, 256),
	}
	d.audioStreams.Store(audio.WireSessionID, audio)
	return audio
}

func (d *Deps) finalizeDesktopAudioStream(audio *DesktopBridge) {
	audio.Close()
	d.audioStreams.CompareAndDelete(audio.WireSessionID, audio)
	audio.finalizeOnce.Do(func() {
		audio.lifecycleMu.Lock()
		defer audio.lifecycleMu.Unlock()
		if audio.started {
			sendDesktopAudioStop(audio.AgentConn, audio.WireSessionID)
		}
	})
}

func (d *Deps) loadDesktopAudioStream(wireID string) (any, bool) {
	if audio, ok := d.audioStreams.Load(wireID); ok {
		return audio, true
	}
	// Legacy direct in-process callers have no wire identity.
	if d.DesktopBridges != nil {
		if raw, ok := d.DesktopBridges.Load(wireID); ok {
			if bridge, ok := raw.(*DesktopBridge); ok && bridge.WireSessionID == "" {
				return bridge, true
			}
		}
	}
	return nil, false
}
