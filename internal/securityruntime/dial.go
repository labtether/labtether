package securityruntime

import (
	"context"
	"net"
	"strings"
	"time"
)

var listNetInterfaces = net.Interfaces
var interfaceAddrs = func(iface net.Interface) ([]net.Addr, error) {
	return iface.Addrs()
}

// PreferSameSubnetPrivateDialContext returns a DialContext wrapper that binds
// private-lan TCP dials to a same-subnet local address when one is available.
//
// This avoids ambiguous route selection on hosts that have overlapping VPN
// routes (for example utun/Tailscale plus a direct Wi-Fi LAN route).
func PreferSameSubnetPrivateDialContext(base *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
	if base == nil {
		base = &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	}

	return func(ctx context.Context, network, address string) (net.Conn, error) {
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(network)), "tcp") {
			return base.DialContext(ctx, network, address)
		}

		host, _, err := net.SplitHostPort(strings.TrimSpace(address))
		if err != nil {
			return base.DialContext(ctx, network, address)
		}

		if binding := sameSubnetBindingForHost(ctx, host); binding != nil && binding.localIP != nil {
			dialer := *base
			dialer.LocalAddr = &net.TCPAddr{IP: append(net.IP(nil), binding.localIP...)}
			dialer.ControlContext = wrapDialerControl(base.ControlContext, binding)
			conn, dialErr := dialer.DialContext(ctx, network, address)
			if dialErr == nil {
				return conn, nil
			}
		}

		return base.DialContext(ctx, network, address)
	}
}

func sameSubnetLocalIPForHost(ctx context.Context, host string) net.IP {
	if binding := sameSubnetBindingForHost(ctx, host); binding != nil {
		return append(net.IP(nil), binding.localIP...)
	}
	return nil
}

type sameSubnetBinding struct {
	localIP    net.IP
	ifaceIndex int
}

func sameSubnetBindingForHost(ctx context.Context, host string) *sameSubnetBinding {
	targets := resolveDialTargetIPs(ctx, host)
	if len(targets) == 0 {
		return nil
	}

	interfaces, err := listNetInterfaces()
	if err != nil {
		return nil
	}

	for _, target := range targets {
		if target == nil || !isPrivateIPAddress(target) || target.IsLoopback() {
			continue
		}

		for _, iface := range interfaces {
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagPointToPoint != 0 {
				continue
			}

			addrs, err := interfaceAddrs(iface)
			if err != nil {
				continue
			}

			for _, addr := range addrs {
				ip, network, ok := ipNetFromAddr(addr)
				if !ok || network == nil {
					continue
				}
				if !sameIPFamily(ip, target) {
					continue
				}
				if !network.Contains(target) {
					continue
				}
				return &sameSubnetBinding{
					localIP:    append(net.IP(nil), ip...),
					ifaceIndex: iface.Index,
				}
			}
		}
	}

	return nil
}

func resolveDialTargetIPs(ctx context.Context, host string) []net.IP {
	if ip := net.ParseIP(strings.TrimSpace(host)); ip != nil {
		return []net.IP{append(net.IP(nil), ip...)}
	}

	addrs, err := lookupIPAddrs(ctx, host)
	if err != nil {
		return nil
	}

	out := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		if addr.IP == nil {
			continue
		}
		out = append(out, append(net.IP(nil), addr.IP...))
	}
	return out
}

func ipNetFromAddr(addr net.Addr) (net.IP, *net.IPNet, bool) {
	switch value := addr.(type) {
	case *net.IPNet:
		if value == nil || value.IP == nil {
			return nil, nil, false
		}
		return value.IP, value, true
	case *net.IPAddr:
		if value == nil || value.IP == nil {
			return nil, nil, false
		}
		maskBits := 128
		if v4 := value.IP.To4(); v4 != nil {
			maskBits = 32
			return v4, &net.IPNet{IP: v4, Mask: net.CIDRMask(maskBits, maskBits)}, true
		}
		return value.IP, &net.IPNet{IP: value.IP, Mask: net.CIDRMask(maskBits, maskBits)}, true
	default:
		return nil, nil, false
	}
}

func sameIPFamily(left, right net.IP) bool {
	if left == nil || right == nil {
		return false
	}
	return (left.To4() != nil) == (right.To4() != nil)
}
