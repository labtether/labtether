package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/connectorsdk"
)

const (
	actionTimeout  = 30 * time.Second
	composeTimeout = 5 * time.Minute
)

// Actions returns the full Docker action catalog.
func (c *Coordinator) Actions() []connectorsdk.ActionDescriptor {
	return []connectorsdk.ActionDescriptor{
		{
			ID: "container.create", Name: "Create Container", RequiresTarget: true,
			Description: "Create and start a new container on the selected Docker host.",
			Parameters: []connectorsdk.ActionParameter{
				{Key: "image", Label: "Image", Required: true, Description: "Image reference (e.g. nginx:latest)"},
				{Key: "name", Label: "Name", Description: "Optional container name"},
				{Key: "command", Label: "Command", Description: "Optional command override"},
				{Key: "env", Label: "Environment", Description: "Comma or newline-separated KEY=VALUE entries"},
				{Key: "ports", Label: "Ports", Description: "Comma-separated host:container mappings (e.g. 8080:80,9443:9443)"},
			},
		},
		{ID: "container.start", Name: "Start Container", RequiresTarget: true},
		{
			ID: "container.stop", Name: "Stop Container", RequiresTarget: true,
			Parameters: []connectorsdk.ActionParameter{
				{Key: "timeout", Label: "Stop Timeout (seconds)", Description: "Seconds to wait before killing"},
			},
		},
		{
			ID: "container.restart", Name: "Restart Container", RequiresTarget: true,
			Parameters: []connectorsdk.ActionParameter{
				{Key: "timeout", Label: "Restart Timeout (seconds)", Description: "Seconds to wait before killing"},
			},
		},
		{
			ID: "container.kill", Name: "Kill Container", RequiresTarget: true,
			Parameters: []connectorsdk.ActionParameter{
				{Key: "signal", Label: "Signal", Description: "Signal to send (default: SIGKILL)"},
			},
		},
		{
			ID: "container.remove", Name: "Remove Container", RequiresTarget: true,
			Parameters: []connectorsdk.ActionParameter{
				{Key: "force", Label: "Force", Description: "Force remove running container (true/false)"},
			},
		},
		{
			ID: "container.logs", Name: "Fetch Container Logs", RequiresTarget: true,
			Description: "Fetch recent log lines from a container.",
			Parameters: []connectorsdk.ActionParameter{
				{Key: "tail", Label: "Tail", Description: "Number of lines to return (default: 200)"},
				{Key: "timestamps", Label: "Timestamps", Description: "Include timestamps (true/false)"},
			},
		},
		{ID: "container.pause", Name: "Pause Container", RequiresTarget: true},
		{ID: "container.unpause", Name: "Unpause Container", RequiresTarget: true},
		{
			ID: "image.pull", Name: "Pull Image",
			Parameters: []connectorsdk.ActionParameter{
				{Key: "image", Label: "Image", Required: true, Description: "Image reference (e.g. nginx:latest)"},
			},
		},
		{
			ID: "image.remove", Name: "Remove Image", RequiresTarget: true,
			Parameters: []connectorsdk.ActionParameter{
				{Key: "force", Label: "Force", Description: "Force remove (true/false)"},
			},
		},
		{ID: "stack.up", Name: "Stack Up", RequiresTarget: true, Description: "Start all containers in a compose stack"},
		{ID: "stack.down", Name: "Stack Down", RequiresTarget: true, Description: "Stop and remove all containers in a compose stack"},
		{ID: "stack.restart", Name: "Stack Restart", RequiresTarget: true, Description: "Restart all containers in a compose stack"},
		{ID: "stack.pull", Name: "Stack Pull", RequiresTarget: true, Description: "Pull latest images for a compose stack"},
		{
			ID: "stack.deploy", Name: "Deploy Stack", RequiresTarget: true,
			Description: "Deploy a new Compose stack from provided YAML on the selected host.",
			Parameters: []connectorsdk.ActionParameter{
				{Key: "stack_name", Label: "Stack Name", Required: true, Description: "Stack/project name"},
				{Key: "compose_yaml", Label: "Compose YAML", Required: true, Description: "docker-compose content"},
				{Key: "config_dir", Label: "Config Directory", Description: "Optional host directory to persist compose file"},
			},
		},
	}
}

// ExecuteAction routes a Docker action to the correct agent via WebSocket.
func (c *Coordinator) ExecuteAction(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	if c.agentMgr == nil {
		return connectorsdk.ActionResult{Status: "failed", Message: "agent manager not configured"}, nil
	}

	if isStackAction(actionID) {
		return c.executeStackAction(ctx, actionID, req)
	}

	return c.executeContainerAction(ctx, actionID, req)
}

// executeContainerAction handles container and image lifecycle actions.
func (c *Coordinator) executeContainerAction(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	var (
		agentID     string
		containerID string
		err         error
	)
	switch actionID {
	case "container.create", "image.pull", "image.remove":
		agentID, err = c.resolveHostTarget(req.TargetID)
	default:
		agentID, containerID, err = c.resolveContainerTarget(req.TargetID)
	}
	if err != nil {
		return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
	}

	requestID := fmt.Sprintf("docker-action-%d", time.Now().UnixNano())

	actionData := agentmgr.DockerActionData{
		RequestID:   requestID,
		Action:      actionID,
		ContainerID: containerID,
		ImageRef:    req.Params["image"],
		Params:      req.Params,
	}
	data, err := json.Marshal(actionData)
	if err != nil {
		return connectorsdk.ActionResult{Status: "failed", Message: "failed to encode action payload: " + err.Error()}, nil
	}

	// Register pending channel before sending to avoid a race where the result
	// arrives before we register the channel.
	resultCh := make(chan agentmgr.DockerActionResultData, 1)
	c.pendingMu.Lock()
	c.pendingResults[requestID] = resultCh
	c.pendingMu.Unlock()

	defer func() {
		c.pendingMu.Lock()
		delete(c.pendingResults, requestID)
		c.pendingMu.Unlock()
	}()

	if err := c.agentMgr.SendToAgent(agentID, agentmgr.Message{
		Type: agentmgr.MsgDockerAction,
		ID:   requestID,
		Data: data,
	}); err != nil {
		return connectorsdk.ActionResult{Status: "failed", Message: "failed to send to agent: " + err.Error()}, nil
	}

	select {
	case result := <-resultCh:
		status := "succeeded"
		if !result.Success {
			status = "failed"
		}
		msg := actionID + " completed"
		if result.Error != "" {
			msg = result.Error
		}
		return connectorsdk.ActionResult{Status: status, Message: msg, Output: result.Data}, nil
	case <-time.After(actionTimeout):
		return connectorsdk.ActionResult{Status: "failed", Message: "action timed out waiting for agent response"}, nil
	case <-ctx.Done():
		return connectorsdk.ActionResult{Status: "failed", Message: "action cancelled"}, nil
	}
}

// executeStackAction handles docker-compose stack lifecycle actions.
func (c *Coordinator) executeStackAction(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	var (
		agentID   string
		stackName string
		configDir string
		err       error
	)
	if actionID == "stack.deploy" {
		agentID, err = c.resolveHostTarget(req.TargetID)
		if err != nil {
			return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
		}
		stackName = strings.TrimSpace(req.Params["stack_name"])
		if stackName == "" {
			return connectorsdk.ActionResult{Status: "failed", Message: "stack_name is required"}, nil
		}
		configDir = strings.TrimSpace(req.Params["config_dir"])
	} else {
		agentID, stackName, configDir, err = c.resolveStackTarget(req.TargetID)
		if err != nil {
			return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
		}
	}

	// Map stack.xxx to compose action name.
	var composeAction string
	switch actionID {
	case "stack.up":
		composeAction = "up"
	case "stack.down":
		composeAction = "down"
	case "stack.restart":
		composeAction = "restart"
	case "stack.pull":
		composeAction = "pull"
	case "stack.deploy":
		composeAction = "deploy"
	default:
		return connectorsdk.ActionResult{Status: "failed", Message: "unknown stack action: " + actionID}, nil
	}

	requestID := fmt.Sprintf("docker-compose-%d", time.Now().UnixNano())

	composeData := agentmgr.DockerComposeActionData{
		RequestID:   requestID,
		StackName:   stackName,
		Action:      composeAction,
		ConfigDir:   configDir,
		ComposeYAML: req.Params["compose_yaml"],
	}
	data, err := json.Marshal(composeData)
	if err != nil {
		return connectorsdk.ActionResult{Status: "failed", Message: "failed to encode compose payload: " + err.Error()}, nil
	}

	resultCh := make(chan agentmgr.DockerComposeResultData, 1)
	c.pendingMu.Lock()
	c.pendingCompose[requestID] = resultCh
	c.pendingMu.Unlock()

	defer func() {
		c.pendingMu.Lock()
		delete(c.pendingCompose, requestID)
		c.pendingMu.Unlock()
	}()

	if err := c.agentMgr.SendToAgent(agentID, agentmgr.Message{
		Type: agentmgr.MsgDockerComposeAction,
		ID:   requestID,
		Data: data,
	}); err != nil {
		return connectorsdk.ActionResult{Status: "failed", Message: "failed to send to agent: " + err.Error()}, nil
	}

	// Compose operations (image pulls, full stack restarts) can take significantly longer.
	select {
	case result := <-resultCh:
		status := "succeeded"
		if !result.Success {
			status = "failed"
		}
		msg := fmt.Sprintf("stack %s %s completed", stackName, composeAction)
		if result.Error != "" {
			msg = result.Error
		}
		return connectorsdk.ActionResult{Status: status, Message: msg, Output: result.Output}, nil
	case <-time.After(composeTimeout):
		return connectorsdk.ActionResult{Status: "failed", Message: "compose action timed out"}, nil
	case <-ctx.Done():
		return connectorsdk.ActionResult{Status: "failed", Message: "action cancelled"}, nil
	}
}

// isStackAction returns true if the action ID is a compose stack operation.
func isStackAction(actionID string) bool {
	switch actionID {
	case "stack.up", "stack.down", "stack.restart", "stack.pull", "stack.deploy":
		return true
	}
	return false
}
