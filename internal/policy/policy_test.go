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
