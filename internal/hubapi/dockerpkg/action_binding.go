package dockerpkg

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/labtether/labtether/internal/connectors/docker"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubapi/maintenanceguard"
	"github.com/labtether/labtether/internal/modelmap"
)

const ActionAgentBindingParam = "docker_agent_id"

const (
	actionContainerBindingParam = "docker_container_id"
	actionStackBindingParam     = "docker_stack_name"
	actionDirectoryBindingParam = "docker_config_dir"
	actionFilesBindingParam     = "docker_config_files"
	actionEndpointBindingParam  = "docker_compose_endpoint_id"
)

// BindAction pins the actual reporting agent, not a synthetic collector. A
// child's own group never replaces the pause rules of its controlling host.
func (d *Deps) BindAction(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionRequest, error) {
	_, target, err := d.resolveActionOwner(ctx, actionID, req.TargetID)
	if err != nil {
		return req, err
	}
	req.Params = maps.Clone(req.Params)
	if req.Params == nil {
		req.Params = make(map[string]string)
	}
	// Never trust a caller-supplied internal binding, including a normalized
	// alias that could name a different reporting agent.
	req.Params[ActionAgentBindingParam] = target.AgentID
	req.Params[actionContainerBindingParam] = target.ContainerID
	req.Params[actionStackBindingParam] = target.StackName
	req.Params[actionDirectoryBindingParam] = target.ConfigDir
	delete(req.Params, actionFilesBindingParam)
	delete(req.Params, actionEndpointBindingParam)
	if target.EndpointID != "" {
		req.Params[actionEndpointBindingParam] = target.EndpointID
	}
	if target.StackName != "" {
		files, err := json.Marshal(target.ConfigFiles)
		if err != nil {
			return req, err
		}
		req.Params[actionFilesBindingParam] = string(files)
	}
	if err := d.checkActionOwners(ctx, req.TargetID, target); err != nil {
		return req, err
	}
	return req, nil
}

// ExecuteAction checks a queued binding against current discovery and current
// maintenance state. It does not silently bind an old job to a new agent.
func (d *Deps) ExecuteAction(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	expected := req.Params[ActionAgentBindingParam]
	if strings.TrimSpace(expected) == "" {
		return connectorsdk.ActionResult{}, fmt.Errorf("Docker action has no saved agent binding; resubmit the action")
	}
	containerID, hasContainer := req.Params[actionContainerBindingParam]
	stackName, hasStack := req.Params[actionStackBindingParam]
	configDir, hasDirectory := req.Params[actionDirectoryBindingParam]
	if !hasContainer || !hasStack || !hasDirectory {
		return connectorsdk.ActionResult{}, fmt.Errorf("Docker action has no saved resource binding; resubmit the action")
	}
	resolvedAction, target, err := d.resolveActionOwner(ctx, actionID, req.TargetID)
	if err != nil {
		return connectorsdk.ActionResult{}, err
	}
	if target.AgentID != expected {
		return connectorsdk.ActionResult{}, fmt.Errorf("Docker action target changed its controlling agent; resubmit the action")
	}
	if target.ContainerID != containerID || target.StackName != stackName || target.ConfigDir != configDir {
		return connectorsdk.ActionResult{}, fmt.Errorf("Docker action target changed its resource identity; resubmit the action")
	}
	if target.EndpointID != "" && req.Params[actionEndpointBindingParam] != target.EndpointID {
		return connectorsdk.ActionResult{}, fmt.Errorf("Docker Compose action endpoint binding is missing or changed; resubmit the action")
	}
	if target.StackName != "" {
		var files []string
		if err := json.Unmarshal([]byte(req.Params[actionFilesBindingParam]), &files); err != nil || len(files) == 0 {
			return connectorsdk.ActionResult{}, fmt.Errorf("Docker Compose action has no saved file binding; resubmit the action")
		}
		if !slices.Equal(files, target.ConfigFiles) {
			return connectorsdk.ActionResult{}, fmt.Errorf("Docker Compose action target changed its file selection; resubmit the action")
		}
	}
	if err := d.checkActionOwners(ctx, req.TargetID, target); err != nil {
		return connectorsdk.ActionResult{}, err
	}
	invoke := req
	invoke.Params = maps.Clone(req.Params)
	delete(invoke.Params, ActionAgentBindingParam)
	delete(invoke.Params, actionContainerBindingParam)
	delete(invoke.Params, actionStackBindingParam)
	delete(invoke.Params, actionDirectoryBindingParam)
	delete(invoke.Params, actionFilesBindingParam)
	delete(invoke.Params, actionEndpointBindingParam)
	// The coordinator resolves once more before sending. Preserve the exact
	// checked agent and resource across a discovery change during those checks.
	return d.DockerCoordinator.ExecuteAction(docker.WithActionTarget(ctx, target), resolvedAction, invoke)
}

func (d *Deps) resolveActionOwner(ctx context.Context, actionID, logicalTarget string) (string, docker.ActionTarget, error) {
	if err := ctx.Err(); err != nil {
		return "", docker.ActionTarget{}, err
	}
	if d.DockerCoordinator == nil {
		return "", docker.ActionTarget{}, fmt.Errorf("Docker coordinator unavailable")
	}
	actionID = modelmap.ResolveActionID(actionID, logicalTarget, d.DockerCoordinator.Actions())
	target, err := d.DockerCoordinator.ResolveActionTarget(actionID, logicalTarget)
	return actionID, target, err
}

func (d *Deps) checkActionOwners(ctx context.Context, logicalTarget string, target docker.ActionTarget) error {
	if err := maintenanceguard.CheckAssetActions([]string{logicalTarget, target.AssetID, target.AgentID, target.HostAssetID}, d.EvaluateAssetGuardrails); err != nil {
		return err
	}
	return ctx.Err()
}
