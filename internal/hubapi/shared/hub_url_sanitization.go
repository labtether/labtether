package shared

import (
	"net"
	"net/url"
	"strconv"
	"strings"
)

// HTTPURLToWS converts an http:// or https:// base URL into its ws:// / wss://
// WebSocket equivalent.
func HTTPURLToWS(httpURL string) string {
	switch {
	case strings.HasPrefix(httpURL, "https://"):
		return "wss://" + strings.TrimPrefix(httpURL, "https://")
	case strings.HasPrefix(httpURL, "http://"):
		return "ws://" + strings.TrimPrefix(httpURL, "http://")
	default:
		return "ws://" + httpURL
	}
}

func SanitizeExternalBaseURL(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", false
	}
	scheme := strings.ToLower(strings.TrimSpace(u.Scheme))
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	if u.User != nil {
		return "", false
	}
	host, ok := SanitizeHostPort(u.Host)
	if !ok {
		return "", false
	}

	u.Scheme = scheme
	u.Host = host
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/"), true
}

func SanitizeHostPort(raw string) (string, bool) {
	hostPort := strings.TrimSpace(raw)
	if hostPort == "" {
		return "", false
	}
	if strings.ContainsAny(hostPort, " \t\r\n/\\@\"'`") {
		return "", false
	}

	host := hostPort
	port := ""
	if parsedHost, parsedPort, err := net.SplitHostPort(hostPort); err == nil {
		host = parsedHost
		port = parsedPort
	} else if strings.Contains(hostPort, ":") {
		// Reject ambiguous host:port forms (for example unbracketed IPv6).
		return "", false
	}
	if host == "" {
		return "", false
	}
	if net.ParseIP(strings.Trim(host, "[]")) == nil {
		if len(host) > 253 {
			return "", false
		}
		for _, r := range host {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
				continue
			}
			switch r {
			case '.', '-', '_':
				continue
			default:
				return "", false
			}
		}
	}
	if port == "" {
		return host, true
	}
	portNum, err := strconv.Atoi(port)
	if err != nil || portNum < 1 || portNum > 65535 {
		return "", false
	}
	return net.JoinHostPort(host, strconv.Itoa(portNum)), true
}
