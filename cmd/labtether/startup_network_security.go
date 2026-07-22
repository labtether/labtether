package main

import (
	"fmt"
	"net"
	"os"
	"strings"
)

const (
	defaultDirectHubBindAddress  = "127.0.0.1"
	defaultNetworkHubBindAddress = "0.0.0.0"
	minimumExposedAuthTokenBytes = 32
)

// resolveHubBindAddress keeps direct development runs on loopback by default.
// Production and container runtimes retain their required network listener,
// while LABTETHER_BIND_ADDRESS is the explicit override for either direction.
func resolveHubBindAddress() (string, error) {
	return resolveHubBindAddressForEnvironment(
		os.Getenv("LABTETHER_BIND_ADDRESS"),
		os.Getenv("LABTETHER_ENV"),
		runningInContainer(),
	)
}

func resolveHubBindAddressForEnvironment(raw, environment string, containerized bool) (string, error) {
	bindAddress := strings.TrimSpace(raw)
	if bindAddress == "" {
		if strings.EqualFold(strings.TrimSpace(environment), "production") || containerized {
			bindAddress = defaultNetworkHubBindAddress
		} else {
			bindAddress = defaultDirectHubBindAddress
		}
	}
	if strings.EqualFold(bindAddress, "localhost") {
		return defaultDirectHubBindAddress, nil
	}
	if net.ParseIP(bindAddress) == nil {
		return "", fmt.Errorf("LABTETHER_BIND_ADDRESS must be localhost or an IP address")
	}
	return bindAddress, nil
}

func runningInContainer() bool {
	for _, marker := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(marker); err == nil {
			return true
		}
	}
	return false
}

func validateHubExposureAuth(bindAddress string, secrets runtimeInstallSecrets) error {
	if isLoopbackHubBindAddress(bindAddress) {
		return nil
	}
	if isWeakOrDefaultRuntimeAuthToken(secrets.OwnerToken) || isWeakOrDefaultRuntimeAuthToken(secrets.APIToken) {
		return weakExposedAuthError()
	}
	return nil
}

// validateExplicitHubExposureAuth runs before install-state resolution so a
// weak environment override cannot be written to disk before startup rejects
// the network exposure. Empty overrides are allowed because the resolver will
// replace them with generated credentials and validate the result afterward.
func validateExplicitHubExposureAuth(bindAddress, ownerToken, apiToken string) error {
	if isLoopbackHubBindAddress(bindAddress) {
		return nil
	}
	for _, token := range []string{ownerToken, apiToken} {
		if strings.TrimSpace(token) != "" && isWeakOrDefaultRuntimeAuthToken(token) {
			return weakExposedAuthError()
		}
	}
	return nil
}

func weakExposedAuthError() error {
	return fmt.Errorf("non-loopback hub binding requires non-default owner and API tokens of at least %d bytes", minimumExposedAuthTokenBytes)
}

func isLoopbackHubBindAddress(bindAddress string) bool {
	if strings.EqualFold(strings.TrimSpace(bindAddress), "localhost") {
		return true
	}
	ip := net.ParseIP(strings.TrimSpace(bindAddress))
	return ip != nil && ip.IsLoopback()
}

func isWeakOrDefaultRuntimeAuthToken(token string) bool {
	token = strings.TrimSpace(token)
	if len(token) < minimumExposedAuthTokenBytes || isWellKnownPlaceholder(token) {
		return true
	}
	switch strings.ToLower(token) {
	case "change-me", "changeme", "default", "dev", "dev-owner-token-change-me", "password", "secret", "test":
		return true
	default:
		return false
	}
}
