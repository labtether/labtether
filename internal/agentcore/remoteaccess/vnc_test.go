package remoteaccess

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentcore/system"
	"github.com/labtether/labtether/internal/agentmgr"
)

func TestBuildX11VNCArgsDefaults(t *testing.T) {
	args := BuildX11VNCArgs("", 5901, "", "", "")

	requireArgPair(t, args, "-display", ":0")
	requireArgPair(t, args, "-rfbport", "5901")
	requireArgPair(t, args, "-auth", "guess")
	requireArgPair(t, args, "-speeds", "dsl")

	requireNoStandaloneArg(t, args, "-quality")
	requireNoStandaloneArg(t, args, "-compress")
	requireNoStandaloneArg(t, args, "-rfbauth")
}

func TestBuildX11VNCArgsQualityMapping(t *testing.T) {
	tests := []struct {
		name    string
		quality string
		speeds  string
	}{
		{name: "low", quality: "low", speeds: "modem"},
		{name: "medium", quality: "medium", speeds: "dsl"},
		{name: "high", quality: "high", speeds: "lan"},
		{name: "unknown-default", quality: "custom", speeds: "dsl"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := BuildX11VNCArgs(":2", 5909, tc.quality, "", "")
			requireArgPair(t, args, "-display", ":2")
			requireArgPair(t, args, "-rfbport", "5909")
			requireArgPair(t, args, "-speeds", tc.speeds)
		})
	}
}

func TestBuildX11VNCArgsUsesAuthFileWhenPresent(t *testing.T) {
	authPath := "/tmp/labtether-x11vnc-auth.rfbauth"
	args := BuildX11VNCArgs(":1", 5902, "medium", authPath, "")

	requireArgPair(t, args, "-rfbauth", authPath)
	requireNoStandaloneArg(t, args, "-nopw")
}

func TestBuildX11VNCArgsUsesXauthPathWhenPresent(t *testing.T) {
	xauthPath := "/tmp/labtether-xauth-99-abc.xauth"
	args := BuildX11VNCArgs(":99", 5903, "", "", xauthPath)

	requireArgPair(t, args, "-auth", xauthPath)
}

func TestBuildX11VNCArgsFallsBackToGuessWithoutXauth(t *testing.T) {
	args := BuildX11VNCArgs(":0", 5901, "", "", "")
	requireArgPair(t, args, "-auth", "guess")
}

func TestBuildX11VNCArgsOmitsAuthWhenNone(t *testing.T) {
	args := BuildX11VNCArgs(":99", 5901, "", "", "none")
	requireNoStandaloneArg(t, args, "-auth")
}

func TestBuildX11ClientEnvIncludesDisplayAndXauthority(t *testing.T) {
	originalDiscover := DiscoverDisplayXAuthorityFn
	t.Cleanup(func() {
		DiscoverDisplayXAuthorityFn = originalDiscover
	})
	DiscoverDisplayXAuthorityFn = func(string) string { return "" }

	t.Setenv("DISPLAY", ":0")
	t.Setenv("XAUTHORITY", "/tmp/original.xauth")

	env := BuildX11ClientEnv(":99", "/tmp/labtether-99.xauth")
	if !ContainsEnvValue(env, "DISPLAY=:99") {
		t.Fatalf("expected DISPLAY override, got %v", env)
	}
	if !ContainsEnvValue(env, "XAUTHORITY=/tmp/labtether-99.xauth") {
		t.Fatalf("expected XAUTHORITY override, got %v", env)
	}
	if ContainsEnvValue(env, "XAUTHORITY=/tmp/original.xauth") {
		t.Fatalf("expected stale XAUTHORITY to be removed, got %v", env)
	}
}

func TestBuildX11ClientEnvOmitsXauthorityWhenUnavailable(t *testing.T) {
	originalDiscover := DiscoverDisplayXAuthorityFn
	t.Cleanup(func() {
		DiscoverDisplayXAuthorityFn = originalDiscover
	})
	DiscoverDisplayXAuthorityFn = func(string) string { return "" }

	t.Setenv("DISPLAY", ":0")
	t.Setenv("XAUTHORITY", "/tmp/original.xauth")

	for _, xauthPath := range []string{"", "none"} {
		env := BuildX11ClientEnv(":98", xauthPath)
		if !ContainsEnvValue(env, "DISPLAY=:98") {
			t.Fatalf("expected DISPLAY override for %q, got %v", xauthPath, env)
		}
		for _, entry := range env {
			if strings.HasPrefix(entry, "XAUTHORITY=") {
				t.Fatalf("expected no XAUTHORITY for %q, got %v", xauthPath, env)
			}
		}
	}
}

func TestStartLinuxVNCServerRejectsWaylandRealDesktopFallback(t *testing.T) {
	originalDetectSession := DetectDesktopSessionFn
	t.Cleanup(func() {
		DetectDesktopSessionFn = originalDetectSession
	})
	DetectDesktopSessionFn = func() DesktopSessionInfo {
		return DesktopSessionInfo{Type: DesktopSessionTypeWayland, Backend: DesktopBackendWaylandPipeWire}
	}

	_, _, _, _, err := StartLinuxVNCServer("", 5901, "", "")
	if err == nil {
		t.Fatal("expected Wayland VNC start to be rejected")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unsupported on wayland") {
		t.Fatalf("error=%q, want unsupported on wayland", err)
	}
}

func TestBuildX11ClientEnvDiscoversRealDisplayXauthority(t *testing.T) {
	originalDiscover := DiscoverDisplayXAuthorityFn
	t.Cleanup(func() {
		DiscoverDisplayXAuthorityFn = originalDiscover
	})
	DiscoverDisplayXAuthorityFn = func(display string) string {
		if display != ":0" {
			t.Fatalf("display=%q, want :0", display)
		}
		return "/run/lightdm/root/:0"
	}

	env := BuildX11ClientEnv(":0", "")
	if !ContainsEnvValue(env, "DISPLAY=:0") {
		t.Fatalf("expected DISPLAY override, got %v", env)
	}
	if !ContainsEnvValue(env, "XAUTHORITY=/run/lightdm/root/:0") {
		t.Fatalf("expected discovered XAUTHORITY, got %v", env)
	}
}

func TestBuildXvfbArgs(t *testing.T) {
	args := BuildXvfbArgs(99, 1920, 1080)
	expected := []string{":99", "-screen", "0", "1920x1080x24"}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("BuildXvfbArgs: got %v, want %v", args, expected)
	}
}

func TestIsDisplayError(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"cannot open display :0", true},
		{"unable to connect to X server", true},
		{"XOpenDisplay failed", true},
		{"no DISPLAY set", true},
		{"DISPLAY variable not set", true},
		{"VNC server not ready: timeout waiting for VNC on port 5901", false},
		{"failed to start x11vnc: exit status 1", false},
	}
	for _, tt := range tests {
		got := IsDisplayError(fmt.Errorf("%s", tt.msg))
		if got != tt.want {
			t.Errorf("IsDisplayError(%q) = %v, want %v", tt.msg, got, tt.want)
		}
	}
}

func TestSummarizeProcessLogTail(t *testing.T) {
	logTail := "\nline-1\nline-2\nline-3\nline-4\n"
	summary := SummarizeProcessLogTail(logTail)
	if summary == "" {
		t.Fatalf("expected non-empty summary")
	}
	if summary != "x11vnc log tail: line-2 | line-3 | line-4" {
		t.Fatalf("unexpected summary: %q", summary)
	}
}

func TestStartLinuxVNCServerUsesExistingDisplayWhenAvailable(t *testing.T) {
	originalLaunch := LaunchDesktopVNCReady
	originalStartXvfb := StartDesktopXvfb
	originalFind := FindDesktopFreeDisplay
	originalBootstrap := StartDesktopBootstrap
	originalCollectUserSessions := system.CollectUserSessionsFn
	originalDiscover := DiscoverDisplayXAuthorityFn
	t.Cleanup(func() {
		LaunchDesktopVNCReady = originalLaunch
		StartDesktopXvfb = originalStartXvfb
		FindDesktopFreeDisplay = originalFind
		StartDesktopBootstrap = originalBootstrap
		system.CollectUserSessionsFn = originalCollectUserSessions
		DiscoverDisplayXAuthorityFn = originalDiscover
	})
	system.CollectUserSessionsFn = func() ([]agentmgr.UserSession, error) { return nil, nil }
	DiscoverDisplayXAuthorityFn = func(display string) string {
		if display != ":0" {
			t.Fatalf("display=%q, want :0", display)
		}
		return "/run/lightdm/root/:0"
	}

	// Create a lock file so hasUsableDisplay(:0) returns true.
	lockFile := "/tmp/.X0-lock"
	if err := os.WriteFile(lockFile, []byte("99999\n"), 0o644); err != nil {
		t.Fatalf("failed to create mock lock file: %v", err)
	}
	defer os.Remove(lockFile)

	primaryCmd := &exec.Cmd{}
	launchCalls := 0
	LaunchDesktopVNCReady = func(display string, port int, quality, vncPassword, xauthPath string, timeout time.Duration) (*exec.Cmd, string, string, error) {
		launchCalls++
		if display != ":0" || port != 5901 || quality != "high" || vncPassword != "secret" {
			t.Fatalf("unexpected launch args display=%q port=%d quality=%q password=%q", display, port, quality, vncPassword)
		}
		if xauthPath != "/run/lightdm/root/:0" {
			t.Fatalf("xauthPath=%q, want /run/lightdm/root/:0", xauthPath)
		}
		return primaryCmd, "/tmp/auth-primary", "", nil
	}
	StartDesktopXvfb = func(int, int, int) (*exec.Cmd, string, error) {
		t.Fatal("did not expect Xvfb fallback for healthy display")
		return nil, "", nil
	}
	StartDesktopBootstrap = func(string, string) (*exec.Cmd, error) {
		t.Fatal("did not expect fallback bootstrap for healthy display")
		return nil, nil
	}

	cmd, xvfbCmd, bootstrapCmd, authPath, err := StartLinuxVNCServer(":0", 5901, "high", "secret")
	if err != nil {
		t.Fatalf("StartLinuxVNCServer returned error: %v", err)
	}
	if cmd != primaryCmd {
		t.Fatalf("cmd=%p, want %p", cmd, primaryCmd)
	}
	if xvfbCmd != nil {
		t.Fatalf("expected no Xvfb command, got %v", xvfbCmd)
	}
	if bootstrapCmd != nil {
		t.Fatalf("expected no bootstrap command, got %v", bootstrapCmd)
	}
	if authPath != "/tmp/auth-primary" {
		t.Fatalf("authPath=%q, want /tmp/auth-primary", authPath)
	}
	if launchCalls != 1 {
		t.Fatalf("launchCalls=%d, want 1", launchCalls)
	}
}

func TestStartLinuxVNCServerIgnoresNonX11DisplaySelection(t *testing.T) {
	originalLaunch := LaunchDesktopVNCReady
	originalStartXvfb := StartDesktopXvfb
	originalFind := FindDesktopFreeDisplay
	originalBootstrap := StartDesktopBootstrap
	originalCollectUserSessions := system.CollectUserSessionsFn
	t.Cleanup(func() {
		LaunchDesktopVNCReady = originalLaunch
		StartDesktopXvfb = originalStartXvfb
		FindDesktopFreeDisplay = originalFind
		StartDesktopBootstrap = originalBootstrap
		system.CollectUserSessionsFn = originalCollectUserSessions
	})
	system.CollectUserSessionsFn = func() ([]agentmgr.UserSession, error) { return nil, nil }

	lockFile := "/tmp/.X0-lock"
	if err := os.WriteFile(lockFile, []byte("99999\n"), 0o644); err != nil {
		t.Fatalf("failed to create mock lock file: %v", err)
	}
	defer os.Remove(lockFile)

	primaryCmd := &exec.Cmd{}
	LaunchDesktopVNCReady = func(display string, port int, quality, vncPassword, xauthPath string, timeout time.Duration) (*exec.Cmd, string, string, error) {
		if display != ":0" {
			t.Fatalf("expected invalid monitor label to resolve to preferred X display, got %q", display)
		}
		return primaryCmd, "/tmp/auth-primary", "", nil
	}
	StartDesktopXvfb = func(int, int, int) (*exec.Cmd, string, error) {
		t.Fatal("did not expect Xvfb fallback when :0 is available")
		return nil, "", nil
	}
	StartDesktopBootstrap = func(string, string) (*exec.Cmd, error) {
		t.Fatal("did not expect fallback bootstrap when :0 is available")
		return nil, nil
	}

	cmd, xvfbCmd, bootstrapCmd, authPath, err := StartLinuxVNCServer("DP-1", 5901, "medium", "")
	if err != nil {
		t.Fatalf("StartLinuxVNCServer returned error: %v", err)
	}
	if cmd != primaryCmd {
		t.Fatalf("cmd=%p, want %p", cmd, primaryCmd)
	}
	if xvfbCmd != nil {
		t.Fatalf("expected no Xvfb command, got %v", xvfbCmd)
	}
	if bootstrapCmd != nil {
		t.Fatalf("expected no bootstrap command, got %v", bootstrapCmd)
	}
	if authPath != "/tmp/auth-primary" {
		t.Fatalf("authPath=%q, want /tmp/auth-primary", authPath)
	}
}

func TestStartLinuxVNCServerFallsBackToXvfbOnDisplayError(t *testing.T) {
	originalLaunch := LaunchDesktopVNCReady
	originalStartXvfb := StartDesktopXvfb
	originalFind := FindDesktopFreeDisplay
	originalBootstrap := StartDesktopBootstrap
	originalCollectUserSessions := system.CollectUserSessionsFn
	t.Cleanup(func() {
		LaunchDesktopVNCReady = originalLaunch
		StartDesktopXvfb = originalStartXvfb
		FindDesktopFreeDisplay = originalFind
		StartDesktopBootstrap = originalBootstrap
		system.CollectUserSessionsFn = originalCollectUserSessions
	})
	system.CollectUserSessionsFn = func() ([]agentmgr.UserSession, error) { return nil, nil }

	// Create lock file so display :0 is considered usable (first attempt runs).
	lockFile := "/tmp/.X0-lock"
	if err := os.WriteFile(lockFile, []byte("99999\n"), 0o644); err != nil {
		t.Fatalf("failed to create mock lock file: %v", err)
	}
	defer os.Remove(lockFile)

	primaryErr := errors.New("cannot open display :0")
	fallbackCmd := &exec.Cmd{}
	xvfbCmd := &exec.Cmd{}
	bootstrapCmd := &exec.Cmd{}
	launchCalls := 0

	LaunchDesktopVNCReady = func(display string, port int, quality, vncPassword, xauthPath string, timeout time.Duration) (*exec.Cmd, string, string, error) {
		launchCalls++
		switch launchCalls {
		case 1:
			if display != ":0" {
				t.Fatalf("first display=%q, want :0", display)
			}
			return nil, "", "", primaryErr
		case 2:
			if display != ":97" {
				t.Fatalf("fallback display=%q, want :97", display)
			}
			return fallbackCmd, "/tmp/auth-fallback", "", nil
		default:
			t.Fatalf("unexpected launch call %d", launchCalls)
			return nil, "", "", nil
		}
	}
	FindDesktopFreeDisplay = func() int { return 97 }
	StartDesktopXvfb = func(display, width, height int) (*exec.Cmd, string, error) {
		if display != 97 || width != 1920 || height != 1080 {
			t.Fatalf("unexpected Xvfb args display=%d width=%d height=%d", display, width, height)
		}
		return xvfbCmd, "", nil
	}
	StartDesktopBootstrap = func(display, xauthPath string) (*exec.Cmd, error) {
		if display != ":97" {
			t.Fatalf("unexpected bootstrap display=%q", display)
		}
		if xauthPath != "" {
			t.Fatalf("expected empty xauth path, got %q", xauthPath)
		}
		return bootstrapCmd, nil
	}

	cmd, gotXvfb, gotBootstrap, authPath, err := StartLinuxVNCServer(":0", 5901, "medium", "")
	if err != nil {
		t.Fatalf("StartLinuxVNCServer returned error: %v", err)
	}
	if cmd != fallbackCmd {
		t.Fatalf("cmd=%p, want %p", cmd, fallbackCmd)
	}
	if gotXvfb != xvfbCmd {
		t.Fatalf("xvfbCmd=%p, want %p", gotXvfb, xvfbCmd)
	}
	if gotBootstrap != bootstrapCmd {
		t.Fatalf("bootstrapCmd=%p, want %p", gotBootstrap, bootstrapCmd)
	}
	if authPath != "/tmp/auth-fallback" {
		t.Fatalf("authPath=%q, want /tmp/auth-fallback", authPath)
	}
	if launchCalls != 2 {
		t.Fatalf("launchCalls=%d, want 2", launchCalls)
	}
}

func TestStartLinuxVNCServerSkipsToXvfbOnHeadless(t *testing.T) {
	originalLaunch := LaunchDesktopVNCReady
	originalStartXvfb := StartDesktopXvfb
	originalFind := FindDesktopFreeDisplay
	originalBootstrap := StartDesktopBootstrap
	originalCollectUserSessions := system.CollectUserSessionsFn
	t.Cleanup(func() {
		LaunchDesktopVNCReady = originalLaunch
		StartDesktopXvfb = originalStartXvfb
		FindDesktopFreeDisplay = originalFind
		StartDesktopBootstrap = originalBootstrap
		system.CollectUserSessionsFn = originalCollectUserSessions
	})
	system.CollectUserSessionsFn = func() ([]agentmgr.UserSession, error) { return nil, nil }

	// No lock file for :0 → headless, should skip first attempt entirely.
	os.Remove("/tmp/.X0-lock")

	fallbackCmd := &exec.Cmd{}
	xvfbCmd := &exec.Cmd{}
	bootstrapCmd := &exec.Cmd{}
	launchCalls := 0

	LaunchDesktopVNCReady = func(display string, port int, quality, vncPassword, xauthPath string, timeout time.Duration) (*exec.Cmd, string, string, error) {
		launchCalls++
		if launchCalls == 1 {
			// Should only be called once (for the Xvfb display), not for :0.
			if display != ":96" {
				t.Fatalf("expected Xvfb display :96, got %q", display)
			}
			return fallbackCmd, "/tmp/auth-xvfb", "", nil
		}
		t.Fatalf("unexpected launch call %d", launchCalls)
		return nil, "", "", nil
	}
	FindDesktopFreeDisplay = func() int { return 96 }
	StartDesktopXvfb = func(display, width, height int) (*exec.Cmd, string, error) {
		if display != 96 {
			t.Fatalf("unexpected Xvfb display=%d", display)
		}
		return xvfbCmd, "/tmp/xauth-96.xauth", nil
	}
	StartDesktopBootstrap = func(display, xauthPath string) (*exec.Cmd, error) {
		if xauthPath != "/tmp/xauth-96.xauth" {
			t.Fatalf("unexpected bootstrap xauth=%q", xauthPath)
		}
		return bootstrapCmd, nil
	}

	cmd, gotXvfb, gotBootstrap, authPath, err := StartLinuxVNCServer("", 5901, "medium", "pw")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd != fallbackCmd {
		t.Fatal("expected Xvfb fallback VNC cmd")
	}
	if gotXvfb != xvfbCmd {
		t.Fatal("expected Xvfb cmd")
	}
	if gotBootstrap != bootstrapCmd {
		t.Fatal("expected bootstrap cmd")
	}
	if authPath != "/tmp/auth-xvfb" {
		t.Fatalf("authPath=%q, want /tmp/auth-xvfb", authPath)
	}
	if launchCalls != 1 {
		t.Fatalf("launchCalls=%d, want 1 (only Xvfb display)", launchCalls)
	}
}

func TestStartLinuxVNCServerReturnsPrimaryErrorWhenDisplayIsHealthyButStartupFails(t *testing.T) {
	originalLaunch := LaunchDesktopVNCReady
	originalStartXvfb := StartDesktopXvfb
	originalFind := FindDesktopFreeDisplay
	originalBootstrap := StartDesktopBootstrap
	originalCollectUserSessions := system.CollectUserSessionsFn
	t.Cleanup(func() {
		LaunchDesktopVNCReady = originalLaunch
		StartDesktopXvfb = originalStartXvfb
		FindDesktopFreeDisplay = originalFind
		StartDesktopBootstrap = originalBootstrap
		system.CollectUserSessionsFn = originalCollectUserSessions
	})
	system.CollectUserSessionsFn = func() ([]agentmgr.UserSession, error) { return nil, nil }

	// Create lock file so :0 is considered usable (triggers first attempt).
	lockFile := "/tmp/.X0-lock"
	if err := os.WriteFile(lockFile, []byte("99999\n"), 0o644); err != nil {
		t.Fatalf("failed to create mock lock file: %v", err)
	}
	defer os.Remove(lockFile)

	LaunchDesktopVNCReady = func(display string, port int, quality, vncPassword, xauthPath string, timeout time.Duration) (*exec.Cmd, string, string, error) {
		return nil, "", "", errors.New("x11vnc not found")
	}
	StartDesktopXvfb = func(int, int, int) (*exec.Cmd, string, error) {
		t.Fatal("did not expect Xvfb fallback for non-display startup error")
		return nil, "", nil
	}
	StartDesktopBootstrap = func(string, string) (*exec.Cmd, error) {
		t.Fatal("did not expect fallback bootstrap for non-display startup error")
		return nil, nil
	}

	_, _, _, _, err := StartLinuxVNCServer(":0", 5901, "medium", "")
	if err == nil {
		t.Fatal("expected startup error")
	}
	if !strings.Contains(err.Error(), "x11vnc not found") {
		t.Fatalf("error=%q, want x11vnc not found", err)
	}
}

func TestStartLinuxVNCServerBootstrapFailureStillStartsVNC(t *testing.T) {
	originalLaunch := LaunchDesktopVNCReady
	originalStartXvfb := StartDesktopXvfb
	originalFind := FindDesktopFreeDisplay
	originalBootstrap := StartDesktopBootstrap
	originalCollectUserSessions := system.CollectUserSessionsFn
	t.Cleanup(func() {
		LaunchDesktopVNCReady = originalLaunch
		StartDesktopXvfb = originalStartXvfb
		FindDesktopFreeDisplay = originalFind
		StartDesktopBootstrap = originalBootstrap
		system.CollectUserSessionsFn = originalCollectUserSessions
	})
	system.CollectUserSessionsFn = func() ([]agentmgr.UserSession, error) { return nil, nil }

	// Create lock file so :0 is considered usable (triggers first attempt → display error → Xvfb).
	lockFile := "/tmp/.X0-lock"
	if err := os.WriteFile(lockFile, []byte("99999\n"), 0o644); err != nil {
		t.Fatalf("failed to create mock lock file: %v", err)
	}
	defer os.Remove(lockFile)

	primaryErr := errors.New("cannot open display :0")
	fallbackCmd := &exec.Cmd{}
	xvfbCmd := &exec.Cmd{}
	launchCalls := 0

	LaunchDesktopVNCReady = func(display string, port int, quality, vncPassword, xauthPath string, timeout time.Duration) (*exec.Cmd, string, string, error) {
		launchCalls++
		if launchCalls == 1 {
			return nil, "", "", primaryErr
		}
		return fallbackCmd, "/tmp/auth-fb", "", nil
	}
	FindDesktopFreeDisplay = func() int { return 95 }
	StartDesktopXvfb = func(display, width, height int) (*exec.Cmd, string, error) {
		return xvfbCmd, "", nil
	}
	StartDesktopBootstrap = func(display, xauthPath string) (*exec.Cmd, error) {
		if xauthPath != "" {
			t.Fatalf("expected empty xauth path, got %q", xauthPath)
		}
		return nil, fmt.Errorf("xterm not found")
	}

	cmd, gotXvfb, gotBootstrap, _, err := StartLinuxVNCServer(":0", 5901, "medium", "")
	if err != nil {
		t.Fatalf("expected success despite bootstrap failure, got: %v", err)
	}
	if cmd != fallbackCmd {
		t.Fatalf("expected fallback VNC cmd")
	}
	if gotXvfb != xvfbCmd {
		t.Fatalf("expected Xvfb cmd")
	}
	if gotBootstrap != nil {
		t.Fatalf("expected nil bootstrap cmd when xterm failed")
	}
}

func TestStartDesktopBootstrapShellRequiresXterm(t *testing.T) {
	originalPath := t.TempDir()
	t.Setenv("PATH", originalPath)

	_, err := StartDesktopBootstrapShell(":99", "")
	if err == nil {
		t.Fatal("expected missing xterm error")
	}
	if !strings.Contains(err.Error(), "xterm not found") {
		t.Fatalf("error=%q, want xterm missing hint", err)
	}
}

func requireArgPair(t *testing.T, args []string, flag, value string) {
	t.Helper()
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag && args[i+1] == value {
			return
		}
	}
	t.Fatalf("expected args to contain pair %q %q, got %v", flag, value, args)
}

func requireNoStandaloneArg(t *testing.T, args []string, flag string) {
	t.Helper()
	for _, arg := range args {
		if arg == flag {
			t.Fatalf("expected args to not contain %q, got %v", flag, args)
		}
	}
}

// ContainsEnvValue moved to test_helpers_test.go

func TestWaitForXvfbReadySucceedsWhenLockFileExists(t *testing.T) {
	display := 88
	lockFile := fmt.Sprintf("/tmp/.X%d-lock", display)
	if err := os.WriteFile(lockFile, []byte("12345\n"), 0o644); err != nil {
		t.Fatalf("failed to create mock lock file: %v", err)
	}
	defer os.Remove(lockFile)
	err := WaitForXvfbReady(display, 2*time.Second)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

func TestWaitForXvfbReadyTimesOutWhenNoLockFile(t *testing.T) {
	err := WaitForXvfbReady(87, 500*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("expected timeout in error message, got: %v", err)
	}
}

func TestGenerateHexCookie(t *testing.T) {
	cookie := GenerateHexCookie(32)
	if len(cookie) != 32 {
		t.Fatalf("expected 32-char cookie, got %d chars: %q", len(cookie), cookie)
	}
	// Verify it's valid hex.
	for _, c := range cookie {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("cookie contains non-hex char %q: %q", string(c), cookie)
		}
	}
	// Two calls should produce different values.
	cookie2 := GenerateHexCookie(32)
	if cookie == cookie2 {
		t.Fatalf("expected unique cookies, got identical: %q", cookie)
	}
}

func TestCreateXauthorityFile(t *testing.T) {
	path, err := CreateXauthorityFile(99)
	if err != nil {
		t.Skipf("xauth not available: %v", err)
	}
	defer os.Remove(path)
	if path == "" {
		t.Fatal("expected non-empty path")
	}
	info, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatalf("file not found: %v", statErr)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600, got %o", info.Mode().Perm())
	}
}

func TestIsDisplayAvailableUsesLockFile(t *testing.T) {
	originalCollectUserSessions := system.CollectUserSessionsFn
	t.Cleanup(func() {
		system.CollectUserSessionsFn = originalCollectUserSessions
	})
	system.CollectUserSessionsFn = func() ([]agentmgr.UserSession, error) { return nil, nil }

	lockFile := "/tmp/.X86-lock"
	if err := os.WriteFile(lockFile, []byte("12345\n"), 0o644); err != nil {
		t.Fatalf("failed to create lock file: %v", err)
	}
	defer os.Remove(lockFile)

	if !IsDisplayAvailable(":86") {
		t.Fatal("expected display to be available with lock file present")
	}

	if IsDisplayAvailable(":85") {
		t.Fatal("expected display without lock file or active session to be unavailable")
	}
}
