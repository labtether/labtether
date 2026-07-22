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

	// ResolveTrustedForwardedRequestOrigin returns a public scheme/host pair
	// only when the request came from a trusted reverse proxy. The returned pair
	// is validated again before use.
	ResolveTrustedForwardedRequestOrigin func(r *http.Request) (scheme, host string, ok bool)
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

// requestOrigin returns the exact origin used for the current request. The
// direct host always uses the hub's transport scheme (or the request's TLS
// state when present). A trusted proxy may supply a different public scheme,
// but only as a complete, valid host/proto pair.
func (d *HubConnectionResolverDeps) requestOrigin(r *http.Request) (httpScheme, wsScheme, host string, forwarded bool) {
	httpScheme, wsScheme = d.hubSchemes()
	if r == nil {
		return httpScheme, wsScheme, "", false
	}
	if r.TLS != nil {
		httpScheme, wsScheme = "https", "wss"
	}

	host, _ = SanitizeHostPort(r.Host)
	if d.ResolveTrustedForwardedRequestOrigin == nil {
		return httpScheme, wsScheme, host, false
	}
	forwardedScheme, forwardedHost, ok := d.ResolveTrustedForwardedRequestOrigin(r)
	forwardedScheme, forwardedHost, ok = sanitizeForwardedOriginPair(forwardedScheme, forwardedHost)
	if !ok {
		return httpScheme, wsScheme, host, false
	}
	if forwardedScheme == "https" {
		return "https", "wss", forwardedHost, true
	}
	return "http", "ws", forwardedHost, true
}

func sanitizeForwardedOriginPair(rawScheme, rawHost string) (scheme, host string, ok bool) {
	scheme = strings.ToLower(strings.TrimSpace(rawScheme))
	if scheme != "http" && scheme != "https" {
		return "", "", false
	}
	host, ok = SanitizeHostPort(rawHost)
	if !ok {
		return "", "", false
	}
	return scheme, host, true
}

// normalizedHTTPOriginKey returns a canonical origin identity using scheme,
// hostname, and effective port. Paths do not participate in HTTP origin
// equality, while an omitted default port equals its explicit form.
func normalizedHTTPOriginKey(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.User != nil {
		return "", false
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	hostname := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(parsed.Hostname()), "."))
	if hostname == "" {
		return "", false
	}
	if ip := net.ParseIP(hostname); ip != nil {
		hostname = ip.String()
	}
	port := parsed.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return scheme + "://" + net.JoinHostPort(hostname, port), true
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

	directHost := ""
	if r != nil {
		directHost, _ = SanitizeHostPort(r.Host)
	}
	if directHost != "" {
		if port := HostPortPort(directHost); port != "" {
			defaultPort = port
		}
	}
	requestHTTPScheme, requestWSScheme, requestHost, requestOriginForwarded := d.requestOrigin(r)

	candidates := make([]HubConnectionCandidate, 0, 8)
	seen := map[string]struct{}{}
	addCandidate := func(kind, label, host, hubURL, wsURL string) {
		host = strings.TrimSpace(host)
		if host == "" {
			return
		}
		candidateKey := "host:" + strings.ToLower(host)
		if originKey, ok := normalizedHTTPOriginKey(hubURL); ok {
			candidateKey = "origin:" + originKey
		}
		if _, exists := seen[candidateKey]; exists {
			return
		}
		seen[candidateKey] = struct{}{}
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
	addRequestCandidate := func() {
		sanitized, ok := SanitizeHostPort(requestHost)
		if !ok {
			return
		}
		label := "Request host"
		if IsLoopbackHost(sanitized) {
			label = "Localhost"
		}
		requestHubURL := fmt.Sprintf("%s://%s", requestHTTPScheme, sanitized)
		requestWSURL := fmt.Sprintf("%s://%s/ws/agent", requestWSScheme, sanitized)
		if requestOriginForwarded {
			requestOriginKey, requestOriginOK := normalizedHTTPOriginKey(requestHubURL)
			if requestOriginOK {
				for i := range candidates {
					candidateOriginKey, candidateOriginOK := normalizedHTTPOriginKey(candidates[i].HubURL)
					if !candidateOriginOK || candidateOriginKey != requestOriginKey {
						continue
					}
					if candidates[i].Kind != "tailscale_https" {
						d.describeForwardedRequestCandidate(&candidates[i])
					}
					return
				}
			}
		}
		candidateCount := len(candidates)
		addCandidate(
			"request",
			label,
			sanitized,
			requestHubURL,
			requestWSURL,
		)
		if requestOriginForwarded && len(candidates) > candidateCount {
			d.describeForwardedRequestCandidate(&candidates[len(candidates)-1])
		}
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

	addRequestCandidate()

	tailscaleHosts, lanHosts := d.discoverInterfaceHosts(defaultPort)
	for _, host := range tailscaleHosts {
		addHostCandidate("tailscale", "Tailscale", host)
	}
	for _, host := range lanHosts {
		addHostCandidate("lan", "LAN", host)
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

// describeForwardedRequestCandidate prevents the hub's direct TLS certificate
// metadata from being applied to an HTTPS origin terminated by a trusted
// reverse proxy. The proxy certificate is operator-managed and cannot safely
// use a bootstrap URL pinned to the hub's internal CA.
func (d *HubConnectionResolverDeps) describeForwardedRequestCandidate(candidate *HubConnectionCandidate) {
	if d == nil || candidate == nil || !strings.HasPrefix(candidate.HubURL, "https://") {
		return
	}
	candidate.TrustMode = d.TrustModeCustomTLS
	candidate.BootstrapStrategy = d.BootstrapStrategyInstall
	candidate.BootstrapURL = ""
	candidate.PreferredReason = "This HTTPS request origin is terminated by a trusted reverse proxy. Agents rely on the proxy certificate chaining to their normal OS trust store."
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
