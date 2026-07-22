package securityruntime

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func withMockLookupIPAddrs(
	t *testing.T,
	fn func(ctx context.Context, host string) ([]net.IPAddr, error),
) {
	t.Helper()
	originalLookupIPAddrs := lookupIPAddrs
	originalLookupIP := lookupIP
	lookupIPAddrs = fn
	lookupIP = func(host string) ([]net.IP, error) {
		addrs, err := fn(context.Background(), host)
		if err != nil {
			return nil, err
		}
		out := make([]net.IP, 0, len(addrs))
		for _, addr := range addrs {
			if addr.IP != nil {
				out = append(out, addr.IP)
			}
		}
		return out, nil
	}
	t.Cleanup(func() {
		lookupIPAddrs = originalLookupIPAddrs
		lookupIP = originalLookupIP
	})
}

func TestValidateOutboundURLRejectsUnsupportedScheme(t *testing.T) {
	if _, err := ValidateOutboundURL("ftp://localhost/resource"); err == nil {
		t.Fatalf("expected unsupported scheme to fail")
	}
}

func TestValidateOutboundURLRejectsInsecureSchemeByDefault(t *testing.T) {
	if _, err := ValidateOutboundURL("http://127.0.0.1:8080/healthz"); err == nil {
		t.Fatalf("expected insecure http scheme to fail without explicit opt-in")
	}
}

func TestValidateOutboundURLAllowsInsecureSchemeWhenOptedIn(t *testing.T) {
	t.Setenv(envAllowInsecureTransport, "true")
	t.Setenv(envOutboundAllowLoopback, "true")
	if _, err := ValidateOutboundURL("http://127.0.0.1:8080/healthz"); err != nil {
		t.Fatalf("expected loopback http host to be allowed with insecure opt-in, got %v", err)
	}
}

func TestValidateOutboundURLAllowlistModeRequiresExplicitLoopbackAllowlist(t *testing.T) {
	t.Setenv(envAllowInsecureTransport, "true")
	t.Setenv(envOutboundAllowlistMode, "true")
	t.Setenv(envOutboundAllowPrivate, "true")
	t.Setenv(envOutboundAllowLoopback, "true")
	if _, err := ValidateOutboundURL("http://127.0.0.1:8080/healthz"); err == nil {
		t.Fatalf("expected non-allowlisted loopback host to be rejected in allowlist mode")
	}
	t.Setenv(envOutboundAllowedHosts, "127.0.0.1")
	if _, err := ValidateOutboundURL("http://127.0.0.1:8080/healthz"); err != nil {
		t.Fatalf("expected allowlisted loopback host to be allowed, got %v", err)
	}
}

func TestValidateOutboundURLRequiresAllowlistedPublicHost(t *testing.T) {
	withMockLookupIPAddrs(t, func(_ context.Context, _ string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}, nil
	})
	t.Setenv(envOutboundAllowlistMode, "true")
	t.Setenv(envAllowInsecureTransport, "false")
	t.Setenv(envOutboundAllowPrivate, "false")
	t.Setenv(envOutboundAllowLoopback, "false")
	t.Setenv(envOutboundAllowedHosts, "api.example.com,*.internal.example.com")
	if _, err := ValidateOutboundURL("https://api.example.com/path"); err != nil {
		t.Fatalf("expected allowlisted host to be allowed, got %v", err)
	}
	if _, err := ValidateOutboundURL("https://blocked.example.net/path"); err == nil {
		t.Fatalf("expected non-allowlisted host to fail")
	}
}

func TestValidateOutboundDialTarget(t *testing.T) {
	withMockLookupIPAddrs(t, func(_ context.Context, _ string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}, nil
	})
	t.Setenv(envOutboundAllowlistMode, "true")
	t.Setenv(envOutboundAllowPrivate, "false")
	t.Setenv(envOutboundAllowLoopback, "false")
	t.Setenv(envOutboundAllowedHosts, "collector.example.com")
	if err := ValidateOutboundDialTarget("collector.example.com", 443); err != nil {
		t.Fatalf("expected dial target to be allowed, got %v", err)
	}
	if err := ValidateOutboundDialTarget("collector.example.com", -1); err == nil {
		t.Fatalf("expected invalid port to fail")
	}
}

func TestValidateOutboundDialTargetFailsClosedOnDNSFailure(t *testing.T) {
	withMockLookupIPAddrs(t, func(_ context.Context, _ string) ([]net.IPAddr, error) {
		return nil, errors.New("resolver unavailable")
	})
	if err := ValidateOutboundDialTarget("public.example.com", 443); err == nil {
		t.Fatal("expected DNS failure to reject outbound target")
	}
}

func TestCanonicalizeOutboundHostRejectsAmbiguousOrCompositeInputs(t *testing.T) {
	tests := []string{
		"https://example.com",
		"user@example.com",
		"example.com/path",
		"example.com?port=3389",
		"example.com#fragment",
		"example.com:3389",
		"[2001:db8::1]:3389",
		"2001:db8::1::2",
		"example.com\nlocalhost",
		"example.com%00",
		"exa mple.com",
	}
	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			if _, err := CanonicalizeOutboundHost(raw); err == nil {
				t.Fatalf("CanonicalizeOutboundHost(%q) succeeded, want rejection", raw)
			}
		})
	}
}

func TestCanonicalizeOutboundHostCanonicalIPv4IPv6AndHostname(t *testing.T) {
	tests := map[string]string{
		"EXAMPLE.COM.":       "example.com",
		"192.0.2.10":         "192.0.2.10",
		"2001:0DB8:0:0::1":   "2001:db8::1",
		"[2001:0DB8:0:0::1]": "2001:db8::1",
	}
	for raw, want := range tests {
		t.Run(raw, func(t *testing.T) {
			got, err := CanonicalizeOutboundHost(raw)
			if err != nil {
				t.Fatalf("CanonicalizeOutboundHost(%q): %v", raw, err)
			}
			if got != want {
				t.Fatalf("CanonicalizeOutboundHost(%q) = %q, want %q", raw, got, want)
			}
		})
	}
}

func TestResolveOutboundTCPHostRejectsMixedSafeAndLoopbackAnswers(t *testing.T) {
	withMockLookupIPAddrs(t, func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host != "remote.example.com" {
			return nil, errors.New("unexpected host")
		}
		return []net.IPAddr{
			{IP: net.ParseIP("203.0.113.10")},
			{IP: net.ParseIP("127.0.0.1")},
		}, nil
	})

	if _, err := ResolveOutboundTCPHost(context.Background(), "remote.example.com", 3389); err == nil {
		t.Fatal("expected mixed public/loopback DNS answers to be rejected")
	} else if !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("expected loopback rejection, got %v", err)
	}
}

func TestResolveOutboundTCPHostReturnsValidatedLiteral(t *testing.T) {
	withMockLookupIPAddrs(t, func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host != "remote.example.com" {
			return nil, errors.New("unexpected host")
		}
		return []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}, nil
	})

	got, err := ResolveOutboundTCPHost(context.Background(), "REMOTE.EXAMPLE.COM.", 3389)
	if err != nil {
		t.Fatalf("ResolveOutboundTCPHost: %v", err)
	}
	if got != "203.0.113.10" {
		t.Fatalf("ResolveOutboundTCPHost = %q, want validated literal", got)
	}
}

func TestValidateOutboundURLRejectsHostResolvingToLoopback(t *testing.T) {
	withMockLookupIPAddrs(t, func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host == "public.example.com" {
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		}
		return nil, errors.New("unexpected host")
	})

	if _, err := ValidateOutboundURL("https://public.example.com/path"); err == nil {
		t.Fatalf("expected DNS-resolved loopback host to fail")
	} else if !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("expected loopback resolution error, got %v", err)
	}
}

func TestValidateOutboundDialTargetRejectsHostResolvingToPrivateIP(t *testing.T) {
	withMockLookupIPAddrs(t, func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host == "public.example.com" {
			return []net.IPAddr{{IP: net.ParseIP("10.0.0.5")}}, nil
		}
		return nil, errors.New("unexpected host")
	})

	if err := ValidateOutboundDialTarget("public.example.com", 443); err == nil {
		t.Fatalf("expected DNS-resolved private host to fail")
	} else if !strings.Contains(err.Error(), "private") {
		t.Fatalf("expected private resolution error, got %v", err)
	}
}

func TestValidateOutboundURLAllowsAllowlistedHostResolvingToPrivateIPOverHTTPSByDefault(t *testing.T) {
	t.Setenv(envOutboundAllowlistMode, "true")
	t.Setenv(envOutboundAllowedHosts, "collector.example.com")
	withMockLookupIPAddrs(t, func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host == "collector.example.com" {
			return []net.IPAddr{{IP: net.ParseIP("192.168.1.25")}}, nil
		}
		return nil, errors.New("unexpected host")
	})

	if _, err := ValidateOutboundURL("https://collector.example.com/metrics"); err != nil {
		t.Fatalf("expected allowlisted private https host to be allowed, got %v", err)
	}
}

func TestPrivateOptInDoesNotAllowIPv4LinkLocalHTTP(t *testing.T) {
	t.Setenv(envAllowInsecureTransport, "true")
	t.Setenv(envOutboundAllowPrivate, "true")
	t.Setenv(envOutboundAllowLinkLocal, "false")

	req, err := http.NewRequest(http.MethodGet, "http://169.254.169.254/latest/meta-data", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if _, err := DoOutboundRequest(&http.Client{Timeout: time.Second}, req); err == nil {
		t.Fatal("expected private opt-in not to permit IPv4 link-local HTTP")
	} else if !strings.Contains(err.Error(), "link-local") {
		t.Fatalf("expected link-local rejection, got %v", err)
	}
}

func TestPrivateOptInDoesNotAllowResolvedIPv4LinkLocalURL(t *testing.T) {
	t.Setenv(envOutboundAllowPrivate, "true")
	t.Setenv(envOutboundAllowLinkLocal, "false")
	withMockLookupIPAddrs(t, func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host == "linklocal.example.com" {
			return []net.IPAddr{{IP: net.ParseIP("169.254.10.20")}}, nil
		}
		return nil, errors.New("unexpected host")
	})

	if _, err := ValidateOutboundURL("https://linklocal.example.com/path"); err == nil {
		t.Fatal("expected resolved IPv4 link-local host to remain denied when private targets are allowed")
	} else if !strings.Contains(err.Error(), "link-local") {
		t.Fatalf("expected link-local rejection, got %v", err)
	}
}

func TestPrivateOptInDoesNotAllowIPv6LinkLocalTCP(t *testing.T) {
	t.Setenv(envOutboundAllowPrivate, "true")
	t.Setenv(envOutboundAllowLinkLocal, "false")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := DialOutboundTCPContext(ctx, "fe80::1", 443, time.Second); err == nil {
		t.Fatal("expected private opt-in not to permit IPv6 link-local TCP")
	} else if !strings.Contains(err.Error(), "link-local") {
		t.Fatalf("expected link-local rejection, got %v", err)
	}
}

func TestLinkLocalRequiresSeparateExplicitOptIn(t *testing.T) {
	t.Setenv(envOutboundAllowPrivate, "false")
	t.Setenv(envOutboundAllowLinkLocal, "true")

	if _, err := ValidateOutboundURL("https://169.254.10.20/path"); err != nil {
		t.Fatalf("expected explicit link-local opt-in to permit IPv4 HTTPS target validation, got %v", err)
	}
	if err := ValidateOutboundDialTarget("fe80::1", 443); err != nil {
		t.Fatalf("expected explicit link-local opt-in to permit IPv6 TCP target validation, got %v", err)
	}
}

func TestValidateOutboundURLAllowsPrivateHTTPSByDefault(t *testing.T) {
	withMockLookupIPAddrs(t, func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host == "homeassistant.simbaslabs.com" {
			return []net.IPAddr{{IP: net.ParseIP("192.168.1.25")}}, nil
		}
		return nil, errors.New("unexpected host")
	})

	if _, err := ValidateOutboundURL("https://homeassistant.simbaslabs.com"); err != nil {
		t.Fatalf("expected private https host to be allowed by default, got %v", err)
	}
}

func TestValidateOutboundURLRejectsPrivateHTTPSWhenExplicitlyDisabled(t *testing.T) {
	t.Setenv(envOutboundAllowPrivate, "false")
	withMockLookupIPAddrs(t, func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host == "homeassistant.simbaslabs.com" {
			return []net.IPAddr{{IP: net.ParseIP("192.168.1.25")}}, nil
		}
		return nil, errors.New("unexpected host")
	})

	if _, err := ValidateOutboundURL("https://homeassistant.simbaslabs.com"); err == nil {
		t.Fatal("expected explicit allow_private=false to reject private https host")
	} else if !strings.Contains(err.Error(), "private") {
		t.Fatalf("expected private-host error, got %v", err)
	}
}

func TestValidateOutboundURLRuntimeOverrideCanDisablePrivateHTTPS(t *testing.T) {
	SetRuntimeEnvOverrides(map[string]string{envOutboundAllowPrivate: "false"})
	t.Cleanup(func() {
		SetRuntimeEnvOverrides(nil)
	})
	withMockLookupIPAddrs(t, func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host == "proxmox-deltaserver.simbaslabs.com" {
			return []net.IPAddr{{IP: net.ParseIP("192.168.0.33")}}, nil
		}
		return nil, errors.New("unexpected host")
	})

	if _, err := ValidateOutboundURL("https://proxmox-deltaserver.simbaslabs.com"); err == nil {
		t.Fatal("expected runtime override to reject private https host")
	} else if !strings.Contains(err.Error(), "private") {
		t.Fatalf("expected private-host error, got %v", err)
	}
}

func TestValidateOutboundURLStillRejectsPrivateLoopbackHTTPSByDefault(t *testing.T) {
	withMockLookupIPAddrs(t, func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host == "localhost.example.test" {
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		}
		return nil, errors.New("unexpected host")
	})

	if _, err := ValidateOutboundURL("https://localhost.example.test"); err == nil {
		t.Fatal("expected loopback https host to remain blocked")
	} else if !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("expected loopback error, got %v", err)
	}
}
