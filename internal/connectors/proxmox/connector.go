package proxmox

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/connectorsdk"
)

const (
	defaultActionPollInterval = 2 * time.Second
	defaultActionWaitTimeout  = 5 * time.Minute
)

type Connector struct {
	client      *Client
	clientErr   error
	defaultNode string
}

func New() *Connector {
	timeout := 10 * time.Second
	if raw := strings.TrimSpace(os.Getenv("PROXMOX_HTTP_TIMEOUT")); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
			timeout = parsed
		}
	}

	skipVerify := false
	if raw := strings.TrimSpace(os.Getenv("PROXMOX_SKIP_VERIFY")); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			skipVerify = parsed
		}
	}

	client, err := NewClient(Config{
		BaseURL:     strings.TrimSpace(os.Getenv("PROXMOX_BASE_URL")),
		TokenID:     strings.TrimSpace(os.Getenv("PROXMOX_TOKEN_ID")),
		TokenSecret: strings.TrimSpace(os.Getenv("PROXMOX_TOKEN_SECRET")),
		SkipVerify:  skipVerify,
		CAPEM:       strings.TrimSpace(os.Getenv("PROXMOX_CA_CERT_PEM")),
		Timeout:     timeout,
	})
	if err != nil {
		return &Connector{
			clientErr:   err,
			defaultNode: strings.TrimSpace(os.Getenv("PROXMOX_DEFAULT_NODE")),
		}
	}

	return &Connector{
		client:      client,
		defaultNode: strings.TrimSpace(os.Getenv("PROXMOX_DEFAULT_NODE")),
	}
}

func (c *Connector) ID() string {
	return "proxmox"
}

func (c *Connector) DisplayName() string {
	return "Proxmox VE"
}

func (c *Connector) Capabilities() connectorsdk.Capabilities {
	return connectorsdk.Capabilities{
		DiscoverAssets: true,
		CollectMetrics: true,
		CollectEvents:  true,
		ExecuteActions: true,
	}
}

func (c *Connector) TestConnection(ctx context.Context) (connectorsdk.Health, error) {
	if c.clientErr != nil {
		return connectorsdk.Health{Status: "failed", Message: c.clientErr.Error()}, nil
	}
	if !c.isConfigured() {
		return connectorsdk.Health{Status: "ok", Message: "proxmox connector running in stub mode (missing env config)"}, nil
	}

	release, err := c.client.GetVersion(ctx)
	if err != nil {
		return connectorsdk.Health{Status: "failed", Message: err.Error()}, nil
	}
	if strings.TrimSpace(release) == "" {
		return connectorsdk.Health{Status: "ok", Message: "proxmox API reachable"}, nil
	}
	return connectorsdk.Health{Status: "ok", Message: fmt.Sprintf("proxmox API reachable (%s)", release)}, nil
}

func (c *Connector) isConfigured() bool {
	return c.client != nil && c.client.IsConfigured()
}
