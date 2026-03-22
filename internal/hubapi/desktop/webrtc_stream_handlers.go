package desktop

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/terminal"
)

// WebRTCSignalingBridge relays SDP/ICE between browser and agent over WS signaling.
type WebRTCSignalingBridge struct {
	BrowserWS       *websocket.Conn
	ExpectedAgentID string
	mu              sync.Mutex
	ClosedCh        chan struct{}
	closeOnce       sync.Once
}

// SendWebRTCStop sends a webrtc.stop message to the agent.
func SendWebRTCStop(agentConn *agentmgr.AgentConn, sessionID string) {
	if agentConn == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	stopData, _ := json.Marshal(agentmgr.WebRTCStoppedData{SessionID: sessionID})
	_ = agentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgWebRTCStop,
		ID:   sessionID,
		Data: stopData,
	})
}

func (b *WebRTCSignalingBridge) SendToBrowser(data []byte) error {
	if b == nil {
		return fmt.Errorf("webrtc bridge unavailable")
	}
	if b.BrowserWS == nil {
		return fmt.Errorf("webrtc browser websocket unavailable")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.BrowserWS.WriteMessage(websocket.TextMessage, data)
}

func (b *WebRTCSignalingBridge) Close() {
	if b == nil {
		return
	}
	b.closeOnce.Do(func() {
		close(b.ClosedCh)
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.BrowserWS != nil {
			_ = b.BrowserWS.Close()
		}
	})
}

func (b *WebRTCSignalingBridge) MatchesAgent(assetID string) bool {
	if b == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(b.ExpectedAgentID), strings.TrimSpace(assetID))
}

// CloseWebRTCBridgesForAsset closes all WebRTC bridges matching the given asset.
func (d *Deps) CloseWebRTCBridgesForAsset(assetID string) {
	trimmedAssetID := strings.TrimSpace(assetID)
	if trimmedAssetID == "" {
		return
	}
	d.WebRTCBridges.Range(func(_ any, value any) bool {
		bridge, ok := value.(*WebRTCSignalingBridge)
		if !ok {
			return true
		}
		if bridge.MatchesAgent(trimmedAssetID) {
			bridge.Close()
		}
		return true
	})
}

// HandleWebRTCStream upgrades the desktop stream endpoint into a signaling relay.
func (d *Deps) HandleWebRTCStream(w http.ResponseWriter, r *http.Request, session terminal.Session) {
	agentConn, ok := d.AgentMgr.Get(session.Target)
	if !ok {
		http.Error(w, "agent not connected", http.StatusServiceUnavailable)
		return
	}

	upgrader := websocket.Upgrader{CheckOrigin: d.CheckSameOrigin}
	browserWS, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("webrtc: websocket upgrade failed: %v", err)
		return
	}
	defer browserWS.Close()

	bridge := &WebRTCSignalingBridge{
		BrowserWS:       browserWS,
		ExpectedAgentID: agentConn.AssetID,
		ClosedCh:        make(chan struct{}),
	}
	d.WebRTCBridges.Store(session.ID, bridge)
	defer func() {
		d.WebRTCBridges.Delete(session.ID)
		bridge.Close()
	}()

	width, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("width")))
	height, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("height")))
	fps, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("fps")))
	startData, _ := json.Marshal(agentmgr.WebRTCSessionData{
		SessionID:    session.ID,
		Display:      strings.TrimSpace(r.URL.Query().Get("display")),
		Quality:      strings.TrimSpace(r.URL.Query().Get("quality")),
		Width:        width,
		Height:       height,
		FPS:          fps,
		AudioEnabled: strings.TrimSpace(r.URL.Query().Get("audio")) != "false",
	})
	if err := agentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgWebRTCStart,
		ID:   session.ID,
		Data: startData,
	}); err != nil {
		log.Printf("webrtc: failed to send start for session=%s: %v", session.ID, err)
		return
	}

	for {
		_, msgBytes, readErr := browserWS.ReadMessage()
		if readErr != nil {
			break
		}

		var sigMsg struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(msgBytes, &sigMsg); err != nil {
			continue
		}

		if DesktopDebugEnabled() {
			log.Printf("webrtc-debug: signaling session=%s type=%s", session.ID, sigMsg.Type)
		}

		switch strings.ToLower(strings.TrimSpace(sigMsg.Type)) {
		case "offer":
			var payload struct {
				Type string `json:"type"`
				SDP  string `json:"sdp"`
			}
			if err := json.Unmarshal(sigMsg.Data, &payload); err != nil {
				continue
			}
			data, _ := json.Marshal(agentmgr.WebRTCSDPData{
				SessionID: session.ID,
				Type:      "offer",
				SDP:       payload.SDP,
			})
			_ = agentConn.Send(agentmgr.Message{Type: agentmgr.MsgWebRTCOffer, ID: session.ID, Data: data})

		case "ice":
			var payload agentmgr.WebRTCICEData
			if err := json.Unmarshal(sigMsg.Data, &payload); err != nil {
				continue
			}
			payload.SessionID = session.ID
			data, _ := json.Marshal(payload)
			_ = agentConn.Send(agentmgr.Message{Type: agentmgr.MsgWebRTCICE, ID: session.ID, Data: data})

		case "input":
			var payload agentmgr.WebRTCInputData
			if err := json.Unmarshal(sigMsg.Data, &payload); err != nil {
				continue
			}
			payload.SessionID = session.ID
			data, _ := json.Marshal(payload)
			_ = agentConn.Send(agentmgr.Message{Type: agentmgr.MsgWebRTCInput, ID: session.ID, Data: data})

		case "stop":
			goto stop
		}
	}

stop:
	SendWebRTCStop(agentConn, session.ID)
}

// ProcessAgentWebRTCCapabilities handles webrtc.capabilities from agent.
func (d *Deps) ProcessAgentWebRTCCapabilities(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var caps agentmgr.WebRTCCapabilitiesData
	if err := json.Unmarshal(msg.Data, &caps); err != nil {
		return
	}
	conn.SetMeta("webrtc_available", strconv.FormatBool(caps.Available))
	conn.SetMeta("webrtc_unavailable_reason", strings.TrimSpace(caps.UnavailableReason))
	conn.SetMeta("webrtc_video_encoders", strings.Join(caps.VideoEncoders, ","))
	conn.SetMeta("webrtc_audio_sources", strings.Join(caps.AudioSources, ","))
	conn.SetMeta("desktop_session_type", strings.TrimSpace(caps.DesktopSessionType))
	conn.SetMeta("desktop_backend", strings.TrimSpace(caps.DesktopBackend))
	conn.SetMeta("desktop_capture_backend", strings.TrimSpace(caps.CaptureBackend))
	conn.SetMeta("desktop_vnc_real_desktop_supported", strconv.FormatBool(caps.VNCRealDesktopSupported))
	conn.SetMeta("desktop_webrtc_real_desktop_supported", strconv.FormatBool(caps.WebRTCRealDesktopSupported))
	log.Printf(
		"webrtc: agent=%s available=%v session_type=%s backend=%s reason=%q encoders=%v audio=%v",
		conn.AssetID, caps.Available, strings.TrimSpace(caps.DesktopSessionType), strings.TrimSpace(caps.DesktopBackend), strings.TrimSpace(caps.UnavailableReason), caps.VideoEncoders, caps.AudioSources,
	)
}

// ProcessAgentWebRTCStarted handles webrtc.started from agent.
func (d *Deps) ProcessAgentWebRTCStarted(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var started agentmgr.WebRTCStartedData
	if err := json.Unmarshal(msg.Data, &started); err != nil {
		return
	}
	raw, ok := d.WebRTCBridges.Load(started.SessionID)
	if !ok {
		return
	}
	bridge, ok := raw.(*WebRTCSignalingBridge)
	if !ok {
		return
	}
	if conn != nil && !bridge.MatchesAgent(conn.AssetID) {
		return
	}
	payload, _ := json.Marshal(map[string]any{"type": "ready", "data": started})
	_ = bridge.SendToBrowser(payload)
}

// ProcessAgentWebRTCAnswer handles webrtc.answer from agent.
func (d *Deps) ProcessAgentWebRTCAnswer(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var answer agentmgr.WebRTCSDPData
	if err := json.Unmarshal(msg.Data, &answer); err != nil {
		return
	}
	raw, ok := d.WebRTCBridges.Load(answer.SessionID)
	if !ok {
		return
	}
	bridge, ok := raw.(*WebRTCSignalingBridge)
	if !ok {
		return
	}
	if conn != nil && !bridge.MatchesAgent(conn.AssetID) {
		return
	}
	payload, _ := json.Marshal(map[string]any{"type": "answer", "data": answer})
	_ = bridge.SendToBrowser(payload)
}

// ProcessAgentWebRTCICE handles webrtc.ice from agent.
func (d *Deps) ProcessAgentWebRTCICE(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var ice agentmgr.WebRTCICEData
	if err := json.Unmarshal(msg.Data, &ice); err != nil {
		return
	}
	raw, ok := d.WebRTCBridges.Load(ice.SessionID)
	if !ok {
		return
	}
	bridge, ok := raw.(*WebRTCSignalingBridge)
	if !ok {
		return
	}
	if conn != nil && !bridge.MatchesAgent(conn.AssetID) {
		return
	}
	payload, _ := json.Marshal(map[string]any{"type": "ice", "data": ice})
	_ = bridge.SendToBrowser(payload)
}

// ProcessAgentWebRTCStopped handles webrtc.stopped from agent.
func (d *Deps) ProcessAgentWebRTCStopped(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var stopped agentmgr.WebRTCStoppedData
	if err := json.Unmarshal(msg.Data, &stopped); err != nil {
		return
	}
	raw, ok := d.WebRTCBridges.Load(stopped.SessionID)
	if !ok {
		return
	}
	bridge, ok := raw.(*WebRTCSignalingBridge)
	if !ok {
		return
	}
	if conn != nil && !bridge.MatchesAgent(conn.AssetID) {
		return
	}
	payload, _ := json.Marshal(map[string]any{"type": "stopped", "data": stopped})
	if err := bridge.SendToBrowser(payload); err != nil {
		log.Printf("webrtc: failed to relay stopped for session=%s: %v", stopped.SessionID, err)
	}
	bridge.Close()
}
