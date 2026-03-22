package agentcore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/metricschema"
	"github.com/labtether/labtether/internal/platforms"
	"github.com/labtether/labtether/internal/securityruntime"
)

func NewHeartbeatPublisher(cfg RuntimeConfig, staticMetadata map[string]string) HeartbeatPublisher {
	cfg.APIBaseURL = normalizeAPIBaseURL(cfg.APIBaseURL)
	if cfg.APIBaseURL == "" || cfg.APIToken == "" {
		log.Printf("%s heartbeat disabled: LABTETHER_API_BASE_URL/LABTETHER_API_TOKEN not configured", cfg.Name)
		return noopHeartbeatPublisher{}
	}

	return &apiHeartbeatPublisher{
		client: &http.Client{Timeout: 6 * time.Second},
		cfg:    cfg,
		meta:   cloneStringMap(staticMetadata),
	}
}

type apiHeartbeatPublisher struct {
	client *http.Client
	cfg    RuntimeConfig
	meta   map[string]string
}

func (p *apiHeartbeatPublisher) Publish(ctx context.Context, sample TelemetrySample) error {
	metadata := cloneStringMap(p.meta)
	metadata[metricschema.HeartbeatKeyCPUPercent] = fmt.Sprintf("%.2f", sample.CPUPercent)
	metadata[metricschema.HeartbeatKeyCPUUsedPercent] = fmt.Sprintf("%.2f", sample.CPUPercent)
	metadata[metricschema.HeartbeatKeyMemoryPercent] = fmt.Sprintf("%.2f", sample.MemoryPercent)
	metadata[metricschema.HeartbeatKeyMemoryUsedPercent] = fmt.Sprintf("%.2f", sample.MemoryPercent)
	metadata[metricschema.HeartbeatKeyDiskPercent] = fmt.Sprintf("%.2f", sample.DiskPercent)
	metadata[metricschema.HeartbeatKeyDiskUsedPercent] = fmt.Sprintf("%.2f", sample.DiskPercent)
	metadata[metricschema.HeartbeatKeyNetworkRXBytesPerSec] = fmt.Sprintf("%.2f", sample.NetRXBytesPerSec)
	metadata[metricschema.HeartbeatKeyNetworkTXBytesPerSec] = fmt.Sprintf("%.2f", sample.NetTXBytesPerSec)
	if sample.TempCelsius != nil {
		metadata[metricschema.HeartbeatKeyTempCelsius] = fmt.Sprintf("%.2f", *sample.TempCelsius)
		metadata[metricschema.HeartbeatKeyTemperatureCelsius] = fmt.Sprintf("%.2f", *sample.TempCelsius)
	}
	resolvedPlatform := resolveHeartbeatPlatform(metadata)
	if resolvedPlatform != "" {
		metadata["platform"] = resolvedPlatform
	}

	payload := assets.HeartbeatRequest{
		AssetID:  sample.AssetID,
		Type:     "host",
		Name:     sample.AssetID,
		Source:   p.cfg.Source,
		GroupID:  p.cfg.GroupID,
		Status:   "online",
		Platform: resolvedPlatform,
		Metadata: metadata,
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal heartbeat: %w", err)
	}

	endpoint := strings.TrimRight(p.cfg.APIBaseURL, "/") + "/assets/heartbeat"
	req, err := securityruntime.NewOutboundRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("build heartbeat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.cfg.APIToken)

	resp, err := securityruntime.DoOutboundRequest(p.client, req)
	if err != nil {
		return fmt.Errorf("send heartbeat: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("heartbeat rejected with status %d", resp.StatusCode)
	}
	return nil
}

type noopHeartbeatPublisher struct{}

func (noopHeartbeatPublisher) Publish(context.Context, TelemetrySample) error {
	return nil
}

func cloneStringMap(input map[string]string) map[string]string {
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func resolveHeartbeatPlatform(metadata map[string]string) string {
	return platforms.Resolve(
		metadata["platform"],
		metadata["os"],
		metadata["os_name"],
		metadata["os_pretty_name"],
	)
}
