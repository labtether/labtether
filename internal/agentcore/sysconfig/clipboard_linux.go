//go:build linux

package sysconfig

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/labtether/labtether/internal/securityruntime"
)

// LinuxDesktopSessionInfo holds the minimal desktop session info needed by
// clipboard operations. The parent package maps its full session info into this.
type LinuxDesktopSessionInfo struct {
	Type           string
	Display        string
	WaylandDisplay string
	XDGRuntimeDir  string
}

// Injectable function variables for desktop session detection.
// The parent agentcore package wires these at init time.
var (
	DetectLinuxDesktopSessionFn    func() LinuxDesktopSessionInfo
	AppendDetectedActiveDisplaysFn func(dst []string) []string
)

var ClipboardLookPath = exec.LookPath
var ClipboardNewCommand = securityruntime.NewCommand

func buildLinuxClipboardEnv() []string {
	if DetectLinuxDesktopSessionFn == nil {
		return buildX11ClipboardEnv(":0", "")
	}
	session := DetectLinuxDesktopSessionFn()
	if session.Type == DesktopSessionTypeWayland {
		env := os.Environ()
		filtered := make([]string, 0, len(env)+2)
		for _, entry := range env {
			if strings.HasPrefix(entry, "XDG_RUNTIME_DIR=") || strings.HasPrefix(entry, "WAYLAND_DISPLAY=") {
				continue
			}
			filtered = append(filtered, entry)
		}
		if runtimeDir := strings.TrimSpace(session.XDGRuntimeDir); runtimeDir != "" {
			filtered = append(filtered, "XDG_RUNTIME_DIR="+runtimeDir)
		}
		if waylandDisplay := strings.TrimSpace(session.WaylandDisplay); waylandDisplay != "" {
			filtered = append(filtered, "WAYLAND_DISPLAY="+waylandDisplay)
		}
		return filtered
	}

	display := session.Display
	if display == "" {
		if AppendDetectedActiveDisplaysFn != nil {
			if detected := AppendDetectedActiveDisplaysFn(nil); len(detected) > 0 {
				display = detected[0]
			}
		}
	}
	if display == "" {
		display = ":0"
	}
	return buildX11ClipboardEnv(display, "")
}

func buildX11ClipboardEnv(display, xauthPath string) []string {
	if BuildX11ClientEnvFn != nil {
		return BuildX11ClientEnvFn(display, xauthPath)
	}
	// Fallback: minimal X11 env
	env := os.Environ()
	result := make([]string, 0, len(env)+2)
	for _, entry := range env {
		if strings.HasPrefix(entry, "DISPLAY=") || strings.HasPrefix(entry, "XAUTHORITY=") {
			continue
		}
		result = append(result, entry)
	}
	result = append(result, "DISPLAY="+display)
	if xauthPath != "" {
		result = append(result, "XAUTHORITY="+xauthPath)
	}
	return result
}

func clipboardCommandOutput(name string, args ...string) ([]byte, error) {
	cmd, err := ClipboardNewCommand(name, args...)
	if err != nil {
		return nil, err
	}
	cmd.Env = buildLinuxClipboardEnv()
	return cmd.Output()
}

// PlatformClipboardRead reads the Linux clipboard using xclip or xsel.
func PlatformClipboardRead(format string) (text string, imgBase64 string, err error) {
	if format == "image" || format == "image/png" {
		return "", "", fmt.Errorf("image clipboard read not yet supported on Linux")
	}

	// Try xclip first, fall back to xsel.
	if _, lookErr := ClipboardLookPath("xclip"); lookErr == nil {
		out, err := clipboardCommandOutput("xclip", "-selection", "clipboard", "-o")
		if err != nil {
			return "", "", fmt.Errorf("xclip failed: %w", err)
		}
		return strings.TrimRight(string(out), "\n"), "", nil
	}

	if _, lookErr := ClipboardLookPath("xsel"); lookErr == nil {
		out, err := clipboardCommandOutput("xsel", "--clipboard", "--output")
		if err != nil {
			return "", "", fmt.Errorf("xsel failed: %w", err)
		}
		return strings.TrimRight(string(out), "\n"), "", nil
	}

	return "", "", fmt.Errorf("clipboard read requires xclip or xsel to be installed")
}

// PlatformClipboardWriteText writes text to the Linux clipboard using xclip or xsel.
func PlatformClipboardWriteText(text string) error {
	// Try xclip first, fall back to xsel.
	if _, lookErr := ClipboardLookPath("xclip"); lookErr == nil {
		cmd, err := ClipboardNewCommand("xclip", "-selection", "clipboard")
		if err != nil {
			return fmt.Errorf("failed to create xclip command: %w", err)
		}
		cmd.Env = buildLinuxClipboardEnv()
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("xclip failed: %w", err)
		}
		return nil
	}

	if _, lookErr := ClipboardLookPath("xsel"); lookErr == nil {
		cmd, err := ClipboardNewCommand("xsel", "--clipboard", "--input")
		if err != nil {
			return fmt.Errorf("failed to create xsel command: %w", err)
		}
		cmd.Env = buildLinuxClipboardEnv()
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("xsel failed: %w", err)
		}
		return nil
	}

	return fmt.Errorf("clipboard write requires xclip or xsel to be installed")
}

// PlatformClipboardWriteImage writes an image to the Linux clipboard.
// Not yet supported — returns an error.
func PlatformClipboardWriteImage(base64Data string) error {
	return fmt.Errorf("image clipboard write not yet supported on Linux")
}
