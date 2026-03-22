package truenas

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/connectorsdk"
)

// Connector implements connectorsdk.Connector for TrueNAS via WebSocket JSON-RPC 2.0.
// When client is nil the connector runs in stub mode, returning synthetic assets
// so that the connector registry can be populated without requiring live config.
type Connector struct {
	client *Client
}

// Config holds the configuration needed to connect to a TrueNAS instance.
type Config struct {
	BaseURL    string
	APIKey     string // #nosec G117 -- Runtime connector credential, not a hardcoded secret.
	SkipVerify bool
	Timeout    time.Duration
}

// New returns a Connector in stub mode (no live TrueNAS connection).
// Used by the connector registry during startup when no config is provided.
func New() *Connector {
	return &Connector{}
}

// NewWithConfig returns a fully-configured Connector that will use the
// WebSocket JSON-RPC transport to communicate with TrueNAS.
func NewWithConfig(cfg Config) *Connector {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Connector{
		client: &Client{
			BaseURL:    strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
			APIKey:     strings.TrimSpace(cfg.APIKey),
			SkipVerify: cfg.SkipVerify,
			Timeout:    timeout,
		},
	}
}

// ID returns the unique connector identifier.
func (c *Connector) ID() string {
	return "truenas"
}

// DisplayName returns the human-readable connector name.
func (c *Connector) DisplayName() string {
	return "TrueNAS"
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

// isConfigured reports whether the connector has a usable client configuration.
func (c *Connector) isConfigured() bool {
	return c.client != nil && c.client.BaseURL != "" && c.client.APIKey != ""
}

// TestConnection verifies that the TrueNAS WebSocket API is reachable and
// returns the system version and hostname in the health message.
func (c *Connector) TestConnection(ctx context.Context) (connectorsdk.Health, error) {
	if !c.isConfigured() {
		return connectorsdk.Health{
			Status:  "ok",
			Message: "truenas connector running in stub mode (missing config)",
		}, nil
	}

	var info map[string]any
	if err := c.client.Call(ctx, "system.info", nil, &info); err != nil {
		return connectorsdk.Health{Status: "failed", Message: err.Error()}, nil
	}

	version := strings.TrimSpace(anyToString(info["version"]))
	hostname := strings.TrimSpace(anyToString(info["hostname"]))
	msg := "truenas reachable"
	if hostname != "" && version != "" {
		msg = fmt.Sprintf("connected to %s running %s", hostname, version)
	} else if hostname != "" {
		msg = fmt.Sprintf("connected to %s", hostname)
	} else if version != "" {
		msg = fmt.Sprintf("truenas reachable, version %s", version)
	}

	return connectorsdk.Health{Status: "ok", Message: msg}, nil
}

func (c *Connector) callQuery(ctx context.Context, method string, dest any) error {
	// Most TrueNAS versions accept empty params for *.query calls.
	err := c.client.Call(ctx, method, nil, dest)
	if err == nil || !IsMethodCallError(err) {
		return err
	}

	// Compatibility retry: some versions require explicit filter/options params.
	retryParams := [][]any{
		{[]any{}, map[string]any{}},
		{[]any{}},
	}
	retryErr := err
	for _, params := range retryParams {
		retryErr = c.client.Call(ctx, method, params, dest)
		if retryErr == nil {
			return nil
		}
		if !IsMethodCallError(retryErr) {
			return retryErr
		}
	}
	return retryErr
}

// stubAssets returns a single synthetic asset used in stub mode so that the
// connector is visible in the UI without requiring a live TrueNAS instance.
func (c *Connector) stubAssets() []connectorsdk.Asset {
	return []connectorsdk.Asset{
		{
			ID:     "truenas-controller-stub",
			Type:   "storage-controller",
			Name:   "truenas-stub",
			Source: c.ID(),
			Metadata: map[string]string{
				"note": "stub mode — configure TRUENAS_BASE_URL and TRUENAS_API_KEY",
			},
		},
	}
}
