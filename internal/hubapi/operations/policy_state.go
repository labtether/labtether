package operations

import (
	"context"
	"log"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/runtimesettings"
)

// PolicyRuntimeState holds atomic policy runtime configuration that can be
// updated from database overrides without restarting.
type PolicyRuntimeState struct {
	base  policy.EvaluatorConfig
	value atomic.Value
}

// NewPolicyRuntimeState creates a PolicyRuntimeState with the given base config.
func NewPolicyRuntimeState(base policy.EvaluatorConfig) *PolicyRuntimeState {
	state := &PolicyRuntimeState{
		base: base,
	}
	state.value.Store(base)
	return state
}

// Current returns the current policy evaluator config.
func (s *PolicyRuntimeState) Current() policy.EvaluatorConfig {
	if s == nil {
		return policy.DefaultEvaluatorConfig()
	}
	raw := s.value.Load()
	cfg, ok := raw.(policy.EvaluatorConfig)
	if !ok {
		return policy.DefaultEvaluatorConfig()
	}
	return cfg
}

// ApplyOverrides applies runtime setting overrides from a map.
func (s *PolicyRuntimeState) ApplyOverrides(overrides map[string]string) {
	if s == nil {
		return
	}
	cfg := s.base
	if raw := strings.TrimSpace(overrides[runtimesettings.KeyPolicyStructuredEnabled]); raw != "" {
		cfg.StructuredEnabled = ParseBoolFallback(raw, cfg.StructuredEnabled)
	}
	if raw := strings.TrimSpace(overrides[runtimesettings.KeyPolicyInteractiveEnabled]); raw != "" {
		cfg.InteractiveEnabled = ParseBoolFallback(raw, cfg.InteractiveEnabled)
	}
	if raw := strings.TrimSpace(overrides[runtimesettings.KeyPolicyConnectorEnabled]); raw != "" {
		cfg.ConnectorEnabled = ParseBoolFallback(raw, cfg.ConnectorEnabled)
	}
	s.value.Store(cfg)
}

// LoadPolicyConfigFromEnv reads the policy evaluator configuration from environment.
func LoadPolicyConfigFromEnv() policy.EvaluatorConfig {
	cfg := policy.DefaultEvaluatorConfig()
	cfg.StructuredEnabled = ParseBoolFallback(shared.EnvOrDefault("STRUCTURED_ENABLED", "true"), cfg.StructuredEnabled)
	cfg.InteractiveEnabled = ParseBoolFallback(shared.EnvOrDefault("INTERACTIVE_ENABLED", "true"), cfg.InteractiveEnabled)
	cfg.ConnectorEnabled = ParseBoolFallback(shared.EnvOrDefault("CONNECTOR_ENABLED", "true"), cfg.ConnectorEnabled)
	// Safer baseline: enforce allowlist unless explicitly disabled.
	cfg.AllowlistMode = ParseBoolFallback(shared.EnvOrDefault("COMMAND_ALLOWLIST_MODE", "true"), true)
	cfg.AllowlistPrefixes = ParseCSVOrDefault(shared.EnvOrDefault("COMMAND_ALLOWLIST_PREFIXES", ""), cfg.AllowlistPrefixes)
	cfg.BlockedSubstrings = ParseCSVOrDefault(shared.EnvOrDefault("BLOCKED_COMMANDS", ""), cfg.BlockedSubstrings)
	return cfg
}

// RefreshPolicyRuntimeSettingsDirect reads overrides from the database directly
// instead of polling over HTTP, since policy is now in-process.
func RefreshPolicyRuntimeSettingsDirect(ctx context.Context, store persistence.RuntimeSettingsStore, state *PolicyRuntimeState) {
	if state == nil || store == nil {
		return
	}

	apply := func() {
		overrides, err := store.ListRuntimeSettingOverrides()
		if err != nil {
			log.Printf("labtether policy runtime settings refresh failed: %v", err)
			return
		}
		state.ApplyOverrides(overrides)
	}

	apply()
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("labtether policy runtime settings refresh stopped")
			return
		case <-ticker.C:
			apply()
		}
	}
}

// ParseBoolFallback parses a string as a boolean, returning the fallback on error.
func ParseBoolFallback(value string, fallback bool) bool {
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

// ParseCSVOrDefault parses a comma-separated string into a slice, returning fallback if empty.
func ParseCSVOrDefault(value string, fallback []string) []string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}

	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}

	if len(out) == 0 {
		return fallback
	}
	return out
}
