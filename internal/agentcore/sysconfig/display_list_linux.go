//go:build linux

package sysconfig

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/securityruntime"
)

// DesktopSessionType and DesktopSessionDetector are injectable hooks for
// desktop session detection that lives in the remoteaccess subpackage (or
// still in root during incremental extraction). The parent agentcore package
// wires these at init time.
var (
	DetectDesktopSessionTypeFn func() string                   // returns session type string
	PreferredX11DisplayFn      func() string                   // returns preferred X11 DISPLAY value
	WakeX11DisplayFn           func(display, xauthPath string) // wakes the display
	BuildX11ClientEnvFn        func(display, xauthPath string) []string
	NewDisplayListCommand      = securityruntime.NewCommandContext
)

// DesktopSessionTypeWayland is the session type constant for Wayland sessions.
const DesktopSessionTypeWayland = "wayland"

func PlatformListDisplays() ([]agentmgr.DisplayInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if DetectDesktopSessionTypeFn != nil && DetectDesktopSessionTypeFn() == DesktopSessionTypeWayland {
		log.Printf("display: skipping xrandr monitor enumeration for Wayland desktop backend")
		return nil, nil
	}

	display := ":0"
	if PreferredX11DisplayFn != nil {
		display = PreferredX11DisplayFn()
	}
	if WakeX11DisplayFn != nil {
		WakeX11DisplayFn(display, "")
	}

	cmd, err := NewDisplayListCommand(ctx, "xrandr", "--listmonitors")
	if err != nil {
		log.Printf("display: failed to build xrandr command for %s: %v", display, err)
		return nil, err
	}
	if BuildX11ClientEnvFn != nil {
		cmd.Env = BuildX11ClientEnvFn(display, "")
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("display: xrandr --listmonitors failed on %s: %v (%s)", display, err, strings.TrimSpace(string(out)))
		return nil, err
	}
	return ParseXrandrMonitors(string(out)), nil
}
