package collectors

import (
	"context"
	"log"
	"math"
	"net"
	neturl "net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentcore"
	"github.com/labtether/labtether/internal/connectors/proxmox"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/telemetry"
)

func proxmoxStorageName(resource proxmox.Resource) string {
	storageID := strings.TrimSpace(resource.ID)
	if storageID != "" {
		parts := strings.Split(storageID, "/")
		storageID = strings.TrimSpace(parts[len(parts)-1])
	}
	if storageID == "" {
		storageID = strings.TrimSpace(resource.Name)
	}
	return storageID
}

func CollectorConfigString(config map[string]any, key string) string {
	return shared.CollectorConfigString(config, key)
}

func collectorConfigBool(config map[string]any, key string) (bool, bool) {
	return shared.CollectorConfigBool(config, key)
}

func collectorConfigDuration(config map[string]any, key string, fallback time.Duration) time.Duration {
	return shared.CollectorConfigDuration(config, key, fallback)
}

func CollectorEndpointIdentity(rawBaseURL string) (string, string) {
	trimmed := strings.TrimSpace(rawBaseURL)
	if trimmed == "" {
		return "", ""
	}

	parsed, err := neturl.Parse(trimmed)
	if err != nil || strings.TrimSpace(parsed.Hostname()) == "" {
		parsed, err = neturl.Parse("https://" + trimmed)
		if err != nil {
			return "", ""
		}
	}

	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return "", ""
	}

	if ip := net.ParseIP(host); ip != nil {
		return host, ip.String()
	}
	return host, ""
}

func NormalizeAssetKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-", ":", "-")
	return replacer.Replace(value)
}

func proxmoxVMIDString(value float64) string {
	if value <= 0 {
		return ""
	}
	return strconv.FormatInt(int64(value), 10)
}

func derivePercent(used, max float64) (float64, bool) {
	if max <= 0 || used < 0 {
		return 0, false
	}
	return clampCollectorPercent((used / max) * 100), true
}

func derivePercentFromRatio(ratio float64) float64 {
	return clampCollectorPercent(ratio * 100)
}

func clampCollectorPercent(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return math.Round(value*100) / 100
}

func formatMetricValue(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func normalizeProxmoxStatus(status string) string {
	lower := strings.ToLower(strings.TrimSpace(status))
	switch {
	case lower == "", lower == "unknown":
		return "stale"
	case strings.Contains(lower, "run"), strings.Contains(lower, "online"), strings.Contains(lower, "up"),
		lower == "available":
		return "online"
	case strings.Contains(lower, "stop"), strings.Contains(lower, "offline"), strings.Contains(lower, "down"):
		return "offline"
	default:
		return "stale"
	}
}

// ingestCollectorTelemetry parses collector output into normalized metric samples
// and stores them in the telemetry store.
func (d *Deps) ingestCollectorTelemetry(collector hubcollector.Collector, output string) {
	if d.TelemetryStore == nil || output == "" {
		return
	}
	responseFormat, _ := collector.Config["response_format"].(string)
	samples, _ := agentcore.ParseCollectorOutput(output, responseFormat, collector.AssetID)
	if len(samples) == 0 {
		return
	}
	metricSamples := make([]telemetry.MetricSample, len(samples))
	for i, cs := range samples {
		metricSamples[i] = telemetry.MetricSample{
			AssetID:     cs.AssetID,
			Metric:      cs.Metric,
			Value:       cs.Value,
			CollectedAt: cs.Timestamp,
		}
	}
	if err := d.TelemetryStore.AppendSamples(context.Background(), metricSamples); err != nil {
		log.Printf("hub collector: failed to ingest telemetry for asset %s: %v", collector.AssetID, err)
	}
}
