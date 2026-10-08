package portainer

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/labtether/labtether/internal/securityruntime"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	maxEndpoints             = 10
	maxContainersPerEndpoint = 100
	maxResponseBytes         = 32 * 1024 * 1024 // 32 MB
	maxJWTBytes              = 16 * 1024
	portainerExecIDLength    = 64
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
	if len(result.JWT) > maxJWTBytes {
		return fmt.Errorf("JWT in auth response exceeds size limit")
	}
	if err := validatePortainerJWT(result.JWT); err != nil {
		return fmt.Errorf("invalid JWT in auth response: %w", err)
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

func validatePortainerJWT(jwt string) error {
	if jwt == "" {
		return fmt.Errorf("JWT is empty")
	}
	if len(jwt) > maxJWTBytes {
		return fmt.Errorf("JWT exceeds size limit")
	}
	for _, char := range []byte(jwt) {
		if char <= 0x20 || char >= 0x7f {
			return fmt.Errorf("JWT contains invalid characters")
		}
	}
	return nil
}
