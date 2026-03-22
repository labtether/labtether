package docker

import (
	"fmt"
	"strings"
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
				return aID, ct.ID, nil
			}
		}
	}

	return "", "", fmt.Errorf("container not found for asset ID: %s", assetID)
}

// resolveStackTarget parses a stack asset ID to find the owning agent, stack name, and config directory.
//
// Asset ID format: docker-stack-{normalizedAgentID}-{normalizedStackName}
func (c *Coordinator) resolveStackTarget(assetID string) (agentID, stackName, configDir string, err error) {
	if !strings.HasPrefix(assetID, "docker-stack-") {
		return "", "", "", fmt.Errorf("invalid stack asset ID: %s", assetID)
	}
	rest := strings.TrimPrefix(assetID, "docker-stack-")

	c.mu.RLock()
	defer c.mu.RUnlock()

	for aID, host := range c.hosts {
		normalizedAgent := normalizeID(aID)
		prefix := normalizedAgent + "-"
		if !strings.HasPrefix(rest, prefix) {
			continue
		}
		stackPart := strings.TrimPrefix(rest, prefix)
		for _, stack := range host.ComposeStacks {
			if normalizeID(stack.Name) == stackPart {
				dir := ""
				if stack.ConfigFile != "" {
					// Extract the directory portion of the config file path.
					idx := strings.LastIndex(stack.ConfigFile, "/")
					if idx >= 0 {
						dir = stack.ConfigFile[:idx]
					}
				}
				return aID, stack.Name, dir, nil
			}
		}
	}

	return "", "", "", fmt.Errorf("stack not found for asset ID: %s", assetID)
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
				return aID, nil
			}
		}
	}

	// Final fallback: exact normalized raw target match.
	for aID := range c.hosts {
		if normalizeID(aID) == normalized {
			return aID, nil
		}
	}

	return "", fmt.Errorf("host not found for target: %s", target)
}
