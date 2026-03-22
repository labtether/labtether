package webservice

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"

	dockerpkg "github.com/labtether/labtether/internal/agentcore/docker"
	"github.com/labtether/labtether/internal/agentmgr"
)

// buildServicesFromContainers builds DiscoveredWebService entries from Docker containers.
func (wsc *WebServiceCollector) buildServicesFromContainers(containers []dockerpkg.DockerContainer) []agentmgr.DiscoveredWebService {
	var services []agentmgr.DiscoveredWebService
	for _, c := range containers {
		if c.State != "running" {
			continue
		}
		name := cleanContainerName(dockerpkg.ContainerName(c.Names))
		category := CatOther
		iconKey := ""
		serviceKey := ""
		healthPath := ""
		var known *KnownService
		if knownSvc, ok := LookupByDockerImage(c.Image); ok {
			known = &knownSvc
		} else if knownSvc, ok := LookupByHint(name); ok {
			known = &knownSvc
		} else if knownSvc, ok := LookupByHint(c.Image); ok {
			known = &knownSvc
		}
		if known != nil {
			name = known.Name
			category = known.Category
			iconKey = known.IconKey
			serviceKey = known.Key
			healthPath = known.HealthPath
		}
		// Apply labtether.* Docker label overrides (takes precedence over auto-detection).
		if labelName := strings.TrimSpace(c.Labels["labtether.name"]); labelName != "" {
			name = labelName
		}
		if labelCat := strings.TrimSpace(c.Labels["labtether.category"]); labelCat != "" {
			category = labelCat
		}
		if labelIcon := strings.TrimSpace(c.Labels["labtether.icon"]); labelIcon != "" {
			iconKey = labelIcon
		}
		ports := extractHostPortsForService(c.Ports, known)
		if len(ports) == 0 {
			continue
		}

		for _, port := range ports {
			host := hostForPublicPort(c.Ports, port, wsc.hostIP)
			url := buildServiceURL(host, port)
			// NOTE: Do NOT apply extractTraefikURL here. Traefik labels are handled
			// by the proxy enrichment pipeline (TraefikProvider), which correctly
			// preserves the raw_url for health-check fallback.
			displayName := name
			if len(ports) > 1 {
				displayName = fmt.Sprintf("%s:%d", name, port)
			}

			identifier := fmt.Sprintf("%s:%d", c.ID, port)
			id := makeServiceID(wsc.assetID, "docker", identifier)
			svc := agentmgr.DiscoveredWebService{
				ID: id, ServiceKey: serviceKey, Name: displayName, Category: category,
				URL: url, Source: "docker", ContainerID: c.ID,
				HostAssetID: wsc.assetID, IconKey: iconKey,
			}
			if image := strings.TrimSpace(c.Image); image != "" {
				if svc.Metadata == nil {
					svc.Metadata = make(map[string]string)
				}
				svc.Metadata["image"] = image
			}
			if healthPath != "" {
				if svc.Metadata == nil {
					svc.Metadata = make(map[string]string)
				}
				svc.Metadata["health_path"] = healthPath
			}
			if strings.EqualFold(strings.TrimSpace(c.Labels["labtether.hidden"]), "true") {
				if svc.Metadata == nil {
					svc.Metadata = make(map[string]string)
				}
				svc.Metadata["hidden"] = "true"
			}
			if binding, ok := bindingForPublicPort(c.Ports, port); ok {
				if svc.Metadata == nil {
					svc.Metadata = make(map[string]string)
				}
				svc.Metadata["public_port"] = strconv.Itoa(binding.PublicPort)
				if binding.PrivatePort > 0 {
					svc.Metadata["private_port"] = strconv.Itoa(binding.PrivatePort)
				}
			}
			services = append(services, svc)
		}
	}
	return services
}

// makeServiceID generates a deterministic 16-character hex ID from host, source, and identifier.
func makeServiceID(host, source, identifier string) string {
	h := sha256.New()
	h.Write([]byte(host))
	h.Write([]byte(":"))
	h.Write([]byte(source))
	h.Write([]byte(":"))
	h.Write([]byte(identifier))
	return hex.EncodeToString(h.Sum(nil)[:8])
}

// buildServiceURL constructs an HTTP(S) URL from a host IP and port.
// A best-effort scheme guess is made from common HTTPS ports, then corrected
// during health probing when needed.
func buildServiceURL(hostIP string, port int) string {
	if hostIP == "" {
		hostIP = "localhost"
	}
	hostIP = formatURLHost(hostIP)
	scheme := "http"
	if isLikelyHTTPSPort(port) {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s:%d", scheme, hostIP, port)
}

// extractHostPortsForService picks host-mapped ports with web-first heuristics.
func extractHostPortsForService(ports []dockerpkg.DockerPort, known *KnownService) []int {
	filtered := filterPublicTCPPorts(ports)
	if len(filtered) == 0 {
		return nil
	}

	// Known services should generally produce a single canonical endpoint.
	if known != nil && known.DefaultPort > 0 && isLikelyWebPort(known.DefaultPort) {
		for _, p := range filtered {
			if p.PrivatePort == known.DefaultPort || p.PublicPort == known.DefaultPort {
				return []int{p.PublicPort}
			}
		}
	}

	likely := make([]int, 0, len(filtered))
	seen := make(map[int]struct{})
	for _, p := range filtered {
		if isLikelyWebPort(p.PublicPort) || isLikelyWebPort(p.PrivatePort) {
			if _, ok := seen[p.PublicPort]; ok {
				continue
			}
			seen[p.PublicPort] = struct{}{}
			likely = append(likely, p.PublicPort)
		}
	}

	if known != nil {
		if len(likely) > 0 {
			return []int{likely[0]}
		}
		return []int{filtered[0].PublicPort}
	}

	if len(likely) > maxUnknownServicePorts {
		return likely[:maxUnknownServicePorts]
	}
	if len(likely) > 0 {
		return likely
	}
	return []int{filtered[0].PublicPort}
}

// extractHostPortForService keeps compatibility for tests/callers expecting a single port.
func extractHostPortForService(ports []dockerpkg.DockerPort, known *KnownService) int {
	selected := extractHostPortsForService(ports, known)
	if len(selected) == 0 {
		return 0
	}
	return selected[0]
}

// extractHostPort returns the first public (host-mapped) port from Docker port bindings.
// Returns 0 if no public port is found.
func extractHostPort(ports []dockerpkg.DockerPort) int {
	filtered := filterPublicTCPPorts(ports)
	if len(filtered) > 0 {
		return filtered[0].PublicPort
	}
	return 0
}

func filterPublicTCPPorts(ports []dockerpkg.DockerPort) []dockerpkg.DockerPort {
	filtered := make([]dockerpkg.DockerPort, 0, len(ports))
	for _, p := range ports {
		if p.PublicPort > 0 && strings.EqualFold(p.Type, "tcp") {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

func bindingForPublicPort(ports []dockerpkg.DockerPort, publicPort int) (dockerpkg.DockerPort, bool) {
	for _, p := range ports {
		if p.PublicPort == publicPort && strings.EqualFold(p.Type, "tcp") {
			return p, true
		}
	}
	return dockerpkg.DockerPort{}, false
}

func hostForPublicPort(ports []dockerpkg.DockerPort, publicPort int, fallbackHost string) string {
	if binding, ok := bindingForPublicPort(ports, publicPort); ok {
		if bindingHost := normalizedBindingHost(binding.IP); bindingHost != "" {
			return bindingHost
		}
	}
	if fallbackHost == "" {
		return "localhost"
	}
	return fallbackHost
}

func normalizedBindingHost(raw string) string {
	value := strings.TrimSpace(raw)
	switch value {
	case "", "0.0.0.0", "::", "[::]":
		return ""
	case "127.0.0.1", "::1":
		return "localhost"
	default:
		return value
	}
}

func formatURLHost(host string) string {
	value := strings.TrimSpace(host)
	if value == "" {
		return "localhost"
	}
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		return value
	}
	if ip := net.ParseIP(value); ip != nil && strings.Contains(value, ":") {
		return "[" + value + "]"
	}
	return value
}

func isLikelyHTTPSPort(port int) bool {
	switch port {
	case 443, 7443, 8006, 8007, 8443, 9443, 10443:
		return true
	default:
		return false
	}
}

func isLikelyWebPort(port int) bool {
	switch port {
	case 80, 81, 82, 88, 443, 591, 593, 631, 2375, 2376, 3000, 3001, 3002, 4000, 5000, 5055, 5601, 6001, 7000, 7080, 7081, 7443, 7474, 8000, 8001, 8006, 8007, 8080, 8081, 8082, 8088, 8089, 8090, 8091, 8096, 8123, 8181, 8443, 8888, 8989, 9000, 9001, 9090, 9091, 9443, 9696, 10443, 19999, 32400:
		return true
	default:
		return false
	}
}

// cleanContainerName strips a leading "/" from a Docker container name.
func cleanContainerName(name string) string {
	return strings.TrimPrefix(name, "/")
}
