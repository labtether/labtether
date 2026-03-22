package main

import (
	"net"
	"net/http"
	"net/netip"

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
		ExternalURL:        s.externalURL,
		TLSEnabled:         s.tlsState.Enabled,
		CACertPEM:          s.tlsState.CACertPEM,
		DefaultPort:        "",
		ListInterfaces:     listHubInterfaces,
		ListInterfaceAddrs: listHubInterfaceAddrs,

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
