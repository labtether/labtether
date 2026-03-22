package webservice

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

func scannedPortMetadata(port int) (name, category, iconKey, serviceKey, healthPath string, known bool) {
	name = fmt.Sprintf("Port %d", port)
	category = CatOther

	knownSvc, ok := LookupUniqueByPort(port)
	if !ok {
		return name, category, "", "", "", false
	}

	return knownSvc.Name, knownSvc.Category, knownSvc.IconKey, knownSvc.Key, knownSvc.HealthPath, true
}

func (wsc *WebServiceCollector) discoverPortScannedServices(ctx context.Context, existing []agentmgr.DiscoveredWebService) []agentmgr.DiscoveredWebService {
	return wsc.discoverPortScannedServicesWithConfig(ctx, existing, normalizeWebServiceDiscoveryConfig(wsc.discoveryCfg))
}

func (wsc *WebServiceCollector) discoverPortScannedServicesWithConfig(ctx context.Context, existing []agentmgr.DiscoveredWebService, cfg WebServiceDiscoveryConfig) []agentmgr.DiscoveredWebService {
	if !cfg.PortScanEnabled {
		return nil
	}

	ports := scanCandidatePortsWithOptions(cfg.PortScanPorts, cfg.PortScanIncludeListening)
	if len(ports) == 0 {
		return nil
	}

	host := strings.TrimSpace(wsc.hostIP)
	if host == "" {
		host = "localhost"
	}

	existingPorts := make(map[int]struct{}, len(existing)*2)
	for _, svc := range existing {
		addKnownServicePorts(existingPorts, svc)
	}

	openCh := make(chan int, len(ports))
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxPortScanConcurrency)

	for _, port := range ports {
		if _, alreadyPresent := existingPorts[port]; alreadyPresent {
			continue
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(p int) {
			defer wg.Done()
			defer func() { <-sem }()
			if isTCPPortReachable(ctx, host, p) {
				openCh <- p
			}
		}(port)
	}

	go func() {
		wg.Wait()
		close(openCh)
	}()

	openPorts := make([]int, 0, len(ports))
	for p := range openCh {
		openPorts = append(openPorts, p)
	}
	if len(openPorts) == 0 {
		return nil
	}
	sort.Slice(openPorts, func(i, j int) bool {
		pi := openPorts[i]
		pj := openPorts[j]

		_, knownI := LookupUniqueByPort(pi)
		_, knownJ := LookupUniqueByPort(pj)
		if knownI != knownJ {
			return knownI
		}

		likelyI := isLikelyWebPort(pi)
		likelyJ := isLikelyWebPort(pj)
		if likelyI != likelyJ {
			return likelyI
		}
		return pi < pj
	})

	services := make([]agentmgr.DiscoveredWebService, 0, len(openPorts))
	unknownAdded := 0

	for _, port := range openPorts {
		name, category, iconKey, serviceKey, healthPath, knownOK := scannedPortMetadata(port)
		if !knownOK && unknownAdded >= maxUnknownScannedServices {
			continue
		}

		if !knownOK {
			unknownAdded++
		}

		svc := agentmgr.DiscoveredWebService{
			ID:          makeServiceID(wsc.assetID, "scan", strconv.Itoa(port)),
			ServiceKey:  serviceKey,
			Name:        name,
			Category:    category,
			URL:         buildServiceURL(host, port),
			Source:      "scan",
			HostAssetID: wsc.assetID,
			IconKey:     iconKey,
			Metadata: map[string]string{
				"public_port": strconv.Itoa(port),
			},
		}
		if healthPath != "" {
			svc.Metadata["health_path"] = healthPath
		}

		services = append(services, svc)
	}

	if len(services) > 0 {
		log.Printf("webservices: port scan discovered %d services on host %s", len(services), host)
	}
	return services
}

func (wsc *WebServiceCollector) discoverLANScannedServices(ctx context.Context, existing []agentmgr.DiscoveredWebService) []agentmgr.DiscoveredWebService {
	return wsc.discoverLANScannedServicesWithConfig(ctx, existing, normalizeWebServiceDiscoveryConfig(wsc.discoveryCfg))
}

func (wsc *WebServiceCollector) discoverLANScannedServicesWithConfig(ctx context.Context, existing []agentmgr.DiscoveredWebService, cfg WebServiceDiscoveryConfig) []agentmgr.DiscoveredWebService {
	if !cfg.LANScanEnabled {
		return nil
	}

	cidrs := parseCIDRList(cfg.LANScanCIDRs)
	if len(cidrs) == 0 {
		log.Printf("webservices: lan scan enabled but no CIDRs configured")
		return nil
	}
	if len(cidrs) > maxLANScanCIDRs {
		cidrs = cidrs[:maxLANScanCIDRs]
	}

	ports := scanLANCandidatePorts(cfg.LANScanPorts)
	if len(ports) == 0 {
		return nil
	}

	maxHosts := cfg.LANScanMaxHosts
	if maxHosts <= 0 {
		maxHosts = 64
	}
	if maxHosts > maxLANScanHosts {
		maxHosts = maxLANScanHosts
	}

	hosts := enumerateLANScanHosts(cidrs, maxHosts)
	if len(hosts) == 0 {
		return nil
	}

	existingEndpoints := knownServiceEndpointMap(existing)

	type endpoint struct {
		host string
		port int
	}

	openCh := make(chan endpoint, len(hosts)*len(ports))
	sem := make(chan struct{}, maxLANScanConcurrency)
	var wg sync.WaitGroup

	for _, host := range hosts {
		for _, port := range ports {
			key := hostPortKey(host, port)
			if _, exists := existingEndpoints[key]; exists {
				continue
			}

			wg.Add(1)
			sem <- struct{}{}
			go func(scanHost string, scanPort int) {
				defer wg.Done()
				defer func() { <-sem }()
				if isTCPPortReachable(ctx, scanHost, scanPort) {
					openCh <- endpoint{host: scanHost, port: scanPort}
				}
			}(host, port)
		}
	}

	go func() {
		wg.Wait()
		close(openCh)
	}()

	openEndpoints := make([]endpoint, 0, len(hosts)*len(ports))
	for item := range openCh {
		openEndpoints = append(openEndpoints, item)
	}
	if len(openEndpoints) == 0 {
		return nil
	}
	sort.Slice(openEndpoints, func(i, j int) bool {
		if openEndpoints[i].host != openEndpoints[j].host {
			return openEndpoints[i].host < openEndpoints[j].host
		}
		return openEndpoints[i].port < openEndpoints[j].port
	})

	services := make([]agentmgr.DiscoveredWebService, 0, len(openEndpoints))
	unknownAdded := 0
	for _, item := range openEndpoints {
		name, category, iconKey, serviceKey, healthPath, knownOK := scannedPortMetadata(item.port)
		if !knownOK {
			if unknownAdded >= maxUnknownLANScanned {
				continue
			}
			unknownAdded++
			name = fmt.Sprintf("Port %d on %s", item.port, item.host)
		}

		svc := agentmgr.DiscoveredWebService{
			ID:          makeServiceID(wsc.assetID, "scan", net.JoinHostPort(item.host, strconv.Itoa(item.port))),
			ServiceKey:  serviceKey,
			Name:        name,
			Category:    category,
			URL:         buildServiceURL(item.host, item.port),
			Source:      "scan",
			HostAssetID: wsc.assetID,
			IconKey:     iconKey,
			Metadata: map[string]string{
				"public_port":      strconv.Itoa(item.port),
				"scan_scope":       "lan",
				"scan_target_host": item.host,
			},
		}
		if healthPath != "" {
			svc.Metadata["health_path"] = healthPath
		}
		services = append(services, svc)
	}

	if len(services) > 0 {
		log.Printf("webservices: lan scan discovered %d services across %d host(s)", len(services), len(hosts))
	}
	return services
}

func scanCandidatePorts() []int {
	custom := strings.TrimSpace(os.Getenv("LABTETHER_WEBSVC_PORTSCAN_PORTS"))
	includeListening := !strings.EqualFold(strings.TrimSpace(os.Getenv("LABTETHER_WEBSVC_PORTSCAN_INCLUDE_LISTENING")), "false")
	return scanCandidatePortsWithOptions(custom, includeListening)
}

func scanCandidatePortsWithOptions(custom string, includeListening bool) []int {
	custom = strings.TrimSpace(custom)
	ports := make([]int, 0, len(defaultPortScanCandidates))
	if custom == "" {
		ports = append(ports, defaultPortScanCandidates...)
	} else {
		parsed := parsePortList(custom)
		if len(parsed) == 0 {
			ports = append(ports, defaultPortScanCandidates...)
		} else {
			ports = append(ports, parsed...)
		}
	}

	if includeListening {
		listening := detectListeningTCPPorts()
		if len(listening) > maxListeningScanPorts {
			listening = listening[:maxListeningScanPorts]
		}
		ports = append(ports, listening...)
	}

	seen := make(map[int]struct{}, len(ports))
	deduped := make([]int, 0, len(ports))
	for _, port := range ports {
		if port <= 0 || port > 65535 {
			continue
		}
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		deduped = append(deduped, port)
	}
	sort.Ints(deduped)
	return deduped
}

func detectListeningTCPPorts() []int {
	ports := make([]int, 0, 64)
	ports = append(ports, parseProcNetListeningPorts(readFileString("/proc/net/tcp"))...)
	ports = append(ports, parseProcNetListeningPorts(readFileString("/proc/net/tcp6"))...)

	seen := make(map[int]struct{}, len(ports))
	deduped := make([]int, 0, len(ports))
	for _, port := range ports {
		if port <= 0 || port > 65535 {
			continue
		}
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		deduped = append(deduped, port)
	}
	sort.Ints(deduped)
	return deduped
}

func readFileString(path string) string {
	payload, err := os.ReadFile(path) // #nosec G304 -- Paths are fixed proc/sysfs discovery targets selected by the scanner.
	if err != nil {
		return ""
	}
	return string(payload)
}

func isTCPPortReachable(ctx context.Context, host string, port int) bool {
	dialCtx, cancel := context.WithTimeout(ctx, portScanDialTimeout)
	defer cancel()

	address := net.JoinHostPort(host, strconv.Itoa(port))
	return probeTCPPortReachable(dialCtx, address, isLikelyHTTPSPort(port))
}

func probeTCPPortReachable(ctx context.Context, address string, preferTLSHello bool) bool {
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return false
	}
	if !preferTLSHello {
		_ = conn.Close()
		return true
	}

	// For likely TLS endpoints, send a TLS ClientHello before close so local TLS
	// servers (including the hub) don't emit handshake EOF noise on scan cycles.
	_ = conn.SetDeadline(time.Now().Add(portScanDialTimeout))
	tlsConn := tls.Client(conn, &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, // #nosec G402 -- discovery probe does not exchange credentials.
	})
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = tlsConn.Close()
		return true
	}
	_ = tlsConn.Close()
	return true
}
