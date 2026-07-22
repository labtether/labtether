package main

import (
	"net"
	"net/http"
	"net/netip"
	"strings"

	adminpkg "github.com/labtether/labtether/internal/hubapi/admin"
	"github.com/labtether/labtether/internal/hubapi/shared"
)

type hubConnectionCandidate = shared.HubConnectionCandidate

type hubConnectionSelection = shared.HubConnectionSelection

// Package-level vars are preserved for test overrides (withMockHubInterfaces).
// They are forwarded into HubConnectionResolverDeps on each call so that test
// overrides take effect without requiring cached-deps invalidation.
var (
	listHubInterfaces     = net.Interfaces
	listHubInterfaceAddrs = func(iface net.Interface) ([]net.Addr, error) {
		return iface.Addrs()
	}

	tailscaleIPv4Prefix = netip.MustParsePrefix("100.64.0.0/10")
	tailscaleIPv6Prefix = netip.MustParsePrefix("fd7a:115c:a1e0::/48")

	skippedLANInterfacePrefixes = []string{
		"docker",
		"br-",
		"veth",
		"cni",
		"flannel",
		"virbr",
		"vmnet",
	}
)

// resolveHubConnectionSelection delegates to the shared resolver.
// It builds a fresh HubConnectionResolverDeps on each call so that package-level
// test overrides (listHubInterfaces, listHubInterfaceAddrs) take effect.
func (s *apiServer) resolveHubConnectionSelection(r *http.Request) hubConnectionSelection {
	return s.buildHubConnectionResolverDeps().ResolveHubConnectionSelection(r)
}

// buildHubConnectionResolverDeps constructs a HubConnectionResolverDeps from
// the current apiServer and package-level vars. Called on every
// resolveHubConnectionSelection invocation so that test overrides of
// listHubInterfaces and listHubInterfaceAddrs are always honoured.
func (s *apiServer) buildHubConnectionResolverDeps() *shared.HubConnectionResolverDeps {
	return &shared.HubConnectionResolverDeps{
		ExternalURL:                          s.externalURL,
		TLSEnabled:                           s.tlsState.Enabled,
		CACertPEM:                            s.tlsState.CACertPEM,
		DefaultPort:                          "",
		ListInterfaces:                       listHubInterfaces,
		ListInterfaceAddrs:                   listHubInterfaceAddrs,
		ResolveTrustedForwardedRequestOrigin: resolveTrustedForwardedRequestOrigin,

		CurrentTLSCertType: s.currentTLSCertType,
		BuildPinnedBootstrapURL: func(hubURL string, caCertPEM []byte) string {
			return buildPinnedBootstrapURL(hubURL, caCertPEM)
		},
		InspectTailscaleServeStatus: func() shared.HubTailscaleServeStatus {
			status := s.inspectTailscaleServeStatus()
			return tailscaleServeStatusToShared(status)
		},

		TrustModePlainHTTP:   tlsTrustModePlainHTTP,
		TrustModePublicTLS:   tlsTrustModePublicTLS,
		TrustModeLabtetherCA: tlsTrustModeLabtetherCA,
		TrustModeCustomTLS:   tlsTrustModeCustomTLS,

		BootstrapStrategyInstall:  tlsBootstrapStrategyInstall,
		BootstrapStrategyPinnedCA: tlsBootstrapStrategyPinnedCA,
	}
}

// resolveTrustedForwardedRequestOrigin returns the complete public origin
// supplied by a trusted reverse proxy. It does not mix X-Forwarded-* and
// Forwarded values, and a malformed or partial pair fails closed.
func resolveTrustedForwardedRequestOrigin(r *http.Request) (scheme, host string, ok bool) {
	if r == nil || !isTrustedForwardedHostSource(r) {
		return "", "", false
	}

	xForwardedHosts := r.Header.Values("X-Forwarded-Host")
	xForwardedProtos := r.Header.Values("X-Forwarded-Proto")
	if len(xForwardedHosts) > 0 || len(xForwardedProtos) > 0 {
		if len(xForwardedHosts) != 1 || len(xForwardedProtos) != 1 ||
			strings.Contains(xForwardedHosts[0], ",") || strings.Contains(xForwardedProtos[0], ",") {
			return "", "", false
		}
		return sanitizeForwardedRequestOrigin(
			xForwardedProtos[0],
			xForwardedHosts[0],
		)
	}

	forwardedValues := r.Header.Values("Forwarded")
	if len(forwardedValues) != 1 || strings.Contains(forwardedValues[0], ",") {
		return "", "", false
	}
	return strictForwardedRequestOrigin(forwardedValues[0])
}

func sanitizeForwardedRequestOrigin(rawScheme, rawHost string) (scheme, host string, ok bool) {
	scheme = strings.ToLower(strings.TrimSpace(rawScheme))
	if scheme != "http" && scheme != "https" {
		return "", "", false
	}
	host, ok = shared.SanitizeHostPort(rawHost)
	if !ok {
		return "", "", false
	}
	return scheme, host, true
}

// strictForwardedRequestOrigin parses the first RFC 7239 Forwarded element
// without changing the more permissive parser used by the existing CORS path.
// Duplicate, partial, or malformed public-origin fields fail closed.
func strictForwardedRequestOrigin(forwarded string) (scheme, host string, ok bool) {
	var rawScheme, rawHost string
	var sawScheme, sawHost bool
	for _, part := range strings.Split(strings.TrimSpace(forwarded), ";") {
		key, value, found := strings.Cut(part, "=")
		if !found {
			return "", "", false
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "proto":
			if sawScheme {
				return "", "", false
			}
			rawScheme, ok = strictForwardedOriginValue(value)
			if !ok {
				return "", "", false
			}
			sawScheme = true
		case "host":
			if sawHost {
				return "", "", false
			}
			rawHost, ok = strictForwardedOriginValue(value)
			if !ok {
				return "", "", false
			}
			sawHost = true
		}
	}
	if !sawScheme || !sawHost {
		return "", "", false
	}
	return sanitizeForwardedRequestOrigin(rawScheme, rawHost)
}

func strictForwardedOriginValue(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", false
	}
	if strings.HasPrefix(value, "\"") {
		if len(value) < 2 || !strings.HasSuffix(value, "\"") {
			return "", false
		}
		value = value[1 : len(value)-1]
		if value == "" || strings.ContainsAny(value, "\\\"") {
			return "", false
		}
		return value, true
	}
	if strings.Contains(value, "\"") {
		return "", false
	}
	return value, true
}

// tailscaleServeStatusToShared converts the admin package's
// TailscaleServeStatusResponse to the shared HubTailscaleServeStatus without
// creating an import cycle from shared back to admin.
func tailscaleServeStatusToShared(s adminpkg.TailscaleServeStatusResponse) shared.HubTailscaleServeStatus {
	return shared.HubTailscaleServeStatus{
		DesiredMode:     s.DesiredMode,
		ServeConfigured: s.ServeConfigured,
		TSNetURL:        s.TSNetURL,
	}
}
