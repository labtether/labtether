package remoteaccess

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

var DesktopDebugEnabled = sync.OnceValue(func() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("LABTETHER_DESKTOP_DEBUG")), "true")
})

var StartDesktopVNCServer = StartVNCServer
var DialDesktopVNC = net.DialTimeout

// DesktopSession tracks an active VNC desktop session on the agent.
type DesktopSession struct {
	sessionID      string
	vncCmd         *exec.Cmd // nil if using pre-existing VNC server
	xvfbCmd        *exec.Cmd // nil if no Xvfb headless fallback was used
	bootstrapCmd   *exec.Cmd // nil if no fallback shell/window was started
	vncConn        net.Conn
	vncAuthFile    string
	port           int
	done           chan struct{}
	ManagedDisplay string // non-empty if display lifecycle is owned by DisplayManager
}

const MaxDesktopSessions = 10

// DesktopManager manages VNC desktop sessions on the agent.
type DesktopManager struct {
	Mu         sync.Mutex
	Sessions   map[string]*DesktopSession
	DisplayMgr *DisplayManager
}

func NewDesktopManager(dispMgr *DisplayManager) *DesktopManager {
	return &DesktopManager{
		Sessions:   make(map[string]*DesktopSession),
		DisplayMgr: dispMgr,
	}
}

// handleDesktopStart starts a VNC server and connects to it.
func (dm *DesktopManager) HandleDesktopStart(transport MessageSender, msg agentmgr.Message) {
	var req agentmgr.DesktopStartData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("desktop: invalid start request: %v", err)
		return
	}

	if req.SessionID == "" {
		log.Printf("desktop: start request missing session_id")
		return
	}

	dm.Mu.Lock()
	if len(dm.Sessions) >= MaxDesktopSessions {
		dm.Mu.Unlock()
		log.Printf("desktop: max sessions (%d) reached", MaxDesktopSessions)
		SendDesktopClosed(transport, req.SessionID, "max desktop sessions reached")
		return
	}
	if _, exists := dm.Sessions[req.SessionID]; exists {
		dm.Mu.Unlock()
		log.Printf("desktop: session %s already exists", req.SessionID)
		return
	}
	// Reserve slot to make session-limit check atomic across concurrent starts.
	dm.Sessions[req.SessionID] = nil
	dm.Mu.Unlock()

	display := req.Display
	quality := req.Quality
	if quality == "" {
		quality = "medium"
	}

	// Start VNC server.
	vncCmd, xvfbCmd, bootstrapCmd, port, vncAuthFile, err := StartDesktopVNCServer(display, quality, req.VNCPassword)
	if err != nil {
		dm.Mu.Lock()
		if current, exists := dm.Sessions[req.SessionID]; exists && current == nil {
			delete(dm.Sessions, req.SessionID)
		}
		dm.Mu.Unlock()
		log.Printf("desktop: failed to start VNC server for session %s: %v", req.SessionID, err)
		SendDesktopClosed(transport, req.SessionID, fmt.Sprintf("failed to start VNC: %v", err))
		return
	}

	// TCP connect to the local VNC server.
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	vncConn, err := DialDesktopVNC("tcp", addr, 5*time.Second)
	if err != nil {
		dm.Mu.Lock()
		if current, exists := dm.Sessions[req.SessionID]; exists && current == nil {
			delete(dm.Sessions, req.SessionID)
		}
		dm.Mu.Unlock()
		log.Printf("desktop: failed to connect to VNC on port %d for session %s: %v", port, req.SessionID, err)
		TerminateProcess(vncCmd)
		TerminateProcess(xvfbCmd)
		TerminateProcess(bootstrapCmd)
		RemoveProcessLog(vncAuthFile)
		SendDesktopClosed(transport, req.SessionID, fmt.Sprintf("failed to connect to VNC: %v", err))
		return
	}

	sess := &DesktopSession{
		sessionID:    req.SessionID,
		vncCmd:       vncCmd,
		xvfbCmd:      xvfbCmd,
		bootstrapCmd: bootstrapCmd,
		vncConn:      vncConn,
		vncAuthFile:  vncAuthFile,
		port:         port,
		done:         make(chan struct{}),
	}

	dm.Mu.Lock()
	if _, stillReserved := dm.Sessions[req.SessionID]; !stillReserved {
		dm.Mu.Unlock()
		_ = vncConn.Close()
		TerminateProcess(vncCmd)
		TerminateProcess(xvfbCmd)
		TerminateProcess(bootstrapCmd)
		RemoveProcessLog(vncAuthFile)
		SendDesktopClosed(transport, req.SessionID, "desktop session cancelled during startup")
		return
	}
	dm.Sessions[req.SessionID] = sess
	dm.Mu.Unlock()

	// Notify hub that desktop is ready.
	SendDesktopStarted(transport, req.SessionID, req.Width, req.Height)

	// Stream VNC output → hub.
	go dm.streamVNCOutput(transport, sess)

	// Monitor Xvfb process — close session if it exits unexpectedly.
	if sess.xvfbCmd != nil {
		go dm.watchXvfbExit(transport, sess)
	}

	log.Printf("desktop: session %s started on port %d", req.SessionID, port)
}

// handleDesktopData writes incoming VNC data to the local VNC connection.
func (dm *DesktopManager) HandleDesktopData(msg agentmgr.Message) {
	var payload agentmgr.DesktopDataPayload
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		log.Printf("desktop: failed to unmarshal input data: %v", err)
		return
	}

	dm.Mu.Lock()
	sess, ok := dm.Sessions[payload.SessionID]
	dm.Mu.Unlock()
	if !ok {
		log.Printf("desktop: input data for unknown session %s", payload.SessionID)
		return
	}

	decoded, err := base64.StdEncoding.DecodeString(payload.Data)
	if err != nil {
		log.Printf("desktop: failed to decode input data for session %s: %v", payload.SessionID, err)
		return
	}

	if _, writeErr := sess.vncConn.Write(decoded); writeErr != nil {
		log.Printf("desktop: failed to write to VNC for session %s: %v", payload.SessionID, writeErr)
	}
}

// handleDesktopClose terminates a desktop session.
func (dm *DesktopManager) HandleDesktopClose(msg agentmgr.Message) {
	var req agentmgr.DesktopCloseData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		return
	}

	dm.cleanup(req.SessionID)
}

// closeAll terminates all active desktop sessions (called on agent shutdown).
func (dm *DesktopManager) CloseAll() {
	dm.Mu.Lock()
	sessions := make([]*DesktopSession, 0, len(dm.Sessions))
	for _, sess := range dm.Sessions {
		sessions = append(sessions, sess)
	}
	dm.Sessions = make(map[string]*DesktopSession)
	dm.Mu.Unlock()

	for _, sess := range sessions {
		dm.closeSessionResources(sess)
	}
}

// streamVNCOutput reads from the VNC TCP connection and sends to the hub.
func (dm *DesktopManager) streamVNCOutput(transport MessageSender, sess *DesktopSession) {
	buf := make([]byte, 16384)
	var totalBytes int64
	var frameCount int64
	lastLog := time.Now()
	for {
		n, err := sess.vncConn.Read(buf)
		if n > 0 {
			totalBytes += int64(n)
			frameCount++
			if DesktopDebugEnabled() && time.Since(lastLog) > 5*time.Second {
				log.Printf("desktop-debug: session=%s vnc_output bytes=%d frames=%d", sess.sessionID, totalBytes, frameCount)
				lastLog = time.Now()
			}
			encoded := base64.StdEncoding.EncodeToString(buf[:n])
			data, _ := json.Marshal(agentmgr.DesktopDataPayload{
				SessionID: sess.sessionID,
				Data:      encoded,
			})
			_ = transport.Send(agentmgr.Message{
				Type: agentmgr.MsgDesktopData,
				ID:   sess.sessionID,
				Data: data,
			})
		}
		if err != nil {
			log.Printf("desktop: VNC stream ended for session %s after %d bytes %d frames: %v", sess.sessionID, totalBytes, frameCount, err)
			dm.cleanup(sess.sessionID)
			SendDesktopClosed(transport, sess.sessionID, "VNC connection closed")
			return
		}
	}
}

func (dm *DesktopManager) watchXvfbExit(transport MessageSender, sess *DesktopSession) {
	if sess.xvfbCmd == nil || sess.xvfbCmd.Process == nil {
		return
	}
	_ = sess.xvfbCmd.Wait()
	// Check if session is still active (not already cleaned up).
	dm.Mu.Lock()
	_, stillActive := dm.Sessions[sess.sessionID]
	dm.Mu.Unlock()
	if stillActive {
		log.Printf("desktop: Xvfb exited unexpectedly for session %s, closing", sess.sessionID)
		dm.cleanup(sess.sessionID)
		SendDesktopClosed(transport, sess.sessionID, "Xvfb process exited unexpectedly")
	}
}

func (dm *DesktopManager) cleanup(sessionID string) {
	dm.Mu.Lock()
	sess, ok := dm.Sessions[sessionID]
	if ok {
		delete(dm.Sessions, sessionID)
	}
	dm.Mu.Unlock()
	if ok {
		dm.closeSessionResources(sess)
	}
}

func (dm *DesktopManager) closeSessionResources(sess *DesktopSession) {
	if sess == nil {
		return
	}
	_ = sess.vncConn.Close()
	TerminateProcess(sess.vncCmd)
	if sess.ManagedDisplay != "" && dm.DisplayMgr != nil {
		dm.DisplayMgr.release(sess.ManagedDisplay)
	} else {
		TerminateProcess(sess.xvfbCmd)
	}
	TerminateProcess(sess.bootstrapCmd)
	RemoveProcessLog(sess.vncAuthFile)
	select {
	case <-sess.done:
	default:
		close(sess.done)
	}
}

func TerminateProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}

func SendDesktopStarted(transport MessageSender, sessionID string, width, height int) {
	data, _ := json.Marshal(agentmgr.DesktopStartedData{
		SessionID: sessionID,
		Width:     width,
		Height:    height,
	})
	_ = transport.Send(agentmgr.Message{
		Type: agentmgr.MsgDesktopStarted,
		ID:   sessionID,
		Data: data,
	})
}

func SendDesktopClosed(transport MessageSender, sessionID, reason string) {
	data, _ := json.Marshal(agentmgr.DesktopCloseData{SessionID: sessionID, Reason: reason})
	_ = transport.Send(agentmgr.Message{
		Type: agentmgr.MsgDesktopClosed,
		ID:   sessionID,
		Data: data,
	})
}
