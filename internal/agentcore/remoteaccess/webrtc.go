package remoteaccess

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/agentcore/files"
	"github.com/labtether/labtether/internal/agentcore/sysconfig"
	"github.com/pion/interceptor"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"

	"github.com/labtether/labtether/internal/agentmgr"
)

type WebRTCInputEvent struct {
	Type    string `json:"type"`
	KeyCode int    `json:"keyCode,omitempty"`
	Code    string `json:"code,omitempty"`
	Key     string `json:"key,omitempty"`
	X       int    `json:"x,omitempty"`
	Y       int    `json:"y,omitempty"`
	Button  int    `json:"button,omitempty"`
	DeltaY  int    `json:"deltaY,omitempty"`
}

type WebRTCClipboardMessage struct {
	Type   string `json:"type"`
	Format string `json:"format,omitempty"`
	Text   string `json:"text,omitempty"`
	Error  string `json:"error,omitempty"`
}

type WebRTCFileTransferMessage struct {
	Type         string `json:"type"`
	RequestID    string `json:"request_id"`
	Name         string `json:"name,omitempty"`
	Path         string `json:"path,omitempty"`
	Data         string `json:"data,omitempty"`
	Done         bool   `json:"done,omitempty"`
	BytesWritten int64  `json:"bytes_written,omitempty"`
	Error        string `json:"error,omitempty"`
}

type WebRTCSession struct {
	sessionID      string
	pc             *webrtc.PeerConnection
	videoTrack     *webrtc.TrackLocalStaticRTP
	audioTrack     *webrtc.TrackLocalStaticRTP
	gstVideoCmd    *exec.Cmd
	gstAudioCmd    *exec.Cmd
	videoLogPath   string
	audioLogPath   string
	videoPort      int
	audioPort      int
	inputCh        chan WebRTCInputEvent
	cancel         context.CancelFunc
	done           chan struct{}
	closeOnce      sync.Once
	ManagedDisplay string          // non-empty if display was acquired from DisplayManager
	xauthPath      string          // Xauthority for ManagedDisplay when one was created
	dispMgr        *DisplayManager // reference for release on close
	desktopBackend string
	sessionInfo    DesktopSessionInfo
	inputBackend   string
}

func ResolveWebRTCDisplay(requested string, caps agentmgr.WebRTCCapabilitiesData) string {
	if strings.EqualFold(strings.TrimSpace(caps.DesktopSessionType), DesktopSessionTypeWayland) {
		return ""
	}
	display := strings.TrimSpace(requested)
	if display != "" {
		for _, candidate := range caps.Displays {
			if display == strings.TrimSpace(candidate) {
				return display
			}
		}
		if strings.HasPrefix(display, ":") {
			return display
		}
	}
	for _, candidate := range caps.Displays {
		trimmed := strings.TrimSpace(candidate)
		if trimmed != "" {
			return trimmed
		}
	}
	return ":0"
}

func WebRTCVideoBitrateForQuality(quality string) int {
	switch strings.ToLower(strings.TrimSpace(quality)) {
	case "low":
		return 1000
	case "high":
		return 20000
	default:
		return 5000
	}
}

func (s *WebRTCSession) close(reason string) {
	s.closeOnce.Do(func() {
		log.Printf(
			"webrtc: closing session=%s reason=%s display=%s xauth=%s video_port=%d audio_port=%d",
			s.sessionID,
			strings.TrimSpace(reason),
			ValueOrDash(strings.TrimSpace(s.ManagedDisplay)),
			ValueOrDash(strings.TrimSpace(s.xauthPath)),
			s.videoPort,
			s.audioPort,
		)
		if s.cancel != nil {
			s.cancel()
		}
		if s.gstVideoCmd != nil && s.gstVideoCmd.Process != nil {
			_ = s.gstVideoCmd.Process.Kill()
		}
		if s.gstAudioCmd != nil && s.gstAudioCmd.Process != nil {
			_ = s.gstAudioCmd.Process.Kill()
		}
		if s.pc != nil {
			_ = s.pc.Close()
		}
		if s.ManagedDisplay != "" && s.dispMgr != nil {
			s.dispMgr.release(s.ManagedDisplay)
		}
		RemoveProcessLog(s.videoLogPath)
		RemoveProcessLog(s.audioLogPath)
		close(s.done)
	})
}

type WebRTCManager struct {
	Mu       sync.Mutex
	Sessions map[string]*WebRTCSession
	caps     agentmgr.WebRTCCapabilitiesData
	settings SettingsProvider
	fileMgr  *files.Manager
	dispMgr  *DisplayManager
}

func NewWebRTCManager(caps agentmgr.WebRTCCapabilitiesData, settings SettingsProvider, fileMgr *files.Manager, dispMgr *DisplayManager) *WebRTCManager {
	return &WebRTCManager{
		Sessions: make(map[string]*WebRTCSession),
		caps:     caps,
		settings: settings,
		fileMgr:  fileMgr,
		dispMgr:  dispMgr,
	}
}

func (wm *WebRTCManager) HandleWebRTCStart(transport MessageSender, msg agentmgr.Message) {
	var req agentmgr.WebRTCSessionData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("webrtc: invalid start request: %v", err)
		return
	}
	if strings.TrimSpace(req.SessionID) == "" {
		log.Printf("webrtc: missing session_id")
		return
	}

	if !wm.caps.Available {
		SendWebRTCStopped(transport, req.SessionID, "webrtc unavailable")
		return
	}

	wm.Mu.Lock()
	if _, exists := wm.Sessions[req.SessionID]; exists {
		wm.Mu.Unlock()
		return
	}
	wm.Mu.Unlock()

	settings := map[string]string{}
	if wm.settings != nil {
		settings = wm.settings.ReportedAgentSettings()
	}
	webrtcCfg := LoadWebRTCConfig(settings)
	if !webrtcCfg.Enabled {
		SendWebRTCStopped(transport, req.SessionID, "webrtc disabled")
		return
	}
	sessionInfo := DetectDesktopSessionFn()

	encName, gstEncoder := BestVideoEncoder(wm.caps)
	if gstEncoder == "" {
		SendWebRTCStopped(transport, req.SessionID, "no supported encoder")
		return
	}

	audioSource := ""
	if req.AudioEnabled {
		audioSource = BestAudioSource(wm.caps)
	}

	videoPort, err := FindFreeUDPPort()
	if err != nil {
		SendWebRTCStopped(transport, req.SessionID, "failed to allocate video RTP port")
		return
	}
	audioPort := 0
	if audioSource != "" {
		audioPort, err = FindFreeUDPPort()
		if err != nil {
			log.Printf("webrtc: audio port unavailable for %s, continuing without audio", req.SessionID)
			audioSource = ""
		}
	}

	m := &webrtc.MediaEngine{}
	if err := m.RegisterDefaultCodecs(); err != nil {
		SendWebRTCStopped(transport, req.SessionID, "codec registration failed")
		return
	}
	i := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(m, i); err != nil {
		SendWebRTCStopped(transport, req.SessionID, "interceptor registration failed")
		return
	}
	api := webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithInterceptorRegistry(i))

	pc, err := api.NewPeerConnection(webrtc.Configuration{ICEServers: webrtcCfg.iceServers()})
	if err != nil {
		SendWebRTCStopped(transport, req.SessionID, "peer connection init failed")
		return
	}

	videoCodec := webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}
	if encName == "vp8" {
		videoCodec = webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8}
	}
	videoTrack, err := webrtc.NewTrackLocalStaticRTP(videoCodec, "video", "labtether-screen")
	if err != nil {
		_ = pc.Close()
		SendWebRTCStopped(transport, req.SessionID, "video track creation failed")
		return
	}
	if _, err := pc.AddTrack(videoTrack); err != nil {
		_ = pc.Close()
		SendWebRTCStopped(transport, req.SessionID, "video track attach failed")
		return
	}

	var audioTrack *webrtc.TrackLocalStaticRTP
	if audioSource != "" {
		audioTrack, err = webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
			"audio", "labtether-audio",
		)
		if err == nil {
			if _, addErr := pc.AddTrack(audioTrack); addErr != nil {
				audioTrack = nil
				audioSource = ""
				audioPort = 0
			}
		} else {
			audioTrack = nil
			audioSource = ""
			audioPort = 0
		}
	}

	// Resolve and acquire the display before session registration so that
	// OnConnectionStateChange cannot fire with an unset ManagedDisplay field.
	display := ResolveWebRTCDisplay(req.Display, wm.caps)
	var acquiredDisplay string // non-empty if acquired from display manager
	var xauthPath string
	var sessDispMgr *DisplayManager
	if sessionInfo.Type != DesktopSessionTypeWayland && !IsDisplayAvailable(display) && wm.dispMgr != nil {
		dynDisplay, dynXAuthPath, acquireErr := wm.dispMgr.acquire()
		if acquireErr != nil {
			_ = pc.Close()
			SendWebRTCStopped(transport, req.SessionID, "no display available: "+acquireErr.Error())
			return
		}
		display = dynDisplay
		acquiredDisplay = dynDisplay
		xauthPath = dynXAuthPath
		sessDispMgr = wm.dispMgr
	}
	if sessionInfo.Type != DesktopSessionTypeWayland {
		if strings.TrimSpace(xauthPath) == "" {
			xauthPath = DiscoverDisplayXAuthorityFn(display)
		}
		WakeX11Display(display, xauthPath)
	}

	ctx, cancel := context.WithCancel(context.Background())
	sess := &WebRTCSession{
		sessionID:      req.SessionID,
		pc:             pc,
		videoTrack:     videoTrack,
		audioTrack:     audioTrack,
		videoPort:      videoPort,
		audioPort:      audioPort,
		inputCh:        make(chan WebRTCInputEvent, 128),
		cancel:         cancel,
		done:           make(chan struct{}),
		ManagedDisplay: acquiredDisplay,
		xauthPath:      xauthPath,
		dispMgr:        sessDispMgr,
		desktopBackend: strings.TrimSpace(wm.caps.DesktopBackend),
		sessionInfo:    sessionInfo,
		inputBackend:   strings.TrimSpace(strings.ToLower(webrtcCfg.WaylandInputBackend)),
	}
	if sess.inputBackend == "" {
		sess.inputBackend = "auto"
	}

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		if dc == nil {
			return
		}
		switch dc.Label() {
		case "input":
			dc.OnMessage(func(msg webrtc.DataChannelMessage) {
				evt, err := DecodeWebRTCInputEvent(msg.Data)
				if err != nil {
					return
				}
				select {
				case sess.inputCh <- evt:
				default:
				}
			})
		case "clipboard":
			dc.OnMessage(func(msg webrtc.DataChannelMessage) {
				wm.handleClipboardDataChannelMessage(dc, msg)
			})
		case "file-transfer":
			dc.OnMessage(func(msg webrtc.DataChannelMessage) {
				wm.handleFileTransferDataChannelMessage(dc, msg)
			})
		}
	})

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		candidate := c.ToJSON()
		data := agentmgr.WebRTCICEData{
			SessionID: req.SessionID,
			Candidate: candidate.Candidate,
		}
		if candidate.SDPMid != nil {
			data.SDPMid = *candidate.SDPMid
		}
		if candidate.SDPMLineIndex != nil {
			idx := int(*candidate.SDPMLineIndex)
			data.SDPMLineIndex = &idx
		}
		raw, _ := json.Marshal(data)
		sendCandidate := func() {
			_ = transport.Send(agentmgr.Message{Type: agentmgr.MsgWebRTCICE, ID: req.SessionID, Data: raw})
		}
		if delay := ICECandidateSendDelay(candidate.Candidate); delay > 0 {
			go func() {
				timer := time.NewTimer(delay)
				defer timer.Stop()
				select {
				case <-ctx.Done():
					return
				case <-timer.C:
					sendCandidate()
				}
			}()
			return
		}
		sendCandidate()
	})

	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Printf("webrtc: ice state session=%s state=%s", req.SessionID, state.String())
	})

	pc.OnICEGatheringStateChange(func(state webrtc.ICEGatheringState) {
		log.Printf("webrtc: ice gathering session=%s state=%s", req.SessionID, state.String())
	})

	pc.OnSignalingStateChange(func(state webrtc.SignalingState) {
		log.Printf("webrtc: signaling state session=%s state=%s", req.SessionID, state.String())
	})

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("webrtc: peer state session=%s state=%s", req.SessionID, state.String())
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
			wm.CleanupWithReason(req.SessionID, "peer connection "+state.String())
			SendWebRTCStopped(transport, req.SessionID, "peer connection "+state.String())
		}
	})

	wm.Mu.Lock()
	wm.Sessions[req.SessionID] = sess
	wm.Mu.Unlock()
	width := req.Width
	if width <= 0 {
		width = 1920
	}
	height := req.Height
	if height <= 0 {
		height = 1080
	}
	fps := req.FPS
	if fps <= 0 {
		fps = webrtcCfg.FPS
	}
	videoBitrate := WebRTCVideoBitrateForQuality(req.Quality)

	videoPipeline := BuildGStreamerVideoPipeline(GstPipelineConfig{
		display: display,
		encoder: gstEncoder,
		width:   width,
		height:  height,
		fps:     fps,
		bitrate: videoBitrate,
		rtpPort: videoPort,
	})
	if sessionInfo.Type == DesktopSessionTypeWayland {
		videoPipeline = BuildWaylandPipeWireVideoPipeline(webrtcCfg.WaylandPipeWireNodeID, GstPipelineConfig{
			encoder: gstEncoder,
			width:   width,
			height:  height,
			fps:     fps,
			bitrate: videoBitrate,
			rtpPort: videoPort,
		})
	}
	if DesktopDebugEnabled() {
		log.Printf("webrtc-debug: session=%s video_pipeline=%s", req.SessionID, videoPipeline)
	}
	gstVideoCmd, err := NewWebRTCSecurityCommand("gst-launch-1.0", ParsePipelineArgs(videoPipeline)...)
	if err != nil {
		wm.CleanupWithReason(req.SessionID, "gst-launch unavailable")
		SendWebRTCStopped(transport, req.SessionID, "gst-launch unavailable")
		return
	}
	if sessionInfo.Type == DesktopSessionTypeWayland {
		gstVideoCmd.Env = BuildWaylandPipeWireEnv(sessionInfo)
	} else {
		gstVideoCmd.Env = BuildX11ClientEnv(display, xauthPath)
	}
	videoLogPath, err := StartWebRTCPipelineWithLog(gstVideoCmd, "labtether-webrtc-video-*.log")
	if err != nil {
		wm.CleanupWithReason(req.SessionID, "failed to start video pipeline")
		SendWebRTCStopped(transport, req.SessionID, "failed to start video pipeline")
		return
	}
	sess.gstVideoCmd = gstVideoCmd
	sess.videoLogPath = videoLogPath
	go wm.WatchPipelineExit(ctx, req.SessionID, "video", gstVideoCmd, videoLogPath, transport)

	if audioTrack != nil && audioPort > 0 && audioSource != "" {
		audioPipeline := BuildGStreamerAudioPipeline(GstAudioConfig{
			source:  audioSource,
			rtpPort: audioPort,
		})
		gstAudioCmd, audioErr := NewWebRTCSecurityCommand("gst-launch-1.0", ParsePipelineArgs(audioPipeline)...)
		if audioErr == nil {
			if sessionInfo.Type == DesktopSessionTypeWayland {
				gstAudioCmd.Env = BuildWaylandPipeWireEnv(sessionInfo)
			} else {
				gstAudioCmd.Env = BuildX11ClientEnv(display, xauthPath)
			}
			audioLogPath, startErr := StartWebRTCPipelineWithLog(gstAudioCmd, "labtether-webrtc-audio-*.log")
			if startErr == nil {
				sess.gstAudioCmd = gstAudioCmd
				sess.audioLogPath = audioLogPath
				go wm.WatchPipelineExit(ctx, req.SessionID, "audio", gstAudioCmd, audioLogPath, transport)
			} else {
				log.Printf("webrtc: failed to start audio pipeline for %s: %v", req.SessionID, startErr)
			}
		} else {
			log.Printf("webrtc: failed to build audio pipeline command for %s: %v", req.SessionID, audioErr)
		}
	}

	go ReadRTPToTrack(ctx, videoPort, videoTrack)
	if audioTrack != nil && audioPort > 0 {
		go ReadRTPToTrack(ctx, audioPort, audioTrack)
	}
	go InjectInputEvents(ctx, sess.inputCh, display, xauthPath, sess)

	startedData, _ := json.Marshal(agentmgr.WebRTCStartedData{
		SessionID:    req.SessionID,
		VideoEncoder: encName,
		AudioSource:  audioSource,
	})
	_ = transport.Send(agentmgr.Message{Type: agentmgr.MsgWebRTCStarted, ID: req.SessionID, Data: startedData})

	log.Printf("webrtc: session started id=%s encoder=%s audio=%s", req.SessionID, encName, audioSource)
}

func (wm *WebRTCManager) HandleWebRTCOffer(msg agentmgr.Message, transport MessageSender) {
	var offer agentmgr.WebRTCSDPData
	if err := json.Unmarshal(msg.Data, &offer); err != nil {
		return
	}
	if strings.TrimSpace(offer.SessionID) == "" || strings.TrimSpace(offer.SDP) == "" {
		return
	}

	wm.Mu.Lock()
	sess, ok := wm.Sessions[offer.SessionID]
	wm.Mu.Unlock()
	if !ok {
		return
	}

	log.Printf("webrtc: received offer for session=%s", offer.SessionID)
	if err := sess.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offer.SDP}); err != nil {
		log.Printf("webrtc: set remote description failed for %s: %v", offer.SessionID, err)
		return
	}

	answer, err := sess.pc.CreateAnswer(nil)
	if err != nil {
		log.Printf("webrtc: create answer failed for %s: %v", offer.SessionID, err)
		return
	}
	if err := sess.pc.SetLocalDescription(answer); err != nil {
		log.Printf("webrtc: set local description failed for %s: %v", offer.SessionID, err)
		return
	}
	log.Printf("webrtc: created answer for session=%s", offer.SessionID)

	answerData, _ := json.Marshal(agentmgr.WebRTCSDPData{
		SessionID: offer.SessionID,
		Type:      "answer",
		SDP:       answer.SDP,
	})
	_ = transport.Send(agentmgr.Message{Type: agentmgr.MsgWebRTCAnswer, ID: offer.SessionID, Data: answerData})
}

func (wm *WebRTCManager) HandleWebRTCICE(msg agentmgr.Message) {
	var data agentmgr.WebRTCICEData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}
	if strings.TrimSpace(data.SessionID) == "" || strings.TrimSpace(data.Candidate) == "" {
		return
	}

	wm.Mu.Lock()
	sess, ok := wm.Sessions[data.SessionID]
	wm.Mu.Unlock()
	if !ok {
		return
	}

	candidate := webrtc.ICECandidateInit{Candidate: data.Candidate}
	if strings.TrimSpace(data.SDPMid) != "" {
		sdpMid := strings.TrimSpace(data.SDPMid)
		candidate.SDPMid = &sdpMid
	}
	if data.SDPMLineIndex != nil {
		if *data.SDPMLineIndex < 0 || *data.SDPMLineIndex > math.MaxUint16 {
			log.Printf("webrtc: ignoring ICE candidate with invalid m-line index %d for %s", *data.SDPMLineIndex, data.SessionID)
			return
		}
		index := uint16(*data.SDPMLineIndex)
		candidate.SDPMLineIndex = &index
	}
	if err := sess.pc.AddICECandidate(candidate); err != nil {
		log.Printf("webrtc: add ICE candidate failed for %s: %v", data.SessionID, err)
	}
}

func (wm *WebRTCManager) HandleWebRTCStop(msg agentmgr.Message, transport MessageSender) {
	var req agentmgr.WebRTCStoppedData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		return
	}
	req.SessionID = strings.TrimSpace(req.SessionID)
	if req.SessionID == "" {
		return
	}
	wm.CleanupWithReason(req.SessionID, "stopped by hub")
	SendWebRTCStopped(transport, req.SessionID, "stopped by hub")
}

func (wm *WebRTCManager) HandleWebRTCInput(msg agentmgr.Message) {
	var data agentmgr.WebRTCInputData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}
	wm.Mu.Lock()
	sess, ok := wm.Sessions[strings.TrimSpace(data.SessionID)]
	wm.Mu.Unlock()
	if !ok {
		return
	}
	select {
	case sess.inputCh <- WebRTCInputEvent{
		Type:    strings.TrimSpace(data.Type),
		KeyCode: data.KeyCode,
		Code:    strings.TrimSpace(data.Code),
		Key:     strings.TrimSpace(data.Key),
		X:       data.X,
		Y:       data.Y,
		Button:  data.Button,
		DeltaY:  data.DeltaY,
	}:
	default:
	}
}

func (wm *WebRTCManager) Cleanup(sessionID string) {
	wm.CleanupWithReason(sessionID, "cleanup requested")
}

func (wm *WebRTCManager) MarkAudioPipelineStopped(sessionID string) {
	wm.Mu.Lock()
	defer wm.Mu.Unlock()
	sess, ok := wm.Sessions[sessionID]
	if !ok {
		return
	}
	sess.gstAudioCmd = nil
	sess.audioLogPath = ""
	sess.audioPort = 0
}

func (wm *WebRTCManager) CleanupWithReason(sessionID, reason string) {
	wm.Mu.Lock()
	sess, ok := wm.Sessions[sessionID]
	if ok {
		delete(wm.Sessions, sessionID)
	}
	wm.Mu.Unlock()
	if !ok {
		return
	}
	log.Printf("webrtc: cleanup session=%s trigger=%s", sessionID, strings.TrimSpace(reason))
	sess.close(reason)
}

func (wm *WebRTCManager) CloseAll() {
	wm.Mu.Lock()
	sessions := make([]*WebRTCSession, 0, len(wm.Sessions))
	for id, sess := range wm.Sessions {
		sessions = append(sessions, sess)
		delete(wm.Sessions, id)
	}
	wm.Mu.Unlock()
	for _, sess := range sessions {
		sess.close("manager closeAll")
	}
}

func (wm *WebRTCManager) WatchPipelineExit(ctx context.Context, sessionID, streamType string, cmd *exec.Cmd, logPath string, transport MessageSender) {
	err := cmd.Wait()
	select {
	case <-ctx.Done():
		RemoveProcessLog(logPath)
		return
	default:
	}
	reason := fmt.Sprintf("%s pipeline stopped", streamType)
	if err != nil {
		reason = fmt.Sprintf("%s pipeline stopped: %v", streamType, err)
	}
	if logTail := strings.TrimSpace(ReadProcessLogTail(logPath, 4096)); logTail != "" {
		log.Printf("webrtc: %s pipeline exit session=%s reason=%s log=%s", streamType, sessionID, reason, SummarizeProcessLogTail(logTail))
	} else {
		log.Printf("webrtc: %s pipeline exit session=%s reason=%s", streamType, sessionID, reason)
	}
	RemoveProcessLog(logPath)
	if streamType == "audio" {
		wm.MarkAudioPipelineStopped(sessionID)
		log.Printf("webrtc: continuing session=%s without audio after pipeline exit", sessionID)
		return
	}
	wm.CleanupWithReason(sessionID, reason)
	SendWebRTCStopped(transport, sessionID, reason)
}

func StartWebRTCPipelineWithLog(cmd *exec.Cmd, pattern string) (string, error) {
	logFile, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", fmt.Errorf("failed to create pipeline log file: %w", err)
	}
	logPath := logFile.Name()
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		RemoveProcessLog(logPath)
		return "", err
	}
	if closeErr := logFile.Close(); closeErr != nil {
		log.Printf("webrtc: warning: failed to close pipeline log handle: %v", closeErr)
	}
	return logPath, nil
}

func ReadRTPToTrack(ctx context.Context, port int, track *webrtc.TrackLocalStaticRTP) {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		log.Printf("webrtc: resolve RTP port %d failed: %v", port, err)
		return
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Printf("webrtc: listen RTP port %d failed: %v", port, err)
		return
	}
	defer conn.Close()

	buf := make([]byte, 2048)
	pkt := &rtp.Packet{}
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		n, _, readErr := conn.ReadFromUDP(buf)
		if readErr != nil {
			if netErr, ok := readErr.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return
		}
		if err := pkt.Unmarshal(buf[:n]); err != nil {
			continue
		}
		if writeErr := track.WriteRTP(pkt); writeErr != nil {
			return
		}
	}
}

func InjectInputEvents(ctx context.Context, ch <-chan WebRTCInputEvent, display, xauthPath string, sess *WebRTCSession) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-ch:
			InjectSingleInput(evt, display, xauthPath, sess)
		}
	}
}

func BuildWaylandPipeWireEnv(session DesktopSessionInfo) []string {
	env := os.Environ()
	filtered := make([]string, 0, len(env)+2)
	for _, e := range env {
		if strings.HasPrefix(e, "XDG_RUNTIME_DIR=") {
			continue
		}
		filtered = append(filtered, e)
	}
	if runtimeDir := strings.TrimSpace(session.XDGRuntimeDir); runtimeDir != "" {
		filtered = append(filtered, "XDG_RUNTIME_DIR="+runtimeDir)
	}
	return filtered
}

func InjectSingleInput(evt WebRTCInputEvent, display, xauthPath string, sess *WebRTCSession) {
	eventType := strings.TrimSpace(strings.ToLower(evt.Type))
	if eventType == "" {
		return
	}
	if sess != nil && sess.sessionInfo.Type == DesktopSessionTypeWayland {
		InjectWaylandInputEvent(evt, sess)
		return
	}
	if strings.TrimSpace(display) == "" {
		display = ":0"
	}

	run := func(args ...string) {
		cmd, err := NewWebRTCSecurityCommand("xdotool", args...)
		if err != nil {
			return
		}
		cmd.Env = BuildX11ClientEnv(display, xauthPath)
		_ = cmd.Run()
	}

	switch eventType {
	case "keydown":
		if keyArg, ok := X11KeyArgument(evt); ok {
			run("keydown", keyArg)
			return
		}
		run("keydown", fmt.Sprintf("0x%x", evt.KeyCode))
	case "keyup":
		if keyArg, ok := X11KeyArgument(evt); ok {
			run("keyup", keyArg)
			return
		}
		run("keyup", fmt.Sprintf("0x%x", evt.KeyCode))
	case "mousemove":
		run("mousemove", "--screen", "0", strconv.Itoa(evt.X), strconv.Itoa(evt.Y))
	case "mousedown":
		run("mousedown", strconv.Itoa(evt.Button+1))
	case "mouseup":
		run("mouseup", strconv.Itoa(evt.Button+1))
	case "scroll":
		if evt.DeltaY < 0 {
			run("click", "4")
		} else if evt.DeltaY > 0 {
			run("click", "5")
		}
	}
}

func X11KeyArgument(evt WebRTCInputEvent) (string, bool) {
	if keysym, ok := DomCodeToX11Keysym(strings.TrimSpace(evt.Code)); ok {
		return keysym, true
	}
	if keysym, ok := DomKeyToX11Keysym(strings.TrimSpace(evt.Key)); ok {
		return keysym, true
	}
	if evt.KeyCode > 0 {
		return fmt.Sprintf("0x%x", evt.KeyCode), true
	}
	return "", false
}

func InjectWaylandInputEvent(evt WebRTCInputEvent, sess *WebRTCSession) {
	if sess == nil {
		return
	}
	backend := ResolveWaylandInputBackend(sess.inputBackend)
	if backend != "ydotool" {
		return
	}

	run := func(args ...string) {
		cmd, err := NewWebRTCSecurityCommand("ydotool", args...)
		if err != nil {
			return
		}
		_ = cmd.Run()
	}

	switch strings.TrimSpace(strings.ToLower(evt.Type)) {
	case "keydown", "keyup":
		code, ok := DomCodeToLinuxInputCode(strings.TrimSpace(evt.Code))
		if !ok {
			return
		}
		state := "0"
		if strings.EqualFold(strings.TrimSpace(evt.Type), "keydown") {
			state = "1"
		}
		run("key", fmt.Sprintf("%d:%s", code, state))
	case "mousemove":
		run("mousemove", "--absolute", "-x", strconv.Itoa(evt.X), "-y", strconv.Itoa(evt.Y))
	case "mousedown":
		if buttonCode, ok := BrowserButtonToYdotoolButton(evt.Button); ok {
			run("click", buttonCode)
		}
	case "scroll":
		if evt.DeltaY < 0 {
			run("click", "0xC3")
		} else if evt.DeltaY > 0 {
			run("click", "0xC4")
		}
	}
}

func ResolveWaylandInputBackend(configured string) string {
	switch strings.TrimSpace(strings.ToLower(configured)) {
	case "none":
		return "none"
	case "ydotool":
		return "ydotool"
	case "auto", "":
		if _, err := WebRTCLookPath("ydotool"); err == nil {
			return "ydotool"
		}
	}
	return "none"
}

func BrowserButtonToYdotoolButton(button int) (string, bool) {
	switch button {
	case 0:
		return "0xC0", true
	case 1:
		return "0xC2", true
	case 2:
		return "0xC1", true
	default:
		return "", false
	}
}

var domCodeLinuxInputCodes = map[string]int{
	"Escape": 1,
	"Digit1": 2, "Digit2": 3, "Digit3": 4, "Digit4": 5, "Digit5": 6,
	"Digit6": 7, "Digit7": 8, "Digit8": 9, "Digit9": 10, "Digit0": 11,
	"Minus": 12, "Equal": 13, "Backspace": 14, "Tab": 15,
	"KeyQ": 16, "KeyW": 17, "KeyE": 18, "KeyR": 19, "KeyT": 20,
	"KeyY": 21, "KeyU": 22, "KeyI": 23, "KeyO": 24, "KeyP": 25,
	"BracketLeft": 26, "BracketRight": 27, "Enter": 28,
	"ControlLeft": 29,
	"KeyA":        30, "KeyS": 31, "KeyD": 32, "KeyF": 33, "KeyG": 34,
	"KeyH": 35, "KeyJ": 36, "KeyK": 37, "KeyL": 38,
	"Semicolon": 39, "Quote": 40, "Backquote": 41,
	"ShiftLeft": 42, "Backslash": 43,
	"KeyZ": 44, "KeyX": 45, "KeyC": 46, "KeyV": 47, "KeyB": 48,
	"KeyN": 49, "KeyM": 50, "Comma": 51, "Period": 52, "Slash": 53,
	"ShiftRight": 54, "NumpadMultiply": 55, "AltLeft": 56, "Space": 57,
	"CapsLock": 58,
	"F1":       59, "F2": 60, "F3": 61, "F4": 62, "F5": 63, "F6": 64,
	"F7": 65, "F8": 66, "F9": 67, "F10": 68,
	"NumLock": 69, "ScrollLock": 70,
	"Numpad7": 71, "Numpad8": 72, "Numpad9": 73, "NumpadSubtract": 74,
	"Numpad4": 75, "Numpad5": 76, "Numpad6": 77, "NumpadAdd": 78,
	"Numpad1": 79, "Numpad2": 80, "Numpad3": 81, "Numpad0": 82, "NumpadDecimal": 83,
	"F11": 87, "F12": 88,
	"NumpadEnter": 96, "ControlRight": 97, "NumpadDivide": 98,
	"PrintScreen": 99, "AltRight": 100,
	"Home": 102, "ArrowUp": 103, "PageUp": 104,
	"ArrowLeft": 105, "ArrowRight": 106,
	"End": 107, "ArrowDown": 108, "PageDown": 109,
	"Insert": 110, "Delete": 111,
	"MetaLeft": 125, "MetaRight": 126, "ContextMenu": 127,
}

var domCodeX11Keysyms = map[string]string{
	"Backquote":      "grave",
	"Backslash":      "backslash",
	"Backspace":      "BackSpace",
	"BracketLeft":    "bracketleft",
	"BracketRight":   "bracketright",
	"CapsLock":       "Caps_Lock",
	"Comma":          "comma",
	"ContextMenu":    "Menu",
	"ControlLeft":    "Control_L",
	"ControlRight":   "Control_R",
	"Delete":         "Delete",
	"Digit0":         "0",
	"Digit1":         "1",
	"Digit2":         "2",
	"Digit3":         "3",
	"Digit4":         "4",
	"Digit5":         "5",
	"Digit6":         "6",
	"Digit7":         "7",
	"Digit8":         "8",
	"Digit9":         "9",
	"End":            "End",
	"Enter":          "Return",
	"Equal":          "equal",
	"Escape":         "Escape",
	"F1":             "F1",
	"F10":            "F10",
	"F11":            "F11",
	"F12":            "F12",
	"F2":             "F2",
	"F3":             "F3",
	"F4":             "F4",
	"F5":             "F5",
	"F6":             "F6",
	"F7":             "F7",
	"F8":             "F8",
	"F9":             "F9",
	"Home":           "Home",
	"Insert":         "Insert",
	"KeyA":           "a",
	"KeyB":           "b",
	"KeyC":           "c",
	"KeyD":           "d",
	"KeyE":           "e",
	"KeyF":           "f",
	"KeyG":           "g",
	"KeyH":           "h",
	"KeyI":           "i",
	"KeyJ":           "j",
	"KeyK":           "k",
	"KeyL":           "l",
	"KeyM":           "m",
	"KeyN":           "n",
	"KeyO":           "o",
	"KeyP":           "p",
	"KeyQ":           "q",
	"KeyR":           "r",
	"KeyS":           "s",
	"KeyT":           "t",
	"KeyU":           "u",
	"KeyV":           "v",
	"KeyW":           "w",
	"KeyX":           "x",
	"KeyY":           "y",
	"KeyZ":           "z",
	"MetaLeft":       "Super_L",
	"MetaRight":      "Super_R",
	"Minus":          "minus",
	"NumLock":        "Num_Lock",
	"Numpad0":        "KP_0",
	"Numpad1":        "KP_1",
	"Numpad2":        "KP_2",
	"Numpad3":        "KP_3",
	"Numpad4":        "KP_4",
	"Numpad5":        "KP_5",
	"Numpad6":        "KP_6",
	"Numpad7":        "KP_7",
	"Numpad8":        "KP_8",
	"Numpad9":        "KP_9",
	"NumpadAdd":      "KP_Add",
	"NumpadDecimal":  "KP_Decimal",
	"NumpadDivide":   "KP_Divide",
	"NumpadEnter":    "KP_Enter",
	"NumpadMultiply": "KP_Multiply",
	"NumpadSubtract": "KP_Subtract",
	"PageDown":       "Page_Down",
	"PageUp":         "Page_Up",
	"Period":         "period",
	"PrintScreen":    "Print",
	"Quote":          "apostrophe",
	"ScrollLock":     "Scroll_Lock",
	"Semicolon":      "semicolon",
	"ShiftLeft":      "Shift_L",
	"ShiftRight":     "Shift_R",
	"Slash":          "slash",
	"Space":          "space",
	"Tab":            "Tab",
	"AltLeft":        "Alt_L",
	"AltRight":       "Alt_R",
	"ArrowDown":      "Down",
	"ArrowLeft":      "Left",
	"ArrowRight":     "Right",
	"ArrowUp":        "Up",
}

var domKeyX11Keysyms = map[string]string{
	"Alt":        "Alt_L",
	"Backspace":  "BackSpace",
	"CapsLock":   "Caps_Lock",
	"Control":    "Control_L",
	"Delete":     "Delete",
	"Down":       "Down",
	"End":        "End",
	"Enter":      "Return",
	"Escape":     "Escape",
	"Home":       "Home",
	"Insert":     "Insert",
	"Left":       "Left",
	"Meta":       "Super_L",
	"NumLock":    "Num_Lock",
	"PageDown":   "Page_Down",
	"PageUp":     "Page_Up",
	"Right":      "Right",
	"ScrollLock": "Scroll_Lock",
	"Shift":      "Shift_L",
	"Tab":        "Tab",
	"Up":         "Up",
}

func DomCodeToLinuxInputCode(code string) (int, bool) {
	keyCode, ok := domCodeLinuxInputCodes[strings.TrimSpace(code)]
	return keyCode, ok
}

func DomCodeToX11Keysym(code string) (string, bool) {
	keysym, ok := domCodeX11Keysyms[strings.TrimSpace(code)]
	return keysym, ok
}

func DomKeyToX11Keysym(key string) (string, bool) {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return "", false
	}
	if keysym, ok := domKeyX11Keysyms[trimmed]; ok {
		return keysym, true
	}
	if len(trimmed) == 1 {
		return strings.ToLower(trimmed), true
	}
	return "", false
}

func FindFreeUDPPort() (int, error) {
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return 0, err
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	_ = conn.Close()
	return port, nil
}

func ParsePipelineArgs(pipeline string) []string {
	parts := strings.Fields(strings.TrimSpace(pipeline))
	if len(parts) == 0 {
		return nil
	}
	return parts
}

func DecodeWebRTCInputEvent(raw []byte) (WebRTCInputEvent, error) {
	var evt WebRTCInputEvent
	var fallback agentmgr.WebRTCInputData

	directErr := json.Unmarshal(raw, &evt)
	fallbackErr := json.Unmarshal(raw, &fallback)
	if directErr != nil && fallbackErr != nil {
		return WebRTCInputEvent{}, directErr
	}

	if strings.TrimSpace(evt.Type) == "" && strings.TrimSpace(fallback.Type) == "" {
		return WebRTCInputEvent{}, fmt.Errorf("missing type")
	}

	if strings.TrimSpace(evt.Type) == "" {
		evt.Type = fallback.Type
	}
	if evt.KeyCode == 0 && fallback.KeyCode != 0 {
		evt.KeyCode = fallback.KeyCode
	}
	if strings.TrimSpace(evt.Code) == "" && strings.TrimSpace(fallback.Code) != "" {
		evt.Code = fallback.Code
	}
	if strings.TrimSpace(evt.Key) == "" && strings.TrimSpace(fallback.Key) != "" {
		evt.Key = fallback.Key
	}
	if evt.X == 0 && fallback.X != 0 {
		evt.X = fallback.X
	}
	if evt.Y == 0 && fallback.Y != 0 {
		evt.Y = fallback.Y
	}
	if evt.Button == 0 && fallback.Button != 0 {
		evt.Button = fallback.Button
	}
	if evt.DeltaY == 0 && fallback.DeltaY != 0 {
		evt.DeltaY = fallback.DeltaY
	}
	return evt, nil
}

func (wm *WebRTCManager) handleClipboardDataChannelMessage(dc *webrtc.DataChannel, msg webrtc.DataChannelMessage) {
	if dc == nil || !msg.IsString {
		return
	}
	var payload WebRTCClipboardMessage
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		return
	}
	reply := WebRTCClipboardMessage{Type: payload.Type, Format: "text"}
	switch strings.TrimSpace(strings.ToLower(payload.Type)) {
	case "get":
		text, _, err := sysconfig.PlatformClipboardRead("text")
		if err != nil {
			reply.Type = "error"
			reply.Error = err.Error()
		} else {
			reply.Type = "data"
			reply.Text = text
		}
	case "set":
		if err := sysconfig.PlatformClipboardWriteText(payload.Text); err != nil {
			reply.Type = "error"
			reply.Error = err.Error()
		} else {
			reply.Type = "ack"
		}
	default:
		return
	}
	raw, err := json.Marshal(reply)
	if err != nil {
		return
	}
	if err := dc.SendText(string(raw)); err != nil {
		log.Printf("webrtc: clipboard data channel reply failed: %v", err)
	}
}

func (wm *WebRTCManager) handleFileTransferDataChannelMessage(dc *webrtc.DataChannel, msg webrtc.DataChannelMessage) {
	if dc == nil || !msg.IsString || wm.fileMgr == nil {
		return
	}
	var payload WebRTCFileTransferMessage
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		return
	}
	reply := WebRTCFileTransferMessage{
		Type:      "ack",
		RequestID: strings.TrimSpace(payload.RequestID),
	}
	if reply.RequestID == "" {
		return
	}
	switch strings.TrimSpace(strings.ToLower(payload.Type)) {
	case "start":
		reply.Type = "ready"
	case "chunk":
		bytesWritten, err := wm.fileMgr.WriteChunk(agentmgr.FileWriteData{
			RequestID: payload.RequestID,
			Path:      payload.Path,
			Data:      payload.Data,
			Done:      payload.Done,
		})
		reply.BytesWritten = bytesWritten
		reply.Done = payload.Done && err == nil
		if err != nil {
			reply.Type = "error"
			reply.Error = err.Error()
		}
	default:
		return
	}
	raw, err := json.Marshal(reply)
	if err != nil {
		return
	}
	if err := dc.SendText(string(raw)); err != nil {
		log.Printf("webrtc: file transfer data channel reply failed: %v", err)
	}
}

func SendWebRTCStopped(transport MessageSender, sessionID, reason string) {
	data, _ := json.Marshal(agentmgr.WebRTCStoppedData{SessionID: sessionID, Reason: reason})
	_ = transport.Send(agentmgr.Message{Type: agentmgr.MsgWebRTCStopped, ID: sessionID, Data: data})
}

func ICECandidateSendDelay(candidate string) time.Duration {
	switch ParseICECandidateType(candidate) {
	case "relay":
		return 300 * time.Millisecond
	case "srflx", "prflx":
		return 150 * time.Millisecond
	default:
		return 0
	}
}

func ParseICECandidateType(candidate string) string {
	parts := strings.Fields(strings.TrimSpace(candidate))
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] == "typ" {
			return strings.ToLower(strings.TrimSpace(parts[i+1]))
		}
	}
	return ""
}

func ValueOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
