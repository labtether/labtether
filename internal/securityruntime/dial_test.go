package securityruntime

import (
	"context"
	"net"
	"testing"
)

func TestSameSubnetLocalIPForHostPrefersMatchingLANInterface(t *testing.T) {
	originalLookupIPAddrs := lookupIPAddrs
	originalListNetInterfaces := listNetInterfaces
	originalInterfaceAddrs := interfaceAddrs
	t.Cleanup(func() {
		lookupIPAddrs = originalLookupIPAddrs
		listNetInterfaces = originalListNetInterfaces
		interfaceAddrs = originalInterfaceAddrs
	})

	lookupIPAddrs = func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host != "pve.local" {
			t.Fatalf("unexpected host lookup: %s", host)
		}
		return []net.IPAddr{{IP: net.ParseIP("192.168.0.32")}}, nil
	}
	listNetInterfaces = func() ([]net.Interface, error) {
		return []net.Interface{
			{Index: 1, MTU: 1500, Name: "en0", Flags: net.FlagUp | net.FlagBroadcast | net.FlagMulticast},
			{Index: 2, MTU: 1280, Name: "utun6", Flags: net.FlagUp | net.FlagMulticast | net.FlagPointToPoint},
		}, nil
	}
	interfaceAddrs = func(iface net.Interface) ([]net.Addr, error) {
		switch iface.Name {
		case "en0":
			return []net.Addr{
				&net.IPNet{IP: net.ParseIP("192.168.0.118"), Mask: net.CIDRMask(24, 32)},
			}, nil
		case "utun6":
			return []net.Addr{
				&net.IPNet{IP: net.ParseIP("100.100.216.70"), Mask: net.CIDRMask(32, 32)},
			}, nil
		default:
			return nil, nil
		}
	}

	got := sameSubnetLocalIPForHost(context.Background(), "pve.local")
	if got == nil || got.String() != "192.168.0.118" {
		t.Fatalf("expected en0 address, got %v", got)
	}
}

func TestSameSubnetLocalIPForHostSkipsPublicTargets(t *testing.T) {
	originalListNetInterfaces := listNetInterfaces
	originalInterfaceAddrs := interfaceAddrs
	t.Cleanup(func() {
		listNetInterfaces = originalListNetInterfaces
		interfaceAddrs = originalInterfaceAddrs
	})

	listNetInterfaces = func() ([]net.Interface, error) {
		return []net.Interface{
			{Index: 1, MTU: 1500, Name: "en0", Flags: net.FlagUp | net.FlagBroadcast | net.FlagMulticast},
		}, nil
	}
	interfaceAddrs = func(_ net.Interface) ([]net.Addr, error) {
		return []net.Addr{
			&net.IPNet{IP: net.ParseIP("192.168.0.118"), Mask: net.CIDRMask(24, 32)},
		}, nil
	}

	if got := sameSubnetLocalIPForHost(context.Background(), "8.8.8.8"); got != nil {
		t.Fatalf("expected nil for public target, got %v", got)
	}
}

func TestSameSubnetLocalIPForHostReturnsNilWithoutSameSubnetMatch(t *testing.T) {
	originalListNetInterfaces := listNetInterfaces
	originalInterfaceAddrs := interfaceAddrs
	t.Cleanup(func() {
		listNetInterfaces = originalListNetInterfaces
		interfaceAddrs = originalInterfaceAddrs
	})

	listNetInterfaces = func() ([]net.Interface, error) {
		return []net.Interface{
			{Index: 1, MTU: 1500, Name: "en0", Flags: net.FlagUp | net.FlagBroadcast | net.FlagMulticast},
		}, nil
	}
	interfaceAddrs = func(_ net.Interface) ([]net.Addr, error) {
		return []net.Addr{
			&net.IPNet{IP: net.ParseIP("10.0.0.5"), Mask: net.CIDRMask(24, 32)},
		}, nil
	}

	if got := sameSubnetLocalIPForHost(context.Background(), "192.168.0.32"); got != nil {
		t.Fatalf("expected nil when no same-subnet interface exists, got %v", got)
	}
}
