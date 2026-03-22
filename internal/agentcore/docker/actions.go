package docker

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

// validContainerID matches Docker container IDs and names.
// Docker IDs are hex strings (12 or 64 chars); names may also contain dots,
// underscores, and hyphens. Length is capped at 128 to prevent oversized inputs.
var validContainerID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.\-]*$`)

func isValidContainerID(id string) bool {
	return len(id) > 0 && len(id) <= 128 && validContainerID.MatchString(id)
}

const dockerActionTimeout = 30 * time.Second

// handleDockerAction dispatches a Docker lifecycle action and sends the result back.
func (dc *DockerCollector) HandleDockerAction(transport Transport, msg agentmgr.Message) {
	var req agentmgr.DockerActionData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("docker: invalid action payload: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), dockerActionTimeout)
	defer cancel()

	var actionErr error
	var actionData string

	// Validate container ID for actions that embed it in Docker API URL paths.
	// container.create is exempt — the ID is produced server-side.
	switch req.Action {
	case "container.start", "container.stop", "container.restart",
		"container.kill", "container.remove", "container.pause",
		"container.unpause", "container.logs":
		if !isValidContainerID(req.ContainerID) {
			actionErr = fmt.Errorf("invalid container ID %q", req.ContainerID)
		}
	}

	if actionErr == nil {
		actionData, actionErr = dc.dispatchDockerAction(ctx, req)
	}

	if actionErr == nil && req.Action != "container.logs" {
		// Push a fast post-action state refresh so UI inventory updates do not wait
		// for the next scheduled discovery tick.
		dc.refreshDockerStateSoon()
	}

	result := agentmgr.DockerActionResultData{
		RequestID: req.RequestID,
		Success:   actionErr == nil,
		Data:      actionData,
	}
	if actionErr != nil {
		result.Error = actionErr.Error()
	}

	data, err := json.Marshal(result)
	if err != nil {
		log.Printf("docker: failed to marshal action result: %v", err)
		return
	}

	if err := transport.Send(agentmgr.Message{
		Type: agentmgr.MsgDockerActionResult,
		ID:   req.RequestID,
		Data: data,
	}); err != nil {
		log.Printf("docker: failed to send action result: %v", err)
	}
}

// dispatchDockerAction executes the Docker action described by req and returns
// any string data payload and action error.
func (dc *DockerCollector) dispatchDockerAction(ctx context.Context, req agentmgr.DockerActionData) (string, error) {
	switch req.Action {
	case "container.create":
		imageRef := strings.TrimSpace(req.Params["image"])
		if imageRef == "" {
			return "", fmt.Errorf("image is required")
		}
		containerName := strings.TrimSpace(req.Params["name"])
		cmd := splitDockerParamList(req.Params["command"], " ")
		env := splitDockerParamList(req.Params["env"], ",")
		portBindings := parseDockerPortBindings(req.Params["ports"])

		containerID, createErr := dc.client.createContainer(ctx, DockerContainerCreateRequest{
			Name:         containerName,
			Image:        imageRef,
			Command:      cmd,
			Environment:  env,
			PortBindings: portBindings,
		})
		if createErr != nil {
			return "", createErr
		}
		if startErr := dc.client.startContainer(ctx, containerID); startErr != nil {
			return "", fmt.Errorf("container created (%s) but failed to start: %w", containerID, startErr)
		}
		return containerID, nil

	case "container.start":
		return "", dc.client.startContainer(ctx, req.ContainerID)

	case "container.stop":
		timeout := 10
		if v, ok := req.Params["timeout"]; ok {
			if parsed, err := parseInt(v); err == nil {
				timeout = parsed
			}
		}
		return "", dc.client.stopContainer(ctx, req.ContainerID, timeout)

	case "container.restart":
		timeout := 10
		if v, ok := req.Params["timeout"]; ok {
			if parsed, err := parseInt(v); err == nil {
				timeout = parsed
			}
		}
		return "", dc.client.restartContainer(ctx, req.ContainerID, timeout)

	case "container.kill":
		signal := req.Params["signal"]
		if signal == "" {
			signal = "SIGKILL"
		}
		return "", dc.client.killContainer(ctx, req.ContainerID, signal)

	case "container.remove":
		force := req.Params["force"] == "true"
		return "", dc.client.removeContainer(ctx, req.ContainerID, force)

	case "container.pause":
		return "", dc.client.pauseContainer(ctx, req.ContainerID)

	case "container.unpause":
		return "", dc.client.unpauseContainer(ctx, req.ContainerID)

	case "container.logs":
		tail := 200
		if v, ok := req.Params["tail"]; ok {
			if parsed, err := parseInt(v); err == nil {
				tail = parsed
			}
		}
		if tail <= 0 {
			tail = 200
		}
		if tail > 5000 {
			tail = 5000
		}
		timestamps := strings.EqualFold(strings.TrimSpace(req.Params["timestamps"]), "true")
		body, logsErr := dc.client.containerLogs(ctx, req.ContainerID, tail, false, timestamps)
		if logsErr != nil {
			return "", logsErr
		}
		defer body.Close()
		content, readErr := io.ReadAll(io.LimitReader(body, 2*1024*1024))
		if readErr != nil {
			return "", readErr
		}
		return strings.TrimSpace(decodeDockerLogPayload(content)), nil

	case "image.pull":
		ref := req.ImageRef
		if ref == "" {
			ref = req.Params["image"]
		}
		if ref == "" {
			return "", fmt.Errorf("image reference required")
		}
		return "", dc.client.pullImage(ctx, ref)

	case "image.remove":
		ref := req.ImageRef
		if ref == "" {
			ref = req.Params["image"]
		}
		force := req.Params["force"] == "true"
		return "", dc.client.removeImage(ctx, ref, force)

	default:
		return "", fmt.Errorf("unsupported Docker action: %s", req.Action)
	}
}

// parseInt is a simple string-to-int converter for action params.
func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

func splitDockerParamList(raw string, preferredSeparator string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	var parts []string
	if preferredSeparator == "," {
		parts = strings.FieldsFunc(trimmed, func(r rune) bool {
			return r == ',' || r == '\n' || r == '\r'
		})
	} else if strings.Contains(trimmed, "\n") || strings.Contains(trimmed, "\r") {
		parts = strings.FieldsFunc(trimmed, func(r rune) bool {
			return r == '\n' || r == '\r'
		})
	} else if preferredSeparator == "," {
		parts = []string{trimmed}
	} else {
		parts = strings.Fields(trimmed)
	}

	values := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item != "" {
			values = append(values, item)
		}
	}
	return values
}

func parseDockerPortBindings(raw string) []DockerPortBinding {
	entries := splitDockerParamList(raw, ",")
	if len(entries) == 0 {
		return nil
	}

	result := make([]DockerPortBinding, 0, len(entries))
	for _, entry := range entries {
		parts := strings.Split(entry, ":")
		if len(parts) != 2 {
			continue
		}
		hostPort := strings.TrimSpace(parts[0])
		containerPart := strings.TrimSpace(parts[1])
		if hostPort == "" || containerPart == "" {
			continue
		}

		containerPort := containerPart
		protocol := "tcp"
		if strings.Contains(containerPart, "/") {
			portParts := strings.SplitN(containerPart, "/", 2)
			containerPort = strings.TrimSpace(portParts[0])
			protocol = strings.TrimSpace(portParts[1])
			if protocol == "" {
				protocol = "tcp"
			}
		}
		if containerPort == "" {
			continue
		}

		result = append(result, DockerPortBinding{
			HostPort:      hostPort,
			ContainerPort: containerPort,
			Protocol:      strings.ToLower(protocol),
		})
	}
	return result
}

func decodeDockerLogPayload(raw []byte) string {
	if len(raw) < 8 {
		return string(raw)
	}

	var (
		out    bytes.Buffer
		offset int
		parsed bool
	)
	for offset+8 <= len(raw) {
		// docker attach/log stream framing:
		// [stream:1][0:3][size:4 big-endian][payload:size]
		if raw[offset+1] != 0 || raw[offset+2] != 0 || raw[offset+3] != 0 {
			break
		}
		frameSize := int(binary.BigEndian.Uint32(raw[offset+4 : offset+8]))
		if frameSize < 0 || offset+8+frameSize > len(raw) {
			break
		}
		out.Write(raw[offset+8 : offset+8+frameSize])
		offset += 8 + frameSize
		parsed = true
	}
	if parsed && out.Len() > 0 {
		return out.String()
	}
	return string(raw)
}

func (dc *DockerCollector) refreshDockerStateSoon() {
	if dc == nil || dc.client == nil || dc.transport == nil {
		return
	}
	dc.queueDiscoveryTrigger(true, true)
	dc.queueStatsTrigger()
}
