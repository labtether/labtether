package webservice

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/labtether/labtether/internal/agentmgr"
)

func knownServiceEndpointMap(services []agentmgr.DiscoveredWebService) map[string]struct{} {
	known := make(map[string]struct{}, len(services)*2)
	for _, svc := range services {
		addKnownServiceEndpointFromURL(known, svc.URL)
		if svc.Metadata == nil {
			continue
		}
		addKnownServiceEndpointFromURL(known, svc.Metadata["raw_url"])
		addKnownServiceEndpointFromURL(known, svc.Metadata["backend_url"])
		if rawAlt := strings.TrimSpace(svc.Metadata["alt_urls"]); rawAlt != "" {
			for _, candidate := range strings.Split(rawAlt, ",") {
				addKnownServiceEndpointFromURL(known, candidate)
			}
		}
	}
	return known
}

func addKnownServiceEndpointFromURL(known map[string]struct{}, rawURL string) {
	host, port, ok := hostPortFromURL(rawURL)
	if !ok {
		return
	}
	known[hostPortKey(host, port)] = struct{}{}
}

func hostPortFromURL(rawURL string) (string, int, bool) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", 0, false
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", 0, false
	}
	host := strings.TrimSpace(strings.ToLower(parsed.Hostname()))
	if host == "" {
		return "", 0, false
	}
	port := parsePortValue(parsed.Port())
	if port == 0 {
		switch strings.ToLower(parsed.Scheme) {
		case "http":
			port = 80
		case "https":
			port = 443
		default:
			return "", 0, false
		}
	}
	return host, port, true
}

func hostPortKey(host string, port int) string {
	return strings.ToLower(strings.TrimSpace(host)) + ":" + strconv.Itoa(port)
}

func addKnownServicePorts(known map[int]struct{}, svc agentmgr.DiscoveredWebService) {
	addKnownPort(known, portFromURL(svc.URL))
	if svc.Metadata == nil {
		return
	}

	addKnownPort(known, portFromURL(svc.Metadata["raw_url"]))
	addKnownPort(known, portFromURL(svc.Metadata["backend_url"]))
	addKnownPort(known, parsePortValue(svc.Metadata["public_port"]))
	addKnownPort(known, parsePortValue(svc.Metadata["private_port"]))

	if rawAlt := strings.TrimSpace(svc.Metadata["alt_urls"]); rawAlt != "" {
		for _, candidate := range strings.Split(rawAlt, ",") {
			addKnownPort(known, portFromURL(strings.TrimSpace(candidate)))
		}
	}
}

func addKnownPort(known map[int]struct{}, port int) {
	if port <= 0 || port > 65535 {
		return
	}
	known[port] = struct{}{}
}
