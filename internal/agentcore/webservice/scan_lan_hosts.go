package webservice

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/labtether/labtether/internal/agentcore/sysconfig"
)

var defaultLANScanCandidates = []int{
	80, 443, 3000, 4000, 5000, 5601, 7443, 8000, 8080, 8081, 8090, 8123, 8443, 9000, 9090, 9443,
}

func scanLANCandidatePorts(custom string) []int {
	custom = strings.TrimSpace(custom)
	ports := make([]int, 0, len(defaultLANScanCandidates))
	if custom == "" {
		ports = append(ports, defaultLANScanCandidates...)
	} else {
		ports = append(ports, parsePortList(custom)...)
	}
	if len(ports) > maxLANScanPorts {
		ports = ports[:maxLANScanPorts]
	}
	return ports
}

func parseCIDRList(raw string) []string {
	normalized, err := sysconfig.NormalizeDiscoveryCIDRListValue(sysconfig.SettingKeyServicesDiscoveryLANScanCIDRs, raw)
	if err != nil || strings.TrimSpace(normalized) == "" {
		return nil
	}
	fields := strings.Split(normalized, ",")
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		value := strings.TrimSpace(field)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func enumerateLANScanHosts(cidrs []string, maxHosts int) []string {
	if maxHosts <= 0 {
		return nil
	}
	if len(cidrs) == 0 {
		return nil
	}

	hosts := make([]string, 0, maxHosts)
	seen := make(map[string]struct{}, maxHosts)
	for _, raw := range cidrs {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(raw))
		if err != nil || !prefix.IsValid() || !prefix.Addr().Is4() {
			continue
		}

		for _, host := range enumerateIPv4PrefixHosts(prefix, maxHosts-len(hosts)) {
			if _, ok := seen[host]; ok {
				continue
			}
			seen[host] = struct{}{}
			hosts = append(hosts, host)
			if len(hosts) >= maxHosts {
				sort.Strings(hosts)
				return hosts
			}
		}
	}
	sort.Strings(hosts)
	return hosts
}

func enumerateIPv4PrefixHosts(prefix netip.Prefix, maxHosts int) []string {
	if maxHosts <= 0 {
		return nil
	}
	addr := prefix.Masked().Addr()
	if !addr.Is4() {
		return nil
	}
	base := addr.As4()
	baseInt := uint32(base[0])<<24 | uint32(base[1])<<16 | uint32(base[2])<<8 | uint32(base[3])
	bits := prefix.Bits()
	if bits < 0 || bits > 32 {
		return nil
	}

	hostBits := 32 - bits
	var size uint64 = 1
	if hostBits > 0 {
		size = uint64(1) << uint(hostBits)
	}
	start := baseInt
	end := baseInt + uint32(size) - 1
	if bits <= 30 && size >= 2 {
		start++
		end--
	}
	if end < start {
		return nil
	}

	hosts := make([]string, 0, maxHosts)
	for ipInt := start; ipInt <= end && len(hosts) < maxHosts; ipInt++ {
		hosts = append(hosts, fmt.Sprintf("%d.%d.%d.%d",
			byte(ipInt>>24),
			byte((ipInt>>16)&0xff),
			byte((ipInt>>8)&0xff),
			byte(ipInt&0xff),
		))
		if ipInt == ^uint32(0) {
			break
		}
	}
	return hosts
}
