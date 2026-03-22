package remoteaccess

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/agentcore/system"
	"github.com/labtether/labtether/internal/agentmgr"
)

func TestVideoEncoderPriority(t *testing.T) {
	priority := VideoEncoderPriority()
	if len(priority) < 3 {
		t.Fatalf("expected at least 3 encoder candidates, got %d", len(priority))
	}
	if priority[0].name != "nvenc_h264" {
		t.Errorf("first priority should be nvenc_h264, got %s", priority[0].name)
	}
	if priority[2].name != "x264" {
		t.Errorf("third priority should prefer x264 software encoding, got %s", priority[2].name)
	}
	if priority[3].name != "vp8" {
		t.Errorf("fourth priority should fall back to vp8, got %s", priority[3].name)
	}
}

func TestBestVideoEncoderPrefersX264OverVP8WhenNoHardwareEncoderExists(t *testing.T) {
	name, gstElement := BestVideoEncoder(agentmgr.WebRTCCapabilitiesData{
		VideoEncoders: []string{"vp8", "x264"},
	})
	if name != "x264" || gstElement != "x264enc" {
		t.Fatalf("expected x264 software encoder to win over vp8, got %q / %q", name, gstElement)
	}
}

func TestBuildGStreamerVideoPipeline(t *testing.T) {
	pipeline := BuildGStreamerVideoPipeline(GstPipelineConfig{
		display: ":0",
		encoder: "x264enc",
		width:   1920,
		height:  1080,
		fps:     30,
		rtpPort: 5004,
	})
	if pipeline == "" {
		t.Fatal("expected non-empty pipeline string")
	}
	if !strings.Contains(pipeline, "x264enc") {
		t.Errorf("pipeline should contain x264enc, got: %s", pipeline)
	}
	if !strings.Contains(pipeline, "5004") {
		t.Errorf("pipeline should contain port 5004, got: %s", pipeline)
	}
}

func TestBuildWaylandPipeWireVideoPipeline(t *testing.T) {
	pipeline := BuildWaylandPipeWireVideoPipeline("42", GstPipelineConfig{
		encoder: "vp8enc",
		width:   1280,
		height:  720,
		fps:     60,
		rtpPort: 5004,
	})
	if !strings.Contains(pipeline, "pipewiresrc path=42") {
		t.Fatalf("expected pipewiresrc node id in %q", pipeline)
	}
	if !strings.Contains(pipeline, "rtpvp8pay") {
		t.Fatalf("expected vp8 payloader in %q", pipeline)
	}
}

func TestBuildGStreamerAudioPipeline(t *testing.T) {
	pipeline := BuildGStreamerAudioPipeline(GstAudioConfig{
		source:  "pulsesrc",
		rtpPort: 5006,
	})
	if pipeline == "" {
		t.Fatal("expected non-empty audio pipeline string")
	}
	if !strings.Contains(pipeline, "opusenc") {
		t.Errorf("audio pipeline should contain opusenc, got: %s", pipeline)
	}
}

func TestBuildGStreamerVideoPipelineEncoderVariantsAndDefaults(t *testing.T) {
	t.Run("nvenc", func(t *testing.T) {
		pipeline := BuildGStreamerVideoPipeline(GstPipelineConfig{
			display: ":1",
			encoder: "nvh264enc",
			bitrate: 7000,
			rtpPort: 5004,
		})
		if !strings.Contains(pipeline, "nvh264enc bitrate=7000 rc-mode=cbr") {
			t.Fatalf("unexpected nvenc pipeline %q", pipeline)
		}
		if !strings.Contains(pipeline, "rtph264pay") {
			t.Fatalf("expected H264 payloader in %q", pipeline)
		}
		if !strings.Contains(pipeline, "config-interval=-1") {
			t.Fatalf("expected H264 config-interval in %q", pipeline)
		}
	})

	t.Run("vaapi", func(t *testing.T) {
		pipeline := BuildGStreamerVideoPipeline(GstPipelineConfig{
			display: ":1",
			encoder: "vaapih264enc",
			bitrate: 6400,
			rtpPort: 5004,
		})
		if !strings.Contains(pipeline, "vaapih264enc bitrate=6400 rate-control=cbr") {
			t.Fatalf("unexpected vaapi pipeline %q", pipeline)
		}
	})

	t.Run("vp8", func(t *testing.T) {
		pipeline := BuildGStreamerVideoPipeline(GstPipelineConfig{
			display: ":1",
			encoder: "vp8enc",
			bitrate: 4500,
			rtpPort: 5004,
		})
		if !strings.Contains(pipeline, "vp8enc target-bitrate=4500000") {
			t.Fatalf("unexpected vp8 pipeline %q", pipeline)
		}
		if !strings.Contains(pipeline, "rtpvp8pay") {
			t.Fatalf("expected VP8 payloader in %q", pipeline)
		}
		if strings.Contains(pipeline, "config-interval=-1") {
			t.Fatalf("did not expect H264-only config-interval in VP8 pipeline %q", pipeline)
		}
	})

	t.Run("defaults", func(t *testing.T) {
		pipeline := BuildGStreamerVideoPipeline(GstPipelineConfig{rtpPort: 5004})
		if !strings.Contains(pipeline, "display-name=:0") {
			t.Fatalf("expected default display in %q", pipeline)
		}
		if !strings.Contains(pipeline, "width=1920,height=1080") {
			t.Fatalf("expected default dimensions in %q", pipeline)
		}
		if !strings.Contains(pipeline, "x264enc tune=zerolatency speed-preset=ultrafast bitrate=5000 key-int-max=30") {
			t.Fatalf("expected default x264 encoder in %q", pipeline)
		}
		if !strings.Contains(pipeline, "rtph264pay config-interval=-1 pt=96") {
			t.Fatalf("expected default H264 payloader config in %q", pipeline)
		}
	})
}

func TestDetectX11DisplayIdentifiers(t *testing.T) {
	originalDetectSession := DetectDesktopSessionFn
	t.Cleanup(func() {
		DetectDesktopSessionFn = originalDetectSession
	})
	DetectDesktopSessionFn = func() DesktopSessionInfo {
		return DesktopSessionInfo{Type: DesktopSessionTypeX11, Display: ":7"}
	}
	t.Setenv("LABTETHER_WEBRTC_X11_DISPLAY", " :1 ")
	t.Setenv("DISPLAY", "DP-1")

	displays := DetectX11DisplayIdentifiers()
	if len(displays) == 0 {
		t.Fatal("expected at least one X11 display identifier")
	}
	if displays[0] != ":1" {
		t.Fatalf("expected explicit override to win, got %q", displays[0])
	}
	for _, display := range displays {
		if display == "DP-1" {
			t.Fatalf("unexpected monitor-style display label in X11 identifiers: %q", display)
		}
	}
	if !containsString(displays, ":7") {
		t.Fatalf("expected detected session display to be included, got %v", displays)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestUnsupportedPlatformWebRTCReason(t *testing.T) {
	if got := UnsupportedPlatformWebRTCReason("darwin"); got != "unsupported_platform:darwin" {
		t.Fatalf("unexpected unsupported platform reason %q", got)
	}
	if got := UnsupportedPlatformWebRTCReason(""); got != "unsupported_platform:unknown" {
		t.Fatalf("expected unknown platform fallback, got %q", got)
	}
}

func TestDetectWebRTCCapabilitiesWaylandRequiresExperimentalBackend(t *testing.T) {
	originalGOOS := WebRTCRuntimeGOOS
	originalLookPath := WebRTCLookPath
	originalNewCommand := NewWebRTCSecurityCommand
	originalDetectSession := DetectDesktopSessionFn
	t.Cleanup(func() {
		WebRTCRuntimeGOOS = originalGOOS
		WebRTCLookPath = originalLookPath
		NewWebRTCSecurityCommand = originalNewCommand
		DetectDesktopSessionFn = originalDetectSession
	})

	WebRTCRuntimeGOOS = "linux"
	WebRTCLookPath = func(name string) (string, error) {
		return "/usr/bin/" + name, nil
	}
	NewWebRTCSecurityCommand = func(string, ...string) (*exec.Cmd, error) {
		return exec.Command("/bin/sh", "-c", "exit 0"), nil
	}
	DetectDesktopSessionFn = func() DesktopSessionInfo {
		return DesktopSessionInfo{Type: DesktopSessionTypeWayland, Backend: DesktopBackendWaylandPipeWire}
	}

	caps := DetectWebRTCCapabilitiesWithConfig(WebRTCConfig{})
	if caps.Available {
		t.Fatalf("expected unavailable capabilities for disabled Wayland backend, got %+v", caps)
	}
	if caps.UnavailableReason != webrtcReasonWaylandDisabled {
		t.Fatalf("reason=%q, want %q", caps.UnavailableReason, webrtcReasonWaylandDisabled)
	}
	if caps.VNCRealDesktopSupported {
		t.Fatalf("expected Wayland VNC real-desktop support to be false, got %+v", caps)
	}
}

func TestDetectWebRTCCapabilitiesWaylandWithPipeWireNode(t *testing.T) {
	originalGOOS := WebRTCRuntimeGOOS
	originalLookPath := WebRTCLookPath
	originalNewCommand := NewWebRTCSecurityCommand
	originalDetectSession := DetectDesktopSessionFn
	t.Cleanup(func() {
		WebRTCRuntimeGOOS = originalGOOS
		WebRTCLookPath = originalLookPath
		NewWebRTCSecurityCommand = originalNewCommand
		DetectDesktopSessionFn = originalDetectSession
	})

	WebRTCRuntimeGOOS = "linux"
	WebRTCLookPath = func(name string) (string, error) {
		return "/usr/bin/" + name, nil
	}
	NewWebRTCSecurityCommand = func(string, ...string) (*exec.Cmd, error) {
		return exec.Command("/bin/sh", "-c", "exit 0"), nil
	}
	DetectDesktopSessionFn = func() DesktopSessionInfo {
		return DesktopSessionInfo{Type: DesktopSessionTypeWayland, Backend: DesktopBackendWaylandPipeWire, UID: 1000}
	}

	caps := DetectWebRTCCapabilitiesWithConfig(WebRTCConfig{
		WaylandExperimentalEnabled: true,
		WaylandPipeWireNodeID:      "42",
	})
	if !caps.Available {
		t.Fatalf("expected Wayland capabilities to be available, got %+v", caps)
	}
	if caps.DesktopSessionType != DesktopSessionTypeWayland {
		t.Fatalf("session_type=%q, want %q", caps.DesktopSessionType, DesktopSessionTypeWayland)
	}
	if caps.CaptureBackend != "pipewiresrc" {
		t.Fatalf("capture_backend=%q, want pipewiresrc", caps.CaptureBackend)
	}
	if caps.VNCRealDesktopSupported {
		t.Fatalf("expected no real-desktop VNC support on Wayland, got %+v", caps)
	}
	if !caps.WebRTCRealDesktopSupported {
		t.Fatalf("expected real-desktop WebRTC support on Wayland, got %+v", caps)
	}
}

func TestBestAudioSourcePrefersPipewireThenPulse(t *testing.T) {
	if got := BestAudioSource(agentmgr.WebRTCCapabilitiesData{AudioSources: []string{"pulseaudio", "pipewire"}}); got != "pipewiresrc" {
		t.Fatalf("audio source=%q, want pipewiresrc", got)
	}
	if got := BestAudioSource(agentmgr.WebRTCCapabilitiesData{AudioSources: []string{"pulseaudio"}}); got != "pulsesrc" {
		t.Fatalf("audio source=%q, want pulsesrc", got)
	}
	if got := BestAudioSource(agentmgr.WebRTCCapabilitiesData{}); got != "" {
		t.Fatalf("audio source=%q, want empty fallback", got)
	}
}

func TestDetectWebRTCCapabilitiesMissingGstLaunch(t *testing.T) {
	originalGOOS := WebRTCRuntimeGOOS
	originalLookPath := WebRTCLookPath
	t.Cleanup(func() {
		WebRTCRuntimeGOOS = originalGOOS
		WebRTCLookPath = originalLookPath
	})

	WebRTCRuntimeGOOS = "linux"
	WebRTCLookPath = func(name string) (string, error) {
		if name == "gst-launch-1.0" {
			return "", errors.New("missing")
		}
		return "/usr/bin/" + name, nil
	}

	caps := DetectWebRTCCapabilities()
	if caps.Available {
		t.Fatalf("expected unavailable capabilities, got %+v", caps)
	}
	if caps.UnavailableReason != webrtcReasonMissingGstLaunch {
		t.Fatalf("reason=%q, want %q", caps.UnavailableReason, webrtcReasonMissingGstLaunch)
	}
}

func TestDetectWebRTCCapabilitiesMissingGstInspect(t *testing.T) {
	originalGOOS := WebRTCRuntimeGOOS
	originalLookPath := WebRTCLookPath
	t.Cleanup(func() {
		WebRTCRuntimeGOOS = originalGOOS
		WebRTCLookPath = originalLookPath
	})

	WebRTCRuntimeGOOS = "linux"
	WebRTCLookPath = func(name string) (string, error) {
		if name == "gst-inspect-1.0" {
			return "", errors.New("missing")
		}
		return "/usr/bin/" + name, nil
	}

	caps := DetectWebRTCCapabilities()
	if caps.Available {
		t.Fatalf("expected unavailable capabilities, got %+v", caps)
	}
	if caps.UnavailableReason != webrtcReasonMissingGstInspect {
		t.Fatalf("reason=%q, want %q", caps.UnavailableReason, webrtcReasonMissingGstInspect)
	}
}

func TestDetectWebRTCCapabilitiesNoSupportedEncoder(t *testing.T) {
	originalGOOS := WebRTCRuntimeGOOS
	originalLookPath := WebRTCLookPath
	originalNewCommand := NewWebRTCSecurityCommand
	t.Cleanup(func() {
		WebRTCRuntimeGOOS = originalGOOS
		WebRTCLookPath = originalLookPath
		NewWebRTCSecurityCommand = originalNewCommand
	})

	WebRTCRuntimeGOOS = "linux"
	WebRTCLookPath = func(name string) (string, error) {
		return "/usr/bin/" + name, nil
	}
	NewWebRTCSecurityCommand = func(string, ...string) (*exec.Cmd, error) {
		return exec.Command("/bin/sh", "-c", "exit 1"), nil
	}

	caps := DetectWebRTCCapabilities()
	if caps.Available {
		t.Fatalf("expected unavailable capabilities, got %+v", caps)
	}
	if caps.UnavailableReason != webrtcReasonNoVideoEncoder {
		t.Fatalf("reason=%q, want %q", caps.UnavailableReason, webrtcReasonNoVideoEncoder)
	}
}

func TestDetectWebRTCCapabilitiesCollectsEncodersAudioAndDisplays(t *testing.T) {
	originalGOOS := WebRTCRuntimeGOOS
	originalLookPath := WebRTCLookPath
	originalNewCommand := NewWebRTCSecurityCommand
	originalDetectSession := DetectDesktopSessionFn
	originalCollectUserSessions := system.CollectUserSessionsFn
	t.Cleanup(func() {
		WebRTCRuntimeGOOS = originalGOOS
		WebRTCLookPath = originalLookPath
		NewWebRTCSecurityCommand = originalNewCommand
		DetectDesktopSessionFn = originalDetectSession
		system.CollectUserSessionsFn = originalCollectUserSessions
	})

	// Mock session detection so CI environments (which may have loginctl
	// sessions or /proc-based display hints) don't inject extra displays.
	DetectDesktopSessionFn = func() DesktopSessionInfo {
		return DesktopSessionInfo{Type: DesktopSessionTypeHeadless, Backend: DesktopBackendHeadless}
	}
	system.CollectUserSessionsFn = func() ([]agentmgr.UserSession, error) { return nil, nil }

	t.Setenv("LABTETHER_WEBRTC_X11_DISPLAY", " :7 ")
	t.Setenv("DISPLAY", ":1")

	WebRTCRuntimeGOOS = "linux"
	WebRTCLookPath = func(name string) (string, error) {
		return "/usr/bin/" + name, nil
	}
	NewWebRTCSecurityCommand = func(_ string, args ...string) (*exec.Cmd, error) {
		if len(args) == 0 {
			t.Fatal("expected gst element argument")
		}
		switch args[0] {
		case "nvh264enc", "vp8enc", "pipewiresrc", "pulsesrc":
			return exec.Command("/bin/sh", "-c", "exit 0"), nil
		default:
			return exec.Command("/bin/sh", "-c", "exit 1"), nil
		}
	}

	caps := DetectWebRTCCapabilities()
	if !caps.Available {
		t.Fatalf("expected available capabilities, got %+v", caps)
	}
	if caps.UnavailableReason != "" {
		t.Fatalf("unexpected unavailable reason %q", caps.UnavailableReason)
	}
	if got := strings.Join(caps.VideoEncoders, ","); got != "nvenc_h264,vp8" {
		t.Fatalf("video encoders=%q, want nvenc_h264,vp8", got)
	}
	if got := strings.Join(caps.AudioSources, ","); got != "pipewire,pulseaudio" {
		t.Fatalf("audio sources=%q, want pipewire,pulseaudio", got)
	}
	if got := strings.Join(caps.Displays, ","); got != ":7,:1,:0" {
		t.Fatalf("displays=%q, want :7,:1,:0", got)
	}
}
