package proxy

import (
	dockerpkg "github.com/labtether/labtether/internal/agentcore/docker"
)

// Route represents a single routing rule discovered from a reverse proxy.
type Route struct {
	Domain     string            // e.g. "plex.home.lab"
	BackendURL string            // e.g. "http://10.0.1.5:32400" (may be empty)
	TLS        bool              // proxy terminates TLS
	RouterName string            // proxy-internal route name
	Metadata   map[string]string // provider extras
}

// Provider is the core abstraction for reverse proxy service discovery.
// Implementations (Traefik, Caddy, NPM, etc.) detect themselves from Docker
// containers and fetch their routing rules.
type Provider interface {
	// Name returns the provider identifier (e.g. "traefik", "caddy", "npm").
	Name() string

	// DetectAndConnect inspects the running containers to find this proxy.
	// Returns the API URL to query and true if detection succeeded.
	DetectAndConnect(containers []dockerpkg.DockerContainer) (apiURL string, ok bool)

	// FetchRoutes queries the proxy API and returns discovered routes.
	FetchRoutes(apiURL string) ([]Route, error)
}

// FindHostPort finds the host-mapped public port for a given container-internal
// (private) port. Returns 0 if no mapping is found.
func FindHostPort(ports []dockerpkg.DockerPort, privatePort int) int {
	for _, p := range ports {
		if p.PrivatePort == privatePort && p.PublicPort > 0 {
			return p.PublicPort
		}
	}
	return 0
}
