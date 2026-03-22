package generic

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	gohost "github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"

	"github.com/labtether/labtether/internal/agentcore"
	"github.com/labtether/labtether/internal/agentplatform/temperature"
)

type Provider struct {
	assetID  string
	source   string
	mode     string
	hostname string

	mu        sync.Mutex
	prevNetRX float64
	prevNetTX float64
	prevNetAt time.Time

	staticMetadata map[string]string
}

func New(assetID, source, mode string) *Provider {
	hostname := ""
	if value, err := gohost.Info(); err == nil {
		hostname = strings.TrimSpace(value.Hostname)
	}
	if hostname == "" {
		hostname = "unknown-host"
	}

	provider := &Provider{
		assetID:   assetID,
		source:    source,
		mode:      mode,
		hostname:  hostname,
		prevNetAt: time.Time{},
	}
	provider.staticMetadata = provider.buildStaticMetadata()
	return provider
}

func (p *Provider) AgentInfo() agentcore.AgentInfo {
	return agentcore.AgentInfo{
		OS:     runtime.GOOS,
		Mode:   p.mode,
		Status: "active",
	}
}

func (p *Provider) StaticMetadata() map[string]string {
	out := make(map[string]string, len(p.staticMetadata))
	for key, value := range p.staticMetadata {
		out[key] = value
	}
	return out
}

func (p *Provider) Collect(now time.Time) (agentcore.TelemetrySample, error) {
	cpuPercent := readCPUPercent()
	memoryPercent := readMemoryPercent()
	diskPercent := readDiskPercent(defaultRootPath())
	netRX, netTX := readNetworkBytes()
	netRXPerSec, netTXPerSec := p.computeNetworkRates(now, netRX, netTX)

	temp, tempErr := temperature.ReadCelsius()

	sample := agentcore.TelemetrySample{
		AssetID:          p.assetID,
		CPUPercent:       agentcore.ClampPercent(cpuPercent),
		MemoryPercent:    agentcore.ClampPercent(memoryPercent),
		DiskPercent:      agentcore.ClampPercent(diskPercent),
		NetRXBytes:       netRX,
		NetTXBytes:       netTX,
		NetRXBytesPerSec: netRXPerSec,
		NetTXBytesPerSec: netTXPerSec,
		TempCelsius:      temp,
		CollectedAt:      now.UTC(),
	}
	return sample, tempErr
}

func (p *Provider) computeNetworkRates(now time.Time, netRX, netTX float64) (float64, float64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	netRXPerSec := 0.0
	netTXPerSec := 0.0
	if !p.prevNetAt.IsZero() {
		deltaSeconds := now.Sub(p.prevNetAt).Seconds()
		if deltaSeconds > 0 {
			deltaRX := netRX - p.prevNetRX
			if deltaRX >= 0 {
				netRXPerSec = deltaRX / deltaSeconds
			}
			deltaTX := netTX - p.prevNetTX
			if deltaTX >= 0 {
				netTXPerSec = deltaTX / deltaSeconds
			}
		}
	}

	p.prevNetRX = netRX
	p.prevNetTX = netTX
	p.prevNetAt = now
	return netRXPerSec, netTXPerSec
}

func (p *Provider) buildStaticMetadata() map[string]string {
	metadata := map[string]string{
		"hostname":         p.hostname,
		"agent":            p.source,
		"cpu_architecture": runtime.GOARCH,
		"os":               runtime.GOOS,
		"platform":         runtime.GOOS,
	}

	info, err := gohost.Info()
	if err != nil {
		return metadata
	}
	if strings.TrimSpace(info.Platform) != "" {
		metadata["os_name"] = info.Platform
	}
	if strings.TrimSpace(info.PlatformVersion) != "" {
		metadata["os_version"] = info.PlatformVersion
	}
	if strings.TrimSpace(info.KernelVersion) != "" {
		metadata["kernel_version"] = info.KernelVersion
	}
	if strings.TrimSpace(info.KernelArch) != "" {
		metadata["kernel_arch"] = info.KernelArch
	}
	if strings.TrimSpace(info.HostID) != "" {
		metadata["host_id"] = info.HostID
	}

	// CPU info
	cpuInfos, err := cpu.Info()
	if err == nil && len(cpuInfos) > 0 {
		if strings.TrimSpace(cpuInfos[0].ModelName) != "" {
			metadata["cpu_model"] = cpuInfos[0].ModelName
		}
		if strings.TrimSpace(cpuInfos[0].VendorID) != "" {
			metadata["cpu_vendor"] = cpuInfos[0].VendorID
		}
		// Count physical cores
		physicalCores, err := cpu.Counts(false)
		if err == nil && physicalCores > 0 {
			metadata["cpu_cores_physical"] = fmt.Sprintf("%d", physicalCores)
		}
		// Count logical cores
		logicalCores, err := cpu.Counts(true)
		if err == nil && logicalCores > 0 {
			metadata["cpu_threads_logical"] = fmt.Sprintf("%d", logicalCores)
		}
		if cpuInfos[0].Mhz > 0 {
			metadata["cpu_max_mhz"] = fmt.Sprintf("%.0f", cpuInfos[0].Mhz)
		}
	}

	// Memory info
	memInfo, err := mem.VirtualMemory()
	if err == nil && memInfo.Total > 0 {
		metadata["memory_total_bytes"] = fmt.Sprintf("%d", memInfo.Total)
	}

	// Disk info
	diskInfo, err := disk.Usage(defaultRootPath())
	if err == nil && diskInfo.Total > 0 {
		metadata["disk_root_total_bytes"] = fmt.Sprintf("%d", diskInfo.Total)
	}

	// Network interface count
	interfaces, err := net.Interfaces()
	if err == nil {
		nonLoopback := 0
		for _, iface := range interfaces {
			// Skip loopback
			isLoopback := false
			for _, flag := range iface.Flags {
				if flag == "loopback" {
					isLoopback = true
					break
				}
			}
			if !isLoopback {
				nonLoopback++
			}
		}
		if nonLoopback > 0 {
			metadata["network_interface_count"] = fmt.Sprintf("%d", nonLoopback)
		}
	}

	return metadata
}

func readCPUPercent() float64 {
	values, err := cpu.Percent(0, false)
	if err != nil || len(values) == 0 {
		return 0
	}
	return values[0]
}

func readMemoryPercent() float64 {
	stats, err := mem.VirtualMemory()
	if err != nil {
		return 0
	}
	return stats.UsedPercent
}

func readDiskPercent(path string) float64 {
	stats, err := disk.Usage(path)
	if err != nil {
		return 0
	}
	return stats.UsedPercent
}

func readNetworkBytes() (float64, float64) {
	stats, err := net.IOCounters(false)
	if err != nil || len(stats) == 0 {
		return 0, 0
	}
	return float64(stats[0].BytesRecv), float64(stats[0].BytesSent)
}

func defaultRootPath() string {
	if runtime.GOOS == "windows" {
		return "C:\\"
	}
	return "/"
}
