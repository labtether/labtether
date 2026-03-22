package sysconfig

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/securityruntime"
)

const (
	NetworkActionCommandTimeout  = 60 * time.Second
	NetworkConnectivityTimeout   = 12 * time.Second
	DefaultConnectivityProbeHost = "1.1.1.1"
)

var NetworkHasCommand = HasCommand
var NetworkRunCommandWithTimeout = RunCommandWithTimeout

func ResolveNetworkMethod(raw string) (string, error) {
	method := strings.ToLower(strings.TrimSpace(raw))
	switch method {
	case "", "auto":
		if NetworkHasCommand("netplan") {
			return "netplan", nil
		}
		if NetworkHasCommand("nmcli") {
			return "nmcli", nil
		}
		return "", errors.New("no supported network tool found (expected netplan or nmcli)")
	case "netplan", "nmcli":
		if !NetworkHasCommand(method) {
			return "", fmt.Errorf("%s is not installed", method)
		}
		return method, nil
	default:
		return "", fmt.Errorf("invalid method %q: must be auto, netplan, or nmcli", method)
	}
}

func HasCommand(name string) bool {
	path, err := exec.LookPath(name)
	return err == nil && strings.TrimSpace(path) != ""
}

func RunCommandWithTimeout(timeout time.Duration, name string, args ...string) ([]byte, error) {
	if timeout <= 0 {
		timeout = NetworkActionCommandTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	out, err := securityruntime.CommandContextCombinedOutput(ctx, name, args...)
	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("%s timed out", name)
	}
	return out, err
}

func VerifyConnectivity(rawTarget string) error {
	routeOut, routeErr := NetworkRunCommandWithTimeout(5*time.Second, "ip", "route", "show", "default")
	if routeErr != nil {
		trimmed := TruncateCommandOutput(routeOut, MaxCommandOutputBytes)
		if trimmed == "" {
			return fmt.Errorf("failed to query default route: %w", routeErr)
		}
		return fmt.Errorf("failed to query default route: %s", trimmed)
	}

	lines := strings.Split(strings.TrimSpace(string(routeOut)), "\n")
	defaultRoute := ""
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			defaultRoute = line
			break
		}
	}
	if defaultRoute == "" {
		return errors.New("no default route detected after apply")
	}

	target := strings.TrimSpace(rawTarget)
	if target == "" {
		target = ParseDefaultRouteGateway(defaultRoute)
	}
	if target == "" {
		target = DefaultConnectivityProbeHost
	}

	if !NetworkHasCommand("ping") {
		return nil
	}
	pingOut, pingErr := NetworkRunCommandWithTimeout(NetworkConnectivityTimeout, "ping", "-c", "1", "-W", "2", target)
	if pingErr != nil {
		trimmed := TruncateCommandOutput(pingOut, MaxCommandOutputBytes)
		if trimmed == "" {
			return fmt.Errorf("ping %s failed: %w", target, pingErr)
		}
		return fmt.Errorf("ping %s failed: %s", target, trimmed)
	}
	return nil
}

func ParseDefaultRouteGateway(routeLine string) string {
	fields := strings.Fields(routeLine)
	for idx := 0; idx < len(fields)-1; idx++ {
		if fields[idx] == "via" {
			return strings.TrimSpace(fields[idx+1])
		}
	}
	return ""
}

func CollectActiveNMConnections() ([]string, error) {
	out, err := NetworkRunCommandWithTimeout(15*time.Second, "nmcli", "-t", "-f", "NAME", "connection", "show", "--active")
	if err != nil {
		trimmed := TruncateCommandOutput(out, MaxCommandOutputBytes)
		if trimmed == "" {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %s", err, trimmed)
	}
	seen := make(map[string]struct{})
	connections := make([]string, 0)
	for _, line := range strings.Split(string(out), "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		connections = append(connections, name)
	}
	return connections, nil
}

func ActivateNMConnections(connections []string) (string, error) {
	if len(connections) == 0 {
		return "", errors.New("no nmcli connections provided")
	}

	var output strings.Builder
	var firstErr error
	for _, connection := range connections {
		connection = strings.TrimSpace(connection)
		if connection == "" {
			continue
		}
		out, err := NetworkRunCommandWithTimeout(NetworkActionCommandTimeout, "nmcli", "connection", "up", connection)
		trimmed := TruncateCommandOutput(out, MaxCommandOutputBytes)
		if trimmed != "" {
			if output.Len() > 0 {
				output.WriteString("\n")
			}
			output.WriteString(trimmed)
		}
		if err != nil && firstErr == nil {
			firstErr = fmt.Errorf("nmcli connection up %s failed: %w", connection, err)
		}
	}

	if firstErr != nil {
		return output.String(), firstErr
	}
	return output.String(), nil
}
