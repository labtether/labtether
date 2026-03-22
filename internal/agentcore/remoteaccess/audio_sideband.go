package remoteaccess

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

var StartAudioCapture = PlatformStartAudioCapture

const (
	audioDefaultBitrate = 128000
	audioChunkSize      = 4096 // bytes per audio data message
)

// AudioSidebandManager manages audio capture sessions for VNC desktop sessions.
// On Linux it shells out to ffmpeg for PulseAudio → Opus encoding.
// On other platforms it reports "unavailable".
type AudioSidebandManager struct {
	Mu       sync.Mutex
	Sessions map[string]context.CancelFunc // sessionID → cancel
}

func NewAudioSidebandManager() *AudioSidebandManager {
	return &AudioSidebandManager{
		Sessions: make(map[string]context.CancelFunc),
	}
}

// closeAll stops all active audio capture sessions.
func (m *AudioSidebandManager) CloseAll() {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	for sid, cancel := range m.Sessions {
		cancel()
		delete(m.Sessions, sid)
	}
}

// handleAudioStart processes a desktop.audio.start message from the hub.
func (m *AudioSidebandManager) HandleAudioStart(transport MessageSender, msg agentmgr.Message) {
	var req agentmgr.DesktopAudioStartData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("audio: invalid start request: %v", err)
		return
	}
	if req.SessionID == "" {
		log.Printf("audio: start request missing session_id")
		return
	}

	bitrate := req.Bitrate
	if bitrate <= 0 {
		bitrate = audioDefaultBitrate
	}

	m.Mu.Lock()
	// Stop any existing session for this ID.
	if cancel, ok := m.Sessions[req.SessionID]; ok {
		cancel()
		delete(m.Sessions, req.SessionID)
	}
	ctx, cancel := context.WithCancel(context.Background()) // #nosec G118 -- Cancel is stored in the session map and invoked by HandleAudioStop/CloseAll.
	m.Sessions[req.SessionID] = cancel
	m.Mu.Unlock()

	go m.runCapture(ctx, transport, req.SessionID, bitrate)
}

// handleAudioStop processes a desktop.audio.stop message from the hub.
func (m *AudioSidebandManager) HandleAudioStop(msg agentmgr.Message) {
	var req agentmgr.DesktopAudioStopData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("audio: invalid stop request: %v", err)
		return
	}

	m.Mu.Lock()
	if cancel, ok := m.Sessions[req.SessionID]; ok {
		cancel()
		delete(m.Sessions, req.SessionID)
	}
	m.Mu.Unlock()
}

// runCapture starts platform-specific audio capture and streams data to the hub.
func (m *AudioSidebandManager) runCapture(ctx context.Context, transport MessageSender, sessionID string, bitrate int) {
	reader, err := StartAudioCapture(ctx, sessionID, bitrate)
	if err != nil {
		log.Printf("audio: capture unavailable for session %s: %v", sessionID, err)
		m.sendState(transport, sessionID, "unavailable", err.Error())
		m.Mu.Lock()
		delete(m.Sessions, sessionID)
		m.Mu.Unlock()
		return
	}

	m.sendState(transport, sessionID, "started", "")
	log.Printf("audio: capture started for session %s at %d bps", sessionID, bitrate)

	buf := make([]byte, audioChunkSize)
	for {
		select {
		case <-ctx.Done():
			m.sendState(transport, sessionID, "stopped", "")
			return
		default:
		}

		n, readErr := reader.Read(buf)
		if n > 0 {
			encoded := base64.StdEncoding.EncodeToString(buf[:n])
			payload := agentmgr.DesktopAudioDataPayload{
				SessionID: sessionID,
				Data:      encoded,
				Timestamp: time.Now().UnixMilli(),
			}
			data, err := json.Marshal(payload)
			if err != nil {
				continue
			}
			_ = transport.Send(agentmgr.Message{
				Type: agentmgr.MsgDesktopAudioData,
				ID:   sessionID,
				Data: data,
			})
		}
		if readErr != nil {
			if readErr != io.EOF {
				log.Printf("audio: capture read error for session %s: %v", sessionID, readErr)
			}
			m.sendState(transport, sessionID, "stopped", "")
			m.Mu.Lock()
			delete(m.Sessions, sessionID)
			m.Mu.Unlock()
			return
		}
	}
}

// sendState sends a desktop.audio.state message to the hub.
func (m *AudioSidebandManager) sendState(transport MessageSender, sessionID, state, errMsg string) {
	payload := agentmgr.DesktopAudioStateData{
		SessionID: sessionID,
		State:     state,
		Error:     errMsg,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_ = transport.Send(agentmgr.Message{
		Type: agentmgr.MsgDesktopAudioState,
		ID:   sessionID,
		Data: data,
	})
}
