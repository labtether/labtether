package demo

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"time"
)

// Host is one pretend machine of the demo home lab. The demo fleet sends the
// same messages for it that a real agent sends (heartbeat, telemetry, Docker
// inventory and stats, discovered web services), so every console page reads
// demo data through the same code paths as real data.
//
// The asset ID is the host name, the way an agent registers. The SQL seed
// (scripts/generate-demo-seed.py) creates the same hosts with their groups,
// links and metric history; keep the two lists in step.
type Host struct {
	ID       string
	Platform string // linux, windows or darwin
	OS       OS
	IP       string
	CPUModel string
	Arch     string // amd64 or arm64
	Cores    int
	Threads  int
	MemoryGB float64
	DiskGB   float64

	// Load shape: CPU swings around CPUBase by CPUSwing over ten minutes;
	// memory and disk stay near their base.
	CPUBase  float64
	CPUSwing float64
	MemBase  float64
	DiskBase float64

	// Offline hosts send nothing; the seed gives them a stale last-seen time.
	Offline bool

	Docker   *DockerEngine
	Services []Service // web services found by the agent's port scan
}

// OS holds the operating system facts an agent reports in its heartbeat.
type OS struct {
	ID      string // os-release ID: debian, ubuntu, fedora
	Name    string
	Version string
	Pretty  string
	Kernel  string
}

// DockerEngine is the Docker daemon on a host and its Compose stacks.
type DockerEngine struct {
	Version    string
	APIVersion string
	Stacks     []Stack
}

// Stack is one Compose project. Its files live in /opt/stacks/<name>.
type Stack struct {
	Name       string
	Containers []Container
}

// Container is one container of a stack.
type Container struct {
	Name     string
	Image    string
	Port     int     // published host port, 0 for none
	Internal int     // container port behind Port
	MemoryMB float64 // typical memory use
	CPU      float64 // typical CPU percent
	AgeDays  int     // days since it was created
	// Restarting marks a container stuck in a restart loop.
	Restarting bool
}

// Service is a web service on a host that is not a container.
type Service struct {
	Name string
	URL  string
	Port int
}

// OnlineHosts returns the hosts that send messages.
func OnlineHosts() []Host {
	hosts := make([]Host, 0, len(fleet))
	for _, host := range fleet {
		if !host.Offline {
			hosts = append(hosts, host)
		}
	}
	return hosts
}

// Hosts returns every host of the demo fleet, offline ones included.
func Hosts() []Host {
	return append([]Host(nil), fleet...)
}

// containerID derives a stable 64-character container ID.
func containerID(hostID, name string) string {
	sum := sha256.Sum256([]byte("labtether-demo:" + hostID + ":" + name))
	return hex.EncodeToString(sum[:])
}

// serviceID mirrors the agent's short web service ID: a hash of host, source
// and identifier.
func serviceID(hostID, source, identifier string) string {
	sum := sha256.Sum256([]byte(hostID + ":" + source + ":" + identifier))
	return hex.EncodeToString(sum[:8])
}

// cpuPercent is a ten-minute wave around the host's base load plus noise.
func (h Host) cpuPercent(now time.Time) float64 {
	phase := 2 * math.Pi * float64(now.Unix()) / 600.0
	noise := randomFloat64()*8 - 4
	return clamp(h.CPUBase+h.CPUSwing*math.Sin(phase)+noise, 1, 97)
}

func (h Host) memoryPercent() float64 {
	return clamp(h.MemBase+randomFloat64()*3-1.5, 5, 97)
}

func (h Host) diskPercent() float64 {
	return clamp(h.DiskBase+randomFloat64()*0.4-0.2, 1, 99)
}

// temperature follows CPU load: about 38 C idle to 70 C at full load.
func temperature(cpuPercent float64) float64 {
	return clamp(38+cpuPercent*0.32+randomFloat64()*3-1.5, 30, 85)
}

// networkBytesPerSec is mostly 1-30 Mbit/s with an occasional burst of
// 50-400 Mbit/s (125000 bytes per second per Mbit/s).
func networkBytesPerSec() float64 {
	mbps := 1 + randomFloat64()*29
	if randomFloat64() > 0.9 {
		mbps = 50 + randomFloat64()*350
	}
	return mbps * 125000
}

// clamp restricts v to the range [low, high].
func clamp(v, low, high float64) float64 {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}
