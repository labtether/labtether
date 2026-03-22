//go:build linux

package linux

import (
	"os"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/agentcore"
	"github.com/labtether/labtether/internal/agentplatform/temperature"
)

var hostnameFunc = os.Hostname

type cpuSample struct {
	idle  uint64
	total uint64
}

const (
	// Disk usage moves relatively slowly; avoid polling Statfs every telemetry tick.
	diskSampleInterval = 1 * time.Minute
	// Temperature probing can be expensive on some systems (gopsutil + sensor scans).
	tempSampleInterval = 30 * time.Second
)

type Provider struct {
	assetID  string
	source   string
	hostname string

	mu        sync.Mutex
	prevCPU   cpuSample
	havePrev  bool
	prevNetRX float64
	prevNetTX float64
	prevNetAt time.Time

	lastDiskPercent float64
	lastDiskAt      time.Time
	lastTemp        *float64
	lastTempAt      time.Time

	staticMetadata map[string]string

	readTemperature func() (*float64, error)
}

func New(assetID, source string) *Provider {
	hostname, _ := hostnameFunc()
	provider := &Provider{
		assetID:         assetID,
		source:          source,
		hostname:        hostname,
		prevNetAt:       time.Time{},
		readTemperature: temperature.ReadCelsius,
	}
	provider.staticMetadata = collectHostMetadata(hostname, source)
	return provider
}

func (p *Provider) AgentInfo() agentcore.AgentInfo {
	return agentcore.AgentInfo{
		OS:     "linux",
		Mode:   "interactive-terminal-first",
		Status: "active",
	}
}

func (p *Provider) StaticMetadata() map[string]string {
	return cloneStringMap(p.staticMetadata)
}

func (p *Provider) Collect(now time.Time) (agentcore.TelemetrySample, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	var cpuPercent float64
	currentCPU, cpuErr := readCPUSample()
	if cpuErr == nil && p.havePrev {
		deltaIdle := float64(currentCPU.idle - p.prevCPU.idle)
		deltaTotal := float64(currentCPU.total - p.prevCPU.total)
		if deltaTotal > 0 {
			cpuPercent = (1.0 - (deltaIdle / deltaTotal)) * 100.0
		}
	}
	p.prevCPU = currentCPU
	p.havePrev = cpuErr == nil

	memPercent := readMemoryUsagePercent()
	diskPercent := p.lastDiskPercent
	if p.lastDiskAt.IsZero() || now.Sub(p.lastDiskAt) >= diskSampleInterval {
		diskPercent = readDiskUsagePercent("/")
		p.lastDiskPercent = diskPercent
		p.lastDiskAt = now
	}

	netRX, netTX := readNetworkBytes()
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

	temp := cloneFloat64Ptr(p.lastTemp)
	var tempErr error
	if p.lastTempAt.IsZero() || now.Sub(p.lastTempAt) >= tempSampleInterval {
		temp, tempErr = p.readTemperature()
		p.lastTempAt = now
		if temp != nil {
			p.lastTemp = cloneFloat64Ptr(temp)
		}
	}
	if temp == nil && p.lastTemp != nil {
		temp = cloneFloat64Ptr(p.lastTemp)
	}

	sample := agentcore.TelemetrySample{
		AssetID:          p.assetID,
		CPUPercent:       agentcore.ClampPercent(cpuPercent),
		MemoryPercent:    agentcore.ClampPercent(memPercent),
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

func cloneStringMap(input map[string]string) map[string]string {
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func cloneFloat64Ptr(input *float64) *float64 {
	if input == nil {
		return nil
	}
	value := *input
	return &value
}
