package agents

import (
	_ "embed"
	"strings"
)

// agentReleasePublicKey is the Ed25519 key that signs official agent release
// binaries. It is a copy of scripts/release/agent-release-public-key.b64
// (go:embed cannot reach outside this package); a test keeps them identical.
//
//go:embed agent_release_public_key.b64
var agentReleasePublicKey string

// AgentReleasePublicKey returns the base64 Ed25519 release-signing key that
// installed agents use to verify self-updates.
func AgentReleasePublicKey() string {
	return strings.TrimSpace(agentReleasePublicKey)
}
