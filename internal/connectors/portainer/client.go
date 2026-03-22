package portainer

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/securityruntime"
)

const (
	maxEndpoints             = 10
	maxContainersPerEndpoint = 100
	maxResponseBytes         = 32 * 1024 * 1024 // 32 MB
	jwtCacheDuration         = 7*time.Hour + 30*time.Minute
)

// Config defines runtime Portainer API connection settings.
type Config struct {
	BaseURL    string
	APIKey     string // #nosec G117 -- Runtime connector credential, not a hardcoded secret.
	Username   string
	Password   string // #nosec G117 -- Runtime connector credential, not a hardcoded secret.
	SkipVerify bool
	Timeout    time.Duration
}

// Client is a thin Portainer API client supporting both API key and JWT auth.
type Client struct {
	baseURL    string
	apiKey     string
	username   string
	password   string
	httpClient *http.Client

	mu        sync.Mutex
	jwt       string
	jwtExpiry time.Time
}

// Endpoint represents a Portainer environment/endpoint.
type Endpoint struct {
	ID     int    `json:"Id"`
	Name   string `json:"Name"`
	Type   int    `json:"Type"`
	URL    string `json:"URL"`
	Status int    `json:"Status"`
}

// Stack represents a Portainer stack.
type Stack struct {
	ID         int    `json:"Id"`
	Name       string `json:"Name"`
	Type       int    `json:"Type"`
	EndpointID int    `json:"EndpointId"`
	Status     int    `json:"Status"`
	EntryPoint string `json:"EntryPoint"`
	CreatedBy  string `json:"CreatedBy"`
	GitConfig  *struct {
		URL string `json:"URL"`
	} `json:"GitConfig"`
}

// Container represents a Docker container as returned by the Portainer API.
type Container struct {
	ID      string            `json:"Id"`
	Names   []string          `json:"Names"`
	Image   string            `json:"Image"`
	State   string            `json:"State"`
	Status  string            `json:"Status"`
	Created int64             `json:"Created"`
	Ports   []ContainerPort   `json:"Ports"`
	Labels  map[string]string `json:"Labels"`
}

type ContainerPort struct {
	IP          string `json:"IP"`
	PrivatePort int    `json:"PrivatePort"`
	PublicPort  int    `json:"PublicPort"`
	Type        string `json:"Type"`
}

// VersionInfo represents the Portainer system version response.
type VersionInfo struct {
	ServerVersion   string `json:"ServerVersion"`
	DatabaseVersion string `json:"DatabaseVersion"`
	Build           struct {
		BuildNumber string `json:"BuildNumber"`
		GoVersion   string `json:"GoVersion"`
	} `json:"Build"`
}

// NewClient creates a Portainer API client from runtime settings.
func NewClient(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			// #nosec G402 -- user-controlled homelab setting for self-signed certs.
			InsecureSkipVerify: cfg.SkipVerify, //nolint:gosec // #nosec G402 -- user-controlled homelab setting
		},
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 5,
		IdleConnTimeout:     90 * time.Second,
	}

	return &Client{
		baseURL:  strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		apiKey:   strings.TrimSpace(cfg.APIKey),
		username: strings.TrimSpace(cfg.Username),
		password: cfg.Password,
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}
}

// IsConfigured reports whether base URL and credentials are available.
func (c *Client) IsConfigured() bool {
	if c == nil {
		return false
	}
	if c.baseURL == "" {
		return false
	}
	return c.apiKey != "" || (c.username != "" && c.password != "")
}

// ---------- Auth ----------

// authenticate performs JWT authentication via POST /api/auth.
// Caller must hold c.mu.
func (c *Client) authenticate(ctx context.Context) error {
	// map[string]string marshaling is deterministic and non-failing.
	body, _ := json.Marshal(map[string]string{
		"Username": c.username,
		"Password": c.password,
	})

	reqURL := c.baseURL + "/api/auth"
	req, err := securityruntime.NewOutboundRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := securityruntime.DoOutboundRequest(c.httpClient, req)
	if err != nil {
		return fmt.Errorf("auth request: %w", err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("read auth response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("auth failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	var result struct {
		JWT string `json:"jwt"` // #nosec G117 -- Response field carries runtime bearer material, not a hardcoded secret.
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		return fmt.Errorf("decode auth response: %w", err)
	}
	if result.JWT == "" {
		return fmt.Errorf("empty JWT in auth response")
	}

	c.jwt = result.JWT
	c.jwtExpiry = time.Now().Add(jwtCacheDuration)
	return nil
}

// getJWT returns a valid JWT, acquiring one if necessary. Thread-safe.
func (c *Client) getJWT(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.jwt != "" && time.Now().Before(c.jwtExpiry) {
		return c.jwt, nil
	}

	if err := c.authenticate(ctx); err != nil {
		return "", err
	}
	return c.jwt, nil
}

// clearJWT invalidates the cached JWT. Thread-safe.
func (c *Client) clearJWT() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.jwt = ""
	c.jwtExpiry = time.Time{}
}

// ---------- Core request ----------

// request performs an HTTP request with auth injection.
// For JWT auth, it retries once on 401 after re-authenticating.
func (c *Client) request(ctx context.Context, method, path string, body io.Reader, contentType string) ([]byte, error) {
	var bodyPayload []byte
	if body != nil {
		var err error
		bodyPayload, err = io.ReadAll(body)
		if err != nil {
			return nil, fmt.Errorf("read request body: %w", err)
		}
	}

	payload, statusCode, err := c.doRequest(ctx, method, path, bodyPayload, contentType)
	if err != nil {
		return nil, err
	}

	// On 401 with JWT auth, clear JWT, re-auth, and retry once.
	if statusCode == http.StatusUnauthorized && c.apiKey == "" {
		c.clearJWT()
		payload, statusCode, err = c.doRequest(ctx, method, path, bodyPayload, contentType)
		if err != nil {
			return nil, err
		}
	}

	if statusCode >= 300 {
		return nil, fmt.Errorf("portainer api returned %d: %s", statusCode, strings.TrimSpace(string(payload)))
	}
	return payload, nil
}

// doRequest performs a single HTTP request with auth headers. Returns body, status code, error.
func (c *Client) doRequest(ctx context.Context, method, path string, body []byte, contentType string) ([]byte, int, error) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	reqURL := c.baseURL + path

	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}

	req, err := securityruntime.NewOutboundRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	// Inject auth.
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	} else {
		jwt, err := c.getJWT(ctx)
		if err != nil {
			return nil, 0, fmt.Errorf("acquire JWT: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+jwt)
	}

	resp, err := securityruntime.DoOutboundRequest(c.httpClient, req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, 0, err
	}
	return payload, resp.StatusCode, nil
}

// ---------- Convenience wrappers ----------

func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	return c.request(ctx, http.MethodGet, path, nil, "")
}

func (c *Client) post(ctx context.Context, path string, jsonBody any) ([]byte, error) {
	var body io.Reader
	var ct string
	if jsonBody != nil {
		data, err := json.Marshal(jsonBody)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		body = bytes.NewReader(data)
		ct = "application/json"
	}
	return c.request(ctx, http.MethodPost, path, body, ct)
}

func (c *Client) put(ctx context.Context, path string, jsonBody any) ([]byte, error) {
	var body io.Reader
	var ct string
	if jsonBody != nil {
		data, err := json.Marshal(jsonBody)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		body = bytes.NewReader(data)
		ct = "application/json"
	}
	return c.request(ctx, http.MethodPut, path, body, ct)
}

func (c *Client) del(ctx context.Context, path string) ([]byte, error) {
	return c.request(ctx, http.MethodDelete, path, nil, "")
}

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

// CreateExec creates a TTY exec instance in a container and returns the exec ID.
// cmd must be non-empty; if nil or empty, ["/bin/sh"] is used.
func (c *Client) CreateExec(ctx context.Context, endpointID int, containerID string, cmd []string) (string, error) {
	if len(cmd) == 0 {
		cmd = []string{"/bin/sh"}
	}
	path := fmt.Sprintf("/api/endpoints/%s/docker/containers/%s/exec",
		url.PathEscape(fmt.Sprintf("%d", endpointID)),
		url.PathEscape(containerID),
	)
	body := map[string]any{
		"AttachStdin":  true,
		"AttachStdout": true,
		"AttachStderr": true,
		"Tty":          true,
		"Cmd":          cmd,
	}
	payload, err := c.post(ctx, path, body)
	if err != nil {
		return "", fmt.Errorf("create exec instance: %w", err)
	}
	var result struct {
		ID string `json:"Id"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		return "", fmt.Errorf("decode exec response: %w", err)
	}
	if result.ID == "" {
		return "", fmt.Errorf("portainer returned empty exec ID")
	}
	return result.ID, nil
}

// ExecWebSocketURL returns the WebSocket URL and auth token for connecting to an
// exec session via Portainer's /api/websocket/exec endpoint.
// wsURL uses the ws:// or wss:// scheme matching the client base URL.
func (c *Client) ExecWebSocketURL(ctx context.Context, endpointID int, execID string) (wsURL string, token string, err error) {
	// Portainer requires a JWT token even when the primary auth mode is API key,
	// because the /api/websocket/exec endpoint authenticates via query parameter.
	// If apiKey auth is configured, we must obtain a JWT by authenticating first.
	if c.apiKey != "" {
		// For API-key-only clients we cannot obtain a JWT; the caller must
		// use username/password credentials for exec WebSocket sessions.
		// Return a clear error rather than silently failing.
		return "", "", fmt.Errorf("portainer exec websocket requires JWT auth; configure username/password credentials for exec sessions")
	}
	jwt, err := c.getJWT(ctx)
	if err != nil {
		return "", "", fmt.Errorf("acquire JWT for exec websocket: %w", err)
	}

	base := strings.TrimRight(c.baseURL, "/")
	// Translate http(s):// to ws(s)://.
	var wsBase string
	switch {
	case strings.HasPrefix(base, "https://"):
		wsBase = "wss://" + strings.TrimPrefix(base, "https://")
	case strings.HasPrefix(base, "http://"):
		wsBase = "ws://" + strings.TrimPrefix(base, "http://")
	default:
		wsBase = "ws://" + base
	}

	wsURL = fmt.Sprintf("%s/api/websocket/exec?endpointId=%d&token=%s&execId=%s",
		wsBase,
		endpointID,
		url.QueryEscape(jwt),
		url.QueryEscape(execID),
	)
	return wsURL, jwt, nil
}
