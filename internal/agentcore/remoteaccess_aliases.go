package agentcore

// Backward-compatible type aliases and function wrappers for types moved to
// the remoteaccess subpackage. This allows the rest of the agentcore root
// package to continue using the same identifiers without import changes.

import (
	"github.com/labtether/labtether/internal/agentcore/files"
	"github.com/labtether/labtether/internal/agentcore/remoteaccess"
	"github.com/labtether/labtether/internal/agentmgr"
)

func init() {
	// Wire self-update callback from root into remoteaccess so that the exec_ws
	// handler can trigger self-update without a circular import.
	remoteaccess.SelfUpdateFn = func(cfg remoteaccess.ExecConfig, force bool) (bool, string, error) {
		// Only the API token is needed from the remote handler context — the rest
		// of the RuntimeConfig is not available outside root agentcore.
		return checkAndApplySelfUpdateWithOptions(RuntimeConfig{APIToken: cfg.APIToken}, selfUpdateOptions{Force: force})
	}
	remoteaccess.SelfUpdateExitFn = agentExitFn
}

// Terminal manager aliases.
type terminalManager = remoteaccess.TerminalManager

var newTerminalManager = remoteaccess.NewTerminalManager

// Desktop manager aliases.
type desktopManager = remoteaccess.DesktopManager
type desktopSession = remoteaccess.DesktopSession

var newDesktopManager = remoteaccess.NewDesktopManager

// Display manager aliases.
type displayManager = remoteaccess.DisplayManager

var newDisplayManager = remoteaccess.NewDisplayManager

// WebRTC manager aliases.
type webrtcManager = remoteaccess.WebRTCManager
type webrtcSession = remoteaccess.WebRTCSession
type webrtcInputEvent = remoteaccess.WebRTCInputEvent

func newWebRTCManager(caps agentmgr.WebRTCCapabilitiesData, settings remoteaccess.SettingsProvider, fileMgr *files.Manager, dispMgr *displayManager) *webrtcManager {
	return remoteaccess.NewWebRTCManager(caps, settings, fileMgr, dispMgr)
}

// Audio sideband manager aliases.
type audioSidebandManager = remoteaccess.AudioSidebandManager

var newAudioSidebandManager = remoteaccess.NewAudioSidebandManager

// Desktop session type aliases.
type desktopSessionInfo = remoteaccess.DesktopSessionInfo

var detectDesktopSessionFn = remoteaccess.DetectDesktopSessionFn

// Desktop session type constants.
const (
	desktopSessionTypeUnknown  = remoteaccess.DesktopSessionTypeUnknown
	desktopSessionTypeHeadless = remoteaccess.DesktopSessionTypeHeadless
	desktopSessionTypeX11      = remoteaccess.DesktopSessionTypeX11
	desktopSessionTypeWayland  = remoteaccess.DesktopSessionTypeWayland

	desktopBackendHeadless        = remoteaccess.DesktopBackendHeadless
	desktopBackendX11             = remoteaccess.DesktopBackendX11
	desktopBackendWaylandPipeWire = remoteaccess.DesktopBackendWaylandPipeWire

	maxDesktopSessions  = remoteaccess.MaxDesktopSessions
	maxTerminalSessions = remoteaccess.MaxTerminalSessions
)

// WebRTC detection and config aliases.
type webrtcConfig = remoteaccess.WebRTCConfig
type gstPipelineConfig = remoteaccess.GstPipelineConfig
type gstAudioConfig = remoteaccess.GstAudioConfig
type encoderCandidate = remoteaccess.EncoderCandidate

func detectWebRTCCapabilitiesForConfig(cfg RuntimeConfig) agentmgr.WebRTCCapabilitiesData {
	return remoteaccess.DetectWebRTCCapabilitiesForSettings(
		AgentSettingValuesFromConfigSnapshot(configSnapshotFromRuntimeConfig(cfg)),
	)
}

var detectWebRTCCapabilities = remoteaccess.DetectWebRTCCapabilities
var detectWebRTCCapabilitiesWithConfig = remoteaccess.DetectWebRTCCapabilitiesWithConfig
var loadWebRTCConfig = remoteaccess.LoadWebRTCConfig
var videoEncoderPriority = remoteaccess.VideoEncoderPriority
var bestVideoEncoder = remoteaccess.BestVideoEncoder
var bestAudioSource = remoteaccess.BestAudioSource
var buildGStreamerVideoPipeline = remoteaccess.BuildGStreamerVideoPipeline
var buildWaylandPipeWireVideoPipeline = remoteaccess.BuildWaylandPipeWireVideoPipeline
var buildGStreamerAudioPipeline = remoteaccess.BuildGStreamerAudioPipeline

// WebRTC detect package-level var aliases.
var webrtcRuntimeGOOS = &remoteaccess.WebRTCRuntimeGOOS
var webrtcLookPath = &remoteaccess.WebRTCLookPath
var newWebRTCSecurityCommand = &remoteaccess.NewWebRTCSecurityCommand

// Display functions aliases.
var normalizeX11DisplayIdentifier = remoteaccess.NormalizeX11DisplayIdentifier
var appendDetectedActiveDisplays = remoteaccess.AppendDetectedActiveDisplays
var isDisplayAvailable = remoteaccess.IsDisplayAvailable
var preferredX11Display = remoteaccess.PreferredX11Display

// VNC aliases.
var startDesktopVNCServer = &remoteaccess.StartDesktopVNCServer
var dialDesktopVNC = &remoteaccess.DialDesktopVNC
var startDesktopXvfb = &remoteaccess.StartDesktopXvfb
var findDesktopFreeDisplay = &remoteaccess.FindDesktopFreeDisplay
var startDesktopBootstrap = &remoteaccess.StartDesktopBootstrap
var launchDesktopVNCReady = &remoteaccess.LaunchDesktopVNCReady

// VNC functions.
var buildX11VNCArgs = remoteaccess.BuildX11VNCArgs
var buildXvfbArgs = remoteaccess.BuildXvfbArgs
var startLinuxVNCServer = remoteaccess.StartLinuxVNCServer
var startDesktopBootstrapShell = remoteaccess.StartDesktopBootstrapShell
var waitForXvfbReady = remoteaccess.WaitForXvfbReady
var isDisplayError = remoteaccess.IsDisplayError
var buildX11ClientEnv = remoteaccess.BuildX11ClientEnv
var generateHexCookie = remoteaccess.GenerateHexCookie
var createXauthorityFile = remoteaccess.CreateXauthorityFile
var findFreeDisplay = remoteaccess.FindFreeDisplay
var terminateProcess = remoteaccess.TerminateProcess
var removeProcessLog = remoteaccess.RemoveProcessLog
var summarizeProcessLogTail = remoteaccess.SummarizeProcessLogTail
var desktopDebugEnabled = remoteaccess.DesktopDebugEnabled

// X11 auth aliases.
var discoverDisplayXAuthorityFn = &remoteaccess.DiscoverDisplayXAuthorityFn
var discoverDisplayXAuthority = remoteaccess.DiscoverDisplayXAuthority
var x11ProcRoot = &remoteaccess.X11ProcRoot

// X11 display aliases.
var newX11UtilityCommand = &remoteaccess.NewX11UtilityCommand
var wakeX11Display = remoteaccess.WakeX11Display

// Audio sideband aliases.
var startAudioCapture = &remoteaccess.StartAudioCapture

// Exec/command handler aliases.
var handleCommandRequest = func(transport *wsTransport, msg agentmgr.Message, cfg RuntimeConfig) {
	remoteaccess.HandleCommandRequest(transport, msg, remoteaccess.ExecConfig{APIToken: cfg.APIToken})
}
var handleUpdateRequest = func(transport *wsTransport, msg agentmgr.Message, cfg RuntimeConfig) {
	remoteaccess.HandleUpdateRequest(transport, msg, remoteaccess.ExecConfig{APIToken: cfg.APIToken})
}

var tokenAllowsAnyCapability = remoteaccess.TokenAllowsAnyCapability
var validateUpdatePackages = remoteaccess.ValidateUpdatePackages

// Diagnostic aliases.
var collectDesktopDiagnostic = remoteaccess.CollectDesktopDiagnostic
var handleDesktopDiagnose = func(transport *wsTransport, msg agentmgr.Message, deskMgr *desktopManager, webrtcMgr *webrtcManager) {
	remoteaccess.HandleDesktopDiagnose(transport, msg, deskMgr, webrtcMgr)
}

// Session detection aliases.
var detectDesktopSession = remoteaccess.DetectDesktopSession
var sessionPreferenceScore = remoteaccess.SessionPreferenceScore

// Default command timeout (moved to remoteaccess).
const defaultCommandTimeout = remoteaccess.DefaultCommandTimeout

// Detect X11 displays aliases.
var detectX11DisplayIdentifiers = remoteaccess.DetectX11DisplayIdentifiers

// WebRTC handler aliases.
var resolveWebRTCDisplay = remoteaccess.ResolveWebRTCDisplay
var webrtcVideoBitrateForQuality = remoteaccess.WebRTCVideoBitrateForQuality
var sendWebRTCStopped = remoteaccess.SendWebRTCStopped
var iceCandidateSendDelay = remoteaccess.ICECandidateSendDelay
var decodeWebRTCInputEvent = remoteaccess.DecodeWebRTCInputEvent
var domCodeToX11Keysym = remoteaccess.DomCodeToX11Keysym
var domKeyToX11Keysym = remoteaccess.DomKeyToX11Keysym
var x11KeyArgument = remoteaccess.X11KeyArgument
var valueOrDash = remoteaccess.ValueOrDash

// Desktop handler aliases.
var sendDesktopClosed = remoteaccess.SendDesktopClosed
var sendDesktopStarted = remoteaccess.SendDesktopStarted
var sendTerminalClosed = remoteaccess.SendTerminalClosed
