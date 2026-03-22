package remoteaccess

import (
	"os/exec"
	"testing"

	"github.com/labtether/labtether/internal/agentcore/system"
	"github.com/labtether/labtether/internal/agentmgr"
)

func TestPreferredX11DisplayUsesDetectedSessionDisplay(t *testing.T) {
	originalDetect := DetectDesktopSessionFn
	originalCollect := system.CollectUserSessionsFn
	t.Cleanup(func() {
		DetectDesktopSessionFn = originalDetect
		system.CollectUserSessionsFn = originalCollect
	})

	DetectDesktopSessionFn = func() DesktopSessionInfo {
		return DesktopSessionInfo{Type: DesktopSessionTypeX11, Display: ":1"}
	}
	system.CollectUserSessionsFn = func() ([]agentmgr.UserSession, error) {
		return nil, nil
	}

	if got := PreferredX11Display(); got != ":1" {
		t.Fatalf("PreferredX11Display() = %q, want :1", got)
	}
}

func TestWakeX11DisplayUsesX11Env(t *testing.T) {
	originalCommand := NewX11UtilityCommand
	originalDiscover := DiscoverDisplayXAuthorityFn
	t.Cleanup(func() {
		NewX11UtilityCommand = originalCommand
		DiscoverDisplayXAuthorityFn = originalDiscover
	})

	DiscoverDisplayXAuthorityFn = func(display string) string {
		if display != ":9" {
			t.Fatalf("display=%q, want :9", display)
		}
		return "/run/lightdm/root/:9"
	}

	var commands []*exec.Cmd
	NewX11UtilityCommand = func(name string, args ...string) (*exec.Cmd, error) {
		cmd := exec.Command("sh", "-c", "exit 0")
		commands = append(commands, cmd)
		return cmd, nil
	}

	WakeX11Display(":9", "")

	if len(commands) != 4 {
		t.Fatalf("expected 4 wake commands, got %d", len(commands))
	}
	for _, cmd := range commands {
		if !ContainsEnvValue(cmd.Env, "DISPLAY=:9") {
			t.Fatalf("expected DISPLAY in env, got %v", cmd.Env)
		}
		if !ContainsEnvValue(cmd.Env, "XAUTHORITY=/run/lightdm/root/:9") {
			t.Fatalf("expected XAUTHORITY in env, got %v", cmd.Env)
		}
	}
}
