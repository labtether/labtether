package remoteaccess

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/securityruntime"
)

const (
	DesktopSessionTypeUnknown  = "unknown"
	DesktopSessionTypeHeadless = "headless"
	DesktopSessionTypeX11      = "x11"
	DesktopSessionTypeWayland  = "wayland"

	DesktopBackendHeadless        = "headless"
	DesktopBackendX11             = "x11"
	DesktopBackendWaylandPipeWire = "wayland_pipewire"
)

type DesktopSessionInfo struct {
	SessionID      string
	UID            int
	Username       string
	Type           string
	State          string
	Class          string
	Display        string
	Remote         bool
	Active         bool
	WaylandDisplay string
	XDGRuntimeDir  string
	Backend        string
}

var DetectDesktopSessionFn = DetectDesktopSession

func DetectDesktopSession() DesktopSessionInfo {
	if runtime.GOOS != "linux" {
		return DesktopSessionInfo{
			Type:    DesktopSessionTypeUnknown,
			Backend: DesktopBackendHeadless,
		}
	}

	session, ok := DetectLinuxDesktopSessionViaLogind()
	if ok {
		return FinalizeDesktopSession(session)
	}

	session = DesktopSessionInfo{
		Type:    DesktopSessionTypeHeadless,
		Backend: DesktopBackendHeadless,
	}
	if display := NormalizeX11DisplayIdentifier(os.Getenv("DISPLAY")); display != "" {
		session.Type = DesktopSessionTypeX11
		session.Display = display
	}
	if waylandDisplay := strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY")); waylandDisplay != "" {
		session.Type = DesktopSessionTypeWayland
		session.WaylandDisplay = waylandDisplay
	}
	if runtimeDir := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR")); runtimeDir != "" {
		session.XDGRuntimeDir = runtimeDir
	}
	return FinalizeDesktopSession(session)
}

func DetectLinuxDesktopSessionViaLogind() (DesktopSessionInfo, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := securityruntime.CommandContextCombinedOutput(ctx, "loginctl", "list-sessions", "--no-legend")
	if err != nil {
		return DesktopSessionInfo{}, false
	}

	var best DesktopSessionInfo
	bestScore := -1
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		fields := strings.Fields(strings.TrimSpace(scanner.Text()))
		if len(fields) == 0 {
			continue
		}
		sessionID := strings.TrimSpace(fields[0])
		if sessionID == "" {
			continue
		}
		session, ok := ReadLogindSession(ctx, sessionID)
		if !ok {
			continue
		}
		score := SessionPreferenceScore(session)
		if score > bestScore {
			best = session
			bestScore = score
		}
	}
	if bestScore < 0 {
		return DesktopSessionInfo{}, false
	}
	return best, true
}

func ReadLogindSession(ctx context.Context, sessionID string) (DesktopSessionInfo, bool) {
	out, err := securityruntime.CommandContextCombinedOutput(
		ctx,
		"loginctl",
		"show-session",
		sessionID,
		"-p", "Id",
		"-p", "Name",
		"-p", "User",
		"-p", "Type",
		"-p", "Class",
		"-p", "State",
		"-p", "Active",
		"-p", "Remote",
		"-p", "Display",
		"-p", "Service",
	)
	if err != nil {
		return DesktopSessionInfo{}, false
	}

	session := DesktopSessionInfo{
		SessionID: sessionID,
		Type:      DesktopSessionTypeUnknown,
	}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "Id":
			if value != "" {
				session.SessionID = value
			}
		case "Name":
			session.Username = value
		case "User":
			if uid, convErr := strconv.Atoi(value); convErr == nil {
				session.UID = uid
			}
		case "Type":
			session.Type = strings.ToLower(value)
		case "Class":
			session.Class = strings.ToLower(value)
		case "State":
			session.State = strings.ToLower(value)
		case "Active":
			session.Active = strings.EqualFold(value, "yes")
		case "Remote":
			session.Remote = strings.EqualFold(value, "yes")
		case "Display":
			session.Display = NormalizeX11DisplayIdentifier(value)
		case "Service":
			if strings.Contains(strings.ToLower(value), "wayland") && session.Type == DesktopSessionTypeUnknown {
				session.Type = DesktopSessionTypeWayland
			}
		}
	}

	if session.UID > 0 {
		session.XDGRuntimeDir = "/run/user/" + strconv.Itoa(session.UID)
	}
	if session.Type == DesktopSessionTypeUnknown {
		if session.Display != "" {
			session.Type = DesktopSessionTypeX11
		} else {
			session.Type = DesktopSessionTypeHeadless
		}
	}
	return session, true
}

func SessionPreferenceScore(session DesktopSessionInfo) int {
	score := 0
	switch session.Type {
	case DesktopSessionTypeWayland:
		score += 200
	case DesktopSessionTypeX11:
		score += 180
	default:
		score += 20
	}
	if session.Active {
		score += 50
	}
	if !session.Remote {
		score += 25
	}
	if session.State == "active" || session.State == "online" {
		score += 20
	}
	switch session.Class {
	case "user":
		score += 20
	case "greeter":
		score -= 20
	}
	if session.Display != "" {
		score += 10
	}
	return score
}

func FinalizeDesktopSession(session DesktopSessionInfo) DesktopSessionInfo {
	session.Type = strings.TrimSpace(strings.ToLower(session.Type))
	switch session.Type {
	case DesktopSessionTypeWayland:
		session.WaylandDisplay = strings.TrimSpace(session.WaylandDisplay)
		session.Backend = DesktopBackendWaylandPipeWire
	case DesktopSessionTypeX11:
		session.Display = NormalizeX11DisplayIdentifier(session.Display)
		session.Backend = DesktopBackendX11
	default:
		if session.Display != "" {
			session.Type = DesktopSessionTypeX11
			session.Backend = DesktopBackendX11
			break
		}
		session.Type = DesktopSessionTypeHeadless
		session.Backend = DesktopBackendHeadless
	}
	return session
}
