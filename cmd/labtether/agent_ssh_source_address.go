package main

import (
	"net"
	"net/http"
	"strings"
)

// agentSSHSourceAddress returns the agent address the Hub may later use for
// its SSH fallback, or "" when the address the Hub saw belongs to something in
// between. Behind a trusted reverse proxy (loopback or
// LABTETHER_TRUST_PROXY_CIDRS/HOSTS) only the address that proxy forwarded is
// used. A gateway of one of the Hub's own networks, such as Docker's bridge
// gateway for proxied connections, is not the agent.
func agentSSHSourceAddress(r *http.Request) string {
	address := requestClientKey(r)
	if isTrustedForwardedHostSource(r) {
		forwarded := trustedRequestClientIP(r)
		if forwarded == address {
			return ""
		}
		address = forwarded
	}
	ip := net.ParseIP(strings.Trim(address, "[]"))
	if ip == nil || isHubNetworkGateway(ip) {
		return ""
	}
	return ip.String()
}

// isHubNetworkGateway reports whether ip is the first host address of a
// network the Hub itself is on, which is where Docker puts the bridge gateway.
func isHubNetworkGateway(ip net.IP) bool {
	interfaces, err := listHubInterfaces()
	if err != nil {
		return false
	}
	for _, iface := range interfaces {
		addrs, err := listHubInterfaceAddrs(iface)
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			network, ok := addr.(*net.IPNet)
			if !ok || network.IP.IsLoopback() || network.IP.Equal(ip) || !network.Contains(ip) {
				continue
			}
			gateway := network.IP.Mask(network.Mask)
			if len(gateway) == 0 {
				continue
			}
			gateway = append(net.IP(nil), gateway...)
			gateway[len(gateway)-1]++
			if gateway.Equal(ip) {
				return true
			}
		}
	}
	return false
}
