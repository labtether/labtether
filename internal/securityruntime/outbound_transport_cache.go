package securityruntime

import (
	"net/http"
	"sync"
)

// Outbound requests used to clone a fresh transport, and so a fresh
// connection pool, for every request: no keep-alive reuse and idle sockets
// piling up until their timeout. Derived transports are now cached per
// distinct base transport, scheme and caller address policy, so repeated
// requests through the same client share one pool. Each cached transport keeps
// the dial-time SSRF guard, which reads the live outbound policy on every dial.
//
// The key uses the base transport's identity: callers keep one client per
// connector, and a base transport must not be mutated after first use.

const maxCachedOutboundTransports = 256

type outboundTransportKey struct {
	base          *http.Transport // nil selects http.DefaultTransport settings
	scheme        string
	hasPolicy     bool
	addressPolicy OutboundAddressPolicy
}

var outboundTransports = struct {
	mu      sync.Mutex
	entries map[outboundTransportKey]*http.Transport
}{entries: make(map[outboundTransportKey]*http.Transport)}

func cachedSecureOutboundTransport(base *http.Transport, scheme string, addressPolicy *OutboundAddressPolicy) *http.Transport {
	key := outboundTransportKey{base: base, scheme: scheme}
	if addressPolicy != nil {
		key.hasPolicy = true
		key.addressPolicy = *addressPolicy
	}
	outboundTransports.mu.Lock()
	defer outboundTransports.mu.Unlock()
	if transport, ok := outboundTransports.entries[key]; ok {
		return transport
	}
	if len(outboundTransports.entries) >= maxCachedOutboundTransports {
		// Bound memory when callers churn base transports. An evicted
		// transport keeps serving clients that still hold it.
		for evictedKey, evicted := range outboundTransports.entries {
			evicted.CloseIdleConnections()
			delete(outboundTransports.entries, evictedKey)
			break
		}
	}
	transport := newSecureOutboundTransport(base, scheme, addressPolicy)
	outboundTransports.entries[key] = transport
	return transport
}
