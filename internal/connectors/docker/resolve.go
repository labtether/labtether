package docker

import (
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"github.com/labtether/labtether/internal/connectorsdk"
)

// resolveContainerTarget parses a container asset ID to find the owning agent and full container ID.
//
// Asset ID format: docker-ct-{normalizedAgentID}-{containerID[:12]}
func (c *Coordinator) resolveContainerTarget(assetID string) (agentID, containerID string, err error) {
	if !strings.HasPrefix(assetID, "docker-ct-") {
		return "", "", fmt.Errorf("invalid container asset ID: %s", assetID)
	}
	rest := strings.TrimPrefix(assetID, "docker-ct-")

	// At minimum we need 1 char for agentID + dash + 12 chars for containerID short.
	if len(rest) < 13 {
		return "", "", fmt.Errorf("invalid container asset ID format: %s", assetID)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	for aID, host := range c.hosts {
		normalizedAgent := normalizeID(aID)
		prefix := normalizedAgent + "-"
		if !strings.HasPrefix(rest, prefix) {
			continue
		}
		shortCT := strings.TrimPrefix(rest, prefix)
		// Find the full container ID matching the short form.
		for _, ct := range host.Containers {
			ctShort := ct.ID
			if len(ctShort) > 12 {
				ctShort = ctShort[:12]
			}
			if ctShort == shortCT {
				if agentID != "" && (agentID != aID || containerID != ct.ID) {
					return "", "", fmt.Errorf("ambiguous container asset ID: %s", assetID)
				}
				agentID, containerID = aID, ct.ID
			}
		}
	}

	if agentID != "" {
		return agentID, containerID, nil
	}
	return "", "", fmt.Errorf("container not found for asset ID: %s", assetID)
}

// ErrComposeUpgradeRequired is safe to show at all Compose admission boundaries.
var ErrComposeUpgradeRequired = connectorsdk.ErrComposeUpgradeRequired

// resolveStackTarget retains native paths as opaque agent-OS strings. The old
// ConfigFile field is display-only and never reconstructs a mutation target.
func (c *Coordinator) resolveStackTarget(assetID string) (ActionTarget, error) {
	if !strings.HasPrefix(assetID, "docker-stack-") {
		return ActionTarget{}, fmt.Errorf("invalid stack asset ID: %s", assetID)
	}
	rest := strings.TrimPrefix(assetID, "docker-stack-")
	c.mu.RLock()
	defer c.mu.RUnlock()
	var target ActionTarget
	var selected ComposeStackState
	var selectedHost *DockerHost
	for aID, host := range c.hosts {
		prefix := normalizeID(aID) + "-"
		if !strings.HasPrefix(rest, prefix) {
			continue
		}
		stackPart := strings.TrimPrefix(rest, prefix)
		for _, stack := range host.ComposeStacks {
			if normalizeID(stack.Name) != stackPart {
				continue
			}
			if target.AgentID != "" && (target.AgentID != aID || target.StackName != stack.Name || target.ConfigDir != stack.ConfigDir || !slices.Equal(target.ConfigFiles, stack.ConfigFiles) || selected.SelectionError != stack.SelectionError) {
				return ActionTarget{}, fmt.Errorf("ambiguous stack asset ID: %s", assetID)
			}
			target = ActionTarget{AgentID: aID, AssetID: assetID, HostAssetID: "docker-host-" + normalizeID(aID), StackName: stack.Name, ConfigDir: stack.ConfigDir, ConfigFiles: slices.Clone(stack.ConfigFiles)}
			selected, selectedHost = stack, host
		}
	}
	if target.AgentID == "" {
		return ActionTarget{}, fmt.Errorf("stack not found for asset ID: %s", assetID)
	}
	endpoint, err := composeEndpoint(selectedHost)
	if err != nil {
		return ActionTarget{}, err
	}
	target.EndpointID = endpoint
	if selected.SelectionError != "" || target.StackName == "" || target.ConfigDir == "" || len(target.ConfigFiles) == 0 {
		return ActionTarget{}, fmt.Errorf("Compose selection is unavailable or inconsistent; refresh agent discovery before retrying")
	}
	for _, file := range target.ConfigFiles {
		if file == "" {
			return ActionTarget{}, fmt.Errorf("Compose selection contains an empty config file; refresh agent discovery before retrying")
		}
	}
	return target, nil
}

func (c *Coordinator) resolveComposeEndpoint(agentID string) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return composeEndpoint(c.hosts[agentID])
}

func composeEndpoint(host *DockerHost) (string, error) {
	if host == nil || host.ComposeActionVersion != 2 {
		return "", ErrComposeUpgradeRequired
	}
	id := host.ComposeEndpointID
	decoded, err := hex.DecodeString(id)
	if err != nil || len(decoded) != 32 || id != strings.ToLower(id) {
		return "", fmt.Errorf("Compose endpoint identity is unavailable; refresh agent discovery before retrying")
	}
	return id, nil
}

// resolveHostTarget resolves a host-style target into an owning agent ID.
//
// Supported inputs:
//   - docker host asset ID: docker-host-{normalizedAgentID}
//   - normalized agent ID: {normalizedAgentID}
//   - raw agent asset ID (case/space/dot-insensitive normalization)
func (c *Coordinator) resolveHostTarget(target string) (agentID string, err error) {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return "", fmt.Errorf("host target is required")
	}

	normalized := normalizeID(trimmed)
	if normalized == "" {
		return "", fmt.Errorf("invalid host target: %s", target)
	}

	candidates := []string{normalized}
	if strings.HasPrefix(normalized, "docker-host-") {
		candidates = append(candidates, strings.TrimPrefix(normalized, "docker-host-"))
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		for aID := range c.hosts {
			if normalizeID(aID) == candidate {
				if agentID != "" && agentID != aID {
					return "", fmt.Errorf("ambiguous docker host target: %s", target)
				}
				agentID = aID
			}
		}
	}

	if agentID != "" {
		return agentID, nil
	}

	return "", fmt.Errorf("host not found for target: %s", target)
}
