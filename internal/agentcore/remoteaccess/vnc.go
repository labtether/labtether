package remoteaccess

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/securityruntime"
)

var LaunchDesktopVNCReady = LaunchX11VNCReady
var StartDesktopXvfb func(int, int, int) (*exec.Cmd, string, error) = StartXvfb
var FindDesktopFreeDisplay = FindFreeDisplay
var StartDesktopBootstrap = StartDesktopBootstrapShell

// startVNCServer launches a local VNC server and returns the VNC command,
// an optional Xvfb command and bootstrap shell (non-nil only when the headless
// fallback was used),
// the port the VNC server is listening on, and an optional x11vnc auth file path.
func StartVNCServer(display, quality, vncPassword string) (*exec.Cmd, *exec.Cmd, *exec.Cmd, int, string, error) {
	port, err := FindFreePort()
	if err != nil {
		return nil, nil, nil, 0, "", fmt.Errorf("failed to find free port: %w", err)
	}

	var cmd *exec.Cmd
	var xvfbCmd *exec.Cmd
	var bootstrapCmd *exec.Cmd
	var vncAuthFile string
	alreadyReady := false
	switch runtime.GOOS {
	case "linux", "freebsd":
		cmd, xvfbCmd, bootstrapCmd, vncAuthFile, err = StartLinuxVNCServer(display, port, quality, vncPassword)
		alreadyReady = true
	case "darwin":
		// macOS Screen Sharing uses port 5900 by default.
		// Check if it's already running, otherwise try to start it.
		cmd, port, err = StartMacVNC(port)
	case "windows":
		cmd, port, err = StartWindowsVNC(port)
	default:
		return nil, nil, nil, 0, "", fmt.Errorf("VNC not supported on %s", runtime.GOOS)
	}

	if err != nil {
		return nil, nil, nil, 0, "", err
	}

	// Wait for VNC server to be ready (Linux/FreeBSD path already verified).
	if !alreadyReady {
		if waitErr := WaitForVNC(port, 10*time.Second); waitErr != nil {
			TerminateProcess(cmd)
			TerminateProcess(xvfbCmd)
			TerminateProcess(bootstrapCmd)
			return nil, nil, nil, 0, "", fmt.Errorf("VNC server not ready: %w", waitErr)
		}
	}

	return cmd, xvfbCmd, bootstrapCmd, port, vncAuthFile, nil
}

func StartLinuxVNCServer(display string, port int, quality, vncPassword string) (*exec.Cmd, *exec.Cmd, *exec.Cmd, string, error) {
	requestedDisplay := strings.TrimSpace(display)
	if requestedDisplay != "" && NormalizeX11DisplayIdentifier(requestedDisplay) == "" {
		log.Printf("desktop: ignoring non-X11 display selection %q; defaulting to primary X display", requestedDisplay)
		display = ""
	}
	if strings.TrimSpace(display) == "" {
		session := DetectDesktopSessionFn()
		if session.Type == DesktopSessionTypeWayland {
			return nil, nil, nil, "", fmt.Errorf("real desktop VNC is unsupported on Wayland; use WebRTC for the active desktop session")
		}
		display = PreferredX11Display()
	}

	// When no display is explicitly requested, check whether :0 has a live X server
	// before attempting to use it. On headless hosts a stale socket may exist without
	// a lock file, causing x11vnc to "succeed" on an empty framebuffer and preventing
	// the Xvfb fallback from triggering.
	skipFirstAttempt := false
	effectiveDisplay := strings.TrimSpace(display)
	if effectiveDisplay == "" {
		effectiveDisplay = PreferredX11Display()
	}
	if !IsDisplayAvailable(effectiveDisplay) {
		log.Printf("desktop: no usable X server on %s, going straight to Xvfb", effectiveDisplay)
		skipFirstAttempt = true
	}

	var firstErr error
	if !skipFirstAttempt {
		discoveredXAuth := DiscoverDisplayXAuthorityFn(FirstLaunchDisplay(display))
		WakeX11Display(FirstLaunchDisplay(display), discoveredXAuth)
		cmd, firstAuthPath, firstLogTail, err := LaunchDesktopVNCReady(display, port, quality, vncPassword, discoveredXAuth, 10*time.Second)
		if err == nil {
			return cmd, nil, nil, firstAuthPath, nil
		}
		firstErr = FormatVNCStartupError(err, firstLogTail)
		if !IsDisplayStartupError(firstErr, firstLogTail) {
			return nil, nil, nil, "", firstErr
		}
		log.Printf("desktop: display %s not available, trying Xvfb headless fallback", FirstLaunchDisplay(display))
	}

	xvfbDisplay := FindDesktopFreeDisplay()
	log.Printf("desktop: starting Xvfb on display :%d for session", xvfbDisplay)
	xvfbCmd, xauthPath, xvfbErr := StartDesktopXvfb(xvfbDisplay, 1920, 1080)
	if xvfbErr != nil {
		if firstErr != nil {
			return nil, nil, nil, "", fmt.Errorf("%w (Xvfb fallback failed: %v)", firstErr, xvfbErr)
		}
		return nil, nil, nil, "", fmt.Errorf("Xvfb headless startup failed: %w", xvfbErr)
	}
	if xauthPath != "" {
		log.Printf("desktop: Xvfb Xauthority file created at %s", xauthPath)
	}

	fallbackDisplay := fmt.Sprintf(":%d", xvfbDisplay)
	bootstrapCmd, bootstrapErr := StartDesktopBootstrap(fallbackDisplay, xauthPath)
	if bootstrapErr != nil {
		log.Printf("desktop: warning: fallback desktop bootstrap unavailable on %s: %v", fallbackDisplay, bootstrapErr)
		SetXvfbFallbackBackground(fallbackDisplay, xauthPath)
	}
	cmd, fallbackAuthPath, fallbackLogTail, fallbackErr := LaunchDesktopVNCReady(fallbackDisplay, port, quality, vncPassword, xauthPath, 10*time.Second)
	if fallbackErr != nil {
		TerminateProcess(xvfbCmd)
		TerminateProcess(bootstrapCmd)
		RemoveProcessLog(xauthPath)
		if firstErr != nil {
			return nil, nil, nil, "", fmt.Errorf("%w (Xvfb fallback failed on %s: %v)", firstErr, fallbackDisplay, FormatVNCStartupError(fallbackErr, fallbackLogTail))
		}
		return nil, nil, nil, "", fmt.Errorf("Xvfb fallback on %s failed: %w", fallbackDisplay, FormatVNCStartupError(fallbackErr, fallbackLogTail))
	}
	return cmd, xvfbCmd, bootstrapCmd, fallbackAuthPath, nil
}

func FirstLaunchDisplay(display string) string {
	display = strings.TrimSpace(display)
	if display == "" {
		return ":0"
	}
	return display
}

func StartDesktopBootstrapShell(display, xauthPath string) (*exec.Cmd, error) {
	display = strings.TrimSpace(display)
	if display == "" {
		return nil, nil
	}

	xtermPath, err := exec.LookPath("xterm")
	if err != nil {
		return nil, fmt.Errorf("xterm not found: install with 'apt install xterm'")
	}

	cmd, err := securityruntime.NewCommand(
		xtermPath,
		"-fa", "Monospace",
		"-fs", "11",
		"-geometry", "120x36+40+40",
		"-title", "LabTether Desktop",
		"-e", "sh", "-lc",
		`printf '\033]0;LabTether Desktop\007'; printf 'LabTether headless desktop fallback\n\n'; printf 'This host does not have a live graphical display, so LabTether started Xvfb and opened this shell.\n'; printf 'Launch a desktop environment or GUI app here if you want a richer remote desktop.\n\n'; if [ -n "$SHELL" ] && [ -x "$SHELL" ]; then exec "$SHELL" -l; fi; if command -v bash >/dev/null 2>&1; then exec bash -l; fi; exec /bin/sh -l`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build xterm bootstrap command: %w", err)
	}
	cmd.Env = BuildX11ClientEnv(display, xauthPath)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start xterm bootstrap: %w", err)
	}
	return cmd, nil
}

// buildX11ClientEnv builds an environment for X11 clients connecting to the
// requested display. It always sets DISPLAY and only sets XAUTHORITY when a
// real auth file is available for the display.
func BuildX11ClientEnv(display, xauthPath string) []string {
	env := os.Environ()
	filtered := make([]string, 0, len(env)+1)
	for _, e := range env {
		if strings.HasPrefix(e, "XAUTHORITY=") || strings.HasPrefix(e, "DISPLAY=") {
			continue
		}
		filtered = append(filtered, e)
	}
	filtered = append(filtered, "DISPLAY="+display)
	xauthPath = strings.TrimSpace(xauthPath)
	if xauthPath == "" {
		xauthPath = DiscoverDisplayXAuthorityFn(display)
	}
	if xauthPath != "" && xauthPath != "none" {
		filtered = append(filtered, "XAUTHORITY="+xauthPath)
	}
	return filtered
}

// setXvfbFallbackBackground sets a dark-grey root window background on the Xvfb display
// so the user sees something other than pure black when the bootstrap shell isn't available.
func SetXvfbFallbackBackground(display, xauthPath string) {
	xsetroot, err := exec.LookPath("xsetroot")
	if err != nil {
		return
	}
	cmd, cmdErr := securityruntime.NewCommand(xsetroot, "-solid", "#1a1a2e", "-display", display)
	if cmdErr != nil {
		return
	}
	cmd.Env = BuildX11ClientEnv(display, xauthPath)
	_ = cmd.Run()
}

func LaunchX11VNCReady(display string, port int, quality, vncPassword, xauthPath string, timeout time.Duration) (*exec.Cmd, string, string, error) {
	cmd, logPath, authPath, err := StartX11VNC(display, port, quality, vncPassword, xauthPath)
	if err != nil {
		RemoveProcessLog(logPath)
		RemoveProcessLog(authPath)
		return nil, "", "", err
	}
	if waitErr := WaitForVNC(port, timeout); waitErr != nil {
		logTail := ReadProcessLogTail(logPath, 4096)
		TerminateProcess(cmd)
		RemoveProcessLog(logPath)
		RemoveProcessLog(authPath)
		return nil, "", logTail, fmt.Errorf("VNC server not ready: %w", waitErr)
	}
	// Log initial x11vnc output for debugging.
	if tail := ReadProcessLogTail(logPath, 2048); tail != "" {
		log.Printf("desktop: x11vnc startup log for %s port %d: %s", display, port, strings.TrimSpace(tail))
	}
	RemoveProcessLog(logPath)
	return cmd, authPath, "", nil
}

func FormatVNCStartupError(baseErr error, logTail string) error {
	if baseErr == nil {
		return nil
	}
	summary := SummarizeProcessLogTail(logTail)
	if summary == "" {
		return baseErr
	}
	return fmt.Errorf("%w (%s)", baseErr, summary)
}

func SummarizeProcessLogTail(logTail string) string {
	logTail = strings.TrimSpace(logTail)
	if logTail == "" {
		return ""
	}
	logTail = strings.ReplaceAll(logTail, "\r\n", "\n")
	logTail = strings.ReplaceAll(logTail, "\r", "\n")
	lines := strings.Split(logTail, "\n")
	nonEmpty := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		nonEmpty = append(nonEmpty, line)
	}
	if len(nonEmpty) == 0 {
		return ""
	}
	if len(nonEmpty) > 3 {
		nonEmpty = nonEmpty[len(nonEmpty)-3:]
	}
	joined := strings.Join(nonEmpty, " | ")
	if len(joined) > 320 {
		joined = joined[len(joined)-320:]
	}
	return "x11vnc log tail: " + joined
}

func IsDisplayStartupError(err error, logTail string) bool {
	if err != nil && IsDisplayError(err) {
		return true
	}
	if strings.TrimSpace(logTail) == "" {
		return false
	}
	return IsDisplayError(fmt.Errorf("%s", logTail))
}

// startX11VNC launches x11vnc for Linux/FreeBSD.
func StartX11VNC(display string, port int, quality, vncPassword, xauthPath string) (*exec.Cmd, string, string, error) {
	x11vncPath, err := exec.LookPath("x11vnc")
	if err != nil {
		return nil, "", "", fmt.Errorf("x11vnc not found: install with 'apt install x11vnc' or 'pkg install x11vnc'")
	}

	authPath, err := CreateX11VNCPasswordFile(x11vncPath, vncPassword)
	if err != nil {
		return nil, "", "", err
	}
	args := BuildX11VNCArgs(display, port, quality, authPath, xauthPath)

	cmd, err := securityruntime.NewCommand(x11vncPath, args...)
	if err != nil {
		RemoveProcessLog(authPath)
		return nil, "", "", fmt.Errorf("failed to build x11vnc command: %w", err)
	}

	logFile, err := os.CreateTemp("", "labtether-x11vnc-*.log")
	if err != nil {
		RemoveProcessLog(authPath)
		return nil, "", "", fmt.Errorf("failed to create x11vnc log file: %w", err)
	}
	logPath := logFile.Name()
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		RemoveProcessLog(logPath)
		RemoveProcessLog(authPath)
		return nil, "", "", fmt.Errorf("failed to start x11vnc: %w", err)
	}
	if closeErr := logFile.Close(); closeErr != nil {
		log.Printf("desktop: warning: failed to close x11vnc log handle: %v", closeErr)
	}

	return cmd, logPath, authPath, nil
}

func CreateX11VNCPasswordFile(x11vncPath, password string) (string, error) {
	password = strings.TrimSpace(password)
	if password == "" {
		return "", nil
	}

	file, err := os.CreateTemp("", "labtether-x11vnc-pass-*.rfbauth")
	if err != nil {
		return "", fmt.Errorf("failed to create x11vnc password file path: %w", err)
	}
	path := file.Name()
	if closeErr := file.Close(); closeErr != nil {
		RemoveProcessLog(path)
		return "", fmt.Errorf("failed to prepare x11vnc password file path: %w", closeErr)
	}

	storeCmd, err := securityruntime.NewCommand(x11vncPath, "-storepasswd", password, path)
	if err != nil {
		RemoveProcessLog(path)
		return "", fmt.Errorf("failed to build x11vnc password command: %w", err)
	}
	if output, runErr := storeCmd.CombinedOutput(); runErr != nil {
		RemoveProcessLog(path)
		if summary := strings.TrimSpace(string(output)); summary != "" {
			return "", fmt.Errorf("failed to create x11vnc password file: %w (%s)", runErr, SummarizeProcessLogTail(summary))
		}
		return "", fmt.Errorf("failed to create x11vnc password file: %w", runErr)
	}
	if chmodErr := os.Chmod(path, 0o600); chmodErr != nil { // #nosec G703 -- Path is a package-created temp password file.
		RemoveProcessLog(path)
		return "", fmt.Errorf("failed to secure x11vnc password file permissions: %w", chmodErr)
	}
	return path, nil
}

func BuildX11VNCArgs(display string, port int, quality, rfbAuthPath, xauthPath string) []string {
	display = strings.TrimSpace(display)
	if display == "" {
		display = ":0"
	}

	// x11vnc expects "-speeds" presets for bandwidth tuning.
	// Previous "-quality"/"-compress" flags are not portable across x11vnc builds
	// and can cause startup failure.
	speeds := "dsl"
	switch strings.ToLower(strings.TrimSpace(quality)) {
	case "low":
		speeds = "modem"
	case "high":
		speeds = "lan"
	}

	args := []string{
		"-display", display,
		"-rfbport", fmt.Sprintf("%d", port),
	}
	if strings.TrimSpace(rfbAuthPath) != "" {
		args = append(args, "-rfbauth", rfbAuthPath)
	} else {
		args = append(args, "-nopw")
	}

	args = append(args,
		"-forever",
		"-shared",
		"-localhost",
	)

	// Use explicit Xauthority path when available (from Xvfb startup).
	// When xauthPath is empty, fall back to "-auth guess" for real displays.
	// When xauthPath is "none", the Xvfb was started without auth (xauth
	// binary unavailable), so omit -auth entirely — x11vnc connects without it.
	if strings.TrimSpace(xauthPath) == "none" {
		// No auth needed — Xvfb was started without -auth.
	} else if strings.TrimSpace(xauthPath) != "" {
		args = append(args, "-auth", xauthPath)
	} else {
		args = append(args, "-auth", "guess")
	}

	args = append(args,
		"-speeds", speeds,
	)
	return args
}

// buildXvfbArgs constructs arguments for launching Xvfb.
func BuildXvfbArgs(display, width, height int) []string {
	return []string{
		fmt.Sprintf(":%d", display),
		"-screen", "0",
		fmt.Sprintf("%dx%dx24", width, height),
	}
}

// startXvfb launches Xvfb on the given display number and returns the command
// and the path to the generated Xauthority file (empty if xauth is unavailable).
func StartXvfb(display, width, height int) (*exec.Cmd, string, error) {
	xvfbPath, err := exec.LookPath("Xvfb")
	if err != nil {
		return nil, "", fmt.Errorf("xvfb not found: install with 'apt install xvfb'")
	}

	// Best-effort: generate an Xauthority file for the display.
	// When xauth is unavailable, return "none" as a sentinel so downstream
	// callers (x11vnc) know to omit -auth rather than using -auth guess.
	xauthPath, xauthErr := CreateXauthorityFile(display)
	if xauthErr != nil {
		log.Printf("desktop: warning: could not create Xauthority for :%d: %v (proceeding without auth)", display, xauthErr)
		xauthPath = "none"
	}

	args := BuildXvfbArgs(display, width, height)
	if xauthPath != "" && xauthPath != "none" {
		args = append(args, "-auth", xauthPath)
	}

	cmd, err := securityruntime.NewCommand(xvfbPath, args...)
	if err != nil {
		RemoveProcessLog(xauthPath)
		return nil, "", fmt.Errorf("failed to build Xvfb command: %w", err)
	}
	if err := cmd.Start(); err != nil {
		RemoveProcessLog(xauthPath)
		return nil, "", fmt.Errorf("failed to start Xvfb: %w", err)
	}

	// Wait for Xvfb to create its lock file instead of a fixed sleep.
	if readyErr := WaitForXvfbReady(display, 5*time.Second); readyErr != nil {
		log.Printf("desktop: warning: Xvfb readiness probe failed for :%d: %v (continuing anyway)", display, readyErr)
	}
	return cmd, xauthPath, nil
}

// generateHexCookie returns a random hex string of n characters.
// Falls back to a timestamp-based value if crypto/rand fails.
func GenerateHexCookie(n int) string {
	b := make([]byte, n/2)
	if _, err := rand.Read(b); err != nil {
		// Fallback: use current UnixNano timestamp repeated to fill.
		ts := fmt.Sprintf("%x", time.Now().UnixNano())
		for len(ts) < n {
			ts += ts
		}
		return ts[:n]
	}
	return hex.EncodeToString(b)
}

// createXauthorityFile generates an Xauthority file for the given display
// using xauth. Returns the file path (caller is responsible for cleanup)
// or an error if xauth is not available.
func CreateXauthorityFile(displayNum int) (string, error) {
	xauthBin, err := exec.LookPath("xauth")
	if err != nil {
		return "", fmt.Errorf("xauth not found: %w", err)
	}

	f, err := os.CreateTemp("", fmt.Sprintf("labtether-xauth-%d-*.xauth", displayNum))
	if err != nil {
		return "", fmt.Errorf("failed to create Xauthority temp file: %w", err)
	}
	path := f.Name()
	if closeErr := f.Close(); closeErr != nil {
		RemoveProcessLog(path)
		return "", fmt.Errorf("failed to prepare Xauthority file: %w", closeErr)
	}

	cookie := GenerateHexCookie(32)
	cmd, err := securityruntime.NewCommand(xauthBin, "-f", path, "add", fmt.Sprintf(":%d", displayNum), ".", cookie)
	if err != nil {
		RemoveProcessLog(path)
		return "", fmt.Errorf("failed to build xauth command: %w", err)
	}
	if output, runErr := cmd.CombinedOutput(); runErr != nil {
		RemoveProcessLog(path)
		return "", fmt.Errorf("xauth add failed: %w (%s)", runErr, strings.TrimSpace(string(output)))
	}

	if chmodErr := os.Chmod(path, 0o600); chmodErr != nil { // #nosec G703 -- Path is a package-created Xauthority file.
		RemoveProcessLog(path)
		return "", fmt.Errorf("failed to secure Xauthority file permissions: %w", chmodErr)
	}
	return path, nil
}

// waitForXvfbReady polls for the Xvfb lock file to appear, indicating the
// server has initialized. Returns nil when found, or a timeout error.
func WaitForXvfbReady(display int, timeout time.Duration) error {
	lockFile := fmt.Sprintf("/tmp/.X%d-lock", display)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(lockFile); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for Xvfb lock file %s", lockFile)
}

// isDisplayError checks if the error is related to X11 display connectivity.
func IsDisplayError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "cannot open display") ||
		strings.Contains(msg, "unable to connect to x server") ||
		strings.Contains(msg, "xopendisplay") ||
		strings.Contains(msg, "no display") ||
		strings.Contains(msg, "display variable")
}

// startMacVNC checks for macOS Screen Sharing VNC or returns an error.
func StartMacVNC(defaultPort int) (*exec.Cmd, int, error) {
	// macOS Screen Sharing listens on port 5900.
	conn, err := net.DialTimeout("tcp", "127.0.0.1:5900", 2*time.Second)
	if err != nil {
		return nil, 0, fmt.Errorf("macOS Screen Sharing is not enabled. " +
			"Enable it in System Settings > General > Sharing > Screen Sharing, " +
			"or run: sudo \"/System/Library/CoreServices/RemoteManagement/ARDAgent.app/Contents/Resources/kickstart\" " +
			"-activate -configure -access -on -users $USER -privs -all -restart -agent")
	}
	_ = conn.Close()

	// Port is open — check that the current user has control (not just observe) privileges.
	if hasControl, checkErr := CheckMacARDControl(); checkErr != nil {
		log.Printf("desktop: could not verify ARD control privileges: %v", checkErr)
	} else if !hasControl {
		return nil, 0, fmt.Errorf("macOS Screen Sharing is enabled but only in observe-only mode. " +
			"Remote mouse and keyboard input will not work. Grant full control with: " +
			"sudo \"/System/Library/CoreServices/RemoteManagement/ARDAgent.app/Contents/Resources/kickstart\" " +
			"-configure -access -on -users $USER -privs -all -restart -agent")
	}

	return nil, 5900, nil
}

// checkMacARDControl checks whether the current user has ARD control privileges
// (not just observe). It reads the naprivs attribute via dscl — if bit 1 is set,
// the user can send mouse/keyboard input over VNC. Returns (true, nil) if control
// is granted, (false, nil) if observe-only, or (false, err) if detection failed.
func CheckMacARDControl() (bool, error) {
	user := os.Getenv("USER")
	if user == "" {
		return false, fmt.Errorf("USER environment variable not set")
	}
	if !IsSafeLocalUsername(user) {
		return false, fmt.Errorf("USER contains unsupported characters")
	}

	// #nosec G702 -- command and args are fixed; USER is validated by IsSafeLocalUsername.
	out, err := securityruntime.CommandOutput("dscl", ".", "-read", "/Users/"+user, "dsAttrTypeNative:naprivs")
	if err != nil {
		// No naprivs key means no ARD privileges configured for this user.
		// This could mean "all local users" mode is active — check that.
		allOut, allErr := securityruntime.CommandOutput(
			"defaults", "read",
			"/Library/Preferences/com.apple.RemoteManagement", "ARD_AllLocalUsers",
		)
		if allErr == nil && strings.TrimSpace(string(allOut)) == "1" {
			return true, nil // All local users have access.
		}
		return false, nil
	}

	// Parse "dsAttrTypeNative:naprivs: <value>" or "dsAttrTypeNative:naprivs:\n <value>"
	raw := strings.TrimSpace(string(out))
	parts := strings.SplitN(raw, ":", 3)
	if len(parts) < 3 {
		return false, fmt.Errorf("unexpected dscl output: %s", raw)
	}
	valStr := strings.TrimSpace(parts[2])

	privs, err := strconv.ParseInt(valStr, 10, 64)
	if err != nil {
		return false, fmt.Errorf("failed to parse naprivs value %q: %w", valStr, err)
	}

	// Bit 1 (0x2) = Control and Observe. Without this, VNC is observe-only.
	const ardControlBit = 0x2
	return privs&ardControlBit != 0, nil
}

func IsSafeLocalUsername(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		switch r {
		case '_', '-', '.':
			continue
		default:
			return false
		}
	}
	return true
}

// startWindowsVNC probes for common Windows VNC servers.
func StartWindowsVNC(defaultPort int) (*exec.Cmd, int, error) {
	// Check if a VNC server is already running.
	conn, err := net.DialTimeout("tcp", "127.0.0.1:5900", 2*time.Second)
	if err == nil {
		_ = conn.Close()
		return nil, 5900, nil
	}

	// Try known VNC server executables.
	for _, name := range []string{"tvnserver", "vncserver", "winvnc4"} {
		path, lookErr := exec.LookPath(name)
		if lookErr == nil {
			cmd, cmdErr := securityruntime.NewCommand(path, "-run")
			if cmdErr != nil {
				continue
			}
			if startErr := cmd.Start(); startErr == nil {
				return cmd, 5900, nil
			}
		}
	}

	return nil, 0, fmt.Errorf("no VNC server found on Windows. Install TightVNC, TigerVNC, or UltraVNC")
}

// findFreeDisplay finds an available X11 display number by checking for
// Xvfb lock files. It scans from 99 down to 90 and returns the first
// display whose lock file does not exist.
func FindFreeDisplay() int {
	for d := 99; d >= 90; d-- {
		lockFile := fmt.Sprintf("/tmp/.X%d-lock", d)
		if _, err := os.Stat(lockFile); os.IsNotExist(err) {
			return d
		}
	}
	return 99 // fallback
}

// findFreePort finds an available TCP port on localhost.
func FindFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port, nil
}

// waitForVNC polls for TCP connectivity on the given port until timeout.
func WaitForVNC(port int, timeout time.Duration) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for VNC on port %d", port)
}

func ReadProcessLogTail(path string, maxBytes int) string {
	if strings.TrimSpace(path) == "" || maxBytes <= 0 {
		return ""
	}
	data, err := os.ReadFile(path) // #nosec G304 -- Path is a bounded local runtime file selected by the VNC helper.
	if err != nil {
		return ""
	}
	if len(data) > maxBytes {
		data = data[len(data)-maxBytes:]
	}
	return string(data)
}

func RemoveProcessLog(path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	_ = os.Remove(path) // #nosec G703 -- Path is a package-managed temp file.
}
