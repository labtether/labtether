//go:build linux

package agentcore

import (
	"github.com/labtether/labtether/internal/agentcore/sysconfig"
)

func init() {
	// Wire clipboard's desktop session detector so xclip/xsel can find the
	// correct DISPLAY and XAUTHORITY for the active graphical session.
	// Without this, clipboard operations fall back to DISPLAY=:0 which may
	// not match the actual active display.
	sysconfig.DetectLinuxDesktopSessionFn = func() sysconfig.LinuxDesktopSessionInfo {
		session := detectDesktopSessionFn()
		return sysconfig.LinuxDesktopSessionInfo{
			Type:           session.Type,
			Display:        session.Display,
			WaylandDisplay: session.WaylandDisplay,
			XDGRuntimeDir:  session.XDGRuntimeDir,
		}
	}
}
