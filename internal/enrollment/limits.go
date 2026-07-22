package enrollment

import "fmt"

const (
	DefaultTokenMaxUses          = 1
	DefaultTokenMaxUsesCeiling   = 256
	HardTokenMaxUsesCeiling      = 4096
	DefaultMaxEnrolledAgents     = 1024
	HardMaxEnrolledAgents        = 65536
	DefaultMaxAgentConnections   = 2048
	HardMaxAgentConnections      = 65536
	DefaultMaxConnectionsPerPeer = 256
	HardMaxConnectionsPerPeer    = 4096
)

// BoundedLimit converts an optional positive configured value into a hard-
// bounded server limit. Invalid values fail closed to the documented default.
func BoundedLimit(value, fallback, hardMax int) int {
	if fallback <= 0 || fallback > hardMax {
		fallback = hardMax
	}
	if value <= 0 {
		return fallback
	}
	if value > hardMax {
		return hardMax
	}
	return value
}

// NormalizeRequestedTokenMaxUses applies the API default and rejects values
// outside both the configured server ceiling and the absolute persistence
// ceiling. Zero means the field was omitted; negative values are invalid.
func NormalizeRequestedTokenMaxUses(value, configuredCeiling int) (int, error) {
	ceiling := BoundedLimit(configuredCeiling, DefaultTokenMaxUsesCeiling, HardTokenMaxUsesCeiling)
	if value == 0 {
		return DefaultTokenMaxUses, nil
	}
	if value < 0 || value > ceiling || value > HardTokenMaxUsesCeiling {
		return 0, fmt.Errorf("max_uses must be between 1 and %d", ceiling)
	}
	return value, nil
}

// ValidateStoredTokenMaxUses is the persistence-layer hard stop used by all
// callers, including tests and legacy in-process code that bypass HTTP.
func ValidateStoredTokenMaxUses(value int) error {
	if value < 1 || value > HardTokenMaxUsesCeiling {
		return fmt.Errorf("max_uses must be between 1 and %d", HardTokenMaxUsesCeiling)
	}
	return nil
}
