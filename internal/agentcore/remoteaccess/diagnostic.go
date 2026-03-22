package remoteaccess

import (
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/labtether/labtether/internal/agentmgr"
)

var strictLocalX11DisplayPattern = regexp.MustCompile(`^:[0-9]+(?:\.[0-9]+)?$`)

// CollectDesktopDiagnostic gathers a snapshot of the desktop stack state on the agent.
// Both deskMgr and webrtcMgr may be nil (e.g. on platforms without desktop support).
func CollectDesktopDiagnostic(deskMgr *DesktopManager, webrtcMgr *WebRTCManager) agentmgr.DesktopDiagnosticData {
	var d agentmgr.DesktopDiagnosticData
	session := DetectDesktopSessionFn()
	d.DesktopSessionType = session.Type
	d.DesktopBackend = session.Backend
	d.DesktopUser = session.Username
	d.RealDisplay = session.Display

	// --- X11 environment ---
	d.EnvDisplay = os.Getenv("DISPLAY")

	d.ActiveDisplays = AppendDetectedActiveDisplays(d.ActiveDisplays)

	// --- Xvfb state ---
	if out, err := exec.Command("pgrep", "-a", "Xvfb").Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			d.XvfbRunning = true
			// "pid args…" — first field is the PID.
			fields := strings.Fields(line)
			if len(fields) > 0 {
				if pid, err := strconv.Atoi(fields[0]); err == nil {
					d.XvfbPIDs = append(d.XvfbPIDs, pid)
				}
			}
			// Extract the display identifier from the command-line args.
			for _, f := range fields[1:] {
				if strings.HasPrefix(f, ":") {
					d.XvfbDisplays = append(d.XvfbDisplays, f)
					break
				}
			}
		}
	}

	// --- x11vnc state ---
	if out, err := exec.Command("pgrep", "-a", "x11vnc").Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			d.X11VNCRunning = true
			fields := strings.Fields(line)
			// Parse -display and -rfbport from x11vnc args.
			for i, f := range fields {
				switch f {
				case "-display":
					if i+1 < len(fields) {
						d.X11VNCDisplay = fields[i+1]
					}
				case "-rfbport":
					if i+1 < len(fields) {
						if port, err := strconv.Atoi(fields[i+1]); err == nil {
							d.X11VNCPort = port
						}
					}
				}
			}
		}
	}

	// --- Bootstrap shell ---
	if out, err := exec.Command("pgrep", "-f", "LabTether Desktop").Output(); err == nil {
		if strings.TrimSpace(string(out)) != "" {
			d.BootstrapRunning = true
		}
	}

	// --- xterm availability ---
	if _, err := exec.LookPath("xterm"); err == nil {
		d.XtermAvailable = true
	}

	// --- GStreamer tool availability ---
	if _, err := exec.LookPath("gst-launch-1.0"); err == nil {
		d.GstLaunchAvailable = true
	}
	if _, err := exec.LookPath("gst-inspect-1.0"); err == nil {
		d.GstInspectAvailable = true
	}

	// --- WebRTC capabilities ---
	caps := DetectWebRTCCapabilities()
	if webrtcMgr != nil {
		caps = webrtcMgr.caps
	}
	d.VideoEncoders = caps.VideoEncoders
	d.AudioSources = caps.AudioSources
	d.WebRTCAvailable = caps.Available
	d.WebRTCReason = caps.UnavailableReason
	d.VNCRealDesktopSupported = caps.VNCRealDesktopSupported
	d.WebRTCRealDesktopSupported = caps.WebRTCRealDesktopSupported
	d.CaptureBackend = caps.CaptureBackend

	// --- Active session counts ---
	if deskMgr != nil {
		deskMgr.Mu.Lock()
		d.ActiveVNCSessions = len(deskMgr.Sessions)
		deskMgr.Mu.Unlock()
	}
	if webrtcMgr != nil {
		webrtcMgr.Mu.Lock()
		d.ActiveWebRTCSessions = len(webrtcMgr.Sessions)
		webrtcMgr.Mu.Unlock()
	}

	// --- Framebuffer content check ---
	display := d.EnvDisplay
	if display == "" && len(d.XvfbDisplays) > 0 {
		display = d.XvfbDisplays[0]
	}
	display = NormalizeX11DisplayIdentifier(display)
	if display != "" && !strictLocalX11DisplayPattern.MatchString(display) {
		d.FramebufferError = "unsupported X11 display identifier"
	}
	if display != "" && strictLocalX11DisplayPattern.MatchString(display) {
		out, err := exec.Command("xdpyinfo", "-display", display).Output() // #nosec G204,G702 -- Invokes a fixed local diagnostic binary with a normalized, strictly local X11 display identifier.
		if err != nil {
			d.FramebufferError = err.Error()
		} else if strings.Contains(string(out), "dimensions:") {
			d.FramebufferHasContent = true
		}
	}

	return d
}

// handleDesktopDiagnose handles a MsgDesktopDiagnose request from the hub.
func HandleDesktopDiagnose(transport MessageSender, msg agentmgr.Message, deskMgr *DesktopManager, webrtcMgr *WebRTCManager) {
	var req agentmgr.DesktopDiagnosticRequest
	_ = json.Unmarshal(msg.Data, &req)

	diag := CollectDesktopDiagnostic(deskMgr, webrtcMgr)
	diag.RequestID = req.RequestID

	data, _ := json.Marshal(diag)
	_ = transport.Send(agentmgr.Message{
		Type: agentmgr.MsgDesktopDiagnosed,
		ID:   req.RequestID,
		Data: data,
	})
}
