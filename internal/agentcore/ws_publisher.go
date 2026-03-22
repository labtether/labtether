package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/metricschema"
	"github.com/labtether/labtether/internal/platforms"
)

// wsHeartbeatPublisher sends heartbeats over a WebSocket transport. If the
// transport is disconnected, it falls back to the HTTP publisher.
type wsHeartbeatPublisher struct {
	transport    *wsTransport
	httpFallback HeartbeatPublisher
	cfg          RuntimeConfig
	meta         map[string]string
	capabilities []string
}

func newWSHeartbeatPublisher(transport *wsTransport, httpFallback HeartbeatPublisher, cfg RuntimeConfig, staticMeta map[string]string, capabilities []string) *wsHeartbeatPublisher {
	return &wsHeartbeatPublisher{
		transport:    transport,
		httpFallback: httpFallback,
		cfg:          cfg,
		meta:         cloneStringMap(staticMeta),
		capabilities: append([]string(nil), capabilities...),
	}
}

func (p *wsHeartbeatPublisher) Publish(ctx context.Context, sample TelemetrySample) error {
	if !p.transport.Connected() {
		if p.httpFallback != nil {
			return p.httpFallback.Publish(ctx, sample)
		}
		return fmt.Errorf("websocket disconnected and no http fallback")
	}

	metadata := cloneStringMap(p.meta)

	// Inject transport self-diagnostics.
	sent, received, reconnects, uptime := p.transport.Stats()
	metadata["agent_uptime_sec"] = strconv.FormatInt(int64(uptime.Seconds()), 10)
	metadata["agent_reconnect_count"] = strconv.FormatInt(reconnects, 10)
	metadata["agent_messages_sent"] = strconv.FormatInt(sent, 10)
	metadata["agent_messages_received"] = strconv.FormatInt(received, 10)

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
	resolvedPlatform := platforms.Resolve(
		metadata["platform"],
		metadata["os"],
		metadata["os_name"],
		metadata["os_pretty_name"],
	)
	if resolvedPlatform != "" {
		metadata["platform"] = resolvedPlatform
	}

	heartbeat := agentmgr.HeartbeatData{
		AssetID:      sample.AssetID,
		Type:         "host",
		Name:         sample.AssetID,
		Source:       p.cfg.Source,
		GroupID:      p.cfg.GroupID,
		Status:       "online",
		Platform:     resolvedPlatform,
		Metadata:     metadata,
		Capabilities: append([]string(nil), p.capabilities...),
		Connectors:   discoverConnectors(),
	}

	data, err := json.Marshal(heartbeat)
	if err != nil {
		return fmt.Errorf("marshal ws heartbeat: %w", err)
	}

	msg := agentmgr.Message{
		Type: agentmgr.MsgHeartbeat,
		Data: data,
	}

	if err := p.transport.Send(msg); err != nil {
		log.Printf("agentws: heartbeat send failed, falling back to HTTP: %v", err)
		if p.httpFallback != nil {
			return p.httpFallback.Publish(ctx, sample)
		}
		return err
	}
	return nil
}
