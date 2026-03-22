package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/securityruntime"
)

var (
	dockerComposeCLIDetect         = isDockerComposeCLIAvailable
	dockerComposeNewCommandContext = securityruntime.NewCommandContext
)

const composeActionTimeout = 5 * time.Minute

// isDockerComposeCLIAvailable checks if docker compose (v2) or docker-compose (v1) is available.
func isDockerComposeCLIAvailable() (version int, available bool) {
	// Try docker compose v2 first
	if out, err := securityruntime.CommandCombinedOutput("docker", "compose", "version"); err == nil {
		if strings.Contains(string(out), "Docker Compose") {
			return 2, true
		}
	}
	// Fall back to docker-compose v1
	if _, err := exec.LookPath("docker-compose"); err == nil {
		return 1, true
	}
	return 0, false
}

// composeCommandArgs builds the CLI arguments for a compose command.
// version: 1 for docker-compose, 2 for docker compose
func composeCommandArgs(version int, projectDir string, action string, extraArgs ...string) (cmd string, args []string) {
	switch version {
	case 2:
		cmd = "docker"
		args = []string{"compose", "--project-directory", projectDir}
		args = append(args, action)
		args = append(args, extraArgs...)
	default: // v1
		cmd = "docker-compose"
		args = []string{"--project-directory", projectDir}
		args = append(args, action)
		args = append(args, extraArgs...)
	}
	return cmd, args
}

// handleComposeAction handles a docker.compose.action message from the hub.
func (dc *DockerCollector) HandleComposeAction(transport Transport, msg agentmgr.Message) {
	var req agentmgr.DockerComposeActionData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("docker-compose: invalid action payload: %v", err)
		return
	}

	version, available := dockerComposeCLIDetect()
	if !available {
		sendComposeResult(transport, req.RequestID, false, "", "docker compose CLI not available on this host")
		return
	}

	configDir := req.ConfigDir
	if req.Action != "deploy" {
		if configDir == "" {
			sendComposeResult(transport, req.RequestID, false, "", "config_dir is required")
			return
		}
		// Validate that the directory exists
		if _, err := os.Stat(configDir); err != nil {
			sendComposeResult(transport, req.RequestID, false, "", fmt.Sprintf("config directory not found: %s", configDir))
			return
		}
	}

	var cmdName string
	var cmdArgs []string

	switch req.Action {
	case "up":
		cmdName, cmdArgs = composeCommandArgs(version, configDir, "up", "-d")
	case "down":
		cmdName, cmdArgs = composeCommandArgs(version, configDir, "down")
	case "restart":
		cmdName, cmdArgs = composeCommandArgs(version, configDir, "restart")
	case "pull":
		cmdName, cmdArgs = composeCommandArgs(version, configDir, "pull")
	case "deploy":
		composeYAML := strings.TrimSpace(req.ComposeYAML)
		if composeYAML == "" {
			sendComposeResult(transport, req.RequestID, false, "", "compose_yaml is required")
			return
		}
		stackSlug := sanitizeDockerStackName(req.StackName)
		if stackSlug == "" {
			sendComposeResult(transport, req.RequestID, false, "", "stack_name is required")
			return
		}
		if strings.TrimSpace(configDir) == "" {
			configDir = filepath.Join(os.TempDir(), "labtether-stacks", stackSlug)
		}
		if err := os.MkdirAll(configDir, 0o750); err != nil {
			sendComposeResult(transport, req.RequestID, false, "", fmt.Sprintf("failed to create config directory: %v", err))
			return
		}
		composePath := filepath.Join(configDir, "docker-compose.yml")
		if err := os.WriteFile(composePath, []byte(composeYAML), 0o600); err != nil {
			sendComposeResult(transport, req.RequestID, false, "", fmt.Sprintf("failed to write compose file: %v", err))
			return
		}
		cmdName, cmdArgs = composeCommandArgs(version, configDir, "up", "-d")
	default:
		sendComposeResult(transport, req.RequestID, false, "", fmt.Sprintf("unsupported compose action: %s", req.Action))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), composeActionTimeout)
	defer cancel()

	cmd, err := dockerComposeNewCommandContext(ctx, cmdName, cmdArgs...)
	if err != nil {
		sendComposeResult(transport, req.RequestID, false, "", err.Error())
		return
	}
	cmd.Dir = configDir
	output, err := cmd.CombinedOutput()
	outputStr := strings.TrimSpace(string(output))

	if err != nil {
		sendComposeResult(transport, req.RequestID, false, outputStr, err.Error())
		return
	}

	// Send a quick refreshed discovery/stats snapshot so newly deployed stacks
	// and containers are visible immediately in the hub UI.
	dc.refreshDockerStateSoon()
	sendComposeResult(transport, req.RequestID, true, outputStr, "")
}

func sanitizeDockerStackName(raw string) string {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return ""
	}
	trimmed = strings.ReplaceAll(trimmed, " ", "-")
	trimmed = strings.ReplaceAll(trimmed, ".", "-")
	var builder strings.Builder
	for _, r := range trimmed {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			builder.WriteRune(r)
		}
	}
	return strings.Trim(builder.String(), "-_")
}

func sendComposeResult(transport Transport, requestID string, success bool, output, errMsg string) {
	result := agentmgr.DockerComposeResultData{
		RequestID: requestID,
		Success:   success,
		Output:    output,
		Error:     errMsg,
	}
	data, _ := json.Marshal(result)
	_ = transport.Send(agentmgr.Message{
		Type: agentmgr.MsgDockerComposeResult,
		ID:   requestID,
		Data: data,
	})
}
