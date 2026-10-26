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
	"github.com/labtether/labtether/internal/hubapi/shared"
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

	wsConn, err := shared.UpgradeWebSocket(&d.TerminalWebSocketUpgrader, w, r, nil)
	if err != nil {
		return
	}
	shared.LimitBrowserInteractiveMessages(wsConn)
	defer wsConn.Close()

	bridgeState := &DockerExecBridge{
		OutputCh:        make(chan []byte, 256),
		ClosedCh:        make(chan struct{}),
		ReadyCh:         make(chan struct{}),
		ExpectedAgentID: agentConn.AssetID,
		AgentConn:       agentConn,
		SessionID:       session.ID,
		WireSessionID:   "docker-exec-stream:" + shared.GenerateRequestID(),
	}
	var outputDone chan struct{}
	defer func() {
		bridgeState.Close("")
		_ = wsConn.Close()
		if outputDone != nil {
			<-outputDone
		}
		d.finalizeDockerExecStream(bridgeState)
	}()
	detachPersistent, published := d.publishDockerExecStream(r.Context(), bridgeState, session, wsConn)
	if !published {
		writeDockerExecError(wsConn, "container host agent disconnected")
		return
	}
	defer detachPersistent()

	cols, rows := d.ParseTerminalSize(r.URL.Query().Get("cols"), r.URL.Query().Get("rows"))
	startData, _ := json.Marshal(agentmgr.DockerExecStartData{
		SessionID:   bridgeState.WireSessionID,
		ContainerID: containerID,
		Command:     DockerExecCommandFromQuery(r.URL.Query().Get("shell")),
		TTY:         true,
		Cols:        cols,
		Rows:        rows,
	})
	var startErr error
	if !shared.RunIfSessionStreamOpen(r.Context(), func() {
		startErr = d.startDockerExecStream(bridgeState, agentmgr.Message{
			Type: agentmgr.MsgDockerExecStart,
			ID:   bridgeState.WireSessionID,
			Data: startData,
		})
	}) {
		return
	}
	if startErr != nil {
		writeDockerExecError(wsConn, "failed to start container terminal")
		return
	}

	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case <-r.Context().Done():
		return
	case <-bridgeState.ClosedCh:
		// A fast shell can queue its last bytes and exit before this select.
		// Only startup failures without output need an error message.
		select {
		case <-bridgeState.ReadyCh:
		default:
			writeDockerExecError(wsConn, bridgeState.ReasonOr("container terminal session closed unexpectedly"))
		}
	case <-timer.C:
		writeDockerExecError(wsConn, "container terminal start timed out")
		return
	case <-bridgeState.ReadyCh:
	}

	var writeMu sync.Mutex
	stopKeepalive := d.StartBrowserWSKeepalive(wsConn, &writeMu, "terminal-docker-exec:"+session.ID)
	defer stopKeepalive()
	outputDone = make(chan struct{})
	go func() {
		defer close(outputDone)
		d.bridgeDockerExecOutput(wsConn, bridgeState, &writeMu)
	}()
	d.bridgeDockerExecInput(wsConn, agentConn, bridgeState.WireSessionID, bridgeState.ClosedCh)
	// The output loop owns browser closure on agent exit and drains final data
	// before releasing the blocked input read. If closure preceded that read,
	// wait here as well instead of racing the handler's deferred socket close.
	select {
	case <-bridgeState.ClosedCh:
		<-outputDone
	default:
	}
}

func writeDockerExecError(wsConn *websocket.Conn, message string) {
	_ = wsConn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_ = wsConn.WriteJSON(map[string]any{"type": "error", "message": message})
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
		select {
		case <-closedCh:
			return
		default:
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
