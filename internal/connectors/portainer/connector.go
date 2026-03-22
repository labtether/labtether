package portainer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/connectorsdk"
)

// Connector implements connectorsdk.Connector for Portainer.
// When client is nil (or unconfigured) the connector runs in stub mode,
// returning synthetic assets so the registry can be populated without
// requiring a live Portainer instance.
type Connector struct {
	client *Client
}

// New creates a Connector from environment variables.
// Returns a stub connector if PORTAINER_BASE_URL is empty.
func New() *Connector {
	baseURL := strings.TrimSpace(os.Getenv("PORTAINER_BASE_URL"))
	if baseURL == "" {
		return &Connector{}
	}

	timeout := 10 * time.Second
	if raw := strings.TrimSpace(os.Getenv("PORTAINER_HTTP_TIMEOUT")); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
			timeout = parsed
		}
	}

	skipVerify := false
	if raw := strings.TrimSpace(os.Getenv("PORTAINER_SKIP_VERIFY")); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			skipVerify = parsed
		}
	}

	client := NewClient(Config{
		BaseURL:    baseURL,
		APIKey:     strings.TrimSpace(os.Getenv("PORTAINER_API_KEY")),
		Username:   strings.TrimSpace(os.Getenv("PORTAINER_USERNAME")),
		Password:   os.Getenv("PORTAINER_PASSWORD"),
		SkipVerify: skipVerify,
		Timeout:    timeout,
	})

	return &Connector{client: client}
}

// NewWithClient creates a Connector with a pre-configured client (for testing).
func NewWithClient(client *Client) *Connector {
	return &Connector{client: client}
}

// ID returns the unique connector identifier.
func (c *Connector) ID() string {
	return "portainer"
}

// DisplayName returns the human-readable connector name.
func (c *Connector) DisplayName() string {
	return "Portainer"
}

// Capabilities returns the set of operations this connector supports.
func (c *Connector) Capabilities() connectorsdk.Capabilities {
	return connectorsdk.Capabilities{
		DiscoverAssets: true,
		CollectMetrics: true,
		CollectEvents:  true,
		ExecuteActions: true,
	}
}

// isConfigured reports whether the connector has a usable client.
func (c *Connector) isConfigured() bool {
	return c.client != nil && c.client.IsConfigured()
}

// TestConnection verifies that the Portainer API is reachable.
func (c *Connector) TestConnection(ctx context.Context) (connectorsdk.Health, error) {
	if !c.isConfigured() {
		return connectorsdk.Health{
			Status:  "ok",
			Message: "portainer connector running in stub mode (missing PORTAINER_BASE_URL)",
		}, nil
	}

	info, err := c.client.GetVersion(ctx)
	if err != nil {
		return connectorsdk.Health{Status: "failed", Message: err.Error()}, nil
	}

	endpoints, err := c.client.GetEndpoints(ctx)
	if err != nil {
		return connectorsdk.Health{Status: "failed", Message: fmt.Sprintf("portainer endpoints: %v", err)}, nil
	}
	if len(endpoints) == 0 {
		return connectorsdk.Health{
			Status:  "failed",
			Message: "portainer API reachable but no endpoints are visible to this credential",
		}, nil
	}

	version := strings.TrimSpace(info.ServerVersion)
	endpointLabel := "endpoints"
	if len(endpoints) == 1 {
		endpointLabel = "endpoint"
	}
	if version == "" {
		return connectorsdk.Health{
			Status:  "ok",
			Message: fmt.Sprintf("portainer API reachable (%d %s available)", len(endpoints), endpointLabel),
		}, nil
	}
	return connectorsdk.Health{
		Status:  "ok",
		Message: fmt.Sprintf("portainer API reachable (v%s, %d %s available)", version, len(endpoints), endpointLabel),
	}, nil
}

// Discover queries Portainer for endpoints, containers, and stacks, returning
// them as connectorsdk.Asset values. Per-endpoint container failures and stack
// failures are logged and skipped rather than failing the whole discovery run.
func (c *Connector) Discover(ctx context.Context) ([]connectorsdk.Asset, error) {
	if !c.isConfigured() {
		return c.stubAssets(), nil
	}

	assets := make([]connectorsdk.Asset, 0, 32)

	// Step 1: Endpoints (container hosts).
	endpoints, err := c.client.GetEndpoints(ctx)
	if err != nil {
		return nil, fmt.Errorf("portainer endpoints: %w", err)
	}

	for _, ep := range endpoints {
		assets = append(assets, connectorsdk.Asset{
			ID:     fmt.Sprintf("portainer-endpoint-%d", ep.ID),
			Type:   "container-host",
			Name:   ep.Name,
			Source: c.ID(),
			Metadata: map[string]string{
				"endpoint_id": strconv.Itoa(ep.ID),
				"name":        ep.Name,
				"type":        endpointTypeString(ep.Type),
				"url":         ep.URL,
				"status":      endpointStatusString(ep.Status),
			},
		})

		// Step 2: Containers per endpoint.
		containers, err := c.client.GetContainers(ctx, ep.ID)
		if err != nil {
			log.Printf("portainer: failed to list containers for endpoint %d (%s): %v", ep.ID, ep.Name, err)
			continue
		}

		for _, ctr := range containers {
			shortID := ctr.ID
			if len(shortID) > 12 {
				shortID = shortID[:12]
			}

			name := ""
			if len(ctr.Names) > 0 {
				name = strings.TrimPrefix(ctr.Names[0], "/")
			}
			if name == "" {
				name = shortID
			}

			stack := ctr.Labels["com.docker.compose.project"]
			labelsJSON := ""
			if len(ctr.Labels) > 0 {
				if encoded, err := json.Marshal(ctr.Labels); err == nil {
					labelsJSON = string(encoded)
				}
			}

			metadata := map[string]string{
				"endpoint_id":  strconv.Itoa(ep.ID),
				"container_id": ctr.ID,
				"image":        ctr.Image,
				"state":        ctr.State,
				"status":       ctr.Status,
				"stack":        stack,
			}
			if ctr.Created > 0 {
				metadata["created_at"] = time.Unix(ctr.Created, 0).UTC().Format(time.RFC3339)
			}
			if ports := formatContainerPorts(ctr.Ports); ports != "" {
				metadata["ports"] = ports
			}
			if labelsJSON != "" {
				metadata["labels_json"] = labelsJSON
			}

			assets = append(assets, connectorsdk.Asset{
				ID:       fmt.Sprintf("portainer-container-%d-%s", ep.ID, shortID),
				Type:     "container",
				Name:     name,
				Source:   c.ID(),
				Metadata: metadata,
			})
		}
	}

	// Step 3: Stacks.
	stacks, err := c.client.GetStacks(ctx)
	if err != nil {
		log.Printf("portainer: failed to list stacks: %v", err)
	} else {
		for _, s := range stacks {
			meta := map[string]string{
				"stack_id":    strconv.Itoa(s.ID),
				"endpoint_id": strconv.Itoa(s.EndpointID),
				"name":        s.Name,
				"type":        stackTypeString(s.Type),
				"status":      stackStatusString(s.Status),
				"entry_point": s.EntryPoint,
				"created_by":  s.CreatedBy,
			}
			if s.GitConfig != nil {
				meta["git_url"] = s.GitConfig.URL
			}

			assets = append(assets, connectorsdk.Asset{
				ID:       fmt.Sprintf("portainer-stack-%d", s.ID),
				Type:     "stack",
				Name:     s.Name,
				Source:   c.ID(),
				Metadata: meta,
			})
		}
	}

	if len(assets) == 0 {
		return c.stubAssets(), nil
	}
	return assets, nil
}

// stubAssets returns a single synthetic asset used in stub mode so that the
// connector is visible in the UI without requiring a live Portainer instance.
func (c *Connector) stubAssets() []connectorsdk.Asset {
	return []connectorsdk.Asset{
		{
			ID:     "portainer-endpoint-stub",
			Type:   "container-host",
			Name:   "portainer-stub",
			Source: "portainer",
			Metadata: map[string]string{
				"note": "stub mode — configure PORTAINER_BASE_URL and auth credentials",
			},
		},
	}
}

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

func formatContainerPorts(ports []ContainerPort) string {
	if len(ports) == 0 {
		return ""
	}

	formatted := make([]string, 0, len(ports))
	for _, port := range ports {
		switch {
		case port.PublicPort > 0 && port.PrivatePort > 0:
			formatted = append(formatted, fmt.Sprintf("%d->%d/%s", port.PublicPort, port.PrivatePort, normalizePortProtocol(port.Type)))
		case port.PrivatePort > 0:
			formatted = append(formatted, fmt.Sprintf("%d/%s", port.PrivatePort, normalizePortProtocol(port.Type)))
		}
	}
	return strings.Join(formatted, ", ")
}

func normalizePortProtocol(raw string) string {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return "tcp"
	}
	return value
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

// endpointTypeString maps Portainer endpoint type integers to human-readable strings.
func endpointTypeString(t int) string {
	switch t {
	case 1:
		return "docker"
	case 2:
		return "agent"
	case 3:
		return "azure"
	case 4:
		return "edge-agent"
	case 5:
		return "kubernetes"
	default:
		return fmt.Sprintf("unknown(%d)", t)
	}
}

// endpointStatusString maps Portainer endpoint status integers to strings.
func endpointStatusString(s int) string {
	switch s {
	case 1:
		return "up"
	case 2:
		return "down"
	default:
		return fmt.Sprintf("unknown(%d)", s)
	}
}

// stackTypeString maps Portainer stack type integers to human-readable strings.
func stackTypeString(t int) string {
	switch t {
	case 1:
		return "swarm"
	case 2:
		return "compose"
	case 3:
		return "kubernetes"
	default:
		return fmt.Sprintf("unknown(%d)", t)
	}
}

// stackStatusString maps Portainer stack status integers to strings.
func stackStatusString(s int) string {
	switch s {
	case 1:
		return "active"
	case 2:
		return "inactive"
	default:
		return fmt.Sprintf("unknown(%d)", s)
	}
}
