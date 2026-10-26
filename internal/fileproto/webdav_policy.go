package fileproto

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/labtether/labtether/internal/securityruntime"
)

// webDAVEndpoint requires both a connection-level acknowledgement and the
// process-wide escape hatch for plaintext. Port numbers never disable TLS.
func webDAVEndpoint(cfg ConnectionConfig) (string, bool, error) {
	useTLS, err := webDAVBoolOption(cfg.ExtraConfig, "webdav_tls", true)
	if err != nil {
		return "", false, err
	}
	allowCleartext, err := webDAVBoolOption(cfg.ExtraConfig, "webdav_allow_cleartext", false)
	if err != nil {
		return "", false, err
	}
	if useTLS && allowCleartext {
		return "", false, fmt.Errorf("webdav_allow_cleartext cannot be enabled when webdav_tls is true")
	}
	if !useTLS && (!allowCleartext || !securityruntime.InsecureTransportAllowed()) {
		return "", false, fmt.Errorf("plain WebDAV requires webdav_allow_cleartext=true and LABTETHER_ALLOW_INSECURE_TRANSPORT=true")
	}
	skipVerify, err := webDAVBoolOption(cfg.ExtraConfig, "webdav_tls_skip_verify", false)
	if err != nil {
		return "", false, err
	}
	if skipVerify && !securityruntime.InsecureTransportAllowed() {
		return "", false, fmt.Errorf("webdav_tls_skip_verify requires LABTETHER_ALLOW_INSECURE_TRANSPORT=true")
	}
	port := cfg.Port
	if port == 0 {
		port = DefaultPort("webdav")
	}
	if port < 1 || port > 65535 {
		return "", false, fmt.Errorf("invalid WebDAV port")
	}
	host := strings.TrimSpace(cfg.Host)
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	}
	if host == "" || strings.ContainsAny(host, "/\\?#@") || strings.IndexFunc(host, unicode.IsSpace) >= 0 {
		return "", false, fmt.Errorf("invalid WebDAV host")
	}
	scheme := "https"
	if !useTLS {
		scheme = "http"
	}
	endpoint := (&url.URL{Scheme: scheme, Host: net.JoinHostPort(host, strconv.Itoa(port))}).String()
	if raw, ok := cfg.ExtraConfig["webdav_base_path"]; ok {
		basePath, valid := raw.(string)
		if !valid || strings.ContainsAny(basePath, "?#") || strings.IndexFunc(basePath, unicode.IsControl) >= 0 {
			return "", false, fmt.Errorf("webdav_base_path must be a URL path")
		}
		basePath = strings.Trim(strings.TrimSpace(basePath), "/")
		if basePath != "" {
			// gowebdav escapes path segments itself; keep this path unescaped.
			endpoint += "/" + basePath
		}
	}
	return endpoint, skipVerify, nil
}

func webDAVBoolOption(config map[string]any, key string, fallback bool) (bool, error) {
	raw, present := config[key]
	if !present {
		return fallback, nil
	}
	value, valid := raw.(bool)
	if !valid {
		return false, fmt.Errorf("%s must be a boolean", key)
	}
	return value, nil
}

// webDAVPolicyTransport guards all requests, including redirects and requests
// made by gowebdav's authentication negotiator, before any bytes reach the wire.
type webDAVPolicyTransport struct {
	base       http.RoundTripper
	origin     *url.URL
	skipVerify bool
}

func newWebDAVPolicyTransport(baseURL string, skipVerify bool, base http.RoundTripper) (*webDAVPolicyTransport, error) {
	origin, err := securityruntime.ValidateOutboundURL(baseURL)
	if err != nil {
		return nil, err
	}
	return &webDAVPolicyTransport{base: base, origin: origin, skipVerify: skipVerify}, nil
}

func (t *webDAVPolicyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.validateRequest(req); err != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, err
	}
	return t.base.RoundTrip(req)
}

func (t *webDAVPolicyTransport) validateRequest(req *http.Request) error {
	if req.URL == nil || !strings.EqualFold(req.URL.Scheme, t.origin.Scheme) ||
		!strings.EqualFold(req.URL.Hostname(), t.origin.Hostname()) ||
		webDAVPort(req.URL) != webDAVPort(t.origin) {
		return fmt.Errorf("WebDAV request must remain on its configured origin")
	}
	if t.skipVerify && !securityruntime.InsecureTransportAllowed() {
		return fmt.Errorf("webdav_tls_skip_verify requires LABTETHER_ALLOW_INSECURE_TRANSPORT=true")
	}
	if _, err := securityruntime.ValidateOutboundURL(req.URL.String()); err != nil {
		return fmt.Errorf("WebDAV outbound policy: %w", err)
	}
	return nil
}

func webDAVPort(endpoint *url.URL) string {
	if port := endpoint.Port(); port != "" {
		return port
	}
	if strings.EqualFold(endpoint.Scheme, "https") {
		return "443"
	}
	return "80"
}

func (t *webDAVPolicyTransport) CloseIdleConnections() {
	if transport, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		transport.CloseIdleConnections()
	}
}
