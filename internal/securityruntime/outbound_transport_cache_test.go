package securityruntime

import (
	"context"
	"net/http"
	"testing"
)

func derivedOutboundTransport(t *testing.T, client *http.Client) *http.Transport {
	t.Helper()
	validating, ok := client.Transport.(outboundValidatingRoundTripper)
	if !ok {
		t.Fatalf("client transport = %T, want validating round tripper", client.Transport)
	}
	transport, ok := validating.base.(*http.Transport)
	if !ok {
		t.Fatalf("validating base = %T, want *http.Transport", validating.base)
	}
	return transport
}

// Every outbound request used to clone a new transport (new connection pool).
// Requests with the same policy must share one, without losing the SSRF guard.
func TestOutboundRequestsWithSamePolicyShareTransport(t *testing.T) {
	t.Setenv(envOutboundAllowLoopback, "")
	base := &http.Client{Transport: &http.Transport{}}

	first := derivedOutboundTransport(t, secureOutboundHTTPClient(base, "https"))
	second := derivedOutboundTransport(t, secureOutboundHTTPClient(base, "https"))
	if first != second {
		t.Fatal("two requests with the same policy built separate transports")
	}
	if first == base.Transport {
		t.Fatal("caller's base transport was used directly instead of a guarded clone")
	}
	if other := derivedOutboundTransport(t, secureOutboundHTTPClient(base, "http")); other == first {
		t.Fatal("different schemes shared one transport")
	}
	policy := OutboundAddressPolicy{AllowPrivate: true}
	if restricted := derivedOutboundTransport(t, secureOutboundHTTPClientWithAddressPolicy(base, "https", &policy)); restricted == first {
		t.Fatal("caller address policy shared a transport with the unrestricted client")
	}
	if defaults := derivedOutboundTransport(t, secureOutboundHTTPClient(nil, "https")); defaults == first {
		t.Fatal("default client shared a transport with a custom base transport")
	}

	if first.Proxy != nil || first.DialTLSContext != nil || first.DialContext == nil {
		t.Fatal("cached transport lost the dial-time guard")
	}
	if conn, err := first.DialContext(context.Background(), "tcp", "127.0.0.1:9"); err == nil {
		_ = conn.Close()
		t.Fatal("cached transport dialled a loopback address")
	}
}
