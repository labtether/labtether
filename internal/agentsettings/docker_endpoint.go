// Package agentsettings contains hub-local copies of agent setting definitions,
// validation logic, and key constants. This allows the hub to manage agent
// settings without depending on the extracted agent packages.
package agentsettings

import (
	"fmt"
	"github.com/labtether/labtether/internal/securityruntime"
	"net/url"
	"strings"
)

const (
	dockerUnixScheme        = "unix://"
	dockerNpipePrefix       = "npipe:////./pipe/"
	dockerNpipeNameMaxBytes = 128
)

func trimDockerUnixScheme(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < len(dockerUnixScheme) {
		return trimmed, false
	}
	if strings.EqualFold(trimmed[:len(dockerUnixScheme)], dockerUnixScheme) {
		return strings.TrimSpace(trimmed[len(dockerUnixScheme):]), true
	}
	return trimmed, false
}

func normalizeDockerEndpointValue(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("%s cannot be empty", SettingKeyDockerEndpoint)
	}
	if strings.HasPrefix(value, "npipe:") {
		return normalizeDockerNpipeEndpoint(value)
	}

	if strings.HasPrefix(value, "/") {
		return value, nil
	}
	if path, ok := trimDockerUnixScheme(value); ok {
		if path == "" || !strings.HasPrefix(path, "/") {
			return "", fmt.Errorf("%s unix path must be absolute", SettingKeyDockerEndpoint)
		}
		return "unix://" + path, nil
	}

	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("%s must be an absolute unix path, unix:// path, canonical npipe path, or http(s) URL", SettingKeyDockerEndpoint)
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("%s URL scheme must be http or https", SettingKeyDockerEndpoint)
	}

	validated, err := securityruntime.ValidateOutboundURL(value)
	if err != nil {
		return "", fmt.Errorf("%s is not allowed by outbound policy: %w", SettingKeyDockerEndpoint, err)
	}
	validated.RawQuery = ""
	validated.Fragment = ""
	return strings.TrimRight(validated.String(), "/"), nil
}

func normalizeDockerNpipeEndpoint(value string) (string, error) {
	if !strings.HasPrefix(value, dockerNpipePrefix) {
		return "", fmt.Errorf("%s npipe path must use canonical %s<name> form", SettingKeyDockerEndpoint, dockerNpipePrefix)
	}

	name := strings.TrimPrefix(value, dockerNpipePrefix)
	if name == "" {
		return "", fmt.Errorf("%s npipe name cannot be empty", SettingKeyDockerEndpoint)
	}
	if len(name) > dockerNpipeNameMaxBytes {
		return "", fmt.Errorf("%s npipe name exceeds %d bytes", SettingKeyDockerEndpoint, dockerNpipeNameMaxBytes)
	}
	if !isDockerNpipeNameStart(name[0]) {
		return "", fmt.Errorf("%s npipe name must begin with an ASCII letter or digit", SettingKeyDockerEndpoint)
	}
	if strings.Contains(name, "..") {
		return "", fmt.Errorf("%s npipe name cannot contain traversal segments", SettingKeyDockerEndpoint)
	}
	if name[len(name)-1] == '.' {
		return "", fmt.Errorf("%s npipe name cannot end with punctuation", SettingKeyDockerEndpoint)
	}
	for i := 1; i < len(name); i++ {
		if !isDockerNpipeNameChar(name[i]) {
			return "", fmt.Errorf("%s npipe name contains unsupported characters", SettingKeyDockerEndpoint)
		}
	}
	return dockerNpipePrefix + name, nil
}

func isDockerNpipeNameStart(ch byte) bool {
	return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9'
}

func isDockerNpipeNameChar(ch byte) bool {
	return isDockerNpipeNameStart(ch) || ch == '_' || ch == '-' || ch == '.'
}
