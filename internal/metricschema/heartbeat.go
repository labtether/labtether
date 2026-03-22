package metricschema

const (
	HeartbeatKeyCPUPercent             = "cpu_percent"
	HeartbeatKeyCPUUsedPercent         = "cpu_used_percent"
	HeartbeatKeyMemoryPercent          = "memory_percent"
	HeartbeatKeyMemoryUsedPercent      = "memory_used_percent"
	HeartbeatKeyDiskPercent            = "disk_percent"
	HeartbeatKeyDiskUsedPercent        = "disk_used_percent"
	HeartbeatKeyTempCelsius            = "temp_celsius"
	HeartbeatKeyTemperatureCelsius     = "temperature_celsius"
	HeartbeatKeyNetworkRXBytesPerSec   = "network_rx_bytes_per_sec"
	HeartbeatKeyNetworkTXBytesPerSec   = "network_tx_bytes_per_sec"
)

var CanonicalHeartbeatMetricKeys = []string{
	HeartbeatKeyCPUPercent,
	HeartbeatKeyCPUUsedPercent,
	HeartbeatKeyMemoryPercent,
	HeartbeatKeyMemoryUsedPercent,
	HeartbeatKeyDiskPercent,
	HeartbeatKeyDiskUsedPercent,
	HeartbeatKeyTempCelsius,
	HeartbeatKeyTemperatureCelsius,
	HeartbeatKeyNetworkRXBytesPerSec,
	HeartbeatKeyNetworkTXBytesPerSec,
}
