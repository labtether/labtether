package sysconfig

import (
	"context"
	"log"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// NetworkMonitor periodically checks local IP addresses and signals OnChange
// when they differ from the previous check. This detects WiFi switches,
// VPN toggles, and sleep/wake network changes.
type NetworkMonitor struct {
	OnChange chan struct{}
	Interval time.Duration
	mu       sync.Mutex
	GetIPs   func() string // injectable for testing
}

func NewNetworkMonitor(onChange chan struct{}) *NetworkMonitor {
	return &NetworkMonitor{
		OnChange: onChange,
		Interval: 30 * time.Second,
		GetIPs:   func() string { return GetLocalIPs() },
	}
}

func (m *NetworkMonitor) Run(ctx context.Context) {
	m.mu.Lock()
	getIPs := m.GetIPs
	m.mu.Unlock()

	lastIPs := getIPs()
	ticker := time.NewTicker(m.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.mu.Lock()
			getIPs = m.GetIPs
			m.mu.Unlock()

			currentIPs := getIPs()
			if currentIPs != lastIPs {
				log.Printf("agentws: network change detected (was=%q, now=%q)", lastIPs, currentIPs)
				lastIPs = currentIPs
				select {
				case m.OnChange <- struct{}{}:
				default:
				}
			}
		}
	}
}

// GetLocalIPs returns a sorted, comma-separated string of all non-loopback
// unicast IP addresses. Used to detect network changes.
func GetLocalIPs() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	var ips []string
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.IsGlobalUnicast() {
			ips = append(ips, ipNet.IP.String())
		}
	}
	sort.Strings(ips)
	return strings.Join(ips, ",")
}
