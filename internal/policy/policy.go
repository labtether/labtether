package policy

import (
	"strings"

	"github.com/labtether/labtether/internal/commandpolicy"
)

type CheckRequest struct {
	ActorID string `json:"actor_id"`
	Target  string `json:"target"`
	Mode    string `json:"mode"`
	Action  string `json:"action"`
	Command string `json:"command,omitempty"`
}

type CheckResponse struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
	Mode    string `json:"mode"`
}

type EvaluatorConfig struct {
	InteractiveEnabled bool
	StructuredEnabled  bool
	ConnectorEnabled   bool
	AllowlistMode      bool
	AllowlistPrefixes  []string
	BlockedSubstrings  []string
}

func DefaultEvaluatorConfig() EvaluatorConfig {
	return EvaluatorConfig{
		InteractiveEnabled: true,
		StructuredEnabled:  true,
		ConnectorEnabled:   true,
		AllowlistMode:      false,
		AllowlistPrefixes: []string{
			"uptime",
			"uname",
			"df",
			"du",
			"free",
			"ps",
			"top",
			"journalctl",
			"systemctl status",
			"docker ps",
			"docker images",
			"ls",
			"cat",
			"grep",
			"tail",
			"head",
		},
		BlockedSubstrings: []string{
			"rm -rf /",
			":(){ :|:& };:",
			"mkfs",
			"shutdown -h now",
		},
	}
}

func Evaluate(req CheckRequest, cfg EvaluatorConfig) CheckResponse {
	mode := req.Mode
	if mode == "" {
		mode = "structured"
	}

	normalizedMode := strings.ToLower(strings.TrimSpace(mode))
	switch normalizedMode {
	case "interactive":
		if !cfg.InteractiveEnabled {
			return CheckResponse{Allowed: false, Reason: "interactive mode disabled by policy", Mode: normalizedMode}
		}
	case "structured":
		if !cfg.StructuredEnabled {
			return CheckResponse{Allowed: false, Reason: "structured mode disabled by policy", Mode: normalizedMode}
		}
	case "connector":
		if !cfg.ConnectorEnabled {
			return CheckResponse{Allowed: false, Reason: "connector mode disabled by policy", Mode: normalizedMode}
		}
	default:
		return CheckResponse{Allowed: false, Reason: "unsupported command mode", Mode: normalizedMode}
	}

	for _, blocked := range cfg.BlockedSubstrings {
		if blocked == "" {
			continue
		}
		if strings.Contains(strings.ToLower(req.Command), strings.ToLower(blocked)) {
			return CheckResponse{Allowed: false, Reason: "command blocked by safety policy", Mode: mode}
		}
	}

	if cfg.AllowlistMode && strings.EqualFold(req.Action, "command_execute") {
		argv, err := commandpolicy.ParseArgv(req.Command)
		if err != nil {
			return CheckResponse{Allowed: false, Reason: err.Error(), Mode: normalizedMode}
		}
		if len(argv) == 0 {
			return CheckResponse{Allowed: false, Reason: "command is required", Mode: normalizedMode}
		}

		allowed := false
		for _, rule := range cfg.AllowlistPrefixes {
			if commandpolicy.MatchesRule(argv, rule) {
				allowed = true
				break
			}
		}
		if !allowed {
			return CheckResponse{Allowed: false, Reason: "command not in allowlist", Mode: normalizedMode}
		}
	}

	return CheckResponse{Allowed: true, Mode: normalizedMode}
}
