package main

import (
	"testing"

	"github.com/labtether/labtether/internal/runtimesettings"
)

func TestBuildSecurityRuntimeEnvOverrides(t *testing.T) {
	values := buildSecurityRuntimeEnvOverrides(map[string]string{
		runtimesettings.KeySecurityOutboundAllowPrivate: "false",
	})
	if got := values["LABTETHER_OUTBOUND_ALLOW_PRIVATE"]; got != "false" {
		t.Fatalf("expected outbound private override false, got %q", got)
	}

	values = buildSecurityRuntimeEnvOverrides(map[string]string{
		runtimesettings.KeySecurityOutboundAllowPrivate: "auto",
	})
	if len(values) != 0 {
		t.Fatalf("expected auto to clear runtime env override, got %#v", values)
	}
}
