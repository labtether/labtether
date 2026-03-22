//go:build linux

package linux

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestNewCollectsStaticMetadataAndClonesResponses(t *testing.T) {
	oldHostnameFunc := hostnameFunc
	oldReadOSMetadataFunc := readOSMetadataFunc
	oldReadDMIInfoFunc := readDMIInfoFunc
	oldReadCPUInfoFunc := readCPUInfoFunc
	oldReadMemTotalBytesFunc := readMemTotalBytesFunc
	oldReadDiskCapacityBytesFunc := readDiskCapacityBytesFunc
	oldReadNetworkInterfaceCountFunc := readNetworkInterfaceCountFunc
	oldReadTailscaleMetadataFunc := readTailscaleMetadataFunc
	oldReadCapabilityMetadataFunc := readCapabilityMetadataFunc
	t.Cleanup(func() {
		hostnameFunc = oldHostnameFunc
		readOSMetadataFunc = oldReadOSMetadataFunc
		readDMIInfoFunc = oldReadDMIInfoFunc
		readCPUInfoFunc = oldReadCPUInfoFunc
		readMemTotalBytesFunc = oldReadMemTotalBytesFunc
		readDiskCapacityBytesFunc = oldReadDiskCapacityBytesFunc
		readNetworkInterfaceCountFunc = oldReadNetworkInterfaceCountFunc
		readTailscaleMetadataFunc = oldReadTailscaleMetadataFunc
		readCapabilityMetadataFunc = oldReadCapabilityMetadataFunc
	})

	hostnameFunc = func() (string, error) { return "audit-host", nil }
	readOSMetadataFunc = func() map[string]string {
		return map[string]string{
			"os_pretty_name": "Audit Linux 1.0",
			"kernel_release": "6.9.0-test",
		}
	}
	readDMIInfoFunc = func() map[string]string {
		return map[string]string{
			"computer_vendor": "Acme",
		}
	}
	readCPUInfoFunc = func() map[string]string {
		return map[string]string{
			"cpu_model": "Audit CPU",
		}
	}
	readMemTotalBytesFunc = func() uint64 { return 32 * 1024 * 1024 * 1024 }
	readDiskCapacityBytesFunc = func(string) (uint64, uint64) { return 512000, 128000 }
	readNetworkInterfaceCountFunc = func() int { return 3 }
	readTailscaleMetadataFunc = func() map[string]string {
		return map[string]string{
			"tailscale_installed": "true",
			"tailscale_tailnet":   "audit.ts.net",
		}
	}
	readCapabilityMetadataFunc = func() map[string]string {
		return map[string]string{
			"cap_services": "list,action",
			"cap_network":  "list,action",
		}
	}

	provider := New("asset-123", "phase4-audit")
	if provider.hostname != "audit-host" {
		t.Fatalf("hostname=%q, want audit-host", provider.hostname)
	}

	info := provider.AgentInfo()
	if info.OS != "linux" || info.Mode != "interactive-terminal-first" || info.Status != "active" {
		t.Fatalf("unexpected agent info: %+v", info)
	}

	metadata := provider.StaticMetadata()
	if metadata["hostname"] != "audit-host" {
		t.Fatalf("hostname metadata=%q, want audit-host", metadata["hostname"])
	}
	if metadata["agent"] != "phase4-audit" {
		t.Fatalf("agent metadata=%q, want phase4-audit", metadata["agent"])
	}
	if metadata["cpu_architecture"] != runtime.GOARCH {
		t.Fatalf("cpu_architecture=%q, want %q", metadata["cpu_architecture"], runtime.GOARCH)
	}
	if metadata["memory_total_bytes"] != "34359738368" {
		t.Fatalf("memory_total_bytes=%q, want 34359738368", metadata["memory_total_bytes"])
	}
	if metadata["disk_root_total_bytes"] != "512000" || metadata["disk_root_available_bytes"] != "128000" {
		t.Fatalf("unexpected disk capacity metadata: total=%q available=%q", metadata["disk_root_total_bytes"], metadata["disk_root_available_bytes"])
	}
	if metadata["network_interface_count"] != "3" {
		t.Fatalf("network_interface_count=%q, want 3", metadata["network_interface_count"])
	}
	if metadata["tailscale_tailnet"] != "audit.ts.net" {
		t.Fatalf("tailscale_tailnet=%q, want audit.ts.net", metadata["tailscale_tailnet"])
	}
	if metadata["cap_services"] != "list,action" {
		t.Fatalf("cap_services=%q, want list,action", metadata["cap_services"])
	}

	metadata["hostname"] = "mutated"
	if provider.StaticMetadata()["hostname"] != "audit-host" {
		t.Fatal("StaticMetadata returned a mutable backing map")
	}
}

func TestCollectReadsTelemetryFixturesAndCachesSlowSensors(t *testing.T) {
	restore := swapProviderTelemetryFixtures(t)
	defer restore()

	procStat := filepath.Join(t.TempDir(), "proc.stat")
	procMeminfo := filepath.Join(t.TempDir(), "proc.meminfo")
	procNetDev := filepath.Join(t.TempDir(), "proc.net.dev")
	procStatPath = procStat
	procMeminfoPath = procMeminfo
	procNetDevPath = procNetDev

	writeFixture(t, procStat, "cpu  100 0 0 400 0 0 0 0 0 0\n")
	writeFixture(t, procMeminfo, "MemTotal:       100000 kB\nMemAvailable:    25000 kB\n")
	writeFixture(t, procNetDev, networkFixture(1000, 2000))

	statfsCalls := 0
	statfsFunc = func(_ string, stats *syscall.Statfs_t) error {
		statfsCalls++
		stats.Bsize = 4096
		stats.Blocks = 1000
		stats.Bfree = 250
		stats.Bavail = 250
		return nil
	}

	tempCalls := 0
	tempValue := 51.5
	provider := &Provider{
		assetID: "asset-123",
		readTemperature: func() (*float64, error) {
			tempCalls++
			return &tempValue, nil
		},
	}

	start := time.Date(2026, time.March, 8, 12, 0, 0, 0, time.FixedZone("UTC+11", 11*60*60))
	first, err := provider.Collect(start)
	if err != nil {
		t.Fatalf("first collect returned error: %v", err)
	}
	assertFloat(t, first.CPUPercent, 0, 0.001, "first cpu percent")
	assertFloat(t, first.MemoryPercent, 75, 0.001, "memory percent")
	assertFloat(t, first.DiskPercent, 75, 0.001, "disk percent")
	assertFloat(t, first.NetRXBytes, 1000, 0.001, "first rx bytes")
	assertFloat(t, first.NetTXBytes, 2000, 0.001, "first tx bytes")
	assertFloat(t, first.NetRXBytesPerSec, 0, 0.001, "first rx rate")
	assertFloat(t, first.NetTXBytesPerSec, 0, 0.001, "first tx rate")
	if first.TempCelsius == nil || *first.TempCelsius != 51.5 {
		t.Fatalf("first temperature=%v, want 51.5", first.TempCelsius)
	}
	if !first.CollectedAt.Equal(start.UTC()) {
		t.Fatalf("first collected_at=%s, want %s", first.CollectedAt, start.UTC())
	}

	writeFixture(t, procStat, "cpu  140 0 0 440 20 0 0 0 0 0\n")
	writeFixture(t, procNetDev, networkFixture(1600, 2600))

	second, err := provider.Collect(start.Add(10 * time.Second))
	if err != nil {
		t.Fatalf("second collect returned error: %v", err)
	}
	assertFloat(t, second.CPUPercent, 40, 0.001, "second cpu percent")
	assertFloat(t, second.MemoryPercent, 75, 0.001, "second memory percent")
	assertFloat(t, second.DiskPercent, 75, 0.001, "second disk percent")
	assertFloat(t, second.NetRXBytes, 1600, 0.001, "second rx bytes")
	assertFloat(t, second.NetTXBytes, 2600, 0.001, "second tx bytes")
	assertFloat(t, second.NetRXBytesPerSec, 60, 0.001, "second rx rate")
	assertFloat(t, second.NetTXBytesPerSec, 60, 0.001, "second tx rate")
	if second.TempCelsius == nil || *second.TempCelsius != 51.5 {
		t.Fatalf("second temperature=%v, want cached 51.5", second.TempCelsius)
	}
	if statfsCalls != 1 {
		t.Fatalf("statfs calls=%d, want 1 due to disk cache", statfsCalls)
	}
	if tempCalls != 1 {
		t.Fatalf("temperature reads=%d, want 1 due to temp cache", tempCalls)
	}
}

func TestCollectRetainsLastTemperatureOnRefreshError(t *testing.T) {
	restore := swapProviderTelemetryFixtures(t)
	defer restore()

	procStat := filepath.Join(t.TempDir(), "proc.stat")
	procMeminfo := filepath.Join(t.TempDir(), "proc.meminfo")
	procNetDev := filepath.Join(t.TempDir(), "proc.net.dev")
	procStatPath = procStat
	procMeminfoPath = procMeminfo
	procNetDevPath = procNetDev

	writeFixture(t, procStat, "cpu  1 0 0 9 0 0 0 0 0 0\n")
	writeFixture(t, procMeminfo, "MemTotal:       2000 kB\nMemAvailable:   1000 kB\n")
	writeFixture(t, procNetDev, networkFixture(10, 20))

	statfsFunc = func(_ string, stats *syscall.Statfs_t) error {
		stats.Bsize = 4096
		stats.Blocks = 100
		stats.Bfree = 50
		stats.Bavail = 50
		return nil
	}

	tempCalls := 0
	lastGood := 44.0
	provider := &Provider{
		assetID: "asset-123",
		readTemperature: func() (*float64, error) {
			tempCalls++
			if tempCalls == 1 {
				return &lastGood, nil
			}
			return nil, errors.New("sensor offline")
		},
	}

	start := time.Date(2026, time.March, 8, 9, 0, 0, 0, time.UTC)
	first, err := provider.Collect(start)
	if err != nil {
		t.Fatalf("first collect returned error: %v", err)
	}
	if first.TempCelsius == nil || *first.TempCelsius != 44 {
		t.Fatalf("first temperature=%v, want 44", first.TempCelsius)
	}

	writeFixture(t, procStat, "cpu  2 0 0 18 0 0 0 0 0 0\n")
	writeFixture(t, procNetDev, networkFixture(40, 70))

	second, err := provider.Collect(start.Add(tempSampleInterval + time.Second))
	if err == nil || err.Error() != "sensor offline" {
		t.Fatalf("second collect error=%v, want sensor offline", err)
	}
	if second.TempCelsius == nil || *second.TempCelsius != 44 {
		t.Fatalf("second temperature=%v, want retained 44", second.TempCelsius)
	}
	if tempCalls != 2 {
		t.Fatalf("temperature reads=%d, want 2", tempCalls)
	}
}

func swapProviderTelemetryFixtures(t *testing.T) func() {
	t.Helper()

	oldProcStatPath := procStatPath
	oldProcMeminfoPath := procMeminfoPath
	oldProcNetDevPath := procNetDevPath
	oldStatfsFunc := statfsFunc

	return func() {
		procStatPath = oldProcStatPath
		procMeminfoPath = oldProcMeminfoPath
		procNetDevPath = oldProcNetDevPath
		statfsFunc = oldStatfsFunc
	}
}

func writeFixture(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", path, err)
	}
}

func networkFixture(rxBytes, txBytes int) string {
	return "Inter-|   Receive                                                |  Transmit\n" +
		" face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed\n" +
		"    lo: 100 0 0 0 0 0 0 0 200 0 0 0 0 0 0 0\n" +
		"  eth0: " + itoa(rxBytes) + " 0 0 0 0 0 0 0 " + itoa(txBytes) + " 0 0 0 0 0 0 0\n"
}

func assertFloat(t *testing.T, got, want, epsilon float64, label string) {
	t.Helper()

	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	if diff > epsilon {
		t.Fatalf("%s=%f, want %f", label, got, want)
	}
}

func itoa(value int) string {
	return strconv.Itoa(value)
}
