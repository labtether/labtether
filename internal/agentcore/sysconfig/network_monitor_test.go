package sysconfig

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetLocalIPs(t *testing.T) {
	t.Parallel()
	ips := GetLocalIPs()
	if ips == "" {
		t.Skip("no non-loopback network interfaces available")
	}
	t.Logf("detected local IPs: %s", ips)
}

func TestNetworkMonitorDetectsChange(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch := make(chan struct{}, 1)
	var ipValue atomic.Value
	ipValue.Store("192.168.1.1")

	mon := &NetworkMonitor{
		OnChange: ch,
		Interval: 50 * time.Millisecond,
		GetIPs:   func() string { return ipValue.Load().(string) },
	}

	go mon.Run(ctx)

	// Let it do one tick with the initial value.
	time.Sleep(80 * time.Millisecond)

	// Change the IP.
	ipValue.Store("10.0.0.5")

	select {
	case <-ch:
		// Got change notification — success.
	case <-ctx.Done():
		t.Fatal("timeout waiting for network change notification")
	}
}

func TestNetworkMonitorNoFalsePositive(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	ch := make(chan struct{}, 1)
	mon := &NetworkMonitor{
		OnChange: ch,
		Interval: 50 * time.Millisecond,
		GetIPs:   func() string { return "192.168.1.1" },
	}

	go mon.Run(ctx)

	select {
	case <-ch:
		t.Fatal("got unexpected network change notification")
	case <-ctx.Done():
		// No false positive — success.
	}
}
