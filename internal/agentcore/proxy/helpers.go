package proxy

import (
	"net/url"
	"os"
	"strings"
)

// normalizeDockerImage normalizes a Docker image name for comparison.
func normalizeDockerImage(image string) string {
	image = strings.TrimSpace(image)
	if image == "" {
		return ""
	}

	// Lowercase
	image = strings.ToLower(image)

	// Strip digest (@sha256:...)
	if idx := strings.Index(image, "@"); idx >= 0 {
		image = image[:idx]
	}

	// Strip tag — but only from the last segment to avoid stripping port from registry URLs.
	lastSlash := strings.LastIndex(image, "/")
	if lastSlash >= 0 {
		name := image[lastSlash+1:]
		if colonIdx := strings.LastIndex(name, ":"); colonIdx >= 0 {
			image = image[:lastSlash+1] + name[:colonIdx]
		}
	} else {
		if colonIdx := strings.LastIndex(image, ":"); colonIdx >= 0 {
			image = image[:colonIdx]
		}
	}

	// Strip common Docker Hub prefixes.
	image = strings.TrimPrefix(image, "registry-1.docker.io/library/")
	image = strings.TrimPrefix(image, "registry-1.docker.io/")
	image = strings.TrimPrefix(image, "index.docker.io/library/")
	image = strings.TrimPrefix(image, "index.docker.io/")
	image = strings.TrimPrefix(image, "docker.io/library/")
	image = strings.TrimPrefix(image, "docker.io/")

	// Strip registry hostname prefixes.
	if slash := strings.Index(image, "/"); slash > 0 {
		hostPrefix := image[:slash]
		if hostPrefix == "localhost" || strings.Contains(hostPrefix, ".") || strings.Contains(hostPrefix, ":") {
			image = image[slash+1:]
		}
	}

	// Normalize Docker Hub library aliases.
	image = strings.TrimPrefix(image, "library/")

	return strings.TrimSpace(image)
}

// normalizeHTTPSURL normalizes a URL to use HTTPS scheme.
func normalizeHTTPSURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || strings.TrimSpace(parsed.Scheme) == "" {
		return trimmed
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme == "http" || scheme == "https" {
		return trimmed
	}
	return trimmed
}

// allowInsecureTransportOptIn checks if LABTETHER_ALLOW_INSECURE_TRANSPORT is set.
func allowInsecureTransportOptIn() bool {
	val := strings.ToLower(strings.TrimSpace(os.Getenv("LABTETHER_ALLOW_INSECURE_TRANSPORT")))
	return val == "true" || val == "1" || val == "yes"
}
