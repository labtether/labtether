package sysconfig

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

// ---------------------------------------------------------------------------
// parseGetNetAdapterOutput
// ---------------------------------------------------------------------------

func TestParseGetNetAdapterOutput_Array(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "get_netadapter.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	adapters, err := parseGetNetAdapterOutput(data)
	if err != nil {
		t.Fatalf("parseGetNetAdapterOutput returned error: %v", err)
	}
	if len(adapters) != 4 {
		t.Fatalf("len(adapters)=%d, want 4", len(adapters))
	}

	eth := adapters[0]
	if eth.Name != "Ethernet" {
		t.Errorf("adapters[0].Name=%q, want Ethernet", eth.Name)
	}
	if eth.Status != "Up" {
		t.Errorf("adapters[0].Status=%q, want Up", eth.Status)
	}
	if eth.MacAddress != "AA-BB-CC-DD-EE-FF" {
		t.Errorf("adapters[0].MacAddress=%q, want AA-BB-CC-DD-EE-FF", eth.MacAddress)
	}
	if eth.InterfaceIndex != 4 {
		t.Errorf("adapters[0].InterfaceIndex=%d, want 4", eth.InterfaceIndex)
	}
	if eth.LinkSpeed != 1000000000 {
		t.Errorf("adapters[0].LinkSpeed=%d, want 1000000000", eth.LinkSpeed)
	}

	wifi := adapters[1]
	if wifi.Name != "Wi-Fi" {
		t.Errorf("adapters[1].Name=%q, want Wi-Fi", wifi.Name)
	}
	if wifi.Status != "Up" {
		t.Errorf("adapters[1].Status=%q, want Up", wifi.Status)
	}

	disconnected := adapters[2]
	if disconnected.Status != "Disconnected" {
		t.Errorf("adapters[2].Status=%q, want Disconnected", disconnected.Status)
	}
}

func TestParseGetNetAdapterOutput_SingleObject(t *testing.T) {
	single := `{
		"Name": "Ethernet",
		"InterfaceDescription": "Intel(R) Ethernet",
		"InterfaceIndex": 4,
		"MacAddress": "AA-BB-CC-DD-EE-FF",
		"Status": "Up",
		"LinkSpeed": 1000000000,
		"ifIndex": 4
	}`

	adapters, err := parseGetNetAdapterOutput([]byte(single))
	if err != nil {
		t.Fatalf("parseGetNetAdapterOutput returned error: %v", err)
	}
	if len(adapters) != 1 {
		t.Fatalf("len(adapters)=%d, want 1", len(adapters))
	}
	if adapters[0].Name != "Ethernet" {
		t.Errorf("Name=%q, want Ethernet", adapters[0].Name)
	}
}

func TestParseGetNetAdapterOutput_Empty(t *testing.T) {
	_, err := parseGetNetAdapterOutput([]byte(""))
	if err == nil {
		t.Fatal("expected error for empty input, got nil")
	}
}

func TestParseGetNetAdapterOutput_InvalidJSON(t *testing.T) {
	_, err := parseGetNetAdapterOutput([]byte("[{invalid json}]"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

// ---------------------------------------------------------------------------
// ParseWindowsIPConfig
// ---------------------------------------------------------------------------

func TestParseWindowsIPConfig_DHCP(t *testing.T) {
	raw := `
Configuration for interface "Ethernet"
    DHCP enabled:                         Yes
    IP Address:                           192.168.1.105
    Subnet Prefix:                        192.168.1.0/24 (mask 255.255.255.0)
    Default Gateway:                      192.168.1.1
    Gateway Metric:                       0
    InterfaceMetric:                      25
`
	snapshot := &WindowsNetworkSnapshot{InterfaceName: "Ethernet"}
	ParseWindowsIPConfig(raw, snapshot)

	if !snapshot.WasDHCP {
		t.Error("WasDHCP should be true")
	}
	if snapshot.StaticIP != "192.168.1.105" {
		t.Errorf("StaticIP=%q, want 192.168.1.105", snapshot.StaticIP)
	}
	if snapshot.SubnetMask != "192.168.1.0/24" {
		t.Errorf("SubnetMask=%q, want 192.168.1.0/24", snapshot.SubnetMask)
	}
	if snapshot.Gateway != "192.168.1.1" {
		t.Errorf("Gateway=%q, want 192.168.1.1", snapshot.Gateway)
	}
}

func TestParseWindowsIPConfig_Static(t *testing.T) {
	raw := `
Configuration for interface "Ethernet"
    DHCP enabled:                         No
    IP Address:                           10.0.0.50
    Subnet Mask:                          255.255.0.0
    Default Gateway:                      10.0.0.1
`
	snapshot := &WindowsNetworkSnapshot{InterfaceName: "Ethernet"}
	ParseWindowsIPConfig(raw, snapshot)

	if snapshot.WasDHCP {
		t.Error("WasDHCP should be false")
	}
	if snapshot.StaticIP != "10.0.0.50" {
		t.Errorf("StaticIP=%q, want 10.0.0.50", snapshot.StaticIP)
	}
	if snapshot.SubnetMask != "255.255.0.0" {
		t.Errorf("SubnetMask=%q, want 255.255.0.0", snapshot.SubnetMask)
	}
	if snapshot.Gateway != "10.0.0.1" {
		t.Errorf("Gateway=%q, want 10.0.0.1", snapshot.Gateway)
	}
}

func TestParseWindowsIPConfig_SubnetWithParenthetical(t *testing.T) {
	// netsh may emit "192.168.1.0/24 (mask 255.255.255.0)" — the parenthetical
	// suffix should be stripped.
	raw := `
    DHCP enabled:                         No
    IP Address:                           192.168.1.10
    Subnet Prefix:                        192.168.1.0/24 (mask 255.255.255.0)
    Default Gateway:                      192.168.1.1
`
	snapshot := &WindowsNetworkSnapshot{}
	ParseWindowsIPConfig(raw, snapshot)
	if snapshot.SubnetMask != "192.168.1.0/24" {
		t.Errorf("SubnetMask=%q, want 192.168.1.0/24", snapshot.SubnetMask)
	}
}

// ---------------------------------------------------------------------------
// ParseWindowsDNSServers
// ---------------------------------------------------------------------------

func TestParseWindowsDNSServers_Static(t *testing.T) {
	raw := `
DNS servers configured through DHCP: none

Statically Configured DNS Servers:    8.8.8.8
                                      1.1.1.1
`
	servers := ParseWindowsDNSServers(raw)
	want := []string{"8.8.8.8", "1.1.1.1"}
	if !reflect.DeepEqual(servers, want) {
		t.Errorf("servers=%v, want %v", servers, want)
	}
}

func TestParseWindowsDNSServers_DHCP(t *testing.T) {
	raw := `
DNS servers configured through DHCP:  8.8.8.8
                                      8.8.4.4
`
	servers := ParseWindowsDNSServers(raw)
	if len(servers) < 1 || servers[0] != "8.8.8.8" {
		t.Errorf("servers=%v, want at least [8.8.8.8 ...]", servers)
	}
}

func TestParseWindowsDNSServers_None(t *testing.T) {
	raw := `
DNS servers configured through DHCP:  None
Statically Configured DNS Servers:    None
`
	servers := ParseWindowsDNSServers(raw)
	if len(servers) != 0 {
		t.Errorf("expected no servers, got %v", servers)
	}
}

// ---------------------------------------------------------------------------
// ResolveWindowsNetworkMethod
// ---------------------------------------------------------------------------

func TestResolveWindowsNetworkMethod(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"", "netsh", false},
		{"auto", "netsh", false},
		{"netsh", "netsh", false},
		{"NETSH", "netsh", false},
		{"nmcli", "", true},
		{"networksetup", "", true},
	}

	for _, tc := range tests {
		got, err := ResolveWindowsNetworkMethod(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ResolveWindowsNetworkMethod(%q) expected error, got nil", tc.input)
			}
		} else {
			if err != nil {
				t.Errorf("ResolveWindowsNetworkMethod(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("ResolveWindowsNetworkMethod(%q)=%q, want %q", tc.input, got, tc.want)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// ResolveWindowsNetworkInterface
// ---------------------------------------------------------------------------

func TestResolveWindowsNetworkInterface_ExplicitName(t *testing.T) {
	iface, err := ResolveWindowsNetworkInterface("Ethernet")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if iface != "Ethernet" {
		t.Errorf("iface=%q, want Ethernet", iface)
	}
}

func TestResolveWindowsNetworkInterface_ColonSubAction(t *testing.T) {
	iface, err := ResolveWindowsNetworkInterface("Ethernet:dhcp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if iface != "Ethernet" {
		t.Errorf("iface=%q, want Ethernet", iface)
	}
}

func TestResolveWindowsNetworkInterface_ColonStatic(t *testing.T) {
	iface, err := ResolveWindowsNetworkInterface("Local Area Connection:static:10.0.0.5:255.255.0.0:10.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if iface != "Local Area Connection" {
		t.Errorf("iface=%q, want Local Area Connection", iface)
	}
}

// ---------------------------------------------------------------------------
// CloneWindowsNetworkSnapshot
// ---------------------------------------------------------------------------

func TestCloneWindowsNetworkSnapshot_Nil(t *testing.T) {
	if got := CloneWindowsNetworkSnapshot(nil); got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestCloneWindowsNetworkSnapshot_Independence(t *testing.T) {
	orig := &WindowsNetworkSnapshot{
		InterfaceName: "Ethernet",
		WasDHCP:       false,
		StaticIP:      "192.168.1.5",
		SubnetMask:    "255.255.255.0",
		Gateway:       "192.168.1.1",
		DNSServers:    []string{"8.8.8.8", "1.1.1.1"},
	}
	clone := CloneWindowsNetworkSnapshot(orig)

	if clone == orig {
		t.Fatal("clone should be a different pointer")
	}
	if !reflect.DeepEqual(orig, clone) {
		t.Errorf("clone does not equal original: orig=%+v clone=%+v", orig, clone)
	}

	// Mutate clone DNS; original must be unaffected.
	clone.DNSServers[0] = "9.9.9.9"
	if orig.DNSServers[0] != "8.8.8.8" {
		t.Error("mutating clone DNS slice affected original")
	}
}

// ---------------------------------------------------------------------------
// isIPAddress
// ---------------------------------------------------------------------------

func TestIsIPAddress(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"8.8.8.8", true},
		{"192.168.1.1", true},
		{"2001:db8::1", true},
		{"fe80::1%eth0", true},
		{"", false},
		{"None", false},
		{"DNS", false},
		{"1234", false},
	}
	for _, tc := range tests {
		got := isIPAddress(tc.s)
		if got != tc.want {
			t.Errorf("isIPAddress(%q)=%v, want %v", tc.s, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// WindowsNetworkBackend ApplyAction / RollbackAction — unit-level with stubs
// ---------------------------------------------------------------------------

// stubWindowsRun replaces WindowsRunCommandWithTimeout for the duration of fn.
// It records each call and feeds back outputs/errors in sequence.
func stubWindowsRun(outputs [][]byte, errs []error, fn func()) [][]string {
	orig := WindowsRunCommandWithTimeout
	var calls [][]string
	idx := 0
	WindowsRunCommandWithTimeout = func(_ time.Duration, name string, args ...string) ([]byte, error) {
		call := append([]string{name}, args...)
		calls = append(calls, call)
		var out []byte
		var err error
		if idx < len(outputs) {
			out = outputs[idx]
		}
		if idx < len(errs) {
			err = errs[idx]
		}
		idx++
		return out, err
	}
	defer func() { WindowsRunCommandWithTimeout = orig }()
	fn()
	return calls
}

func TestWindowsNetworkBackend_ApplyDHCP(t *testing.T) {
	nm := &NetworkManager{}
	backend := WindowsNetworkBackend{}

	outputs := [][]byte{
		// CaptureWindowsNetworkSnapshot: show config
		[]byte("    DHCP enabled:                         No\n    IP Address:                           10.0.0.5\n    Subnet Prefix:                        10.0.0.0/24 (mask 255.255.255.0)\n    Default Gateway:                      10.0.0.1\n"),
		// CaptureWindowsNetworkSnapshot: show dnsservers
		[]byte("    Statically Configured DNS Servers:    8.8.8.8\n"),
		// Apply: netsh interface ip set address ... dhcp
		[]byte("Ok.\n"),
		// VerifyWindowsConnectivity: ping (success)
		[]byte("Reply from 1.1.1.1: bytes=32\n"),
	}
	errs := make([]error, len(outputs))

	req := agentmgr.NetworkActionData{
		RequestID:  "req-1",
		Action:     "apply",
		Connection: "Ethernet:dhcp",
	}

	var result agentmgr.NetworkResultData
	calls := stubWindowsRun(outputs, errs, func() {
		result = backend.ApplyAction(nm, req)
	})

	if !result.OK {
		t.Errorf("expected OK=true, got error: %s", result.Error)
	}
	if result.RequestID != "req-1" {
		t.Errorf("RequestID=%q, want req-1", result.RequestID)
	}
	if result.RollbackReference != "Ethernet" {
		t.Errorf("RollbackReference=%q, want Ethernet", result.RollbackReference)
	}

	// Verify the DHCP apply command was issued.
	foundDHCP := false
	for _, c := range calls {
		if len(c) >= 7 && c[0] == "netsh" && c[len(c)-1] == "dhcp" {
			foundDHCP = true
		}
	}
	if !foundDHCP {
		t.Errorf("expected netsh ... dhcp call; calls=%v", calls)
	}

	// Verify snapshot was saved.
	nm.mu.Lock()
	snap := nm.LastWindowsSnapshot
	nm.mu.Unlock()
	if snap == nil {
		t.Fatal("LastWindowsSnapshot should not be nil after apply")
	}
	if snap.InterfaceName != "Ethernet" {
		t.Errorf("snapshot.InterfaceName=%q, want Ethernet", snap.InterfaceName)
	}
}

func TestWindowsNetworkBackend_RollbackNoSnapshot(t *testing.T) {
	nm := &NetworkManager{}
	backend := WindowsNetworkBackend{}

	req := agentmgr.NetworkActionData{
		RequestID: "req-2",
		Action:    "rollback",
	}
	result := backend.RollbackAction(nm, req)

	if result.OK {
		t.Error("expected OK=false when no snapshot exists")
	}
	if result.Error == "" {
		t.Error("expected a non-empty error message")
	}
}

func TestWindowsNetworkBackend_InvalidMethod(t *testing.T) {
	nm := &NetworkManager{}
	backend := WindowsNetworkBackend{}

	req := agentmgr.NetworkActionData{
		RequestID:  "req-3",
		Action:     "apply",
		Method:     "nmcli",
		Connection: "Ethernet:dhcp",
	}
	result := backend.ApplyAction(nm, req)

	if result.OK {
		t.Error("expected OK=false for unsupported method")
	}
	if result.Error == "" {
		t.Error("expected error message for unsupported method")
	}
}

func TestWindowsNetworkBackend_StaticSubAction(t *testing.T) {
	nm := &NetworkManager{}
	backend := WindowsNetworkBackend{}

	outputs := [][]byte{
		// show config
		[]byte("    DHCP enabled:                         Yes\n    IP Address:                           192.168.1.100\n"),
		// show dnsservers
		[]byte("    Statically Configured DNS Servers:    None\n"),
		// netsh set static
		[]byte("Ok.\n"),
		// ping
		[]byte("Reply from 1.1.1.1\n"),
	}
	errs := make([]error, len(outputs))

	req := agentmgr.NetworkActionData{
		RequestID:  "req-4",
		Action:     "apply",
		Connection: "Ethernet:static:192.168.1.50:255.255.255.0:192.168.1.1",
	}

	var result agentmgr.NetworkResultData
	calls := stubWindowsRun(outputs, errs, func() {
		result = backend.ApplyAction(nm, req)
	})

	if !result.OK {
		t.Errorf("expected OK=true, got error: %s", result.Error)
	}

	foundStatic := false
	for _, c := range calls {
		if len(c) >= 8 && c[0] == "netsh" && c[6] == "static" {
			foundStatic = true
		}
	}
	if !foundStatic {
		t.Errorf("expected netsh ... static call; calls=%v", calls)
	}
}

func TestWindowsNetworkBackend_RollbackWithSnapshot(t *testing.T) {
	nm := &NetworkManager{
		LastMethod: "netsh",
		LastWindowsSnapshot: &WindowsNetworkSnapshot{
			InterfaceName: "Ethernet",
			WasDHCP:       true,
			DNSServers:    []string{"8.8.8.8"},
		},
	}
	backend := WindowsNetworkBackend{}

	outputs := [][]byte{
		// restore DHCP
		[]byte("Ok.\n"),
		// restore primary DNS
		[]byte("Ok.\n"),
	}
	errs := make([]error, len(outputs))

	req := agentmgr.NetworkActionData{
		RequestID: "req-5",
		Action:    "rollback",
	}

	var result agentmgr.NetworkResultData
	calls := stubWindowsRun(outputs, errs, func() {
		result = backend.RollbackAction(nm, req)
	})

	if !result.OK {
		t.Errorf("expected OK=true, got error: %s", result.Error)
	}
	if !result.RollbackAttempted {
		t.Error("expected RollbackAttempted=true")
	}
	if !result.RollbackSucceeded {
		t.Error("expected RollbackSucceeded=true")
	}
	if result.RollbackReference != "Ethernet" {
		t.Errorf("RollbackReference=%q, want Ethernet", result.RollbackReference)
	}

	// Should have issued netsh set address dhcp and netsh set dnsservers.
	if len(calls) < 2 {
		t.Errorf("expected at least 2 netsh calls; calls=%v", calls)
	}
}

func TestWindowsNetworkBackend_DNSSubAction(t *testing.T) {
	nm := &NetworkManager{}
	backend := WindowsNetworkBackend{}

	outputs := [][]byte{
		// show config
		[]byte("    DHCP enabled:                         Yes\n"),
		// show dnsservers
		[]byte("    Statically Configured DNS Servers:    None\n"),
		// netsh set dnsservers primary
		[]byte("Ok.\n"),
		// ping
		[]byte("Reply from 1.1.1.1\n"),
	}
	errs := make([]error, len(outputs))

	req := agentmgr.NetworkActionData{
		RequestID:  "req-6",
		Action:     "apply",
		Connection: "Ethernet:dns:8.8.8.8:1.1.1.1",
	}

	var result agentmgr.NetworkResultData
	calls := stubWindowsRun(outputs, errs, func() {
		result = backend.ApplyAction(nm, req)
	})

	if !result.OK {
		t.Errorf("expected OK=true, got error: %s", result.Error)
	}

	foundDNS := false
	for _, c := range calls {
		if len(c) >= 5 && c[0] == "netsh" && c[4] == "dnsservers" {
			foundDNS = true
		}
	}
	if !foundDNS {
		t.Errorf("expected netsh dnsservers call; calls=%v", calls)
	}
}
