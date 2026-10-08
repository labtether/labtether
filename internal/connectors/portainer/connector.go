package portainer

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/labtether/labtether/internal/connectorsdk"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// Connector implements connectorsdk.Connector for Portainer.
type Connector struct {
	client *Client
}

// New creates a Connector from environment variables.
// Returns an unconfigured connector if PORTAINER_BASE_URL is empty.
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
			Status:  "failed",
			Message: "portainer connector is not configured",
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
		return []connectorsdk.Asset{}, nil
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
		return []connectorsdk.Asset{}, nil
	}
	return assets, nil
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
