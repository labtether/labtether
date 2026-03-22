package remoteaccess

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/securityruntime"
)

type EncoderCandidate struct {
	name       string // internal name (e.g. "nvenc_h264")
	gstElement string // GStreamer element name (e.g. "nvh264enc")
}

var WebRTCRuntimeGOOS = runtime.GOOS
var WebRTCLookPath = exec.LookPath
var NewWebRTCSecurityCommand = securityruntime.NewCommand

func VideoEncoderPriority() []EncoderCandidate {
	return []EncoderCandidate{
		{name: "nvenc_h264", gstElement: "nvh264enc"},
		{name: "vaapi_h264", gstElement: "vaapih264enc"},
		{name: "x264", gstElement: "x264enc"},
		{name: "vp8", gstElement: "vp8enc"},
	}
}

type GstPipelineConfig struct {
	display string
	encoder string
	width   int
	height  int
	fps     int
	rtpPort int
	bitrate int // kbps
}

type GstAudioConfig struct {
	source  string // pulsesrc or pipewiresrc
	rtpPort int
	bitrate int // bps
}

const (
	webrtcReasonUnsupportedPlatform        = "unsupported_platform"
	webrtcReasonMissingGstLaunch           = "gst_launch_not_found"
	webrtcReasonMissingGstInspect          = "gst_inspect_not_found"
	webrtcReasonNoVideoEncoder             = "no_supported_video_encoder"
	webrtcReasonWaylandDisabled            = "wayland_experimental_disabled"
	webrtcReasonWaylandPipeWireMissing     = "wayland_pipewiresrc_not_found"
	webrtcReasonWaylandPipeWireNodeMissing = "wayland_pipewire_node_missing"
)

func UnsupportedPlatformWebRTCReason(goos string) string {
	platform := strings.TrimSpace(goos)
	if platform == "" {
		platform = "unknown"
	}
	return fmt.Sprintf("%s:%s", webrtcReasonUnsupportedPlatform, platform)
}

// detectWebRTCCapabilities checks for GStreamer and available encoder/audio elements.
func DetectWebRTCCapabilities() agentmgr.WebRTCCapabilitiesData {
	return DetectWebRTCCapabilitiesWithConfig(WebRTCCapabilityConfigFromEnv())
}

func DetectWebRTCCapabilitiesForSettings(settings map[string]string) agentmgr.WebRTCCapabilitiesData {
	return DetectWebRTCCapabilitiesWithConfig(LoadWebRTCConfig(settings))
}

func WebRTCCapabilityConfigFromEnv() WebRTCConfig {
	cfg := WebRTCConfig{
		Enabled: true,
		STUNURL: "stun:stun.l.google.com:19302",
		FPS:     30,
	}
	cfg.WaylandExperimentalEnabled = parseBoolEnv("LABTETHER_WEBRTC_WAYLAND_EXPERIMENTAL_ENABLED", false)
	cfg.WaylandPipeWireNodeID = strings.TrimSpace(os.Getenv("LABTETHER_WEBRTC_WAYLAND_PIPEWIRE_NODE_ID"))
	cfg.WaylandInputBackend = strings.TrimSpace(strings.ToLower(os.Getenv("LABTETHER_WEBRTC_WAYLAND_INPUT_BACKEND")))
	if cfg.WaylandInputBackend == "" {
		cfg.WaylandInputBackend = "auto"
	}
	return cfg
}

func DetectWebRTCCapabilitiesWithConfig(cfg WebRTCConfig) agentmgr.WebRTCCapabilitiesData {
	caps := agentmgr.WebRTCCapabilitiesData{}
	session := DetectDesktopSessionFn()
	caps.DesktopSessionType = session.Type
	caps.DesktopBackend = session.Backend

	if WebRTCRuntimeGOOS != "linux" {
		// Phase 3 targets Linux first. Other platforms continue using non-WebRTC paths.
		caps.UnavailableReason = UnsupportedPlatformWebRTCReason(WebRTCRuntimeGOOS)
		return caps
	}

	if _, err := WebRTCLookPath("gst-launch-1.0"); err != nil {
		log.Printf("webrtc: gst-launch-1.0 not found, WebRTC unavailable")
		caps.UnavailableReason = webrtcReasonMissingGstLaunch
		return caps
	}

	inspectPath, err := WebRTCLookPath("gst-inspect-1.0")
	if err != nil {
		log.Printf("webrtc: gst-inspect-1.0 not found, WebRTC unavailable")
		caps.UnavailableReason = webrtcReasonMissingGstInspect
		return caps
	}

	for _, enc := range VideoEncoderPriority() {
		cmd, cmdErr := NewWebRTCSecurityCommand(inspectPath, enc.gstElement)
		if cmdErr == nil && cmd.Run() == nil {
			caps.VideoEncoders = append(caps.VideoEncoders, enc.name)
		}
	}
	if len(caps.VideoEncoders) == 0 {
		log.Printf("webrtc: no supported video encoders found")
		caps.UnavailableReason = webrtcReasonNoVideoEncoder
		return caps
	}

	if cmd, cmdErr := NewWebRTCSecurityCommand(inspectPath, "pipewiresrc"); cmdErr == nil && cmd.Run() == nil {
		caps.AudioSources = append(caps.AudioSources, "pipewire")
	}
	if cmd, cmdErr := NewWebRTCSecurityCommand(inspectPath, "pulsesrc"); cmdErr == nil && cmd.Run() == nil {
		caps.AudioSources = append(caps.AudioSources, "pulseaudio")
	}

	switch session.Type {
	case DesktopSessionTypeWayland:
		caps.CaptureBackend = "pipewiresrc"
		caps.VNCRealDesktopSupported = false
		if !cfg.WaylandExperimentalEnabled {
			caps.UnavailableReason = webrtcReasonWaylandDisabled
			return caps
		}
		if cmd, cmdErr := NewWebRTCSecurityCommand(inspectPath, "pipewiresrc"); cmdErr != nil || cmd.Run() != nil {
			caps.UnavailableReason = webrtcReasonWaylandPipeWireMissing
			return caps
		}
		if strings.TrimSpace(cfg.WaylandPipeWireNodeID) == "" {
			caps.UnavailableReason = webrtcReasonWaylandPipeWireNodeMissing
			return caps
		}
		caps.WebRTCRealDesktopSupported = true
		caps.Available = true
	case DesktopSessionTypeX11:
		caps.CaptureBackend = "ximagesrc"
		caps.Displays = DetectX11DisplayIdentifiers()
		caps.VNCRealDesktopSupported = true
		caps.WebRTCRealDesktopSupported = true
		caps.Available = true
	default:
		caps.CaptureBackend = "ximagesrc"
		caps.Displays = DetectX11DisplayIdentifiers()
		caps.VNCRealDesktopSupported = false
		caps.WebRTCRealDesktopSupported = false
		caps.Available = true
	}
	return caps
}

func DetectX11DisplayIdentifiers() []string {
	session := DetectDesktopSessionFn()
	candidates := []string{
		strings.TrimSpace(os.Getenv("LABTETHER_WEBRTC_X11_DISPLAY")),
		strings.TrimSpace(session.Display),
		strings.TrimSpace(os.Getenv("DISPLAY")),
	}
	candidates = append(candidates, AppendDetectedActiveDisplays(nil)...)
	candidates = append(candidates, ":0")

	seen := make(map[string]struct{}, len(candidates))
	displays := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		normalized := NormalizeX11DisplayIdentifier(candidate)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		displays = append(displays, normalized)
	}
	return displays
}

func NormalizeX11DisplayIdentifier(raw string) string {
	display := strings.TrimSpace(raw)
	if display == "" {
		return ""
	}
	if strings.Contains(display, ":") {
		return display
	}
	return ""
}

// buildGStreamerVideoPipeline builds a gst-launch pipeline for screen capture -> RTP.
func BuildGStreamerVideoPipeline(cfg GstPipelineConfig) string {
	display := strings.TrimSpace(cfg.display)
	if display == "" {
		display = ":0"
	}
	width := cfg.width
	if width <= 0 {
		width = 1920
	}
	height := cfg.height
	if height <= 0 {
		height = 1080
	}
	fps := cfg.fps
	if fps <= 0 {
		fps = 30
	}
	bitrate := cfg.bitrate
	if bitrate <= 0 {
		bitrate = 5000
	}

	encoder := "x264enc tune=zerolatency speed-preset=ultrafast bitrate=5000 key-int-max=30"
	rtpPayloader := "rtph264pay config-interval=-1"
	switch cfg.encoder {
	case "nvh264enc":
		encoder = fmt.Sprintf("nvh264enc bitrate=%d rc-mode=cbr", bitrate)
		rtpPayloader = "rtph264pay config-interval=-1"
	case "vaapih264enc":
		encoder = fmt.Sprintf("vaapih264enc bitrate=%d rate-control=cbr", bitrate)
		rtpPayloader = "rtph264pay config-interval=-1"
	case "vp8enc":
		encoder = fmt.Sprintf("vp8enc target-bitrate=%d deadline=1 cpu-used=4", bitrate*1000)
		rtpPayloader = "rtpvp8pay"
	default:
		encoder = fmt.Sprintf("x264enc tune=zerolatency speed-preset=ultrafast bitrate=%d key-int-max=30", bitrate)
		rtpPayloader = "rtph264pay config-interval=-1"
	}

	return fmt.Sprintf(
		"ximagesrc display-name=%s use-damage=false show-pointer=true ! "+
			"video/x-raw,framerate=%d/1 ! videoconvert ! videoscale ! "+
			"video/x-raw,width=%d,height=%d ! %s ! %s pt=96 ! "+
			"udpsink host=127.0.0.1 port=%d sync=false async=false",
		display, fps, width, height, encoder, rtpPayloader, cfg.rtpPort,
	)
}

func BuildWaylandPipeWireVideoPipeline(nodeID string, cfg GstPipelineConfig) string {
	width := cfg.width
	if width <= 0 {
		width = 1920
	}
	height := cfg.height
	if height <= 0 {
		height = 1080
	}
	fps := cfg.fps
	if fps <= 0 {
		fps = 30
	}
	bitrate := cfg.bitrate
	if bitrate <= 0 {
		bitrate = 5000
	}

	encoder := fmt.Sprintf("x264enc tune=zerolatency speed-preset=ultrafast bitrate=%d key-int-max=30", bitrate)
	rtpPayloader := "rtph264pay config-interval=-1"
	switch cfg.encoder {
	case "nvh264enc":
		encoder = fmt.Sprintf("nvh264enc bitrate=%d rc-mode=cbr", bitrate)
	case "vaapih264enc":
		encoder = fmt.Sprintf("vaapih264enc bitrate=%d rate-control=cbr", bitrate)
	case "vp8enc":
		encoder = fmt.Sprintf("vp8enc target-bitrate=%d deadline=1 cpu-used=4", bitrate*1000)
		rtpPayloader = "rtpvp8pay"
	}

	return fmt.Sprintf(
		"pipewiresrc path=%s do-timestamp=true keepalive-time=1000 ! "+
			"video/x-raw,framerate=%d/1 ! videoconvert ! videoscale ! "+
			"video/x-raw,width=%d,height=%d ! %s ! %s pt=96 ! "+
			"udpsink host=127.0.0.1 port=%d sync=false async=false",
		strings.TrimSpace(nodeID), fps, width, height, encoder, rtpPayloader, cfg.rtpPort,
	)
}

// buildGStreamerAudioPipeline builds an audio capture -> Opus RTP pipeline.
func BuildGStreamerAudioPipeline(cfg GstAudioConfig) string {
	source := strings.TrimSpace(cfg.source)
	if source == "" {
		source = "pulsesrc"
	}
	bitrate := cfg.bitrate
	if bitrate <= 0 {
		bitrate = 128000
	}
	return fmt.Sprintf(
		"%s ! audioconvert ! audioresample ! opusenc bitrate=%d frame-size=20 ! "+
			"rtpopuspay pt=111 ! udpsink host=127.0.0.1 port=%d sync=false async=false",
		source, bitrate, cfg.rtpPort,
	)
}

// bestVideoEncoder returns the preferred encoder capability and GStreamer element.
func BestVideoEncoder(caps agentmgr.WebRTCCapabilitiesData) (name, gstElement string) {
	for _, cand := range VideoEncoderPriority() {
		for _, found := range caps.VideoEncoders {
			if cand.name == strings.TrimSpace(found) {
				return cand.name, cand.gstElement
			}
		}
	}
	return "", ""
}

// bestAudioSource returns the preferred GStreamer source element.
func BestAudioSource(caps agentmgr.WebRTCCapabilitiesData) string {
	hasPipewire := false
	hasPulse := false
	for _, src := range caps.AudioSources {
		switch strings.TrimSpace(strings.ToLower(src)) {
		case "pipewire":
			hasPipewire = true
		case "pulseaudio":
			hasPulse = true
		}
	}
	if hasPipewire {
		return "pipewiresrc"
	}
	if hasPulse {
		return "pulsesrc"
	}
	return ""
}
