package agentmgr

import "encoding/json"

// Wire protocol message types.
const (
	MsgHeartbeat            = "heartbeat"
	MsgTelemetry            = "telemetry"
	MsgCommandRequest       = "command.request"
	MsgCommandResult        = "command.result"
	MsgPing                 = "ping"
	MsgPong                 = "pong"
	MsgLogStream            = "log.stream"
	MsgLogBatch             = "log.batch"
	MsgJournalQuery         = "journal.query"   // Hub → Agent: historical journalctl query
	MsgJournalEntries       = "journal.entries" // Agent → Hub: historical journalctl query response
	MsgConfigUpdate         = "config.update"
	MsgConfigApplied        = "config.applied"
	MsgAgentSettingsApply   = "agent.settings.apply"
	MsgAgentSettingsApplied = "agent.settings.applied"
	MsgAgentSettingsState   = "agent.settings.state"
	MsgUpdateRequest        = "update.request"
	MsgUpdateProgress       = "update.progress"
	MsgUpdateResult         = "update.result"

	// Terminal session messages (interactive PTY over agent WebSocket).
	MsgTerminalProbe    = "terminal.probe"     // Hub → Agent: probe tmux availability
	MsgTerminalProbed   = "terminal.probed"    // Agent → Hub: tmux probe result
	MsgTerminalStart    = "terminal.start"     // Hub → Agent: start a PTY shell
	MsgTerminalStarted  = "terminal.started"   // Agent → Hub: shell ready
	MsgTerminalData     = "terminal.data"      // Bidirectional: stdin/stdout chunks (base64)
	MsgTerminalResize   = "terminal.resize"    // Hub → Agent: change PTY window size
	MsgTerminalTmuxKill = "terminal.tmux.kill" // Hub → Agent: end a saved tmux session by stable name
	MsgTerminalClose    = "terminal.close"     // Hub → Agent: end terminal session
	MsgTerminalClosed   = "terminal.closed"    // Agent → Hub: session ended

	// SSH key auto-provisioning messages.
	MsgSSHKeyInstall   = "ssh_key.install"   // Hub → Agent: install public key
	MsgSSHKeyInstalled = "ssh_key.installed" // Agent → Hub: key installed confirmation
	MsgSSHKeyRemove    = "ssh_key.remove"    // Hub → Agent: remove public key
	MsgSSHKeyRemoved   = "ssh_key.removed"   // Agent → Hub: key removed confirmation

	// Desktop (VNC) session messages.
	MsgDesktopStart        = "desktop.start"         // Hub → Agent: start VNC + bridge
	MsgDesktopStarted      = "desktop.started"       // Agent → Hub: VNC ready
	MsgDesktopData         = "desktop.data"          // Bidirectional: base64 VNC bytes
	MsgDesktopClose        = "desktop.close"         // Hub → Agent: end desktop session
	MsgDesktopClosed       = "desktop.closed"        // Agent → Hub: session ended
	MsgDesktopListDisplays = "desktop.list-displays" // Hub → Agent: enumerate displays
	MsgDesktopDisplays     = "desktop.displays"      // Agent → Hub: display enumeration response
	MsgDesktopDiagnose     = "desktop.diagnose"      // Hub → Agent: request desktop stack diagnostic
	MsgDesktopDiagnosed    = "desktop.diagnosed"     // Agent → Hub: desktop stack diagnostic response

	// WebRTC streaming messages.
	MsgWebRTCCapabilities = "webrtc.capabilities" // Agent → Hub: available encoders, audio sources
	MsgWebRTCOffer        = "webrtc.offer"        // Hub → Agent: SDP offer from browser
	MsgWebRTCAnswer       = "webrtc.answer"       // Agent → Hub: SDP answer
	MsgWebRTCICE          = "webrtc.ice"          // Bidirectional: ICE candidate
	MsgWebRTCStart        = "webrtc.start"        // Hub → Agent: start WebRTC session
	MsgWebRTCStarted      = "webrtc.started"      // Agent → Hub: WebRTC session ready
	MsgWebRTCStop         = "webrtc.stop"         // Hub → Agent: stop WebRTC session
	MsgWebRTCStopped      = "webrtc.stopped"      // Agent → Hub: WebRTC session ended
	MsgWebRTCInput        = "webrtc.input"        // Hub → Agent: keyboard/mouse input (fallback)

	// Clipboard sync messages.
	MsgClipboardGet    = "clipboard.get"     // Hub → Agent: read remote clipboard
	MsgClipboardData   = "clipboard.data"    // Agent → Hub: clipboard contents
	MsgClipboardSet    = "clipboard.set"     // Hub → Agent: write to remote clipboard
	MsgClipboardSetAck = "clipboard.set_ack" // Agent → Hub: write confirmation

	// Desktop audio sideband messages (VNC sessions).
	MsgDesktopAudioStart = "desktop.audio.start" // Hub → Agent: start audio capture
	MsgDesktopAudioStop  = "desktop.audio.stop"  // Hub → Agent: stop audio capture
	MsgDesktopAudioData  = "desktop.audio.data"  // Agent → Hub: Opus audio frame
	MsgDesktopAudioState = "desktop.audio.state" // Agent → Hub: state change

	// Wake-on-LAN messages.
	MsgWoLSend   = "wol.send"   // Hub → Agent: send a WoL packet on local network
	MsgWoLResult = "wol.result" // Agent → Hub: WoL send result

	// File transfer messages.
	MsgFileList         = "file.list"          // Hub → Agent: list directory
	MsgFileListed       = "file.listed"        // Agent → Hub: directory listing result
	MsgFileRead         = "file.read"          // Hub → Agent: read file
	MsgFileData         = "file.data"          // Agent → Hub: file data chunk (base64)
	MsgFileWrite        = "file.write"         // Hub → Agent: write file chunk (base64)
	MsgFileWritten      = "file.written"       // Agent → Hub: write confirmation
	MsgFileMkdir        = "file.mkdir"         // Hub → Agent: create directory
	MsgFileDelete       = "file.delete"        // Hub → Agent: delete file/directory
	MsgFileRename       = "file.rename"        // Hub → Agent: rename/move file or directory
	MsgFileCopy         = "file.copy"          // Hub → Agent: copy file/directory
	MsgFileResult       = "file.result"        // Agent → Hub: operation result (mkdir, delete, rename, copy)
	MsgFileSearch       = "file.search"        // Hub → Agent: recursive filename search
	MsgFileSearchResult = "file.search_result" // Agent → Hub: filename search results

	// Process list messages.
	MsgProcessList       = "process.list"        // Hub → Agent: request process list
	MsgProcessListed     = "process.listed"      // Agent → Hub: process list response
	MsgProcessKill       = "process.kill"        // Hub → Agent: send signal to a process
	MsgProcessKillResult = "process.kill_result" // Agent → Hub: signal delivery result

	// Service management messages.
	MsgServiceList   = "service.list"   // Hub → Agent: request service list
	MsgServiceListed = "service.listed" // Agent → Hub: service list response
	MsgServiceAction = "service.action" // Hub → Agent: perform service action
	MsgServiceResult = "service.result" // Agent → Hub: service action result

	// Disk/mount info messages.
	MsgDiskList   = "disk.list"   // Hub → Agent: request mount/disk info
	MsgDiskListed = "disk.listed" // Agent → Hub: mount/disk info response

	// Network interface messages.
	MsgNetworkList   = "network.list"   // Hub → Agent: request network interface info
	MsgNetworkListed = "network.listed" // Agent → Hub: network interface response
	MsgNetworkAction = "network.action" // Hub → Agent: perform network action (apply/rollback)
	MsgNetworkResult = "network.result" // Agent → Hub: network action result

	// Package inventory messages.
	MsgPackageList   = "package.list"   // Hub → Agent: request installed packages
	MsgPackageListed = "package.listed" // Agent → Hub: installed packages response
	MsgPackageAction = "package.action" // Hub → Agent: run package action (install/remove/upgrade)
	MsgPackageResult = "package.result" // Agent → Hub: package action result

	// Cron/timer visibility messages.
	MsgCronList   = "cron.list"   // Hub → Agent: request cron/timer entries
	MsgCronListed = "cron.listed" // Agent → Hub: cron/timer entries response

	// User session messages.
	MsgUsersList   = "users.list"   // Hub → Agent: request active user sessions
	MsgUsersListed = "users.listed" // Agent → Hub: active user sessions response

	// Alert notification messages.
	MsgAlertNotify = "alert.notify" // Hub → Agent: push alert for local display

	// Enrollment approval messages.
	MsgEnrollmentChallenge = "enrollment.challenge" // Hub → Agent: prove device-key possession
	MsgEnrollmentProof     = "enrollment.proof"     // Agent → Hub: signed challenge proof
	MsgEnrollmentApproved  = "enrollment.approved"  // Hub → Agent: enrollment approved, token attached
	MsgEnrollmentRejected  = "enrollment.rejected"  // Hub → Agent: enrollment rejected

	// Docker container management messages.
	MsgDockerDiscovery      = "docker.discovery"       // Agent → Hub: full inventory
	MsgDockerDiscoveryDelta = "docker.discovery.delta" // Agent → Hub: incremental inventory update
	MsgDockerStats          = "docker.stats"           // Agent → Hub: container stats
	MsgDockerEvents         = "docker.events"          // Agent → Hub: daemon events
	MsgDockerAction         = "docker.action"          // Hub → Agent: lifecycle command
	MsgDockerActionResult   = "docker.action.result"   // Agent → Hub: action result
	MsgDockerLogsStart      = "docker.logs.start"      // Hub → Agent: start log stream
	MsgDockerLogsStop       = "docker.logs.stop"       // Hub → Agent: stop log stream
	MsgDockerLogsStream     = "docker.logs.stream"     // Agent → Hub: log lines
	MsgDockerExecStart      = "docker.exec.start"      // Hub → Agent: start exec session
	MsgDockerExecStarted    = "docker.exec.started"    // Agent → Hub: exec ready
	MsgDockerExecData       = "docker.exec.data"       // Agent → Hub: exec output
	MsgDockerExecInput      = "docker.exec.input"      // Hub → Agent: exec stdin
	MsgDockerExecResize     = "docker.exec.resize"     // Hub → Agent: resize exec PTY
	MsgDockerExecClose      = "docker.exec.close"      // Hub → Agent: close exec
	MsgDockerExecClosed     = "docker.exec.closed"     // Agent → Hub: exec ended
	MsgDockerComposeAction  = "docker.compose.action"  // Hub → Agent: stack operation
	MsgDockerComposeResult  = "docker.compose.result"  // Agent → Hub: stack result

	// Web service discovery messages.
	MsgWebServiceReport = "webservice.report" // Agent → Hub: discovered services
	MsgWebServiceSync   = "webservice.sync"   // Hub → Agent: request immediate rediscovery
)

// KnownMessageTypes is the set of all valid agent protocol message types.
var KnownMessageTypes = map[string]bool{
	MsgHeartbeat: true, MsgTelemetry: true,
	MsgCommandRequest: true, MsgCommandResult: true,
	MsgPing: true, MsgPong: true,
	MsgLogStream: true, MsgLogBatch: true,
	MsgJournalQuery: true, MsgJournalEntries: true,
	MsgConfigUpdate: true, MsgConfigApplied: true,
	MsgAgentSettingsApply: true, MsgAgentSettingsApplied: true, MsgAgentSettingsState: true,
	MsgUpdateRequest: true, MsgUpdateProgress: true, MsgUpdateResult: true,
	MsgTerminalProbe: true, MsgTerminalProbed: true,
	MsgTerminalStart: true, MsgTerminalStarted: true, MsgTerminalData: true,
	MsgTerminalResize: true, MsgTerminalTmuxKill: true, MsgTerminalClose: true, MsgTerminalClosed: true,
	MsgSSHKeyInstall: true, MsgSSHKeyInstalled: true,
	MsgSSHKeyRemove: true, MsgSSHKeyRemoved: true,
	MsgDesktopStart: true, MsgDesktopStarted: true, MsgDesktopData: true,
	MsgDesktopClose: true, MsgDesktopClosed: true,
	MsgDesktopListDisplays: true, MsgDesktopDisplays: true,
	MsgDesktopDiagnose: true, MsgDesktopDiagnosed: true,
	MsgWebRTCCapabilities: true, MsgWebRTCOffer: true, MsgWebRTCAnswer: true,
	MsgWebRTCICE: true, MsgWebRTCStart: true, MsgWebRTCStarted: true,
	MsgWebRTCStop: true, MsgWebRTCStopped: true, MsgWebRTCInput: true,
	MsgClipboardGet: true, MsgClipboardData: true, MsgClipboardSet: true, MsgClipboardSetAck: true,
	MsgDesktopAudioStart: true, MsgDesktopAudioStop: true, MsgDesktopAudioData: true, MsgDesktopAudioState: true,
	MsgWoLSend: true, MsgWoLResult: true,
	MsgFileList: true, MsgFileListed: true,
	MsgFileRead: true, MsgFileData: true,
	MsgFileWrite: true, MsgFileWritten: true,
	MsgFileMkdir: true, MsgFileDelete: true, MsgFileRename: true, MsgFileCopy: true, MsgFileResult: true,
	MsgFileSearch: true, MsgFileSearchResult: true,
	MsgProcessList: true, MsgProcessListed: true,
	MsgProcessKill: true, MsgProcessKillResult: true,
	MsgServiceList: true, MsgServiceListed: true, MsgServiceAction: true, MsgServiceResult: true,
	MsgDiskList: true, MsgDiskListed: true,
	MsgNetworkList: true, MsgNetworkListed: true, MsgNetworkAction: true, MsgNetworkResult: true,
	MsgPackageList: true, MsgPackageListed: true, MsgPackageAction: true, MsgPackageResult: true,
	MsgCronList: true, MsgCronListed: true,
	MsgUsersList: true, MsgUsersListed: true,
	MsgAlertNotify:         true,
	MsgEnrollmentChallenge: true, MsgEnrollmentProof: true,
	MsgEnrollmentApproved: true, MsgEnrollmentRejected: true,
	MsgDockerDiscovery: true, MsgDockerDiscoveryDelta: true, MsgDockerStats: true, MsgDockerEvents: true,
	MsgDockerAction: true, MsgDockerActionResult: true,
	MsgDockerLogsStart: true, MsgDockerLogsStop: true, MsgDockerLogsStream: true,
	MsgDockerExecStart: true, MsgDockerExecStarted: true, MsgDockerExecData: true,
	MsgDockerExecInput: true, MsgDockerExecResize: true,
	MsgDockerExecClose: true, MsgDockerExecClosed: true,
	MsgDockerComposeAction: true, MsgDockerComposeResult: true,
	MsgWebServiceReport: true, MsgWebServiceSync: true,
}

// IsKnownMessageType returns true if the message type is a valid protocol message.
func IsKnownMessageType(msgType string) bool {
	return KnownMessageTypes[msgType]
}

// Message is the envelope for all agent ↔ hub WebSocket communication.
type Message struct {
	Type string          `json:"type"`
	ID   string          `json:"id,omitempty"`
	Data json.RawMessage `json:"data,omitempty"`
}

// HeartbeatData mirrors assets.HeartbeatRequest fields for WebSocket transport.
type HeartbeatData struct {
	AssetID      string            `json:"asset_id"`
	Type         string            `json:"type"`
	Name         string            `json:"name"`
	Source       string            `json:"source"`
	GroupID      string            `json:"group_id,omitempty"`
	Status       string            `json:"status,omitempty"`
	Platform     string            `json:"platform,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	Capabilities []string          `json:"capabilities,omitempty"` // e.g. ["webrtc", "desktop", "terminal", "files"]
	Connectors   []ConnectorInfo   `json:"connectors,omitempty"`
}

// ConnectorInfo describes a locally discovered connector endpoint.
type ConnectorInfo struct {
	Type      string `json:"type"`
	Endpoint  string `json:"endpoint"`
	Reachable bool   `json:"reachable"`
}

// TelemetryData carries a telemetry sample over WebSocket.
type TelemetryData struct {
	AssetID          string   `json:"asset_id"`
	CPUPercent       float64  `json:"cpu_percent"`
	MemoryPercent    float64  `json:"memory_percent"`
	DiskPercent      float64  `json:"disk_percent"`
	NetRXBytesPerSec float64  `json:"net_rx_bytes_per_sec"`
	NetTXBytesPerSec float64  `json:"net_tx_bytes_per_sec"`
	TempCelsius      *float64 `json:"temp_celsius,omitempty"`
}

// CommandRequestData is sent from hub to agent to execute a command.
type CommandRequestData struct {
	JobID     string `json:"job_id"`
	SessionID string `json:"session_id"`
	CommandID string `json:"command_id"`
	Command   string `json:"command"`
	Timeout   int    `json:"timeout"`
}

// CommandResultData is sent from agent to hub with the command result.
type CommandResultData struct {
	JobID     string `json:"job_id"`
	SessionID string `json:"session_id"`
	CommandID string `json:"command_id"`
	Status    string `json:"status"`
	Output    string `json:"output"`
}

// LogStreamData carries a single log entry from agent to hub.
type LogStreamData struct {
	AssetID   string `json:"asset_id"`
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Source    string `json:"source"`
}

// LogBatchData carries multiple buffered log entries from agent to hub.
type LogBatchData struct {
	AssetID string          `json:"asset_id"`
	Entries []LogStreamData `json:"entries"`
}

// JournalQueryData is sent from hub to agent to query historical journal entries.
type JournalQueryData struct {
	RequestID string `json:"request_id"`
	Since     string `json:"since,omitempty"`    // e.g. "1h ago", "2026-02-24 08:00:00"
	Until     string `json:"until,omitempty"`    // e.g. "now", "2026-02-24 09:00:00"
	Unit      string `json:"unit,omitempty"`     // systemd unit name (e.g. "ssh.service")
	Priority  string `json:"priority,omitempty"` // debug|info|notice|warning|err|crit|alert|emerg
	Search    string `json:"search,omitempty"`   // free-text grep filter
	Limit     int    `json:"limit"`              // max entries
}

// JournalEntriesData is sent from agent to hub with historical journal entries.
type JournalEntriesData struct {
	RequestID string          `json:"request_id"`
	Entries   []LogStreamData `json:"entries"`
	Error     string          `json:"error,omitempty"`
}

// ConfigUpdateData is sent from hub to agent to update runtime configuration.
type ConfigUpdateData struct {
	CollectIntervalSec   *int    `json:"collect_interval_sec,omitempty"`
	HeartbeatIntervalSec *int    `json:"heartbeat_interval_sec,omitempty"`
	LogLevel             *string `json:"log_level,omitempty"`
}

// ConfigAppliedData is sent from agent to hub confirming config application.
type ConfigAppliedData struct {
	CollectIntervalSec   int `json:"collect_interval_sec"`
	HeartbeatIntervalSec int `json:"heartbeat_interval_sec"`
}

// AgentSettingsApplyData is sent from hub to agent to apply settings.
type AgentSettingsApplyData struct {
	RequestID           string            `json:"request_id,omitempty"`
	Revision            string            `json:"revision,omitempty"`
	Values              map[string]string `json:"values"`
	ExpectedFingerprint string            `json:"expected_fingerprint,omitempty"`
}

// AgentSettingsAppliedData is sent from agent to hub after settings apply attempt.
type AgentSettingsAppliedData struct {
	RequestID       string            `json:"request_id,omitempty"`
	Revision        string            `json:"revision,omitempty"`
	Applied         bool              `json:"applied"`
	RestartRequired bool              `json:"restart_required,omitempty"`
	Error           string            `json:"error,omitempty"`
	AppliedValues   map[string]string `json:"applied_values,omitempty"`
	Fingerprint     string            `json:"fingerprint,omitempty"`
	AppliedAt       string            `json:"applied_at,omitempty"`
}

// AgentSettingsStateData is sent from agent to hub to report current effective settings.
type AgentSettingsStateData struct {
	Revision             string            `json:"revision,omitempty"`
	Values               map[string]string `json:"values,omitempty"`
	Fingerprint          string            `json:"fingerprint,omitempty"`
	AllowRemoteOverrides bool              `json:"allow_remote_overrides"`
	ReportedAt           string            `json:"reported_at,omitempty"`
}

// UpdateRequestData is sent from hub to agent to request a system update.
type UpdateRequestData struct {
	JobID    string   `json:"job_id"`
	Mode     string   `json:"mode"`
	Force    bool     `json:"force,omitempty"`
	Packages []string `json:"packages,omitempty"`
}

// UpdateProgressData is sent from agent to hub with update progress.
type UpdateProgressData struct {
	JobID   string `json:"job_id"`
	Stage   string `json:"stage"`
	Message string `json:"message"`
}

// UpdateResultData is sent from agent to hub with the final update result.
type UpdateResultData struct {
	JobID  string `json:"job_id"`
	Status string `json:"status"`
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

// TerminalProbeResponse is sent from agent to hub with tmux availability info.
type TerminalProbeResponse struct {
	HasTmux  bool   `json:"has_tmux"`
	TmuxPath string `json:"tmux_path,omitempty"`
}

// TerminalStartData is sent from hub to agent to start a PTY shell session.
type TerminalStartData struct {
	SessionID   string `json:"session_id"`
	Cols        int    `json:"cols"`
	Rows        int    `json:"rows"`
	Shell       string `json:"shell,omitempty"`        // optional: override default shell
	UseTmux     bool   `json:"use_tmux,omitempty"`     // if true, start inside tmux
	TmuxSession string `json:"tmux_session,omitempty"` // tmux session name to create/attach
}

// TerminalStartedData is sent from agent to hub when the PTY shell is ready.
type TerminalStartedData struct {
	SessionID    string `json:"session_id"`
	TmuxAttached bool   `json:"tmux_attached,omitempty"` // true if running inside tmux
}

// TerminalDataPayload carries terminal I/O data (base64-encoded bytes).
type TerminalDataPayload struct {
	SessionID string `json:"session_id"`
	Data      string `json:"data"` // base64-encoded terminal bytes
}

// TerminalResizeData is sent from hub to agent to resize the PTY window.
type TerminalResizeData struct {
	SessionID string `json:"session_id"`
	Cols      int    `json:"cols"`
	Rows      int    `json:"rows"`
}

// TerminalTmuxKillData is sent from hub to agent to end a saved tmux session.
type TerminalTmuxKillData struct {
	JobID       string `json:"job_id"`
	SessionID   string `json:"session_id,omitempty"`
	CommandID   string `json:"command_id,omitempty"`
	TmuxSession string `json:"tmux_session"`
	Timeout     int    `json:"timeout,omitempty"`
}

// TerminalCloseData is sent to close a terminal session.
type TerminalCloseData struct {
	SessionID string `json:"session_id"`
	Reason    string `json:"reason,omitempty"`
}

// SSHKeyInstallData is sent from hub to agent to install the hub's public key.
type SSHKeyInstallData struct {
	PublicKey  string `json:"public_key"`
	TargetUser string `json:"target_user,omitempty"`
}

// SSHKeyRemoveData is sent from hub to agent to remove the hub's public key.
// Same shape as SSHKeyInstallData — contains the public key to match and remove.
type SSHKeyRemoveData = SSHKeyInstallData

// SSHKeyInstalledData is sent from agent to hub confirming key installation.
type SSHKeyInstalledData struct {
	Username string `json:"username"`
	Hostname string `json:"hostname"`
	HomeDir  string `json:"home_dir"`
}

// DesktopStartData is sent from hub to agent to start a VNC session.
type DesktopStartData struct {
	SessionID   string `json:"session_id"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
	Quality     string `json:"quality,omitempty"` // low, medium, high
	Display     string `json:"display,omitempty"` // e.g. ":0"
	VNCPassword string `json:"vnc_password,omitempty"`
}

// DesktopStartedData is sent from agent to hub when VNC is ready.
type DesktopStartedData struct {
	SessionID string `json:"session_id"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
}

// DesktopDataPayload carries VNC protocol bytes (base64-encoded).
type DesktopDataPayload struct {
	SessionID string `json:"session_id"`
	Data      string `json:"data"` // base64-encoded VNC bytes
}

// DesktopCloseData is sent to close a desktop session.
type DesktopCloseData struct {
	SessionID string `json:"session_id"`
	Reason    string `json:"reason,omitempty"`
}

// WebRTCCapabilitiesData describes the agent's WebRTC streaming capabilities.
type WebRTCCapabilitiesData struct {
	Available                  bool     `json:"available"`
	UnavailableReason          string   `json:"unavailable_reason,omitempty"`   // e.g. "unsupported_platform:darwin", "gst_launch_not_found"
	VideoEncoders              []string `json:"video_encoders,omitempty"`       // e.g. ["nvenc_h264", "vaapi_h264", "vp8", "x264"]
	AudioSources               []string `json:"audio_sources,omitempty"`        // e.g. ["pulseaudio", "pipewire"]
	Displays                   []string `json:"displays,omitempty"`             // available X11 displays
	DesktopSessionType         string   `json:"desktop_session_type,omitempty"` // x11, wayland, headless
	DesktopBackend             string   `json:"desktop_backend,omitempty"`      // x11, wayland_pipewire, headless
	CaptureBackend             string   `json:"capture_backend,omitempty"`      // ximagesrc, pipewiresrc
	VNCRealDesktopSupported    bool     `json:"vnc_real_desktop_supported"`
	WebRTCRealDesktopSupported bool     `json:"webrtc_real_desktop_supported"`
}

// WebRTCSessionData is sent with webrtc.start.
type WebRTCSessionData struct {
	SessionID    string `json:"session_id"`
	Display      string `json:"display,omitempty"`
	Quality      string `json:"quality,omitempty"` // low, medium, high
	Width        int    `json:"width,omitempty"`
	Height       int    `json:"height,omitempty"`
	FPS          int    `json:"fps,omitempty"`
	AudioEnabled bool   `json:"audio_enabled"`
}

// WebRTCSDPData carries SDP offer/answer payloads.
type WebRTCSDPData struct {
	SessionID string `json:"session_id"`
	Type      string `json:"type"` // "offer" or "answer"
	SDP       string `json:"sdp"`
}

// WebRTCICEData carries an ICE candidate.
type WebRTCICEData struct {
	SessionID     string `json:"session_id"`
	Candidate     string `json:"candidate"`
	SDPMid        string `json:"sdp_mid,omitempty"`
	SDPMLineIndex *int   `json:"sdp_mline_index,omitempty"`
}

// WebRTCStartedData confirms WebRTC session is ready for signaling.
type WebRTCStartedData struct {
	SessionID    string `json:"session_id"`
	VideoEncoder string `json:"video_encoder"`
	AudioSource  string `json:"audio_source,omitempty"`
}

// WebRTCStoppedData confirms WebRTC session teardown.
type WebRTCStoppedData struct {
	SessionID string `json:"session_id"`
	Reason    string `json:"reason,omitempty"`
}

// WebRTCInputData carries keyboard/mouse events over signaling (fallback for DataChannel).
type WebRTCInputData struct {
	SessionID string `json:"session_id"`
	Type      string `json:"type"` // keydown, keyup, mousemove, mousedown, mouseup
	KeyCode   int    `json:"key_code,omitempty"`
	Code      string `json:"code,omitempty"`
	Key       string `json:"key,omitempty"`
	X         int    `json:"x,omitempty"`
	Y         int    `json:"y,omitempty"`
	Button    int    `json:"button,omitempty"`
	DeltaY    int    `json:"delta_y,omitempty"`
}

// ClipboardGetData requests the agent's clipboard contents.
type ClipboardGetData struct {
	RequestID string `json:"request_id"`
	Format    string `json:"format"` // "text" or "image"
}

// ClipboardDataPayload carries clipboard contents from agent to hub.
type ClipboardDataPayload struct {
	RequestID string `json:"request_id"`
	Format    string `json:"format"`         // "text" or "image/png"
	Text      string `json:"text,omitempty"` // plaintext content
	Data      string `json:"data,omitempty"` // base64-encoded binary (images)
	Error     string `json:"error,omitempty"`
}

// ClipboardSetData writes content to the agent's clipboard.
type ClipboardSetData struct {
	RequestID string `json:"request_id"`
	Format    string `json:"format"` // "text" or "image/png"
	Text      string `json:"text,omitempty"`
	Data      string `json:"data,omitempty"` // base64-encoded binary
}

// ClipboardSetAckData confirms clipboard write.
type ClipboardSetAckData struct {
	RequestID string `json:"request_id"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
}

// DesktopAudioStartData requests audio capture start for a VNC session.
type DesktopAudioStartData struct {
	SessionID string `json:"session_id"`
	Bitrate   int    `json:"bitrate,omitempty"` // bps, default 128000
}

// DesktopAudioStopData requests audio capture stop for a VNC session.
type DesktopAudioStopData struct {
	SessionID string `json:"session_id"`
}

// DesktopAudioDataPayload carries an Opus audio frame from agent to hub.
type DesktopAudioDataPayload struct {
	SessionID string `json:"session_id"`
	Data      string `json:"data"`      // base64-encoded Opus frame
	Timestamp int64  `json:"timestamp"` // unix milliseconds
}

// DesktopAudioStateData reports audio capture state changes.
type DesktopAudioStateData struct {
	SessionID string `json:"session_id"`
	State     string `json:"state"` // "started", "stopped", "unavailable"
	Error     string `json:"error,omitempty"`
}

// DisplayInfo describes one monitor/display geometry on a node.
type DisplayInfo struct {
	Name    string `json:"name"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Primary bool   `json:"primary"`
	OffsetX int    `json:"offset_x"`
	OffsetY int    `json:"offset_y"`
}

// DisplayListData carries the display enumeration response.
type DisplayListData struct {
	RequestID string        `json:"request_id"`
	Displays  []DisplayInfo `json:"displays"`
	Error     string        `json:"error,omitempty"`
}

// DesktopDiagnosticRequest is sent Hub → Agent to request a desktop stack diagnostic.
type DesktopDiagnosticRequest struct {
	RequestID string `json:"request_id"`
}

// DesktopDiagnosticData carries the full desktop stack diagnostic response from the agent.
type DesktopDiagnosticData struct {
	RequestID string `json:"request_id"`

	DesktopSessionType         string `json:"desktop_session_type,omitempty"`
	DesktopBackend             string `json:"desktop_backend,omitempty"`
	DesktopUser                string `json:"desktop_user,omitempty"`
	RealDisplay                string `json:"real_display,omitempty"`
	VNCRealDesktopSupported    bool   `json:"vnc_real_desktop_supported"`
	WebRTCRealDesktopSupported bool   `json:"webrtc_real_desktop_supported"`
	CaptureBackend             string `json:"capture_backend,omitempty"`

	// Xvfb state
	XvfbRunning  bool     `json:"xvfb_running"`
	XvfbDisplays []string `json:"xvfb_displays"`
	XvfbPIDs     []int    `json:"xvfb_pids"`

	// X11 display state
	ActiveDisplays []string `json:"active_displays"`
	EnvDisplay     string   `json:"env_display"`

	// VNC state
	X11VNCRunning bool   `json:"x11vnc_running"`
	X11VNCDisplay string `json:"x11vnc_display"`
	X11VNCPort    int    `json:"x11vnc_port"`

	// Bootstrap shell state
	BootstrapRunning bool `json:"bootstrap_running"`
	XtermAvailable   bool `json:"xterm_available"`

	// WebRTC state
	GstLaunchAvailable  bool     `json:"gst_launch_available"`
	GstInspectAvailable bool     `json:"gst_inspect_available"`
	VideoEncoders       []string `json:"video_encoders"`
	AudioSources        []string `json:"audio_sources"`
	WebRTCAvailable     bool     `json:"webrtc_available"`
	WebRTCReason        string   `json:"webrtc_reason"`

	// Active sessions
	ActiveVNCSessions    int `json:"active_vnc_sessions"`
	ActiveWebRTCSessions int `json:"active_webrtc_sessions"`

	// Framebuffer check
	FramebufferHasContent bool   `json:"framebuffer_has_content"`
	FramebufferError      string `json:"framebuffer_error,omitempty"`
}

// WoLSendData requests a Wake-on-LAN packet send operation.
type WoLSendData struct {
	RequestID string `json:"request_id,omitempty"`
	MAC       string `json:"mac"`
	Broadcast string `json:"broadcast,omitempty"`
}

// WoLResultData reports result of a WoL send operation.
type WoLResultData struct {
	RequestID string `json:"request_id,omitempty"`
	MAC       string `json:"mac"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
}

// FileListData is sent from hub to agent to list a directory.
type FileListData struct {
	RequestID  string `json:"request_id"`
	Path       string `json:"path"`
	ShowHidden bool   `json:"show_hidden,omitempty"`
}

// FileEntry describes a single file/directory in a listing or search result.
type FileEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path,omitempty"` // absolute path; populated in search results
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime string `json:"mod_time"`
	IsDir   bool   `json:"is_dir"`
}

// FileListedData is sent from agent to hub with directory listing results.
type FileListedData struct {
	RequestID string      `json:"request_id"`
	Path      string      `json:"path"`
	Entries   []FileEntry `json:"entries"`
	Error     string      `json:"error,omitempty"`
}

// FileReadData is sent from hub to agent to read a file.
type FileReadData struct {
	RequestID string `json:"request_id"`
	Path      string `json:"path"`
}

// FileDataPayload carries file content chunks (base64-encoded).
type FileDataPayload struct {
	RequestID string `json:"request_id"`
	Data      string `json:"data"`   // base64-encoded chunk
	Offset    int64  `json:"offset"` // byte offset
	Done      bool   `json:"done"`   // true if this is the last chunk
	Error     string `json:"error,omitempty"`
}

// FileWriteData is sent from hub to agent to write a file chunk.
type FileWriteData struct {
	RequestID string `json:"request_id"`
	Path      string `json:"path"`
	Data      string `json:"data"`   // base64-encoded chunk
	Offset    int64  `json:"offset"` // byte offset
	Done      bool   `json:"done"`   // true if this is the last chunk
}

// FileWrittenData is sent from agent to hub confirming write.
type FileWrittenData struct {
	RequestID    string `json:"request_id"`
	BytesWritten int64  `json:"bytes_written"`
	Error        string `json:"error,omitempty"`
}

// FileMkdirData is sent from hub to agent to create a directory.
type FileMkdirData struct {
	RequestID string `json:"request_id"`
	Path      string `json:"path"`
}

// FileDeleteData is sent from hub to agent to delete a file or directory.
type FileDeleteData struct {
	RequestID string `json:"request_id"`
	Path      string `json:"path"`
}

// FileRenameData is sent from hub to agent to rename/move a file or directory.
type FileRenameData struct {
	RequestID string `json:"request_id"`
	OldPath   string `json:"old_path"`
	NewPath   string `json:"new_path"`
}

// FileCopyData is sent from hub to agent to copy a file or directory.
type FileCopyData struct {
	RequestID string `json:"request_id"`
	SrcPath   string `json:"src_path"`
	DstPath   string `json:"dst_path"`
}

// FileResultData is a generic result for file operations (mkdir, delete, rename, copy).
type FileResultData struct {
	RequestID string `json:"request_id"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
}

// FileSearchData is sent from hub to agent to search for files matching a pattern.
type FileSearchData struct {
	RequestID  string `json:"request_id"`
	Path       string `json:"path"`
	Pattern    string `json:"pattern"`     // glob pattern matched against filename (e.g. "*.log")
	MaxResults int    `json:"max_results"` // default 100, capped at 500
}

// FileSearchResultData is sent from agent to hub with the filename search results.
type FileSearchResultData struct {
	RequestID string      `json:"request_id"`
	Matches   []FileEntry `json:"matches"`
	Error     string      `json:"error,omitempty"`
	Truncated bool        `json:"truncated"` // true when results were capped at MaxResults
}

// AlertNotifyData is sent from the hub to the agent when an alert fires or resolves.
type AlertNotifyData struct {
	ID        string `json:"id"`
	Severity  string `json:"severity"` // critical, high, medium, low
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	State     string `json:"state"` // firing, resolved
	Timestamp string `json:"timestamp"`
}

// EnrollmentApprovedData is sent from hub to agent when enrollment is approved.
type EnrollmentApprovedData struct {
	Token   string `json:"token"`
	AssetID string `json:"asset_id"`
}

// EnrollmentChallengeData is sent from hub to agent while pending enrollment.
// The agent signs the challenge and returns EnrollmentProofData.
type EnrollmentChallengeData struct {
	ConnectionID string `json:"connection_id"`
	Nonce        string `json:"nonce"`
	ExpiresAt    string `json:"expires_at,omitempty"`
}

// EnrollmentProofData is sent from agent to hub to prove possession of the
// device private key for pending enrollment verification.
type EnrollmentProofData struct {
	ConnectionID string `json:"connection_id"`
	Nonce        string `json:"nonce"`
	KeyAlgorithm string `json:"key_algorithm"`
	PublicKey    string `json:"public_key"`  // base64 raw public key bytes
	Fingerprint  string `json:"fingerprint"` // human-friendly fingerprint
	Signature    string `json:"signature"`   // base64 signature over canonical challenge payload
}

// EnrollmentRejectedData is sent from hub to agent when enrollment is rejected.
type EnrollmentRejectedData struct {
	Reason string `json:"reason,omitempty"`
}

// --- Docker Integration Data Structs ---

// DockerEngineInfo describes the Docker daemon on an agent host.
type DockerEngineInfo struct {
	Version    string `json:"version"`
	APIVersion string `json:"api_version"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
}

// DockerPortMapping describes a container port mapping.
type DockerPortMapping struct {
	Host      int    `json:"host"`
	Container int    `json:"container"`
	Protocol  string `json:"protocol"`
}

// DockerMountInfo describes a container mount/bind.
type DockerMountInfo struct {
	Type        string `json:"type"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

// DockerContainerInfo describes a container in the discovery payload.
type DockerContainerInfo struct {
	ID       string              `json:"id"`
	Name     string              `json:"name"`
	Image    string              `json:"image"`
	State    string              `json:"state"`
	Status   string              `json:"status"`
	Created  string              `json:"created"`
	Ports    []DockerPortMapping `json:"ports,omitempty"`
	Networks []string            `json:"networks,omitempty"`
	Labels   map[string]string   `json:"labels,omitempty"`
	Mounts   []DockerMountInfo   `json:"mounts,omitempty"`
}

// DockerImageInfo describes a Docker image.
type DockerImageInfo struct {
	ID      string   `json:"id"`
	Tags    []string `json:"tags"`
	Size    int64    `json:"size"`
	Created string   `json:"created"`
}

// DockerNetworkInfo describes a Docker network.
type DockerNetworkInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Driver string `json:"driver"`
	Scope  string `json:"scope"`
}

// DockerVolumeInfo describes a Docker volume.
type DockerVolumeInfo struct {
	Name       string `json:"name"`
	Driver     string `json:"driver"`
	Mountpoint string `json:"mountpoint"`
}

// DockerComposeStack describes an inferred Compose stack.
type DockerComposeStack struct {
	Name       string   `json:"name"`
	Status     string   `json:"status"`
	ConfigFile string   `json:"config_file,omitempty"`
	Containers []string `json:"containers"`
}

// DockerDiscoveryData is the payload for docker.discovery messages.
type DockerDiscoveryData struct {
	HostID        string                `json:"host_id"`
	Engine        DockerEngineInfo      `json:"engine"`
	Containers    []DockerContainerInfo `json:"containers"`
	Images        []DockerImageInfo     `json:"images"`
	Networks      []DockerNetworkInfo   `json:"networks"`
	Volumes       []DockerVolumeInfo    `json:"volumes"`
	ComposeStacks []DockerComposeStack  `json:"compose_stacks"`
}

// DockerDiscoveryDeltaData is the payload for docker.discovery.delta messages.
// It allows the agent to upsert/remove subsets of inventory without sending
// a full docker.discovery snapshot on every change.
type DockerDiscoveryDeltaData struct {
	HostID               string                `json:"host_id"`
	Engine               *DockerEngineInfo     `json:"engine,omitempty"`
	UpsertContainers     []DockerContainerInfo `json:"upsert_containers,omitempty"`
	RemoveContainerIDs   []string              `json:"remove_container_ids,omitempty"`
	UpsertImages         []DockerImageInfo     `json:"upsert_images,omitempty"`
	RemoveImageIDs       []string              `json:"remove_image_ids,omitempty"`
	UpsertNetworks       []DockerNetworkInfo   `json:"upsert_networks,omitempty"`
	RemoveNetworkIDs     []string              `json:"remove_network_ids,omitempty"`
	UpsertVolumes        []DockerVolumeInfo    `json:"upsert_volumes,omitempty"`
	RemoveVolumeNames    []string              `json:"remove_volume_names,omitempty"`
	ComposeStacks        []DockerComposeStack  `json:"compose_stacks,omitempty"`
	ReplaceComposeStacks bool                  `json:"replace_compose_stacks,omitempty"`
}

// DockerContainerStats holds per-container resource metrics.
type DockerContainerStats struct {
	ID              string  `json:"id"`
	CPUPercent      float64 `json:"cpu_percent"`
	MemoryBytes     int64   `json:"memory_bytes"`
	MemoryLimit     int64   `json:"memory_limit"`
	MemoryPercent   float64 `json:"memory_percent"`
	NetRXBytes      int64   `json:"net_rx_bytes"`
	NetTXBytes      int64   `json:"net_tx_bytes"`
	BlockReadBytes  int64   `json:"block_read_bytes"`
	BlockWriteBytes int64   `json:"block_write_bytes"`
	PIDs            int     `json:"pids"`
}

// DockerStatsData is the payload for docker.stats messages.
type DockerStatsData struct {
	HostID     string                 `json:"host_id"`
	Containers []DockerContainerStats `json:"containers"`
}

// DockerEventActor describes the target of a Docker event.
type DockerEventActor struct {
	ID         string            `json:"id"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// DockerEventData is the payload for docker.events messages.
type DockerEventData struct {
	HostID    string           `json:"host_id"`
	Type      string           `json:"type"`
	Action    string           `json:"action"`
	Actor     DockerEventActor `json:"actor"`
	Timestamp int64            `json:"timestamp"`
}

// DockerActionData is the payload for docker.action messages (Hub -> Agent).
type DockerActionData struct {
	RequestID   string            `json:"request_id"`
	Action      string            `json:"action"`
	ContainerID string            `json:"container_id,omitempty"`
	ImageRef    string            `json:"image_ref,omitempty"`
	Params      map[string]string `json:"params,omitempty"`
}

// DockerActionResultData is the payload for docker.action.result messages.
type DockerActionResultData struct {
	RequestID string `json:"request_id"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
	Data      string `json:"data,omitempty"`
}

// DockerLogsStartData is the payload for docker.logs.start messages.
type DockerLogsStartData struct {
	SessionID   string `json:"session_id"`
	ContainerID string `json:"container_id"`
	Tail        int    `json:"tail"`
	Follow      bool   `json:"follow"`
	Timestamps  bool   `json:"timestamps,omitempty"`
}

// DockerLogsStopData is the payload for docker.logs.stop messages.
type DockerLogsStopData struct {
	SessionID string `json:"session_id"`
}

// DockerLogsStreamData is the payload for docker.logs.stream messages.
type DockerLogsStreamData struct {
	SessionID string `json:"session_id"`
	Stream    string `json:"stream"` // "stdout" or "stderr"
	Data      string `json:"data"`   // log line content
	Timestamp string `json:"timestamp,omitempty"`
}

// DockerExecStartData is the payload for docker.exec.start messages.
type DockerExecStartData struct {
	SessionID   string   `json:"session_id"`
	ContainerID string   `json:"container_id"`
	Command     []string `json:"command"`
	TTY         bool     `json:"tty"`
	Cols        int      `json:"cols,omitempty"`
	Rows        int      `json:"rows,omitempty"`
}

// DockerExecStartedData is the payload for docker.exec.started messages.
type DockerExecStartedData struct {
	SessionID string `json:"session_id"`
}

// DockerExecDataPayload carries exec session I/O (base64-encoded).
type DockerExecDataPayload struct {
	SessionID string `json:"session_id"`
	Data      string `json:"data"` // base64-encoded bytes
}

// DockerExecInputData carries exec stdin from hub to agent (base64-encoded).
type DockerExecInputData struct {
	SessionID string `json:"session_id"`
	Data      string `json:"data"` // base64-encoded bytes
}

// DockerExecResizeData is the payload for docker.exec.resize messages.
type DockerExecResizeData struct {
	SessionID string `json:"session_id"`
	Cols      int    `json:"cols"`
	Rows      int    `json:"rows"`
}

// DockerExecCloseData is the payload for docker.exec.close/closed messages.
type DockerExecCloseData struct {
	SessionID string `json:"session_id"`
	Reason    string `json:"reason,omitempty"`
}

// DockerComposeActionData is the payload for docker.compose.action messages.
type DockerComposeActionData struct {
	RequestID   string `json:"request_id"`
	StackName   string `json:"stack_name"`
	Action      string `json:"action"` // up, down, restart, pull, deploy
	ConfigDir   string `json:"config_dir,omitempty"`
	ComposeYAML string `json:"compose_yaml,omitempty"`
}

// DockerComposeResultData is the payload for docker.compose.result messages.
type DockerComposeResultData struct {
	RequestID string `json:"request_id"`
	Success   bool   `json:"success"`
	Output    string `json:"output,omitempty"`
	Error     string `json:"error,omitempty"`
}

// --- Platform Enrichment Data Structs ---
// ProcessListData is sent from hub to agent to request a process list.
type ProcessListData struct {
	RequestID string `json:"request_id"`
	SortBy    string `json:"sort_by,omitempty"` // "cpu" or "memory", default "cpu"
	Limit     int    `json:"limit,omitempty"`   // default 25
}

// ProcessListedData is sent from agent to hub with the process list result.
type ProcessListedData struct {
	RequestID string        `json:"request_id"`
	Processes []ProcessInfo `json:"processes"`
	Error     string        `json:"error,omitempty"`
}

// ProcessInfo describes a single running process.
type ProcessInfo struct {
	PID     int     `json:"pid"`
	Name    string  `json:"name"`
	User    string  `json:"user"`
	CPUPct  float64 `json:"cpu_pct"`
	MemPct  float64 `json:"mem_pct"`
	MemRSS  int64   `json:"mem_rss"`
	Command string  `json:"command"`
}

// ProcessKillData is sent from hub to agent to signal a process.
type ProcessKillData struct {
	PID    int    `json:"pid"`
	Signal string `json:"signal"` // "SIGTERM" | "SIGKILL" | "SIGINT" | "SIGHUP"; empty defaults to SIGTERM
}

// ProcessKillResultData is sent from agent to hub with the signal delivery result.
type ProcessKillResultData struct {
	PID     int    `json:"pid"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// ServiceListData is sent from hub to agent to request a service list.
type ServiceListData struct {
	RequestID string `json:"request_id"`
}

// ServiceListedData is sent from agent to hub with the service list result.
type ServiceListedData struct {
	RequestID string        `json:"request_id"`
	Services  []ServiceInfo `json:"services"`
	Error     string        `json:"error,omitempty"`
}

// ServiceInfo describes a single systemd service unit.
type ServiceInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ActiveState string `json:"active_state"` // active, inactive, failed, activating, deactivating
	SubState    string `json:"sub_state"`    // running, dead, exited, etc.
	Enabled     string `json:"enabled"`      // enabled, disabled, static, masked
	LoadState   string `json:"load_state"`   // loaded, not-found, masked
}

// ServiceActionData is sent from hub to agent to perform a service action.
type ServiceActionData struct {
	RequestID string `json:"request_id"`
	Service   string `json:"service"`
	Action    string `json:"action"` // start, stop, restart, enable, disable
}

// ServiceResultData is sent from agent to hub with the service action result.
type ServiceResultData struct {
	RequestID string `json:"request_id"`
	OK        bool   `json:"ok"`
	Output    string `json:"output,omitempty"`
	Error     string `json:"error,omitempty"`
}

// DiskListData is sent from hub to agent to request mount/disk info.
type DiskListData struct {
	RequestID string `json:"request_id"`
}

// DiskListedData is sent from agent to hub with mount/disk info results.
type DiskListedData struct {
	RequestID string      `json:"request_id"`
	Mounts    []MountInfo `json:"mounts"`
	Error     string      `json:"error,omitempty"`
}

// MountInfo describes a single mounted filesystem.
type MountInfo struct {
	Device     string  `json:"device"`
	MountPoint string  `json:"mount_point"`
	FSType     string  `json:"fs_type"`
	Total      uint64  `json:"total"`
	Used       uint64  `json:"used"`
	Available  uint64  `json:"available"`
	UsePct     float64 `json:"use_pct"`
}

// NetworkListData is sent from hub to agent to request network interface info.
type NetworkListData struct {
	RequestID string `json:"request_id"`
}

// NetworkListedData is sent from agent to hub with network interface results.
type NetworkListedData struct {
	RequestID  string         `json:"request_id"`
	Interfaces []NetInterface `json:"interfaces"`
	Error      string         `json:"error,omitempty"`
}

// NetInterface describes a single network interface and its counters.
type NetInterface struct {
	Name      string   `json:"name"`
	State     string   `json:"state"`
	MAC       string   `json:"mac"`
	MTU       int      `json:"mtu"`
	IPs       []string `json:"ips"`
	RXBytes   uint64   `json:"rx_bytes"`
	TXBytes   uint64   `json:"tx_bytes"`
	RXPackets uint64   `json:"rx_packets"`
	TXPackets uint64   `json:"tx_packets"`
}

// NetworkActionData is sent from hub to agent to perform a network action.
type NetworkActionData struct {
	RequestID    string `json:"request_id"`
	Action       string `json:"action"` // apply|rollback
	Method       string `json:"method,omitempty"`
	Connection   string `json:"connection,omitempty"`    // nmcli connection name
	VerifyTarget string `json:"verify_target,omitempty"` // optional host/ip to ping
}

// NetworkResultData is sent from agent to hub with network action results.
type NetworkResultData struct {
	RequestID         string `json:"request_id"`
	OK                bool   `json:"ok"`
	Output            string `json:"output,omitempty"`
	Error             string `json:"error,omitempty"`
	RollbackAttempted bool   `json:"rollback_attempted,omitempty"`
	RollbackSucceeded bool   `json:"rollback_succeeded,omitempty"`
	RollbackOutput    string `json:"rollback_output,omitempty"`
	RollbackReference string `json:"rollback_reference,omitempty"`
}

// PackageListData is sent from hub to agent to request installed packages.
type PackageListData struct {
	RequestID string `json:"request_id"`
}

// PackageListedData is sent from agent to hub with the installed packages result.
type PackageListedData struct {
	RequestID string        `json:"request_id"`
	Packages  []PackageInfo `json:"packages"`
	Error     string        `json:"error,omitempty"`
}

// PackageInfo describes a single installed package.
type PackageInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Status  string `json:"status"` // "installed", "config-files", etc.
}

// PackageActionData is sent from hub to agent to run package-manager actions.
type PackageActionData struct {
	RequestID string   `json:"request_id"`
	Action    string   `json:"action"` // install|remove|upgrade
	Packages  []string `json:"packages,omitempty"`
}

// PackageResultData is sent from agent to hub with package action results.
type PackageResultData struct {
	RequestID      string `json:"request_id"`
	OK             bool   `json:"ok"`
	Output         string `json:"output"`
	Error          string `json:"error,omitempty"`
	RebootRequired bool   `json:"reboot_required,omitempty"`
}

// CronListData is sent from hub to agent to request cron/timer entries.
type CronListData struct {
	RequestID string `json:"request_id"`
}

// CronListedData is sent from agent to hub with cron/timer entries.
type CronListedData struct {
	RequestID string      `json:"request_id"`
	Entries   []CronEntry `json:"entries"`
	Error     string      `json:"error,omitempty"`
}

// CronEntry describes a single cron job or systemd timer.
type CronEntry struct {
	Source   string `json:"source"`   // "systemd-timer" or "crontab"
	Schedule string `json:"schedule"` // cron expression or systemd calendar spec
	Command  string `json:"command"`
	User     string `json:"user"`
	NextRun  string `json:"next_run,omitempty"` // RFC3339 or empty
	LastRun  string `json:"last_run,omitempty"` // RFC3339 or empty
}

// UsersListData is sent from hub to agent to request active user sessions.
type UsersListData struct {
	RequestID string `json:"request_id"`
}

// UsersListedData is sent from agent to hub with active user sessions.
type UsersListedData struct {
	RequestID string        `json:"request_id"`
	Sessions  []UserSession `json:"sessions"`
	Error     string        `json:"error,omitempty"`
}

// UserSession describes a single logged-in user session.
type UserSession struct {
	Username    string `json:"username"`
	Terminal    string `json:"terminal"`
	RemoteHost  string `json:"remote_host,omitempty"`
	LoginTime   string `json:"login_time"` // RFC3339 or human-readable
	SessionType string `json:"session_type,omitempty"`
	Display     string `json:"display,omitempty"`
}

// --- Web Service Discovery Data Structs ---

// WebServiceReportData is sent by the agent with discovered web services.
type WebServiceReportData struct {
	HostAssetID string                    `json:"host_asset_id"`
	Services    []DiscoveredWebService    `json:"services"`
	Discovery   *WebServiceDiscoveryStats `json:"discovery,omitempty"`
}

// WebServiceDiscoveryStats captures one discovery cycle summary from an agent.
type WebServiceDiscoveryStats struct {
	CollectedAt      string                                   `json:"collected_at"`
	CycleDurationMs  int                                      `json:"cycle_duration_ms"`
	TotalServices    int                                      `json:"total_services"`
	Sources          map[string]WebServiceDiscoverySourceStat `json:"sources,omitempty"`
	FinalSourceCount map[string]int                           `json:"final_source_count,omitempty"`
}

// WebServiceDiscoverySourceStat captures per-source cycle behavior and yield.
type WebServiceDiscoverySourceStat struct {
	Enabled       bool `json:"enabled"`
	DurationMs    int  `json:"duration_ms"`
	ServicesFound int  `json:"services_found"`
}

// WebServiceHealthPoint captures one status check sample for a web service.
type WebServiceHealthPoint struct {
	At         string `json:"at"`
	Status     string `json:"status"`
	ResponseMs int    `json:"response_ms,omitempty"`
}

// WebServiceHealthSummary captures rolling uptime and recent status history.
type WebServiceHealthSummary struct {
	Window        string                  `json:"window"`
	Checks        int                     `json:"checks"`
	UpChecks      int                     `json:"up_checks"`
	UptimePercent float64                 `json:"uptime_percent"`
	LastCheckedAt string                  `json:"last_checked_at,omitempty"`
	LastChangeAt  string                  `json:"last_change_at,omitempty"`
	Recent        []WebServiceHealthPoint `json:"recent,omitempty"`
}

// DiscoveredWebService represents a single discovered web service.
type DiscoveredWebService struct {
	ID          string                   `json:"id"`
	ServiceKey  string                   `json:"service_key"`
	Name        string                   `json:"name"`
	Category    string                   `json:"category"`
	URL         string                   `json:"url"`
	Source      string                   `json:"source"`
	Status      string                   `json:"status"`
	ResponseMs  int                      `json:"response_ms"`
	ContainerID string                   `json:"container_id,omitempty"`
	ServiceUnit string                   `json:"service_unit,omitempty"`
	HostAssetID string                   `json:"host_asset_id"`
	IconKey     string                   `json:"icon_key"`
	Metadata    map[string]string        `json:"metadata,omitempty"`
	Health      *WebServiceHealthSummary `json:"health,omitempty"`
}
