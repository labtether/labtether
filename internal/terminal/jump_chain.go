package terminal

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"unicode"
)

const (
	MaxJumpChainHops       = 8
	MaxJumpChainHostLength = 253
	MaxJumpChainFieldLen   = 255
)

// DecodeJumpChain parses and normalizes persisted or API-provided jump-chain
// JSON. Strict decoding prevents silently accepting misspelled security fields.
func DecodeJumpChain(raw json.RawMessage) (JumpChain, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return JumpChain{}, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var chain JumpChain
	if err := dec.Decode(&chain); err != nil {
		return JumpChain{}, fmt.Errorf("invalid jump chain: %w", err)
	}
	if err := ensureJumpChainJSONEOF(dec); err != nil {
		return JumpChain{}, err
	}
	return NormalizeJumpChain(chain)
}

func ensureJumpChainJSONEOF(dec *json.Decoder) error {
	var extra any
	if err := dec.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return fmt.Errorf("invalid jump chain: %w", err)
	}
	return errors.New("invalid jump chain: multiple JSON values")
}

// NormalizeJumpChain applies hard work bounds and validates every connection
// field before a chain is persisted or resolved.
func NormalizeJumpChain(chain JumpChain) (JumpChain, error) {
	if len(chain.Hops) > MaxJumpChainHops {
		return JumpChain{}, fmt.Errorf("jump chain exceeds maximum of %d hops", MaxJumpChainHops)
	}
	normalized := JumpChain{Hops: make([]HopConfig, 0, len(chain.Hops))}
	seen := make(map[string]struct{}, len(chain.Hops))
	for i, hop := range chain.Hops {
		if containsControl(hop.Host) {
			return JumpChain{}, fmt.Errorf("jump chain hop %d: host contains control characters", i)
		}
		host := strings.TrimSpace(hop.Host)
		if err := validateJumpChainHost(host); err != nil {
			return JumpChain{}, fmt.Errorf("jump chain hop %d: %w", i, err)
		}
		port := hop.Port
		if port == 0 {
			port = 22
		}
		if port < 1 || port > 65535 {
			return JumpChain{}, fmt.Errorf("jump chain hop %d: port must be between 1 and 65535", i)
		}
		if containsControl(hop.Username) {
			return JumpChain{}, fmt.Errorf("jump chain hop %d: username contains control characters", i)
		}
		username := strings.TrimSpace(hop.Username)
		if err := validateJumpChainField("username", username, true); err != nil {
			return JumpChain{}, fmt.Errorf("jump chain hop %d: %w", i, err)
		}
		if containsControl(hop.CredentialProfileID) {
			return JumpChain{}, fmt.Errorf("jump chain hop %d: credential_profile_id contains control characters", i)
		}
		profileID := strings.TrimSpace(hop.CredentialProfileID)
		if err := validateJumpChainField("credential_profile_id", profileID, true); err != nil {
			return JumpChain{}, fmt.Errorf("jump chain hop %d: %w", i, err)
		}

		key := strings.ToLower(host) + "\x00" + fmt.Sprint(port) + "\x00" + username + "\x00" + profileID
		if _, duplicate := seen[key]; duplicate {
			return JumpChain{}, fmt.Errorf("jump chain hop %d duplicates an earlier hop", i)
		}
		seen[key] = struct{}{}
		normalized.Hops = append(normalized.Hops, HopConfig{
			Host:                host,
			Port:                port,
			Username:            username,
			CredentialProfileID: profileID,
		})
	}
	return normalized, nil
}

func validateJumpChainHost(host string) error {
	if host == "" {
		return errors.New("host is required")
	}
	if len(host) > MaxJumpChainHostLength {
		return fmt.Errorf("host exceeds %d bytes", MaxJumpChainHostLength)
	}
	if strings.ContainsAny(host, "/\\@?#") || strings.Contains(host, "://") {
		return errors.New("host must be a hostname or IP address without URL or path syntax")
	}
	for _, r := range host {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return errors.New("host contains whitespace or control characters")
		}
	}
	if strings.Contains(host, ":") {
		candidate := host
		if zoneAt := strings.LastIndex(candidate, "%"); zoneAt >= 0 {
			if zoneAt == len(candidate)-1 {
				return errors.New("host has an empty IPv6 zone")
			}
			candidate = candidate[:zoneAt]
		}
		if net.ParseIP(candidate) == nil {
			return errors.New("host contains an invalid IPv6 address")
		}
	}
	return nil
}

func validateJumpChainField(name, value string, required bool) error {
	if required && value == "" {
		return fmt.Errorf("%s is required", name)
	}
	if len(value) > MaxJumpChainFieldLen {
		return fmt.Errorf("%s exceeds %d bytes", name, MaxJumpChainFieldLen)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return fmt.Errorf("%s contains control characters", name)
		}
	}
	return nil
}

func containsControl(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func JumpChainUsesCredential(chain JumpChain) bool {
	for _, hop := range chain.Hops {
		if strings.TrimSpace(hop.CredentialProfileID) != "" {
			return true
		}
	}
	return false
}
