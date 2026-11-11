package policy

import "testing"

func TestEvaluateBlocksDangerousCommand(t *testing.T) {
	cfg := DefaultEvaluatorConfig()
	res := Evaluate(CheckRequest{Mode: "interactive", Command: "rm -rf /"}, cfg)

	if res.Allowed {
		t.Fatalf("expected command to be blocked")
	}
}

func TestEvaluateAllowsNormalCommand(t *testing.T) {
	cfg := DefaultEvaluatorConfig()
	res := Evaluate(CheckRequest{Mode: "interactive", Command: "ls -la"}, cfg)

	if !res.Allowed {
		t.Fatalf("expected command to be allowed")
	}
}

func TestEvaluateAllowlistModeBlocksUnknownCommand(t *testing.T) {
	cfg := DefaultEvaluatorConfig()
	cfg.AllowlistMode = true
	cfg.AllowlistPrefixes = []string{"uptime", "ls"}

	res := Evaluate(CheckRequest{Mode: "structured", Action: "command_execute", Command: "curl example.com"}, cfg)
	if res.Allowed {
		t.Fatalf("expected command to be blocked by allowlist")
	}
}

func TestEvaluateAllowlistModeAllowsKnownCommand(t *testing.T) {
	cfg := DefaultEvaluatorConfig()
	cfg.AllowlistMode = true
	cfg.AllowlistPrefixes = []string{"uptime", "ls"}

	res := Evaluate(CheckRequest{Mode: "structured", Action: "command_execute", Command: "uptime"}, cfg)
	if !res.Allowed {
		t.Fatalf("expected command to be allowed by allowlist")
	}
}

func TestEvaluateAllowlistRequiresTokenBoundariesAndRejectsShellOperators(t *testing.T) {
	cfg := DefaultEvaluatorConfig()
	cfg.AllowlistMode = true
	cfg.AllowlistPrefixes = []string{"uptime", "systemctl status"}

	for _, command := range []string{"uptime-extra", "uptime; id", "uptime && id", "systemctl restart sshd"} {
		res := Evaluate(CheckRequest{Mode: "structured", Action: "command_execute", Command: command}, cfg)
		if res.Allowed {
			t.Fatalf("expected %q to be denied", command)
		}
	}
	if res := Evaluate(CheckRequest{Mode: "structured", Action: "command_execute", Command: "systemctl status sshd"}, cfg); !res.Allowed {
		t.Fatalf("expected exact allowlist rule with arguments to pass: %s", res.Reason)
	}
}

// The default allowlist must let Windows devices run their read-only commands
// without opening the interpreters or the setting-changing forms of them.
func TestDefaultAllowlistCoversWindowsReadOnlyCommands(t *testing.T) {
	cfg := DefaultEvaluatorConfig()
	cfg.AllowlistMode = true
	check := func(command string) CheckResponse {
		return Evaluate(CheckRequest{Mode: "structured", Action: "command_execute", Command: command}, cfg)
	}
	for _, command := range []string{
		"whoami", "whoami /all", "hostname", "ipconfig", "ipconfig /all",
		"systeminfo", "tasklist", "tasklist /svc", "cmd /c ver", "powershell -Command Get-Date",
	} {
		if res := check(command); !res.Allowed {
			t.Errorf("expected %q to be allowed: %s", command, res.Reason)
		}
	}
	for _, command := range []string{
		"ipconfig /release", "ipconfig /all /release", "hostname newname",
		"cmd", "cmd /c", "cmd /c ver \"& del x\"", "cmd /c dir",
		"powershell", "powershell -Command", "powershell -Command Get-Date \"; Remove-Item x\"",
		"pwsh -Command Get-Date",
	} {
		if res := check(command); res.Allowed {
			t.Errorf("expected %q to be refused", command)
		}
	}
}

// The reason is shown as-is on the Actions page and in API answers, so it
// names the Settings switch people turned off.
func TestEvaluateConnectorOffNamesTheSettingsSwitch(t *testing.T) {
	cfg := DefaultEvaluatorConfig()
	cfg.ConnectorEnabled = false
	res := Evaluate(CheckRequest{Mode: "connector", Command: "vm.start"}, cfg)
	if want := "Integration actions are turned off in Settings."; res.Allowed || res.Reason != want {
		t.Fatalf("got allowed=%v %q, want %q", res.Allowed, res.Reason, want)
	}
}
