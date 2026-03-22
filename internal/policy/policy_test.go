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
