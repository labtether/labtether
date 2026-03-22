package sysconfig

import (
	"reflect"
	"testing"
)

func TestResolveDarwinNetworkMethodWith(t *testing.T) {
	commandExists := func(name string) bool { return name == "networksetup" }

	method, err := ResolveDarwinNetworkMethodWith("auto", commandExists)
	if err != nil {
		t.Fatalf("ResolveDarwinNetworkMethodWith returned error: %v", err)
	}
	if method != "networksetup" {
		t.Fatalf("method=%q, want networksetup", method)
	}

	if _, err := ResolveDarwinNetworkMethodWith("nmcli", commandExists); err == nil {
		t.Fatal("expected invalid-method error")
	}
}

func TestParseDarwinNetworkServicesOutput(t *testing.T) {
	raw := `An asterisk (*) denotes that a network service is disabled.
Wi-Fi
*USB 10/100/1000 LAN
Thunderbolt Bridge`

	services := ParseDarwinNetworkServicesOutput(raw)
	if len(services) != 3 {
		t.Fatalf("len(services)=%d, want 3", len(services))
	}
	if services[0].Name != "Wi-Fi" || services[0].Disabled {
		t.Fatalf("unexpected first service: %+v", services[0])
	}
	if services[1].Name != "USB 10/100/1000 LAN" || !services[1].Disabled {
		t.Fatalf("unexpected second service: %+v", services[1])
	}
}

func TestParseDarwinDNSServersOutput(t *testing.T) {
	servers, hasDNS := ParseDarwinDNSServersOutput("8.8.8.8\n1.1.1.1\n")
	if !hasDNS {
		t.Fatal("expected hasDNS=true")
	}
	wantServers := []string{"8.8.8.8", "1.1.1.1"}
	if !reflect.DeepEqual(servers, wantServers) {
		t.Fatalf("servers=%v, want %v", servers, wantServers)
	}

	none, hasDNS := ParseDarwinDNSServersOutput("There aren't any DNS Servers set on Wi-Fi.\n")
	if hasDNS {
		t.Fatal("expected hasDNS=false when DNS is unset")
	}
	if len(none) != 0 {
		t.Fatalf("expected no DNS servers, got %v", none)
	}
}

func TestParseDarwinNetworkServiceEnabledOutput(t *testing.T) {
	enabled, ok := ParseDarwinNetworkServiceEnabledOutput("Enabled")
	if !ok || !enabled {
		t.Fatalf("expected enabled=true, ok=true; got enabled=%v ok=%v", enabled, ok)
	}

	enabled, ok = ParseDarwinNetworkServiceEnabledOutput("Disabled")
	if !ok || enabled {
		t.Fatalf("expected enabled=false, ok=true; got enabled=%v ok=%v", enabled, ok)
	}
}
