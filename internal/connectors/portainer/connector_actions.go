package portainer

import (
	"context"
	"fmt"
	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/connectorsdk"
	"strconv"
	"strings"
)

// Actions returns the set of operations that can be executed against Portainer.
func (c *Connector) Actions() []connectorsdk.ActionDescriptor {
	return []connectorsdk.ActionDescriptor{
		// Container actions
		{
			ID:             "container.start",
			Name:           "Start Container",
			Description:    "Start a stopped container.",
			RequiresTarget: true,
			SupportsDryRun: true,
		},
		{
			ID:             "container.stop",
			Name:           "Stop Container",
			Description:    "Stop a running container.",
			RequiresTarget: true,
			SupportsDryRun: true,
		},
		{
			ID:             "container.restart",
			Name:           "Restart Container",
			Description:    "Restart a container.",
			RequiresTarget: true,
			SupportsDryRun: true,
		},
		{
			ID:             "container.kill",
			Name:           "Kill Container",
			Description:    "Send SIGKILL to a container.",
			RequiresTarget: true,
			SupportsDryRun: true,
		},
		{
			ID:             "container.pause",
			Name:           "Pause Container",
			Description:    "Pause a running container.",
			RequiresTarget: true,
			SupportsDryRun: true,
		},
		{
			ID:             "container.unpause",
			Name:           "Unpause Container",
			Description:    "Unpause a paused container.",
			RequiresTarget: true,
			SupportsDryRun: true,
		},
		{
			ID:             "container.remove",
			Name:           "Remove Container",
			Description:    "Remove a container.",
			RequiresTarget: true,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{Key: "force", Label: "Force", Required: false, Description: "Force remove (true/false)"},
			},
		},
		// Stack actions
		{
			ID:             "stack.start",
			Name:           "Start Stack",
			Description:    "Start a stopped stack.",
			RequiresTarget: true,
			SupportsDryRun: true,
		},
		{
			ID:             "stack.stop",
			Name:           "Stop Stack",
			Description:    "Stop a running stack.",
			RequiresTarget: true,
			SupportsDryRun: true,
		},
		{
			ID:             "stack.remove",
			Name:           "Remove Stack",
			Description:    "Remove a stack.",
			RequiresTarget: true,
			SupportsDryRun: true,
		},
		{
			ID:             "stack.redeploy",
			Name:           "Redeploy Stack",
			Description:    "Redeploy a git-based stack.",
			RequiresTarget: true,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{Key: "pull_image", Label: "Pull Image", Required: false, Description: "Pull latest images before redeploying (default true)"},
			},
		},
	}
}

// ExecuteAction dispatches the requested action to the Portainer API.
func (c *Connector) ExecuteAction(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	if !c.isConfigured() {
		return connectorsdk.ActionResult{
			Status:  "failed",
			Message: "portainer connector not configured (missing PORTAINER_BASE_URL or auth credentials)",
		}, nil
	}

	switch {
	case strings.HasPrefix(actionID, "container."):
		return c.executeContainerAction(ctx, actionID, req)
	case strings.HasPrefix(actionID, "stack."):
		return c.executeStackAction(ctx, actionID, req)
	default:
		return connectorsdk.ActionResult{
			Status:  "failed",
			Message: fmt.Sprintf("unsupported action: %s", actionID),
		}, nil
	}
}

// executeContainerAction handles container.* actions.
func (c *Connector) executeContainerAction(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	endpointID, containerID, err := parseContainerTarget(req.TargetID)
	if err != nil {
		return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
	}

	if req.DryRun {
		return connectorsdk.ActionResult{
			Status:  "succeeded",
			Message: "dry-run: action validated",
			Output:  fmt.Sprintf("would execute %s on endpoint %d container %s", actionID, endpointID, containerID),
		}, nil
	}

	action := strings.TrimPrefix(actionID, "container.")

	if action == "remove" {
		force := false
		if raw := strings.TrimSpace(req.Params["force"]); raw != "" {
			if parsed, parseErr := strconv.ParseBool(raw); parseErr == nil {
				force = parsed
			}
		}
		if err := c.client.RemoveContainer(ctx, endpointID, containerID, force); err != nil {
			return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
		}
		return connectorsdk.ActionResult{
			Status:  "succeeded",
			Message: fmt.Sprintf("container %s removed", containerID),
		}, nil
	}

	if err := c.client.ContainerAction(ctx, endpointID, containerID, action); err != nil {
		return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
	}
	return connectorsdk.ActionResult{
		Status:  "succeeded",
		Message: fmt.Sprintf("container %s: %s completed", containerID, action),
	}, nil
}

// executeStackAction handles stack.* actions.
func (c *Connector) executeStackAction(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	stackID, err := parseStackTarget(req.TargetID)
	if err != nil {
		return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
	}

	endpointIDStr := strings.TrimSpace(req.Params["endpoint_id"])
	if endpointIDStr == "" {
		return connectorsdk.ActionResult{
			Status:  "failed",
			Message: "endpoint_id param is required for stack actions",
		}, nil
	}
	endpointID, err := strconv.Atoi(endpointIDStr)
	if err != nil {
		return connectorsdk.ActionResult{
			Status:  "failed",
			Message: fmt.Sprintf("invalid endpoint_id: %s", endpointIDStr),
		}, nil
	}

	if req.DryRun {
		return connectorsdk.ActionResult{
			Status:  "succeeded",
			Message: "dry-run: action validated",
			Output:  fmt.Sprintf("would execute %s on stack %d (endpoint %d)", actionID, stackID, endpointID),
		}, nil
	}

	action := strings.TrimPrefix(actionID, "stack.")

	switch action {
	case "start":
		if err := c.client.StartStack(ctx, stackID, endpointID); err != nil {
			return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
		}
	case "stop":
		if err := c.client.StopStack(ctx, stackID, endpointID); err != nil {
			return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
		}
	case "redeploy":
		pullImage := true
		if raw := strings.TrimSpace(req.Params["pull_image"]); strings.EqualFold(raw, "false") {
			pullImage = false
		}
		if err := c.client.RedeployStack(ctx, stackID, endpointID, pullImage); err != nil {
			return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
		}
	case "remove":
		if err := c.client.RemoveStack(ctx, stackID, endpointID); err != nil {
			return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
		}
	default:
		return connectorsdk.ActionResult{
			Status:  "failed",
			Message: fmt.Sprintf("unsupported stack action: %s", actionID),
		}, nil
	}

	return connectorsdk.ActionResult{
		Status:  "succeeded",
		Message: fmt.Sprintf("stack %d: %s completed", stackID, action),
	}, nil
}

// parseContainerTarget extracts endpointID and containerID from a target
// formatted as "portainer-container-{endpointID}-{containerID}".
func parseContainerTarget(target string) (int, string, error) {
	target = assetid.NativeCollectorAssetID(target)
	const prefix = "portainer-container-"
	if !strings.HasPrefix(target, prefix) {
		return 0, "", fmt.Errorf("invalid container target format: %q (expected prefix %q)", target, prefix)
	}

	rest := target[len(prefix):]
	idx := strings.Index(rest, "-")
	if idx <= 0 {
		return 0, "", fmt.Errorf("invalid container target format: %q (expected portainer-container-{epID}-{containerID})", target)
	}

	epIDStr := rest[:idx]
	containerID := rest[idx+1:]

	epID, err := strconv.Atoi(epIDStr)
	if err != nil {
		return 0, "", fmt.Errorf("invalid endpoint ID in target %q: %w", target, err)
	}
	if containerID == "" {
		return 0, "", fmt.Errorf("empty container ID in target %q", target)
	}

	return epID, containerID, nil
}

// parseStackTarget extracts stackID from a target formatted as "portainer-stack-{id}".
func parseStackTarget(target string) (int, error) {
	target = assetid.NativeCollectorAssetID(target)
	const prefix = "portainer-stack-"
	if !strings.HasPrefix(target, prefix) {
		return 0, fmt.Errorf("invalid stack target format: %q (expected prefix %q)", target, prefix)
	}

	idStr := target[len(prefix):]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return 0, fmt.Errorf("invalid stack ID in target %q: %w", target, err)
	}
	return id, nil
}
