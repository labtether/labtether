package remotewrite

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeConfigStrictBoundsAndEndpointShape(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{name: "missing enabled URL", config: Config{Enabled: true, Interval: MinInterval}},
		{name: "interval below minimum", config: Config{Enabled: true, URL: "https://metrics.example.test/write", Interval: MinInterval - time.Nanosecond}},
		{name: "interval above maximum", config: Config{Enabled: true, URL: "https://metrics.example.test/write", Interval: MaxInterval + time.Nanosecond}},
		{name: "userinfo", config: Config{Enabled: true, URL: "https://user:secret@metrics.example.test/write", Interval: MinInterval}},
		{name: "query", config: Config{Enabled: true, URL: "https://metrics.example.test/write?token=secret", Interval: MinInterval}},
		{name: "fragment", config: Config{Enabled: true, URL: "https://metrics.example.test/write#secret", Interval: MinInterval}},
		{name: "unsupported scheme", config: Config{Enabled: true, URL: "ftp://metrics.example.test/write", Interval: MinInterval}},
		{name: "port zero", config: Config{Enabled: true, URL: "https://metrics.example.test:0/write", Interval: MinInterval}},
		{name: "port above range", config: Config{Enabled: true, URL: "https://metrics.example.test:65536/write", Interval: MinInterval}},
		{name: "oversized URL", config: Config{Enabled: true, URL: "https://metrics.example.test/" + strings.Repeat("x", MaxURLBytes), Interval: MinInterval}},
		{name: "oversized username", config: Config{Enabled: true, URL: "https://metrics.example.test/write", Username: strings.Repeat("x", MaxUsernameBytes+1), Interval: MinInterval}},
		{name: "oversized password", config: Config{Enabled: true, URL: "https://metrics.example.test/write", Username: "user", Password: strings.Repeat("x", MaxPasswordBytes+1), Interval: MinInterval}},
		{name: "password without username", config: Config{Enabled: true, URL: "https://metrics.example.test/write", Password: "secret", Interval: MinInterval}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NormalizeConfig(test.config); err == nil {
				t.Fatal("expected invalid configuration to be rejected")
			}
		})
	}
}

func TestNormalizeConfigCanonicalizesWithoutChangingSecretBytes(t *testing.T) {
	config, err := NormalizeConfig(Config{
		Enabled:  true,
		URL:      " HTTPS://Metrics.Example.Test/api/v1/write ",
		Username: " user ",
		Password: "  exact secret bytes  ",
		Interval: MinInterval,
	})
	if err != nil {
		t.Fatalf("NormalizeConfig: %v", err)
	}
	if config.URL != "https://metrics.example.test/api/v1/write" || config.Username != "user" {
		t.Fatalf("canonical endpoint/user mismatch: URL=%q username=%q", config.URL, config.Username)
	}
	if config.Password != "  exact secret bytes  " {
		t.Fatalf("password bytes changed: %q", config.Password)
	}
}

func TestEndpointFingerprintStableAndDoesNotContainURL(t *testing.T) {
	config := Config{Enabled: true, URL: "https://metrics.example.test/api/v1/write", Username: "tenant-a", Password: "old", Interval: MinInterval}
	first, err := config.EndpointFingerprint()
	if err != nil {
		t.Fatalf("EndpointFingerprint: %v", err)
	}
	second, err := (Config{Enabled: true, URL: "HTTPS://METRICS.EXAMPLE.TEST/api/v1/write", Username: "tenant-a", Password: "rotated", Interval: MinInterval}).EndpointFingerprint()
	if err != nil {
		t.Fatalf("EndpointFingerprint canonical: %v", err)
	}
	if first != second || len(first) != 64 || strings.Contains(first, "metrics") {
		t.Fatalf("invalid endpoint fingerprint: %q / %q", first, second)
	}
	tenantB, err := (Config{Enabled: true, URL: config.URL, Username: "tenant-b", Interval: MinInterval}).EndpointFingerprint()
	if err != nil {
		t.Fatalf("EndpointFingerprint tenant change: %v", err)
	}
	if tenantB == first {
		t.Fatal("tenant username change retained the old replay identity")
	}
}
