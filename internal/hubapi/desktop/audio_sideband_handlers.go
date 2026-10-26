package desktop

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
)

const desktopAudioDefaultBitrate = 128000

func sendDesktopAudioStop(agentConn *agentmgr.AgentConn, sessionID string) {
	if agentConn == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	payload, _ := json.Marshal(agentmgr.DesktopAudioStopData{SessionID: sessionID})
	_ = agentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgDesktopAudioStop,
		ID:   sessionID,
		Data: payload,
	})
}

// HandleDesktopAudioStream handles the desktop audio sideband WebSocket.
func (d *Deps) HandleDesktopAudioStream(w http.ResponseWriter, r *http.Request, session terminal.Session) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if d.AgentMgr == nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "agent manager unavailable")
		return
	}
	agentConn, ok := d.AgentMgr.Get(session.Target)
	if !ok {
		servicehttp.WriteError(w, http.StatusBadGateway, "agent disconnected")
		return
	}

	rawBridge, ok := d.loadDesktopOwner(session.ID)
	if !ok {
		servicehttp.WriteError(w, http.StatusConflict, "desktop stream is not active")
		return
	}
	bridge, ok := rawBridge.(*DesktopBridge)
	if !ok || bridge == nil {
		servicehttp.WriteError(w, http.StatusConflict, "desktop stream bridge unavailable")
		return
	}
	if !bridge.MatchesConnection(agentConn) {
		servicehttp.WriteError(w, http.StatusConflict, "desktop stream agent mismatch")
		return
	}
	if !bridge.AttachAudio() {
		servicehttp.WriteError(w, http.StatusConflict, "desktop audio stream already attached")
		return
	}
	defer bridge.DetachAudio()

	wsConn, err := shared.UpgradeWebSocket(d.TerminalWebSocketUpgrader, w, r, nil)
	if err != nil {
		return
	}
	shared.LimitBrowserControlMessages(wsConn)
	defer wsConn.Close()

	audio := d.registerDesktopAudioStream(bridge)
	defer d.finalizeDesktopAudioStream(audio)
	select {
	case <-bridge.ClosedCh:
		return
	default:
	}
	payload, _ := json.Marshal(agentmgr.DesktopAudioStartData{SessionID: audio.WireSessionID, Bitrate: desktopAudioDefaultBitrate})
	if err := audio.start(agentmgr.Message{Type: agentmgr.MsgDesktopAudioStart, ID: audio.WireSessionID, Data: payload}); err != nil {
		SafeWriteClose(wsConn, websocket.CloseTryAgainLater, "failed to start desktop audio")
		return
	}

	var writeMu sync.Mutex
	stopKeepalive := d.StartBrowserWSKeepalive(wsConn, &writeMu, "desktop-audio:"+session.ID)
	defer stopKeepalive()

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-bridge.ClosedCh:
			audio.Close()
			_ = wsConn.Close()
		case <-done:
		}
	}()

	go d.BridgeDesktopAudioOutput(wsConn, audio.AudioCh, audio.ClosedCh, &writeMu)

	for {
		_, _, err := wsConn.ReadMessage()
		if err != nil {
			return
		}
		_ = d.TouchBrowserWSReadDeadline(wsConn)
	}
}

// BridgeDesktopAudioOutput reads audio from the bridge channel and writes to browser WebSocket.
func (d *Deps) BridgeDesktopAudioOutput(
	wsConn *websocket.Conn,
	audioCh <-chan DesktopAudioOutbound,
	closedCh <-chan struct{},
	writeMu *sync.Mutex,
) {
	for {
		select {
		case outbound, ok := <-audioCh:
			if !ok {
				return
			}
			writeMu.Lock()
			_ = wsConn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			err := wsConn.WriteMessage(outbound.MessageType, outbound.Payload)
			writeMu.Unlock()
			if err != nil {
				_ = wsConn.Close()
				return
			}
		case <-closedCh:
			return
		}
	}
}

// ProcessAgentDesktopAudioData handles desktop.audio.data from agent.
func (d *Deps) ProcessAgentDesktopAudioData(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var payload agentmgr.DesktopAudioDataPayload
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		return
	}

	decoded, err := base64.StdEncoding.DecodeString(payload.Data)
	if err != nil || len(decoded) == 0 {
		return
	}

	if raw, ok := d.loadDesktopAudioStream(payload.SessionID); ok {
		bridge, ok := raw.(*DesktopBridge)
		if !ok || bridge == nil {
			return
		}
		if !bridge.MatchesConnection(conn) {
			return
		}
		bridge.TrySendAudio(websocket.BinaryMessage, decoded)
	}
}

// ProcessAgentDesktopAudioState handles desktop.audio.state from agent.
func (d *Deps) ProcessAgentDesktopAudioState(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var payload agentmgr.DesktopAudioStateData
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		return
	}

	if raw, ok := d.loadDesktopAudioStream(payload.SessionID); ok {
		bridge, ok := raw.(*DesktopBridge)
		if !ok || bridge == nil {
			return
		}
		if !bridge.MatchesConnection(conn) {
			return
		}
		if bridge.SessionID != "" {
			payload.SessionID = bridge.SessionID
		}
		publicPayload, _ := json.Marshal(payload)
		bridge.TrySendAudio(websocket.TextMessage, publicPayload)
	}
}
