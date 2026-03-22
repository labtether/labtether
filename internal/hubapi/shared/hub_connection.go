package shared

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// HubTailscaleServeStatus carries the subset of Tailscale serve status data
// that the hub connection resolver needs to identify a healthy HTTPS candidate.
// Callers populate this from adminpkg.TailscaleServeStatusResponse without
// creating an import cycle back into internal/hubapi/admin.
type HubTailscaleServeStatus struct {
	DesiredMode     string
	ServeConfigured bool
	TSNetURL        string
}

// HubConnectionResolverDeps holds the external dependencies for resolving the
// best hub connection URL to advertise to agents and the browser console.
type HubConnectionResolverDeps struct {
	// ExternalURL is the operator-configured LABTETHER_EXTERNAL_URL value,
	// already sanitized and stripped of a trailing slash. Empty when not set.
	ExternalURL string

	// TLSEnabled reports whether the hub is serving TLS.
	TLSEnabled bool

	// CurrentTLSCertType returns the active TLS certificate source type
	// (e.g. "self-signed", "operator-supplied"). Called per-request.
	CurrentTLSCertType func() string

	// CACertPEM is the DER-encoded PEM of the hub's built-in CA certificate,
	// populated only when TLS source is the built-in CA. Nil otherwise.
	CACertPEM []byte

	// BuildPinnedBootstrapURL constructs the installer URL that embeds the
	// CA fingerprint so agents can verify the hub's self-signed cert.
	BuildPinnedBootstrapURL func(hubURL string, caCertPEM []byte) string

	// InspectTailscaleServeStatus returns the current Tailscale serve status.
	// May return an empty struct when Tailscale is not installed or configured.
	InspectTailscaleServeStatus func() HubTailscaleServeStatus

	// ListInterfaces returns the host's network interfaces. Defaults to
	// net.Interfaces when nil; override in tests.
	ListInterfaces func() ([]net.Interface, error)

	// ListInterfaceAddrs returns the addresses for a given interface. Defaults
	// to iface.Addrs when nil; override in tests.
	ListInterfaceAddrs func(iface net.Interface) ([]net.Addr, error)

	// TrustModePlainHTTP is the trust-mode string for plain HTTP targets.
	TrustModePlainHTTP string
	// TrustModePublicTLS is the trust-mode string for public TLS targets.
	TrustModePublicTLS string
	// TrustModeLabtetherCA is the trust-mode string for built-in CA targets.
	TrustModeLabtetherCA string
	// TrustModeCustomTLS is the trust-mode string for operator-managed TLS.
	TrustModeCustomTLS string

	// BootstrapStrategyInstall is the bootstrap-strategy string for standard
	// script-download install.
	BootstrapStrategyInstall string
	// BootstrapStrategyPinnedCA is the bootstrap-strategy string for the
	// pinned-CA installer.
	BootstrapStrategyPinnedCA string

	// DefaultPort is the hub's listen port as a string (e.g. "8080").
	DefaultPort string
}

var (
	defaultTailscaleIPv4Prefix = netip.MustParsePrefix("100.64.0.0/10")
	defaultTailscaleIPv6Prefix = netip.MustParsePrefix("fd7a:115c:a1e0::/48")

	defaultSkippedLANInterfacePrefixes = []string{
		"docker",
		"br-",
		"veth",
		"cni",
		"flannel",
		"virbr",
		"vmnet",
	}
)

// listInterfaces returns the effective network interface lister.
func (d *HubConnectionResolverDeps) listInterfaces() ([]net.Interface, error) {
	if d.ListInterfaces != nil {
		return d.ListInterfaces()
	}
	return net.Interfaces()
}

// listInterfaceAddrs returns the effective interface address lister.
func (d *HubConnectionResolverDeps) listInterfaceAddrs(iface net.Interface) ([]net.Addr, error) {
	if d.ListInterfaceAddrs != nil {
		return d.ListInterfaceAddrs(iface)
	}
	return iface.Addrs()
}

// hubSchemes returns the HTTP and WebSocket scheme pair based on TLS state.
func (d *HubConnectionResolverDeps) hubSchemes() (string, string) {
	if d.TLSEnabled {
		return "https", "wss"
	}
	return "http", "ws"
}

// sanitizedExternalHubURL validates and returns the external base URL.
// In TLS mode only https:// external URLs are accepted.
func (d *HubConnectionResolverDeps) sanitizedExternalHubURL() (string, bool) {
	if d == nil || strings.TrimSpace(d.ExternalURL) == "" {
		return "", false
	}
	sanitized, ok := SanitizeExternalBaseURL(strings.TrimSpace(d.ExternalURL))
	if !ok {
		return "", false
	}
	if !d.TLSEnabled {
		return sanitized, true
	}
	parsed, err := url.Parse(sanitized)
	if err != nil {
		return "", false
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return "", false
	}
	return sanitized, true
}

// ResolveHubConnectionSelection returns the full set of candidate hub
// connection endpoints for the given HTTP request, ordered by priority.
func (d *HubConnectionResolverDeps) ResolveHubConnectionSelection(r *http.Request) HubConnectionSelection {
	httpScheme, wsScheme := d.hubSchemes()
	defaultPort := d.DefaultPort
	if defaultPort == "" {
		defaultPort = strconv.Itoa(EnvOrDefaultInt("API_PORT", 8080))
	}

	requestHost := ""
	requestHostIsLoopback := false
	if sanitized, ok := SanitizeHostPort(r.Host); ok {
		requestHost = sanitized
		requestHostIsLoopback = IsLoopbackHost(sanitized)
		if port := HostPortPort(sanitized); port != "" {
			defaultPort = port
		}
	}

	candidates := make([]HubConnectionCandidate, 0, 8)
	seen := map[string]struct{}{}
	addCandidate := func(kind, label, host, hubURL, wsURL string) {
		host = strings.TrimSpace(host)
		if host == "" {
			return
		}
		hostKey := strings.ToLower(host)
		if _, exists := seen[hostKey]; exists {
			return
		}
		seen[hostKey] = struct{}{}
		candidate := HubConnectionCandidate{
			Kind:   kind,
			Label:  label,
			Host:   host,
			HubURL: hubURL,
			WSURL:  wsURL,
		}
		d.describeCandidate(&candidate)
		candidates = append(candidates, candidate)
	}
	addHostCandidate := func(kind, label, host string) {
		sanitized, ok := SanitizeHostPort(host)
		if !ok {
			return
		}
		addCandidate(
			kind,
			label,
			sanitized,
			fmt.Sprintf("%s://%s", httpScheme, sanitized),
			fmt.Sprintf("%s://%s/ws/agent", wsScheme, sanitized),
		)
	}

	if candidate, ok := d.resolveHealthyTailscaleHTTPSCandidate(); ok {
		addCandidate(candidate.Kind, candidate.Label, candidate.Host, candidate.HubURL, candidate.WSURL)
	}

	if d.ExternalURL != "" {
		if sanitized, ok := d.sanitizedExternalHubURL(); ok {
			hubURL := strings.TrimRight(sanitized, "/")
			wsURL := strings.TrimRight(HTTPURLToWS(hubURL), "/") + "/ws/agent"
			host := hubURL
			if parsed, err := url.Parse(hubURL); err == nil && parsed.Host != "" {
				host = parsed.Host
			}
			addCandidate("external", "Configured external URL", host, hubURL, wsURL)
		}
	}

	if requestHost != "" && !requestHostIsLoopback {
		addHostCandidate("request", "Request host", requestHost)
	}

	tailscaleHosts, lanHosts := d.discoverInterfaceHosts(defaultPort)
	for _, host := range tailscaleHosts {
		addHostCandidate("tailscale", "Tailscale", host)
	}
	for _, host := range lanHosts {
		addHostCandidate("lan", "LAN", host)
	}

	if requestHost != "" && requestHostIsLoopback {
		addHostCandidate("request", "Localhost", requestHost)
	}

	if len(candidates) == 0 {
		addHostCandidate("fallback", "Local fallback", net.JoinHostPort("localhost", defaultPort))
	}

	selection := HubConnectionSelection{Candidates: candidates}
	selection.HubURL = candidates[0].HubURL
	selection.WSURL = candidates[0].WSURL
	return selection
}

// describeCandidate fills TrustMode, BootstrapStrategy, BootstrapURL, and
// PreferredReason on a candidate based on the active TLS configuration.
func (d *HubConnectionResolverDeps) describeCandidate(candidate *HubConnectionCandidate) {
	if d == nil || candidate == nil {
		return
	}
	switch {
	case strings.HasPrefix(candidate.HubURL, "http://"):
		candidate.TrustMode = d.TrustModePlainHTTP
		candidate.BootstrapStrategy = d.BootstrapStrategyInstall
		candidate.PreferredReason = "Uses plain HTTP. Prefer this only when your transport is already protected, such as an internal Tailscale-only path."
	case candidate.Kind == "tailscale_https":
		candidate.TrustMode = d.TrustModePublicTLS
		candidate.BootstrapStrategy = d.BootstrapStrategyInstall
		candidate.PreferredReason = "Healthy Tailscale HTTPS is available, so agents can use a trusted hostname without installing the LabTether CA."
	case d.isBuiltInCA():
		candidate.TrustMode = d.TrustModeLabtetherCA
		candidate.BootstrapStrategy = d.BootstrapStrategyPinnedCA
		if d.BuildPinnedBootstrapURL != nil {
			candidate.BootstrapURL = d.BuildPinnedBootstrapURL(candidate.HubURL, d.CACertPEM)
		}
		candidate.PreferredReason = "Direct hub TLS uses the LabTether built-in CA. Use the pinned bootstrap installer so the agent can trust that CA automatically."
	default:
		candidate.TrustMode = d.TrustModeCustomTLS
		candidate.BootstrapStrategy = d.BootstrapStrategyInstall
		candidate.PreferredReason = "This target uses operator-managed TLS. Agents rely on the endpoint certificate chaining to their normal OS trust store."
	}
}

// isBuiltInCA reports whether the hub is using its own self-signed CA cert.
func (d *HubConnectionResolverDeps) isBuiltInCA() bool {
	if d.CurrentTLSCertType == nil {
		return false
	}
	return d.CurrentTLSCertType() == "self-signed" && len(d.CACertPEM) > 0
}

// resolveHealthyTailscaleHTTPSCandidate returns a candidate using a healthy
// Tailscale HTTPS endpoint, if one is available.
func (d *HubConnectionResolverDeps) resolveHealthyTailscaleHTTPSCandidate() (HubConnectionCandidate, bool) {
	if d == nil || d.InspectTailscaleServeStatus == nil {
		return HubConnectionCandidate{}, false
	}
	status := d.InspectTailscaleServeStatus()
	if status.DesiredMode != "serve" || !status.ServeConfigured || strings.TrimSpace(status.TSNetURL) == "" {
		return HubConnectionCandidate{}, false
	}
	hubURL := strings.TrimRight(strings.TrimSpace(status.TSNetURL), "/")
	parsed, err := url.Parse(hubURL)
	if err != nil || strings.TrimSpace(parsed.Host) == "" {
		return HubConnectionCandidate{}, false
	}
	return HubConnectionCandidate{
		Kind:   "tailscale_https",
		Label:  "Tailscale HTTPS",
		Host:   strings.TrimSpace(parsed.Host),
		HubURL: hubURL,
		WSURL:  strings.TrimRight(HTTPURLToWS(hubURL), "/") + "/ws/agent",
	}, true
}

// discoverInterfaceHosts enumerates network interfaces and returns Tailscale
// and LAN host:port strings.
func (d *HubConnectionResolverDeps) discoverInterfaceHosts(port string) (tailscaleHosts []string, lanHosts []string) {
	ifaces, err := d.listInterfaces()
	if err != nil {
		return nil, nil
	}

	tailscaleSet := map[string]struct{}{}
	lanSet := map[string]struct{}{}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addresses, err := d.listInterfaceAddrs(iface)
		if err != nil {
			continue
		}

		ifaceName := strings.ToLower(strings.TrimSpace(iface.Name))
		skipLAN := ShouldSkipLANInterface(ifaceName)
		for _, address := range addresses {
			ip := ExtractIPFromNetAddr(address)
			if ip == nil {
				continue
			}
			if ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
				continue
			}

			host := net.JoinHostPort(ip.String(), port)
			if IsTailscaleInterface(ifaceName) || IsTailscaleIP(ip) {
				tailscaleSet[host] = struct{}{}
				continue
			}
			if skipLAN {
				continue
			}
			if ip.To4() == nil {
				continue
			}
			lanSet[host] = struct{}{}
		}
	}

	tailscaleHosts = sortedStringSet(tailscaleSet)
	lanHosts = sortedStringSet(lanSet)
	return tailscaleHosts, lanHosts
}

// sortedStringSet returns the keys of a string set sorted ascending.
func sortedStringSet(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	hosts := make([]string, 0, len(set))
	for host := range set {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	return hosts
}

// ExtractIPFromNetAddr extracts the IP address from a net.Addr value.
func ExtractIPFromNetAddr(address net.Addr) net.IP {
	switch typed := address.(type) {
	case *net.IPNet:
		if typed == nil {
			return nil
		}
		return typed.IP
	case *net.IPAddr:
		if typed == nil {
			return nil
		}
		return typed.IP
	default:
		return nil
	}
}

// ShouldSkipLANInterface reports whether a network interface with the given
// name should be excluded from the LAN candidate set (e.g. Docker bridges).
func ShouldSkipLANInterface(name string) bool {
	for _, prefix := range defaultSkippedLANInterfacePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// IsTailscaleInterface reports whether the interface name indicates a
// Tailscale virtual NIC.
func IsTailscaleInterface(name string) bool {
	return strings.Contains(name, "tailscale")
}

// IsTailscaleIP reports whether ip falls within the Tailscale CGNAT ranges.
func IsTailscaleIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	if addr.Is4() {
		return defaultTailscaleIPv4Prefix.Contains(addr)
	}
	return defaultTailscaleIPv6Prefix.Contains(addr)
}

// IsLoopbackHost reports whether the given host or host:port string refers
// to a loopback address (localhost, 127.x.x.x, ::1, etc.).
func IsLoopbackHost(hostPort string) bool {
	host := strings.TrimSpace(hostPort)
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// HostPortPort returns the port portion of a host:port string, or empty
// string if the input cannot be parsed.
func HostPortPort(hostPort string) string {
	_, port, err := net.SplitHostPort(strings.TrimSpace(hostPort))
	if err != nil {
		return ""
	}
	return port
}
