package modelregistry

import "github.com/labtether/labtether/internal/model"

var metricCatalog = []model.MetricDescriptor{
	{ID: "cpu_used_percent", Unit: "percent", Type: model.MetricTypeGauge, Class: model.MetricClassUtilization, TargetKinds: []string{"host", "hypervisor-node", "vm", "container", "container-host"}},
	{ID: "memory_used_percent", Unit: "percent", Type: model.MetricTypeGauge, Class: model.MetricClassUtilization, TargetKinds: []string{"host", "hypervisor-node", "vm", "container", "container-host"}},
	{ID: "disk_used_percent", Unit: "percent", Type: model.MetricTypeGauge, Class: model.MetricClassCapacity, TargetKinds: []string{"host", "hypervisor-node", "vm", "container", "container-host", "storage-pool", "datastore", "dataset"}},
	{ID: "temperature_celsius", Unit: "celsius", Type: model.MetricTypeGauge, Class: model.MetricClassTemperature, TargetKinds: []string{"host", "hypervisor-node", "disk", "storage-controller"}},
	{ID: "network_rx_bytes_per_sec", Unit: "bytes_per_second", Type: model.MetricTypeRate, Class: model.MetricClassThroughput, TargetKinds: []string{"host", "hypervisor-node", "vm", "container", "container-host", "interface"}},
	{ID: "network_tx_bytes_per_sec", Unit: "bytes_per_second", Type: model.MetricTypeRate, Class: model.MetricClassThroughput, TargetKinds: []string{"host", "hypervisor-node", "vm", "container", "container-host", "interface"}},
	{ID: "uptime_seconds", Unit: "seconds", Type: model.MetricTypeGauge, Class: model.MetricClassAvailability, TargetKinds: []string{"host", "hypervisor-node", "vm", "container", "storage-controller"}},
	{ID: "availability_state", Unit: "state", Type: model.MetricTypeState, Class: model.MetricClassAvailability},
	{ID: "backup_age_seconds", Unit: "seconds", Type: model.MetricTypeGauge, Class: model.MetricClassAvailability, TargetKinds: []string{"vm", "container", "datastore"}},
	{ID: "iops_read", Unit: "ops_per_second", Type: model.MetricTypeRate, Class: model.MetricClassThroughput, TargetKinds: []string{"storage-pool", "datastore", "disk"}},
	{ID: "iops_write", Unit: "ops_per_second", Type: model.MetricTypeRate, Class: model.MetricClassThroughput, TargetKinds: []string{"storage-pool", "datastore", "disk"}},
	{ID: "latency_ms", Unit: "milliseconds", Type: model.MetricTypeGauge, Class: model.MetricClassAvailability, TargetKinds: []string{"service", "ha-entity", "network"}},
}

func MetricCatalog() []model.MetricDescriptor {
	if len(metricCatalog) == 0 {
		return nil
	}
	out := make([]model.MetricDescriptor, len(metricCatalog))
	copy(out, metricCatalog)
	for idx := range out {
		out[idx].TargetKinds = cloneStrings(out[idx].TargetKinds)
	}
	return out
}
