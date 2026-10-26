package dockerpkg

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const dockerExecFinalOutputTimeout = 2 * time.Second

func (d *Deps) bridgeDockerExecOutput(wsConn *websocket.Conn, bridge *DockerExecBridge, writeMu *sync.Mutex) {
	// ReadMessage in the input loop cannot observe ClosedCh while blocked.
	// Closing this specific browser releases it on shell exit or write failure.
	defer wsConn.Close()
	for {
		select {
		case <-bridge.ClosedCh:
			if drainDockerExecOutput(wsConn, bridge.OutputCh, writeMu) {
				closeDockerExecBrowser(wsConn, bridge.ReasonOr(""))
			}
			return
		default:
		}
		select {
		case <-bridge.ClosedCh:
			if drainDockerExecOutput(wsConn, bridge.OutputCh, writeMu) {
				closeDockerExecBrowser(wsConn, bridge.ReasonOr(""))
			}
			return
		case data, ok := <-bridge.OutputCh:
			if !ok {
				return
			}
			if len(data) != 0 && !writeDockerExecOutput(wsConn, writeMu, data, time.Now().Add(10*time.Second)) {
				return
			}
		}
	}
}

func drainDockerExecOutput(wsConn *websocket.Conn, outputCh <-chan []byte, writeMu *sync.Mutex) bool {
	// Close fences TrySendOutput, so this is a finite snapshot. All remaining
	// writes share one deadline; a stalled browser cannot hold cleanup forever.
	deadline := time.Now().Add(dockerExecFinalOutputTimeout)
	for remaining := len(outputCh); remaining > 0; remaining-- {
		select {
		case data, ok := <-outputCh:
			if !ok || time.Now().After(deadline) {
				return false
			}
			if len(data) != 0 && !writeDockerExecOutput(wsConn, writeMu, data, deadline) {
				return false
			}
		default:
			return true
		}
	}
	return true
}

func writeDockerExecOutput(wsConn *websocket.Conn, writeMu *sync.Mutex, data []byte, deadline time.Time) bool {
	writeMu.Lock()
	defer writeMu.Unlock()
	_ = wsConn.SetWriteDeadline(deadline)
	return wsConn.WriteMessage(websocket.BinaryMessage, data) == nil
}

func closeDockerExecBrowser(wsConn *websocket.Conn, reason string) {
	code, message := websocket.CloseInternalServerErr, "Container terminal ended"
	switch reason {
	case "", "exec ended":
		code, message = websocket.CloseNormalClosure, ""
	case "session attached elsewhere":
		code, message = websocket.CloseNormalClosure, "Session attached elsewhere"
	case "container host agent disconnected":
		code, message = websocket.CloseGoingAway, "Container host disconnected"
	case "container terminal output buffer overflow":
		message = "Container terminal output buffer overflow"
	}
	_ = wsConn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, message), time.Now().Add(dockerExecFinalOutputTimeout))
}
