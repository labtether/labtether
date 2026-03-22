package proxmox

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	neturl "net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/securityruntime"
)

// AuthMode selects Proxmox API authentication strategy.
type AuthMode string

const (
	AuthModeAPIToken AuthMode = "api_token"
	AuthModePassword AuthMode = "password"
)

// Config defines runtime Proxmox API connection settings.
type Config struct {
	BaseURL     string
	TokenID     string
	TokenSecret string
	SkipVerify  bool
	CAPEM       string
	Timeout     time.Duration

	// Password-based auth fields (used when AuthMode == AuthModePassword).
	AuthMode AuthMode
	Username string // e.g. "root@pam"
	Password string // #nosec G117 -- Runtime connector credential, not a hardcoded secret.
}

// Client is a thin Proxmox VE API client.
type Client struct {
	baseURL     string
	tokenID     string
	tokenSecret string
	httpClient  *http.Client

	// Password-based auth state.
	authMode      AuthMode
	username      string
	password      string
	mu            sync.Mutex
	ticket        string
	csrfToken     string
	ticketExpires time.Time
}

// NewClient creates a Proxmox API client from runtime settings.
func NewClient(cfg Config) (*Client, error) {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			// #nosec G402 -- user-controlled homelab setting for self-signed certs.
			InsecureSkipVerify: cfg.SkipVerify, //nolint:gosec // #nosec G402 -- user-controlled homelab setting
		},
		MaxIdleConns:        20,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     20,
		IdleConnTimeout:     90 * time.Second,
	}
	transport.DialContext = securityruntime.PreferSameSubnetPrivateDialContext(&net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
	})
	if strings.TrimSpace(cfg.CAPEM) != "" {
		pool := x509.NewCertPool()
		if ok := pool.AppendCertsFromPEM([]byte(cfg.CAPEM)); !ok {
			return nil, fmt.Errorf("invalid Proxmox CA PEM")
		}
		transport.TLSClientConfig.RootCAs = pool
	}

	return &Client{
		baseURL:     strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		tokenID:     strings.TrimSpace(cfg.TokenID),
		tokenSecret: strings.TrimSpace(cfg.TokenSecret),
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
		authMode: cfg.AuthMode,
		username: strings.TrimSpace(cfg.Username),
		password: cfg.Password,
	}, nil
}

// IsConfigured reports whether base URL and credentials are available.
func (c *Client) IsConfigured() bool {
	if c == nil {
		return false
	}
	if c.baseURL == "" {
		return false
	}
	if c.authMode == AuthModePassword {
		return c.username != "" && c.password != ""
	}
	return c.tokenID != "" && c.tokenSecret != ""
}

// GetAuthMode returns the client's authentication mode.
func (c *Client) GetAuthMode() AuthMode {
	if c == nil {
		return AuthModeAPIToken
	}
	return c.authMode
}

// GetTicket returns a valid Proxmox session ticket, acquiring or refreshing
// one if necessary. Only valid for password-mode clients.
func (c *Client) GetTicket(ctx context.Context) (ticket, csrfToken string, err error) {
	if c.authMode != AuthModePassword {
		return "", "", fmt.Errorf("GetTicket called on API token client")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Return cached ticket if still valid (5min safety margin before 2h expiry).
	if c.ticket != "" && time.Now().Before(c.ticketExpires) {
		return c.ticket, c.csrfToken, nil
	}

	// Acquire new ticket via POST /api2/json/access/ticket.
	values := neturl.Values{}
	values.Set("username", c.username)
	values.Set("password", c.password)

	url := c.baseURL + "/api2/json/access/ticket"
	req, err := securityruntime.NewOutboundRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(values.Encode()))
	if err != nil {
		return "", "", fmt.Errorf("create ticket request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := securityruntime.DoOutboundRequest(c.httpClient, req)
	if err != nil {
		return "", "", fmt.Errorf("ticket request: %w", err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
	if err != nil {
		return "", "", fmt.Errorf("read ticket response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("ticket auth failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	var wrapped struct {
		Data struct {
			Ticket              string `json:"ticket"`
			CSRFPreventionToken string `json:"CSRFPreventionToken"`
			Username            string `json:"username"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &wrapped); err != nil {
		return "", "", fmt.Errorf("decode ticket response: %w", err)
	}
	if wrapped.Data.Ticket == "" {
		return "", "", fmt.Errorf("empty ticket in response")
	}

	c.ticket = wrapped.Data.Ticket
	c.csrfToken = wrapped.Data.CSRFPreventionToken
	// Proxmox tickets expire after 2 hours; refresh 5 minutes early.
	c.ticketExpires = time.Now().Add(115 * time.Minute)

	return c.ticket, c.csrfToken, nil
}

func (c *Client) postTask(ctx context.Context, path string, values neturl.Values) (string, error) {
	payload, err := c.requestRaw(ctx, http.MethodPost, path, values)
	if err != nil {
		return "", err
	}
	return parseDataString(payload)
}

func (c *Client) deleteTask(ctx context.Context, path string) (string, error) {
	payload, err := c.requestRaw(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return "", err
	}
	return parseDataString(payload)
}

func (c *Client) getData(ctx context.Context, path string, out any) error {
	payload, err := c.requestRaw(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	return unmarshalDataEnvelope(payload, out)
}

func (c *Client) postData(ctx context.Context, path string, values neturl.Values, out any) error {
	payload, err := c.requestRaw(ctx, http.MethodPost, path, values)
	if err != nil {
		return err
	}
	return unmarshalDataEnvelope(payload, out)
}

func (c *Client) requestRaw(ctx context.Context, method, path string, values neturl.Values) ([]byte, error) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	url := c.baseURL + path

	var body io.Reader
	if values != nil {
		body = strings.NewReader(values.Encode())
	}

	req, err := securityruntime.NewOutboundRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if values != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	if c.authMode == AuthModePassword {
		ticket, csrf, err := c.GetTicket(ctx)
		if err != nil {
			return nil, fmt.Errorf("acquire session ticket: %w", err)
		}
		req.AddCookie(&http.Cookie{Name: "PVEAuthCookie", Value: ticket}) // #nosec G124 -- Outbound API client cookie header, not a browser session cookie.
		if method != http.MethodGet {
			req.Header.Set("CSRFPreventionToken", csrf)
		}
	} else {
		req.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", c.tokenID, c.tokenSecret))
	}

	resp, err := securityruntime.DoOutboundRequest(c.httpClient, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("proxmox api returned %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	return payload, nil
}

func unmarshalDataEnvelope(payload []byte, out any) error {
	var wrapped struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(payload, &wrapped); err != nil {
		return fmt.Errorf("decode proxmox response: %w", err)
	}
	if len(wrapped.Data) == 0 || strings.EqualFold(strings.TrimSpace(string(wrapped.Data)), "null") {
		return nil
	}
	if err := json.Unmarshal(wrapped.Data, out); err != nil {
		return fmt.Errorf("decode proxmox data payload: %w", err)
	}
	return nil
}

func parseDataString(payload []byte) (string, error) {
	var wrapped struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(payload, &wrapped); err != nil {
		return "", fmt.Errorf("decode proxmox response: %w", err)
	}
	raw := strings.TrimSpace(string(wrapped.Data))
	if raw == "" || strings.EqualFold(raw, "null") {
		return "", nil
	}

	var asString string
	if err := json.Unmarshal(wrapped.Data, &asString); err == nil {
		return strings.TrimSpace(asString), nil
	}

	var asFloat float64
	if err := json.Unmarshal(wrapped.Data, &asFloat); err == nil {
		return strconv.FormatFloat(asFloat, 'f', -1, 64), nil
	}
	return raw, nil
}
