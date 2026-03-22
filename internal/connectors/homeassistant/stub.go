package homeassistant

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/securityruntime"
)

const maxResponseBytes = 32 * 1024 * 1024
const maxMetadataValueLength = 512

type Connector struct {
	baseURL    string
	token      string
	skipVerify bool
	httpClient *http.Client
}

type Config struct {
	BaseURL    string
	Token      string
	SkipVerify bool
	Timeout    time.Duration
}

func New() *Connector {
	timeout := envDuration("HA_HTTP_TIMEOUT", 10*time.Second)
	skipVerify := envBool("HA_SKIP_VERIFY", false)
	return NewWithConfig(Config{
		BaseURL:    strings.TrimSpace(os.Getenv("HA_BASE_URL")),
		Token:      strings.TrimSpace(os.Getenv("HA_TOKEN")),
		SkipVerify: skipVerify,
		Timeout:    timeout,
	})
}

func NewWithConfig(cfg Config) *Connector {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Connector{
		baseURL:    strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		token:      strings.TrimSpace(cfg.Token),
		skipVerify: cfg.SkipVerify,
		httpClient: newHTTPClient(timeout, cfg.SkipVerify),
	}
}

func (c *Connector) ID() string {
	return "home-assistant"
}

func (c *Connector) DisplayName() string {
	return "Home Assistant"
}

func (c *Connector) Capabilities() connectorsdk.Capabilities {
	return connectorsdk.Capabilities{
		DiscoverAssets: true,
		CollectMetrics: true,
		CollectEvents:  true,
		ExecuteActions: true,
	}
}

func (c *Connector) Discover(ctx context.Context) ([]connectorsdk.Asset, error) {
	if !c.isConfigured() {
		return c.stubAssets(), nil
	}

	payload, err := c.request(ctx, http.MethodGet, "/api/states", nil)
	if err != nil {
		return nil, err
	}

	var states []map[string]any
	if err := json.Unmarshal(payload, &states); err != nil {
		return nil, fmt.Errorf("failed to decode home assistant states: %w", err)
	}

	assets := make([]connectorsdk.Asset, 0, 24)
	for _, state := range states {
		entityID := strings.TrimSpace(anyToString(state["entity_id"]))
		if entityID == "" {
			continue
		}
		friendlyName := entityID
		attributes, _ := state["attributes"].(map[string]any)
		if attributes != nil {
			candidate := strings.TrimSpace(anyToString(attributes["friendly_name"]))
			if candidate != "" {
				friendlyName = candidate
			}
		}
		domain := entityID
		if dot := strings.Index(entityID, "."); dot > 0 {
			domain = entityID[:dot]
		}

		metadata := map[string]string{
			"entity_id": entityID,
			"domain":    domain,
			"state":     strings.TrimSpace(anyToString(state["state"])),
		}
		setMetadataValue(metadata, "last_changed", state["last_changed"])
		setMetadataValue(metadata, "last_updated", state["last_updated"])
		for key, value := range attributes {
			if _, exists := metadata[key]; exists {
				continue
			}
			setMetadataValue(metadata, key, value)
		}

		assets = append(assets, connectorsdk.Asset{
			ID:       "ha-entity-" + strings.ReplaceAll(entityID, ".", "-"),
			Type:     "ha-entity",
			Name:     friendlyName,
			Source:   c.ID(),
			Metadata: metadata,
		})

		if len(assets) >= 40 {
			break
		}
	}

	return assets, nil
}

func (c *Connector) TestConnection(ctx context.Context) (connectorsdk.Health, error) {
	if !c.isConfigured() {
		return connectorsdk.Health{Status: "ok", Message: "home assistant connector running in stub mode (missing env config)"}, nil
	}

	_, err := c.request(ctx, http.MethodGet, "/api/", nil)
	if err != nil {
		return connectorsdk.Health{Status: "failed", Message: err.Error()}, nil
	}
	return connectorsdk.Health{Status: "ok", Message: "home assistant API reachable"}, nil
}

// HAConfig holds selected fields from the Home Assistant /api/config response.
type HAConfig struct {
	Version      string
	LocationName string
}

func (c *Connector) FetchConfig(ctx context.Context) (HAConfig, error) {
	if !c.isConfigured() {
		return HAConfig{}, nil
	}
	payload, err := c.request(ctx, http.MethodGet, "/api/config", nil)
	if err != nil {
		return HAConfig{}, err
	}
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return HAConfig{}, fmt.Errorf("failed to decode home assistant config: %w", err)
	}
	return HAConfig{
		Version:      strings.TrimSpace(anyToString(raw["version"])),
		LocationName: strings.TrimSpace(anyToString(raw["location_name"])),
	}, nil
}

// SupervisorStats holds host metrics from the Supervisor API (HAOS/Supervised installs only).
type SupervisorStats struct {
	Available         bool
	CPUPercent        float64
	MemoryUsedPercent float64
	DiskUsedPercent   float64
	OSName            string
	Hostname          string
}

// FetchSupervisorStats fetches host metrics from the Supervisor API.
// Returns Available=false without error for non-supervised installs (404/connection error).
func (c *Connector) FetchSupervisorStats(ctx context.Context) (SupervisorStats, error) {
	if !c.isConfigured() {
		return SupervisorStats{}, nil
	}

	// Core stats: CPU + memory
	corePayload, coreErr := c.request(ctx, http.MethodGet, "/api/hassio/core/stats", nil)
	if coreErr != nil {
		// Supervisor not available — not an error, just unsupported install type.
		return SupervisorStats{Available: false}, nil
	}
	var coreResp struct {
		Data struct {
			CPUPercent  float64 `json:"cpu_percent"`
			MemoryUsage int64   `json:"memory_usage"`
			MemoryLimit int64   `json:"memory_limit"`
		} `json:"data"`
	}
	if err := json.Unmarshal(corePayload, &coreResp); err != nil {
		return SupervisorStats{Available: false}, nil
	}

	var memPercent float64
	if coreResp.Data.MemoryLimit > 0 {
		memPercent = float64(coreResp.Data.MemoryUsage) / float64(coreResp.Data.MemoryLimit) * 100
	}

	stats := SupervisorStats{
		Available:         true,
		CPUPercent:        coreResp.Data.CPUPercent,
		MemoryUsedPercent: memPercent,
	}

	// Host info: disk + OS (best-effort, don't fail if unavailable)
	hostPayload, hostErr := c.request(ctx, http.MethodGet, "/api/hassio/host/info", nil)
	if hostErr == nil {
		var hostResp struct {
			Data struct {
				Hostname        string  `json:"hostname"`
				OperatingSystem string  `json:"operating_system"`
				DiskTotal       float64 `json:"disk_total"`
				DiskUsed        float64 `json:"disk_used"`
			} `json:"data"`
		}
		if err := json.Unmarshal(hostPayload, &hostResp); err == nil {
			stats.Hostname = strings.TrimSpace(hostResp.Data.Hostname)
			stats.OSName = strings.TrimSpace(hostResp.Data.OperatingSystem)
			if hostResp.Data.DiskTotal > 0 {
				stats.DiskUsedPercent = hostResp.Data.DiskUsed / hostResp.Data.DiskTotal * 100
			}
		}
	}

	return stats, nil
}

func (c *Connector) Actions() []connectorsdk.ActionDescriptor {
	return []connectorsdk.ActionDescriptor{
		{
			ID:             "entity.toggle",
			Name:           "Toggle Entity",
			Description:    "Toggle an entity state.",
			RequiresTarget: true,
			SupportsDryRun: true,
		},
		{
			ID:             "service.call",
			Name:           "Call Service",
			Description:    "Invoke a Home Assistant domain service.",
			RequiresTarget: false,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{
					Key:         "service",
					Label:       "Service",
					Required:    true,
					Description: "Service key, for example light.turn_on.",
				},
			},
		},
	}
}

func (c *Connector) ExecuteAction(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	target := strings.TrimSpace(req.TargetID)
	if req.DryRun {
		switch actionID {
		case "entity.toggle":
			if target == "" {
				return connectorsdk.ActionResult{Status: "failed", Message: "target_id is required"}, nil
			}
			return connectorsdk.ActionResult{Status: "succeeded", Message: "dry-run: toggle validated", Output: fmt.Sprintf("would toggle %s", target)}, nil
		case "service.call":
			service := strings.TrimSpace(req.Params["service"])
			if service == "" {
				service = "homeassistant.update_entity"
			}
			return connectorsdk.ActionResult{Status: "succeeded", Message: "dry-run: service call validated", Output: fmt.Sprintf("would call %s", service)}, nil
		default:
			return connectorsdk.ActionResult{Status: "failed", Message: "unsupported action"}, nil
		}
	}

	if !c.isConfigured() {
		switch actionID {
		case "entity.toggle":
			if target == "" {
				return connectorsdk.ActionResult{Status: "failed", Message: "target_id is required"}, nil
			}
			return connectorsdk.ActionResult{Status: "succeeded", Message: "entity toggled (stub mode)", Output: fmt.Sprintf("toggled %s", target)}, nil
		case "service.call":
			service := strings.TrimSpace(req.Params["service"])
			if service == "" {
				service = "homeassistant.update_entity"
			}
			return connectorsdk.ActionResult{Status: "succeeded", Message: "service called (stub mode)", Output: fmt.Sprintf("called %s", service)}, nil
		default:
			return connectorsdk.ActionResult{Status: "failed", Message: "unsupported action"}, nil
		}
	}

	switch actionID {
	case "entity.toggle":
		if target == "" {
			return connectorsdk.ActionResult{Status: "failed", Message: "target_id is required"}, nil
		}
		body := map[string]any{"entity_id": target}
		payload, err := c.request(ctx, http.MethodPost, "/api/services/homeassistant/toggle", body)
		if err != nil {
			return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
		}
		return connectorsdk.ActionResult{Status: "succeeded", Message: "entity toggled", Output: truncatePayload(payload)}, nil
	case "service.call":
		service := strings.TrimSpace(req.Params["service"])
		if service == "" {
			service = "homeassistant.update_entity"
		}
		domain, action, err := parseService(service)
		if err != nil {
			return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
		}

		body := map[string]any{}
		for key, value := range req.Params {
			if strings.TrimSpace(key) == "service" {
				continue
			}
			body[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
		if target != "" {
			body["entity_id"] = target
		}

		payload, err := c.request(ctx, http.MethodPost, fmt.Sprintf("/api/services/%s/%s", domain, action), body)
		if err != nil {
			return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
		}
		return connectorsdk.ActionResult{Status: "succeeded", Message: "service called", Output: truncatePayload(payload)}, nil
	default:
		return connectorsdk.ActionResult{Status: "failed", Message: "unsupported action"}, nil
	}
}

func (c *Connector) request(ctx context.Context, method, path string, body map[string]any) ([]byte, error) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	url := c.baseURL + path

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(payload)
	}

	req, err := securityruntime.NewOutboundRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := securityruntime.DoOutboundRequest(c.httpClient, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(payload) > maxResponseBytes {
		return nil, fmt.Errorf("home assistant response exceeded %d bytes", maxResponseBytes)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("home assistant api returned %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	return payload, nil
}

func (c *Connector) isConfigured() bool {
	return c.baseURL != "" && c.token != ""
}

func (c *Connector) stubAssets() []connectorsdk.Asset {
	return []connectorsdk.Asset{
		{
			ID:     "ha-entity-sensor-labtemp",
			Type:   "ha-entity",
			Name:   "Lab Temperature",
			Source: c.ID(),
			Metadata: map[string]string{
				"domain": "sensor",
				"unit":   "C",
			},
		},
		{
			ID:     "ha-entity-switch-rack-fan",
			Type:   "ha-entity",
			Name:   "Rack Fan",
			Source: c.ID(),
			Metadata: map[string]string{
				"domain": "switch",
			},
		},
	}
}

func parseService(value string) (string, string, error) {
	trimmed := strings.TrimSpace(value)
	parts := strings.Split(trimmed, ".")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("service must be domain.action")
	}
	domain := strings.TrimSpace(parts[0])
	action := strings.TrimSpace(parts[1])
	if domain == "" || action == "" {
		return "", "", fmt.Errorf("service must be domain.action")
	}
	return domain, action, nil
}

func truncatePayload(payload []byte) string {
	trimmed := strings.TrimSpace(string(payload))
	if len(trimmed) <= 512 {
		return trimmed
	}
	return trimmed[:512] + "..."
}

func anyToString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	default:
		return fmt.Sprintf("%v", value)
	}
}

func setMetadataValue(metadata map[string]string, key string, value any) {
	serialized := stringifyMetadataValue(value)
	if serialized == "" {
		return
	}
	metadata[key] = serialized
}

func stringifyMetadataValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return truncateMetadataValue(strings.TrimSpace(typed))
	case bool:
		return strconv.FormatBool(typed)
	case float32:
		return truncateMetadataValue(strconv.FormatFloat(float64(typed), 'f', -1, 32))
	case float64:
		return truncateMetadataValue(strconv.FormatFloat(typed, 'f', -1, 64))
	case int:
		return strconv.Itoa(typed)
	case int8, int16, int32, int64:
		return truncateMetadataValue(strings.TrimSpace(fmt.Sprintf("%v", typed)))
	case uint, uint8, uint16, uint32, uint64:
		return truncateMetadataValue(strings.TrimSpace(fmt.Sprintf("%v", typed)))
	case json.Number:
		return truncateMetadataValue(strings.TrimSpace(typed.String()))
	case map[string]any:
		clone := maps.Clone(typed)
		payload, err := json.Marshal(clone)
		if err != nil {
			return truncateMetadataValue(strings.TrimSpace(fmt.Sprintf("%v", value)))
		}
		return truncateMetadataValue(strings.TrimSpace(string(payload)))
	default:
		payload, err := json.Marshal(typed)
		if err == nil {
			return truncateMetadataValue(strings.TrimSpace(string(payload)))
		}
		return truncateMetadataValue(strings.TrimSpace(fmt.Sprintf("%v", value)))
	}
}

func truncateMetadataValue(value string) string {
	if len(value) <= maxMetadataValueLength {
		return value
	}
	return value[:maxMetadataValueLength] + "..."
}

func envDuration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

func newHTTPClient(timeout time.Duration, skipVerify bool) *http.Client {
	transport := http.DefaultTransport
	if skipVerify {
		transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion:         tls.VersionTLS12,
				InsecureSkipVerify: true, // #nosec G402 -- explicitly controlled by connector skip_verify option for lab environments.
			},
		}
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}
