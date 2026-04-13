package protocols

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestValidateManualDeviceHost(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		wantErr bool
	}{
		{"private 192.168", "192.168.1.50", false},
		{"private 10.x", "10.0.0.1", false},
		{"private 172.16", "172.16.0.1", false},
		{"private 172.31", "172.31.255.255", false},
		{"public IP", "8.8.8.8", false},
		{"hostname", "nas.local", false},
		{"fqdn", "server.example.com", false},
		{"loopback", "127.0.0.1", true},
		{"loopback full", "127.0.0.2", true},
		{"link-local", "169.254.1.1", true},
		{"metadata", "169.254.169.254", true},
		{"empty", "", true},
		{"whitespace only", "   ", true},
		{"contains whitespace", "nas local", true},
		{"localhost", "localhost", true},
		{"unspecified ipv4", "0.0.0.0", true},
		{"broadcast ipv4", "255.255.255.255", true},
		{"multicast ipv4", "224.0.0.1", true},
		{"unspecified ipv6", "::", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateManualDeviceHost(tt.host)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateManualDeviceHost(%q) error = %v, wantErr %v", tt.host, err, tt.wantErr)
			}
		})
	}
}

func TestDialManualDeviceTCPTimeoutRejectsDisallowedResolvedIP(t *testing.T) {
	originalLookup := lookupManualDeviceIPAddrs
	lookupManualDeviceIPAddrs = func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host != "bad.example.test" {
			t.Fatalf("unexpected host lookup: %s", host)
		}
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}
	defer func() {
		lookupManualDeviceIPAddrs = originalLookup
	}()

	conn, err := DialManualDeviceTCPTimeout(context.Background(), "bad.example.test", 5900, 100*time.Millisecond)
	if err == nil {
		if conn != nil {
			conn.Close()
		}
		t.Fatal("expected disallowed resolution error")
	}
	if conn != nil {
		t.Fatal("expected nil connection on disallowed resolution")
	}
}

func TestDialManualDeviceTCPTimeoutUsesValidatedResolvedIP(t *testing.T) {
	originalLookup := lookupManualDeviceIPAddrs
	originalDial := dialManualDeviceTCPContext
	defer func() {
		lookupManualDeviceIPAddrs = originalLookup
		dialManualDeviceTCPContext = originalDial
	}()

	lookupManualDeviceIPAddrs = func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host != "nas.example.test" {
			t.Fatalf("unexpected host lookup: %s", host)
		}
		return []net.IPAddr{{IP: net.ParseIP("192.168.1.20")}}, nil
	}

	dialedAddress := ""
	dialManualDeviceTCPContext = func(_ context.Context, network, address string, timeout time.Duration) (net.Conn, error) {
		if network != "tcp" {
			t.Fatalf("unexpected network: %s", network)
		}
		if timeout != time.Second {
			t.Fatalf("unexpected timeout: %s", timeout)
		}
		dialedAddress = address
		serverConn, clientConn := net.Pipe()
		go serverConn.Close()
		return clientConn, nil
	}

	conn, err := DialManualDeviceTCPTimeout(context.Background(), "nas.example.test", 5900, time.Second)
	if err != nil {
		t.Fatalf("DialManualDeviceTCPTimeout returned error: %v", err)
	}
	defer conn.Close()

	if dialedAddress != "192.168.1.20:5900" {
		t.Fatalf("expected dial to validated IP, got %q", dialedAddress)
	}
}

func TestDialManualDeviceTCPTimeoutFallsBackAcrossResolvedIPs(t *testing.T) {
	originalLookup := lookupManualDeviceIPAddrs
	originalDial := dialManualDeviceTCPContext
	defer func() {
		lookupManualDeviceIPAddrs = originalLookup
		dialManualDeviceTCPContext = originalDial
	}()

	lookupManualDeviceIPAddrs = func(_ context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{
			{IP: net.ParseIP("192.168.1.10")},
			{IP: net.ParseIP("192.168.1.11")},
		}, nil
	}

	attempts := 0
	dialManualDeviceTCPContext = func(_ context.Context, _ string, address string, _ time.Duration) (net.Conn, error) {
		attempts++
		if attempts == 1 {
			return nil, errors.New("first address refused")
		}
		if address != "192.168.1.11:5901" {
			t.Fatalf("unexpected fallback dial address: %s", address)
		}
		serverConn, clientConn := net.Pipe()
		go serverConn.Close()
		return clientConn, nil
	}

	conn, err := DialManualDeviceTCPTimeout(context.Background(), "multi.example.test", 5901, time.Second)
	if err != nil {
		t.Fatalf("DialManualDeviceTCPTimeout returned error: %v", err)
	}
	defer conn.Close()

	if attempts != 2 {
		t.Fatalf("expected 2 dial attempts, got %d", attempts)
	}
}
