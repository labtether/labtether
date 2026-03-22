package dockerpkg

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
)

// ResolveDockerExecSessionTarget resolves a terminal session target to a
// docker exec agent + container pair.
func (d *Deps) ResolveDockerExecSessionTarget(target string) (agentID, containerID string, ok bool) {
	if d == nil || d.DockerCoordinator == nil {
		return "", "", false
	}
	host, container, found := d.DockerCoordinator.FindContainer(target)
	if !found || host == nil || container == nil {
		return "", "", false
	}
	return host.AgentID, container.ID, true
}

// HandleDockerExecTerminalStream handles a docker exec terminal WebSocket stream.
func (d *Deps) HandleDockerExecTerminalStream(
	w http.ResponseWriter,
	r *http.Request,
	session terminal.Session,
	agentID string,
	containerID string,
) {
	agentConn, ok := d.AgentMgr.Get(agentID)
	if !ok {
		servicehttp.WriteError(w, http.StatusBadGateway, "container host agent disconnected")
		return
	}

	wsConn, err := d.TerminalWebSocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer wsConn.Close()

	cols, rows := d.ParseTerminalSize(r.URL.Query().Get("cols"), r.URL.Query().Get("rows"))
	command := DockerExecCommandFromQuery(r.URL.Query().Get("shell"))

	outputCh := make(chan []byte, 256)
	closedCh := make(chan struct{})
	bridgeState := &DockerExecBridge{
		OutputCh:        outputCh,
		ClosedCh:        closedCh,
		ExpectedAgentID: agentID,
	}
	d.DockerExecBridges.Store(session.ID, bridgeState)
	defer func() {
		d.DockerExecBridges.Delete(session.ID)
		bridgeState.Close("")
	}()

	startData, _ := json.Marshal(agentmgr.DockerExecStartData{
		SessionID:   session.ID,
		ContainerID: containerID,
		Command:     command,
		TTY:         true,
		Cols:        cols,
		Rows:        rows,
	})
	if err := agentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgDockerExecStart,
		ID:   session.ID,
		Data: startData,
	}); err != nil {
		_ = wsConn.WriteJSON(map[string]any{"type": "error", "message": "failed to start container terminal"})
		return
	}
	defer func() {
		closeData, _ := json.Marshal(agentmgr.DockerExecCloseData{SessionID: session.ID})
		_ = agentConn.Send(agentmgr.Message{
			Type: agentmgr.MsgDockerExecClose,
			ID:   session.ID,
			Data: closeData,
		})
	}()
	var writeMu sync.Mutex

	select {
	case <-closedCh:
		closeReason := bridgeState.ReasonOr("container terminal session closed unexpectedly")
		_ = wsConn.WriteJSON(map[string]any{"type": "error", "message": closeReason})
		return
	case <-time.After(10 * time.Second):
		_ = wsConn.WriteJSON(map[string]any{"type": "error", "message": "container terminal start timed out"})
		return
	case data := <-outputCh:
		if data != nil {
			stopKeepalive := d.StartBrowserWSKeepalive(wsConn, &writeMu, "terminal-docker-exec:"+session.ID)
			defer stopKeepalive()
			go func() {
				writeMu.Lock()
				_ = wsConn.WriteMessage(websocket.BinaryMessage, data)
				writeMu.Unlock()
				d.bridgeDockerExecOutput(wsConn, outputCh, closedCh, &writeMu)
			}()
			d.bridgeDockerExecInput(wsConn, agentConn, session.ID, closedCh)
			return
		}
	}

	stopKeepalive := d.StartBrowserWSKeepalive(wsConn, &writeMu, "terminal-docker-exec:"+session.ID)
	defer stopKeepalive()
	go d.bridgeDockerExecOutput(wsConn, outputCh, closedCh, &writeMu)
	d.bridgeDockerExecInput(wsConn, agentConn, session.ID, closedCh)
}

func (d *Deps) bridgeDockerExecOutput(
	wsConn *websocket.Conn,
	outputCh <-chan []byte,
	closedCh <-chan struct{},
	writeMu *sync.Mutex,
) {
	for {
		select {
		case data, ok := <-outputCh:
			if !ok {
				return
			}
			if data == nil {
				continue
			}
			writeMu.Lock()
			_ = wsConn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			err := wsConn.WriteMessage(websocket.BinaryMessage, data)
			writeMu.Unlock()
			if err != nil {
				return
			}
		case <-closedCh:
			return
		}
	}
}

func (d *Deps) bridgeDockerExecInput(
	wsConn *websocket.Conn,
	agentConn *agentmgr.AgentConn,
	sessionID string,
	closedCh <-chan struct{},
) {
	for {
		select {
		case <-closedCh:
			return
		default:
		}

		messageType, payload, err := wsConn.ReadMessage()
		if err != nil {
			return
		}
		_ = d.TouchBrowserWSReadDeadline(wsConn)
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			continue
		}

		if d.IsControlMessage(payload) {
			msg := ControlMessage{}
			if json.Unmarshal(payload, &msg) == nil {
				switch msg.Type {
				case "resize":
					resizeData, _ := json.Marshal(agentmgr.DockerExecResizeData{
						SessionID: sessionID,
						Cols:      msg.Cols,
						Rows:      msg.Rows,
					})
					_ = agentConn.Send(agentmgr.Message{
						Type: agentmgr.MsgDockerExecResize,
						ID:   sessionID,
						Data: resizeData,
					})
					continue
				case "ping":
					continue
				case "input":
					payload = []byte(msg.Data)
				}
			}
		}

		encoded := base64.StdEncoding.EncodeToString(payload)
		inputData, _ := json.Marshal(agentmgr.DockerExecInputData{
			SessionID: sessionID,
			Data:      encoded,
		})
		_ = agentConn.Send(agentmgr.Message{
			Type: agentmgr.MsgDockerExecInput,
			ID:   sessionID,
			Data: inputData,
		})
	}
}

// ProcessAgentDockerExecStarted handles the docker.exec.started agent message.
func (d *Deps) ProcessAgentDockerExecStarted(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.DockerExecStartedData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}
	if bridge, ok := d.DockerExecBridges.Load(data.SessionID); ok {
		if b, ok := bridge.(*DockerExecBridge); ok {
			if conn == nil || !b.MatchesAgent(conn.AssetID) {
				return
			}
			b.TrySendOutput(nil)
		}
	}
}

// ProcessAgentDockerExecData handles the docker.exec.data agent message.
func (d *Deps) ProcessAgentDockerExecData(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var payload agentmgr.DockerExecDataPayload
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		return
	}
	decoded, err := base64.StdEncoding.DecodeString(payload.Data)
	if err != nil || len(decoded) == 0 {
		return
	}
	if bridge, ok := d.DockerExecBridges.Load(payload.SessionID); ok {
		if b, ok := bridge.(*DockerExecBridge); ok {
			if conn == nil || !b.MatchesAgent(conn.AssetID) {
				return
			}
			b.TrySendOutput(decoded)
		}
	}
}

// ProcessAgentDockerExecClosed handles the docker.exec.closed agent message.
func (d *Deps) ProcessAgentDockerExecClosed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.DockerExecCloseData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}
	if bridge, ok := d.DockerExecBridges.Load(data.SessionID); ok {
		if b, ok := bridge.(*DockerExecBridge); ok {
			if conn == nil || !b.MatchesAgent(conn.AssetID) {
				return
			}
			b.Close(data.Reason)
		}
	}
}

// CloseDockerExecBridgesForAsset closes all docker exec bridges for the given asset.
func (d *Deps) CloseDockerExecBridgesForAsset(assetID string) {
	trimmedAssetID := strings.TrimSpace(assetID)
	if trimmedAssetID == "" {
		return
	}
	d.DockerExecBridges.Range(func(_ any, value any) bool {
		bridge, ok := value.(*DockerExecBridge)
		if !ok {
			return true
		}
		if bridge.MatchesAgent(trimmedAssetID) {
			bridge.Close("container host agent disconnected")
		}
		return true
	})
}

// DockerExecCommandFromQuery parses the shell parameter for docker exec.
func DockerExecCommandFromQuery(raw string) []string {
	const (
		defaultShell = "sh"
		maxTokens    = 8
		maxTokenLen  = 128
	)

	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return []string{defaultShell}
	}

	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return []string{defaultShell}
	}
	if len(fields) > maxTokens {
		fields = fields[:maxTokens]
	}

	command := make([]string, 0, len(fields))
	for _, token := range fields {
		candidate := strings.TrimSpace(token)
		if candidate == "" || len(candidate) > maxTokenLen {
			continue
		}
		command = append(command, candidate)
	}
	if len(command) == 0 {
		return []string{defaultShell}
	}
	return command
}
