package docker

import (
	"context"
	"fmt"
	"slices"
)

// ActionTarget identifies the real agent and canonical asset affected by an action.
type ActionTarget struct {
	AgentID     string
	AssetID     string
	HostAssetID string
	ContainerID string
	StackName   string
	ConfigDir   string
	ConfigFiles []string
	EndpointID  string
}

// ResolveActionTarget uses the same resource lookup as action dispatch.
func (c *Coordinator) ResolveActionTarget(actionID, targetID string) (ActionTarget, error) {
	target := ActionTarget{AssetID: targetID}
	var err error
	switch {
	case isHostAction(actionID):
		target.AgentID, err = c.resolveHostTarget(targetID)
		target.AssetID = "docker-host-" + normalizeID(target.AgentID)
	case isStackAction(actionID):
		target, err = c.resolveStackTarget(targetID)
	default:
		target.AgentID, target.ContainerID, err = c.resolveContainerTarget(targetID)
	}
	if err == nil && actionID == "stack.deploy" {
		target.EndpointID, err = c.resolveComposeEndpoint(target.AgentID)
	}
	target.HostAssetID = "docker-host-" + normalizeID(target.AgentID)
	return target, err
}

func isHostAction(actionID string) bool {
	switch actionID {
	case "container.create", "image.pull", "image.remove", "stack.deploy":
		return true
	default:
		return false
	}
}

type actionHostKey struct{}

// WithActionHost binds dispatch to the agent already checked by the API. A
// discovery change between admission and dispatch must not select another host.
func WithActionHost(ctx context.Context, agentID string) context.Context {
	return context.WithValue(ctx, actionHostKey{}, agentID)
}

func checkActionHost(ctx context.Context, agentID string) error {
	if expected, ok := ctx.Value(actionHostKey{}).(string); ok && expected != agentID {
		return fmt.Errorf("action target does not belong to the authorized docker host")
	}
	return nil
}

// WithActionTarget preserves the exact resource admitted by the API through the
// coordinator's last discovery lookup. The context never becomes wire data.
func WithActionTarget(ctx context.Context, target ActionTarget) context.Context {
	target.ConfigFiles = slices.Clone(target.ConfigFiles)
	return context.WithValue(WithActionHost(ctx, target.AgentID), actionTargetKey{}, target)
}

type actionTargetKey struct{}

func checkActionTarget(ctx context.Context, actionID, agentID, containerID, stackName, configDir string, configFiles []string, endpointID string) error {
	if err := checkActionHost(ctx, agentID); err != nil {
		return err
	}
	if expected, ok := ctx.Value(actionTargetKey{}).(ActionTarget); ok {
		// Deploy targets a host; its new stack name/directory come from the
		// persisted action parameters, not an existing discovered resource.
		if isHostAction(actionID) {
			containerID, stackName, configDir, configFiles = "", "", "", nil
		}
		if expected.ContainerID != containerID || expected.StackName != stackName || expected.ConfigDir != configDir || !slices.Equal(expected.ConfigFiles, configFiles) || expected.EndpointID != endpointID {
			return fmt.Errorf("action target does not match the authorized docker resource")
		}
	}
	return nil
}
