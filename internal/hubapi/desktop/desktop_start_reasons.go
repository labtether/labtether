package desktop

import (
	"strings"
	"unicode"
)

// Words for a Remote View that could not start on the device. Every agent
// reason used to reach the viewer as "agent desktop session closed
// unexpectedly", so the console kept retrying and a person never learnt that
// the device was missing x11vnc or Xvfb. Each answer fits a WebSocket close
// frame (123 bytes) and starts with "Remote View", which the console treats
// as final instead of retrying.
const (
	remoteViewNeedsX11VNC     = "Remote View needs x11vnc on this device. Install it, then try again: sudo apt install x11vnc"
	remoteViewNeedsXvfb       = "Remote View needs Xvfb on this device, which has no screen of its own: sudo apt install xvfb"
	remoteViewNeedsXauth      = "Remote View needs xauth on this device. Install it, then try again: sudo apt install xauth"
	remoteViewWayland         = "Remote View cannot show this Wayland desktop over VNC. Log in to an X11 session on the device."
	remoteViewSessionLimit    = "Remote View is already open on this device as often as it allows. Close one and try again."
	remoteViewStartTimedOut   = "Remote View did not start on this device within 15 seconds. Try again."
	remoteViewCouldNotStart   = "Remote View could not start on this device"
	remoteViewAgentWentAway   = "Remote View stopped: the device's agent disconnected. Try again when it is back."
	remoteViewStartNotSent    = "Remote View could not reach the device's agent. Try again."
	remoteViewOpenedElsewhere = "Remote View was opened again elsewhere, so this view closed."
	maxAgentReasonInViewText  = 75
)

// desktopClosedBeforeReady says why a Remote View closed before it started:
// the agent's reason, another viewer taking the session, or the agent leaving.
func desktopClosedBeforeReady(agentReason string, agentConnected bool) string {
	switch {
	case strings.TrimSpace(agentReason) == "session attached elsewhere":
		return remoteViewOpenedElsewhere
	case strings.TrimSpace(agentReason) == "" && !agentConnected:
		return remoteViewAgentWentAway
	}
	return desktopStartFailureReason(agentReason)
}

// desktopStartFailureReason turns the agent's reason for a Remote View that
// closed before it started into words for the person who opened it.
func desktopStartFailureReason(agentReason string) string {
	lower := strings.ToLower(agentReason)
	switch {
	case strings.Contains(lower, "x11vnc not found"):
		return remoteViewNeedsX11VNC
	case strings.Contains(lower, "xvfb not found"):
		return remoteViewNeedsXvfb
	case strings.Contains(lower, "xauth not found"):
		return remoteViewNeedsXauth
	case strings.Contains(lower, "wayland"):
		return remoteViewWayland
	case strings.Contains(lower, "max desktop sessions"):
		return remoteViewSessionLimit
	}
	detail := plainAgentReason(agentReason)
	if detail == "" {
		return remoteViewCouldNotStart + "."
	}
	return remoteViewCouldNotStart + ": " + detail
}

// plainAgentReason keeps the agent's own words short and printable, without
// the "failed to start VNC:" prefix every reason carries.
func plainAgentReason(reason string) string {
	reason = strings.TrimSpace(reason)
	for _, prefix := range []string{"failed to start VNC:", "failed to connect to VNC:"} {
		if strings.HasPrefix(strings.ToLower(reason), strings.ToLower(prefix)) {
			reason = strings.TrimSpace(reason[len(prefix):])
		}
	}
	reason = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, reason)
	reason = strings.Join(strings.Fields(reason), " ")
	if runes := []rune(reason); len(runes) > maxAgentReasonInViewText {
		reason = strings.TrimSpace(string(runes[:maxAgentReasonInViewText])) + "…"
	}
	return reason
}
