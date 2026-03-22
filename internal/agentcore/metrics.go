package agentcore

import (
	"fmt"
	"strings"
)

func RenderPrometheus(sample TelemetrySample) string {
	label := fmt.Sprintf(`asset_id=%q`, sample.AssetID)

	builder := strings.Builder{}
	builder.WriteString("# HELP labtether_agent_cpu_usage_percent CPU usage percentage.\n")
	builder.WriteString("# TYPE labtether_agent_cpu_usage_percent gauge\n")
	builder.WriteString(fmt.Sprintf("labtether_agent_cpu_usage_percent{%s} %.4f\n", label, sample.CPUPercent))

	builder.WriteString("# HELP labtether_agent_memory_used_percent Memory usage percentage.\n")
	builder.WriteString("# TYPE labtether_agent_memory_used_percent gauge\n")
	builder.WriteString(fmt.Sprintf("labtether_agent_memory_used_percent{%s} %.4f\n", label, sample.MemoryPercent))

	builder.WriteString("# HELP labtether_agent_disk_used_percent Disk usage percentage.\n")
	builder.WriteString("# TYPE labtether_agent_disk_used_percent gauge\n")
	builder.WriteString(fmt.Sprintf("labtether_agent_disk_used_percent{%s} %.4f\n", label, sample.DiskPercent))

	builder.WriteString("# HELP labtether_agent_network_rx_bytes_total Total received network bytes.\n")
	builder.WriteString("# TYPE labtether_agent_network_rx_bytes_total counter\n")
	builder.WriteString(fmt.Sprintf("labtether_agent_network_rx_bytes_total{%s} %.0f\n", label, sample.NetRXBytes))

	builder.WriteString("# HELP labtether_agent_network_tx_bytes_total Total transmitted network bytes.\n")
	builder.WriteString("# TYPE labtether_agent_network_tx_bytes_total counter\n")
	builder.WriteString(fmt.Sprintf("labtether_agent_network_tx_bytes_total{%s} %.0f\n", label, sample.NetTXBytes))

	if sample.TempCelsius != nil {
		builder.WriteString("# HELP labtether_agent_temp_celsius Host temperature in celsius when available.\n")
		builder.WriteString("# TYPE labtether_agent_temp_celsius gauge\n")
		builder.WriteString(fmt.Sprintf("labtether_agent_temp_celsius{%s} %.4f\n", label, *sample.TempCelsius))
	}

	builder.WriteString("# HELP labtether_agent_scrape_timestamp_seconds Last collector update timestamp in unix seconds.\n")
	builder.WriteString("# TYPE labtether_agent_scrape_timestamp_seconds gauge\n")
	builder.WriteString(fmt.Sprintf("labtether_agent_scrape_timestamp_seconds{%s} %d\n", label, sample.CollectedAt.Unix()))

	return builder.String()
}

func ClampPercent(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 100:
		return 100
	default:
		return value
	}
}
