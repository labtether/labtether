package remoteaccess

import (
	"log"
	"os/exec"
	"strings"

	"github.com/labtether/labtether/internal/securityruntime"
)

var NewX11UtilityCommand = securityruntime.NewCommand

func PreferredX11Display() string {
	session := DetectDesktopSessionFn()
	if session.Type == DesktopSessionTypeX11 {
		if display := NormalizeX11DisplayIdentifier(session.Display); display != "" {
			return display
		}
	}

	if detected := AppendDetectedActiveDisplays(nil); len(detected) > 0 {
		return detected[0]
	}

	return ":0"
}

func WakeX11Display(display, xauthPath string) {
	display = NormalizeX11DisplayIdentifier(display)
	if display == "" {
		return
	}

	if strings.TrimSpace(xauthPath) == "" {
		xauthPath = DiscoverDisplayXAuthorityFn(display)
	}

	run := func(name string, args ...string) {
		cmd, err := NewX11UtilityCommand(name, args...)
		if err != nil {
			if _, ok := err.(*exec.Error); ok {
				return
			}
			log.Printf("desktop: unable to prepare %s wake command for %s: %v", name, display, err)
			return
		}
		cmd.Env = BuildX11ClientEnv(display, xauthPath)
		if output, runErr := cmd.CombinedOutput(); runErr != nil {
			log.Printf(
				"desktop: %s wake command failed for %s: %v (%s)",
				name,
				display,
				runErr,
				strings.TrimSpace(string(output)),
			)
		}
	}

	// Wake blanked/DPMS-suspended displays before attaching capture streams.
	run("xset", "-display", display, "dpms", "force", "on")
	run("xset", "-display", display, "s", "reset")

	// Nudge pointer state so locker/blanker stacks notice user activity.
	run("xdotool", "mousemove_relative", "--", "1", "0")
	run("xdotool", "mousemove_relative", "--", "-1", "0")
}
