package portainer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// ---------- API Methods ----------

// GetVersion returns Portainer system version info.
func (c *Client) GetVersion(ctx context.Context) (VersionInfo, error) {
	payload, err := c.get(ctx, "/api/system/version")
	if err != nil {
		return VersionInfo{}, err
	}
	var info VersionInfo
	if err := json.Unmarshal(payload, &info); err != nil {
		return VersionInfo{}, fmt.Errorf("decode version response: %w", err)
	}
	return info, nil
}

// GetEndpoints returns Portainer endpoints (environments), capped at maxEndpoints.
func (c *Client) GetEndpoints(ctx context.Context) ([]Endpoint, error) {
	payload, err := c.get(ctx, fmt.Sprintf("/api/endpoints?limit=%d", maxEndpoints))
	if err != nil {
		return nil, err
	}
	var endpoints []Endpoint
	if err := json.Unmarshal(payload, &endpoints); err != nil {
		return nil, fmt.Errorf("decode endpoints response: %w", err)
	}
	if len(endpoints) > maxEndpoints {
		endpoints = endpoints[:maxEndpoints]
	}
	return endpoints, nil
}

// GetContainers returns Docker containers for the given endpoint, capped at maxContainersPerEndpoint.
func (c *Client) GetContainers(ctx context.Context, endpointID int) ([]Container, error) {
	path := fmt.Sprintf("/api/endpoints/%s/docker/containers/json?all=true&limit=%d",
		url.PathEscape(fmt.Sprintf("%d", endpointID)),
		maxContainersPerEndpoint,
	)
	payload, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}
	var containers []Container
	if err := json.Unmarshal(payload, &containers); err != nil {
		return nil, fmt.Errorf("decode containers response: %w", err)
	}
	if len(containers) > maxContainersPerEndpoint {
		containers = containers[:maxContainersPerEndpoint]
	}
	return containers, nil
}

// ContainerAction performs an action (start, stop, restart, kill, pause, unpause) on a container.
func (c *Client) ContainerAction(ctx context.Context, endpointID int, containerID, action string) error {
	path := fmt.Sprintf("/api/endpoints/%s/docker/containers/%s/%s",
		url.PathEscape(fmt.Sprintf("%d", endpointID)),
		url.PathEscape(containerID),
		url.PathEscape(action),
	)
	_, err := c.post(ctx, path, nil)
	return err
}

// RemoveContainer deletes a container.
func (c *Client) RemoveContainer(ctx context.Context, endpointID int, containerID string, force bool) error {
	path := fmt.Sprintf("/api/endpoints/%s/docker/containers/%s?force=%t",
		url.PathEscape(fmt.Sprintf("%d", endpointID)),
		url.PathEscape(containerID),
		force,
	)
	_, err := c.del(ctx, path)
	return err
}

// GetStacks returns all Portainer stacks.
func (c *Client) GetStacks(ctx context.Context) ([]Stack, error) {
	payload, err := c.get(ctx, "/api/stacks")
	if err != nil {
		return nil, err
	}
	var stacks []Stack
	if err := json.Unmarshal(payload, &stacks); err != nil {
		return nil, fmt.Errorf("decode stacks response: %w", err)
	}
	return stacks, nil
}

// StartStack starts a stopped stack.
func (c *Client) StartStack(ctx context.Context, stackID, endpointID int) error {
	path := fmt.Sprintf("/api/stacks/%s/start?endpointId=%d",
		url.PathEscape(fmt.Sprintf("%d", stackID)),
		endpointID,
	)
	_, err := c.post(ctx, path, nil)
	return err
}

// StopStack stops a running stack.
func (c *Client) StopStack(ctx context.Context, stackID, endpointID int) error {
	path := fmt.Sprintf("/api/stacks/%s/stop?endpointId=%d",
		url.PathEscape(fmt.Sprintf("%d", stackID)),
		endpointID,
	)
	_, err := c.post(ctx, path, nil)
	return err
}

// RedeployStack redeploys a git-based stack, optionally pulling new images.
func (c *Client) RedeployStack(ctx context.Context, stackID, endpointID int, pullImage bool) error {
	path := fmt.Sprintf("/api/stacks/%s/git/redeploy?endpointId=%d",
		url.PathEscape(fmt.Sprintf("%d", stackID)),
		endpointID,
	)
	body := map[string]any{
		"pullImage": pullImage,
	}
	_, err := c.put(ctx, path, body)
	return err
}

// RemoveStack deletes a stack.
func (c *Client) RemoveStack(ctx context.Context, stackID, endpointID int) error {
	path := fmt.Sprintf("/api/stacks/%s?endpointId=%d",
		url.PathEscape(fmt.Sprintf("%d", stackID)),
		endpointID,
	)
	_, err := c.del(ctx, path)
	return err
}

// GetContainerLogs returns stdout/stderr logs for a container.
func (c *Client) GetContainerLogs(ctx context.Context, endpointID int, containerID string, tail int, timestamps bool) (string, error) {
	path := fmt.Sprintf("/api/endpoints/%s/docker/containers/%s/logs?stdout=true&stderr=true&tail=%d&timestamps=%t",
		url.PathEscape(fmt.Sprintf("%d", endpointID)),
		url.PathEscape(containerID),
		tail,
		timestamps,
	)
	payload, err := c.get(ctx, path)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

// InspectContainer returns detailed inspection data for a container.
func (c *Client) InspectContainer(ctx context.Context, endpointID int, containerID string) (json.RawMessage, error) {
	path := fmt.Sprintf("/api/endpoints/%s/docker/containers/%s/json",
		url.PathEscape(fmt.Sprintf("%d", endpointID)),
		url.PathEscape(containerID),
	)
	payload, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(payload), nil
}

// GetImages returns all Docker images for the given endpoint.
func (c *Client) GetImages(ctx context.Context, endpointID int) ([]json.RawMessage, error) {
	path := fmt.Sprintf("/api/endpoints/%s/docker/images/json?all=false",
		url.PathEscape(fmt.Sprintf("%d", endpointID)),
	)
	payload, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}
	var images []json.RawMessage
	if err := json.Unmarshal(payload, &images); err != nil {
		return nil, fmt.Errorf("decode images response: %w", err)
	}
	return images, nil
}

// PullImage pulls a Docker image on the given endpoint.
func (c *Client) PullImage(ctx context.Context, endpointID int, image string) error {
	path := fmt.Sprintf("/api/endpoints/%s/docker/images/create?fromImage=%s",
		url.PathEscape(fmt.Sprintf("%d", endpointID)),
		url.QueryEscape(image),
	)
	_, err := c.post(ctx, path, nil)
	return err
}

// RemoveImage removes a Docker image from the given endpoint.
func (c *Client) RemoveImage(ctx context.Context, endpointID int, imageID string) error {
	path := fmt.Sprintf("/api/endpoints/%s/docker/images/%s",
		url.PathEscape(fmt.Sprintf("%d", endpointID)),
		url.PathEscape(imageID),
	)
	_, err := c.del(ctx, path)
	return err
}

// GetVolumes returns all Docker volumes for the given endpoint.
// The response is returned as raw JSON; the Docker API wraps volumes in a {"Volumes": [...]} object.
func (c *Client) GetVolumes(ctx context.Context, endpointID int) (json.RawMessage, error) {
	path := fmt.Sprintf("/api/endpoints/%s/docker/volumes",
		url.PathEscape(fmt.Sprintf("%d", endpointID)),
	)
	payload, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(payload), nil
}

// CreateVolume creates a Docker volume on the given endpoint.
func (c *Client) CreateVolume(ctx context.Context, endpointID int, name, driver string) (json.RawMessage, error) {
	path := fmt.Sprintf("/api/endpoints/%s/docker/volumes/create",
		url.PathEscape(fmt.Sprintf("%d", endpointID)),
	)
	body := map[string]any{
		"Name":   name,
		"Driver": driver,
	}
	payload, err := c.post(ctx, path, body)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(payload), nil
}

// RemoveVolume removes a Docker volume from the given endpoint.
func (c *Client) RemoveVolume(ctx context.Context, endpointID int, volumeName string) error {
	path := fmt.Sprintf("/api/endpoints/%s/docker/volumes/%s",
		url.PathEscape(fmt.Sprintf("%d", endpointID)),
		url.PathEscape(volumeName),
	)
	_, err := c.del(ctx, path)
	return err
}

// GetNetworks returns all Docker networks for the given endpoint.
func (c *Client) GetNetworks(ctx context.Context, endpointID int) ([]json.RawMessage, error) {
	path := fmt.Sprintf("/api/endpoints/%s/docker/networks",
		url.PathEscape(fmt.Sprintf("%d", endpointID)),
	)
	payload, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}
	var networks []json.RawMessage
	if err := json.Unmarshal(payload, &networks); err != nil {
		return nil, fmt.Errorf("decode networks response: %w", err)
	}
	return networks, nil
}

// CreateNetwork creates a Docker network on the given endpoint.
// If subnet and gateway are non-empty, IPAM config is included in the request.
func (c *Client) CreateNetwork(ctx context.Context, endpointID int, name, driver, subnet, gateway string) (json.RawMessage, error) {
	path := fmt.Sprintf("/api/endpoints/%s/docker/networks/create",
		url.PathEscape(fmt.Sprintf("%d", endpointID)),
	)
	body := map[string]any{
		"Name":   name,
		"Driver": driver,
	}
	if subnet != "" || gateway != "" {
		body["IPAM"] = map[string]any{
			"Config": []map[string]any{
				{
					"Subnet":  subnet,
					"Gateway": gateway,
				},
			},
		}
	}
	payload, err := c.post(ctx, path, body)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(payload), nil
}

// RemoveNetwork removes a Docker network from the given endpoint.
func (c *Client) RemoveNetwork(ctx context.Context, endpointID int, networkID string) error {
	path := fmt.Sprintf("/api/endpoints/%s/docker/networks/%s",
		url.PathEscape(fmt.Sprintf("%d", endpointID)),
		url.PathEscape(networkID),
	)
	_, err := c.del(ctx, path)
	return err
}

// GetStackCompose returns the compose file content for a stack.
func (c *Client) GetStackCompose(ctx context.Context, stackID int) (string, error) {
	path := fmt.Sprintf("/api/stacks/%s/file",
		url.PathEscape(fmt.Sprintf("%d", stackID)),
	)
	payload, err := c.get(ctx, path)
	if err != nil {
		return "", err
	}
	var result struct {
		StackFileContent string `json:"StackFileContent"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		return "", fmt.Errorf("decode stack file response: %w", err)
	}
	return result.StackFileContent, nil
}

// UpdateStackCompose updates the compose file content for a stack.
func (c *Client) UpdateStackCompose(ctx context.Context, stackID, endpointID int, composeContent string) error {
	path := fmt.Sprintf("/api/stacks/%s?endpointId=%d",
		url.PathEscape(fmt.Sprintf("%d", stackID)),
		endpointID,
	)
	body := map[string]any{
		"StackFileContent": composeContent,
		"Prune":            false,
	}
	_, err := c.put(ctx, path, body)
	return err
}
