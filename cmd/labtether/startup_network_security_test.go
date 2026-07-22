package main

import (
	"strings"
	"testing"
)

func TestResolveHubBindAddressDefaultsDirectDevelopmentToLoopback(t *testing.T) {
	got, err := resolveHubBindAddressForEnvironment("", "development", false)
	if err != nil {
		t.Fatalf("resolve bind address: %v", err)
	}
	if got != defaultDirectHubBindAddress {
		t.Fatalf("direct development bind address = %q, want loopback", got)
	}
}

func TestResolveHubBindAddressUsesNetworkListenerForDeploymentRuntimes(t *testing.T) {
	for _, test := range []struct {
		name          string
		environment   string
		containerized bool
	}{
		{name: "production", environment: "production"},
		{name: "container", environment: "development", containerized: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveHubBindAddressForEnvironment("", test.environment, test.containerized)
			if err != nil {
				t.Fatalf("resolve bind address: %v", err)
			}
			if got != defaultNetworkHubBindAddress {
				t.Fatalf("deployment bind address = %q, want all interfaces", got)
			}
		})
	}
}

func TestResolveHubBindAddressHonorsValidatedExplicitOverride(t *testing.T) {
	got, err := resolveHubBindAddressForEnvironment(" ::1 ", "production", true)
	if err != nil {
		t.Fatalf("resolve explicit bind address: %v", err)
	}
	if got != "::1" {
		t.Fatalf("explicit bind address = %q, want ::1", got)
	}

	if _, err := resolveHubBindAddressForEnvironment("public.example.test", "development", false); err == nil {
		t.Fatal("expected hostname bind address to be rejected")
	}
}

func TestValidateHubExposureAuthRejectsWeakOrDefaultCredentials(t *testing.T) {
	strongOwner := strings.Repeat("a", minimumExposedAuthTokenBytes)
	strongAPI := strings.Repeat("b", minimumExposedAuthTokenBytes)
	tests := []struct {
		name    string
		secrets runtimeInstallSecrets
	}{
		{
			name: "published development owner default",
			secrets: runtimeInstallSecrets{
				OwnerToken: "dev-owner-token-change-me",
				APIToken:   strongAPI,
			},
		},
		{
			name: "short owner token",
			secrets: runtimeInstallSecrets{
				OwnerToken: "short-owner-token",
				APIToken:   strongAPI,
			},
		},
		{
			name: "short api token",
			secrets: runtimeInstallSecrets{
				OwnerToken: strongOwner,
				APIToken:   "short-api-token",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateHubExposureAuth("0.0.0.0", test.secrets); err == nil {
				t.Fatal("expected non-loopback exposure to reject weak/default auth")
			}
		})
	}
}

func TestValidateHubExposureAuthAllowsStrongRemoteAndLoopbackDevelopment(t *testing.T) {
	strong := runtimeInstallSecrets{
		OwnerToken: strings.Repeat("a", minimumExposedAuthTokenBytes),
		APIToken:   strings.Repeat("b", minimumExposedAuthTokenBytes),
	}
	if err := validateHubExposureAuth("0.0.0.0", strong); err != nil {
		t.Fatalf("strong non-loopback auth rejected: %v", err)
	}

	development := runtimeInstallSecrets{
		OwnerToken: "dev-owner-token-change-me",
		APIToken:   "dev-owner-token-change-me",
	}
	if err := validateHubExposureAuth("127.0.0.1", development); err != nil {
		t.Fatalf("loopback development auth rejected: %v", err)
	}
}

func TestValidateExplicitHubExposureAuthRejectsWeakOverrideBeforePersistence(t *testing.T) {
	const weakCredential = "dev-owner-token-change-me"
	err := validateExplicitHubExposureAuth("0.0.0.0", weakCredential, "")
	if err == nil {
		t.Fatal("expected weak explicit owner credential to be rejected")
	}
	if strings.Contains(err.Error(), weakCredential) {
		t.Fatal("weak-auth startup error exposed the credential")
	}

	if err := validateExplicitHubExposureAuth("0.0.0.0", "", ""); err != nil {
		t.Fatalf("blank overrides should defer to generated credentials: %v", err)
	}
	if err := validateExplicitHubExposureAuth("127.0.0.1", weakCredential, weakCredential); err != nil {
		t.Fatalf("loopback development overrides rejected: %v", err)
	}
}
