package securityruntime

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
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

func TestValidateOutboundURLAllowsHostResolvingToLinkLocalIPOverHTTPSByDefault(t *testing.T) {
	withMockLookupIPAddrs(t, func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host == "linklocal.example.com" {
			return []net.IPAddr{{IP: net.ParseIP("169.254.10.20")}}, nil
		}
		return nil, errors.New("unexpected host")
	})

	if _, err := ValidateOutboundURL("https://linklocal.example.com/path"); err != nil {
		t.Fatalf("expected secure link-local host to be allowed by default, got %v", err)
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
