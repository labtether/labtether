package tailscale

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/securityruntime"
)

// CommandRunner is a function type for executing commands and returning combined output.
type CommandRunner func(timeout time.Duration, name string, args ...string) ([]byte, error)

// ReadMetadata collects Tailscale metadata using the real tailscale binary.
func ReadMetadata() map[string]string {
	return ReadMetadataWith(exec.LookPath, RunCommandOutput)
}

// ReadMetadataWith collects Tailscale metadata using the provided lookPath and runner,
// allowing tests to inject fakes.
func ReadMetadataWith(
	lookPath func(string) (string, error),
	runner CommandRunner,
) map[string]string {
	metadata := map[string]string{
		"tailscale_installed": "false",
	}

	path, err := lookPath("tailscale")
	if err != nil || strings.TrimSpace(path) == "" {
		return metadata
	}
	metadata["tailscale_installed"] = "true"

	statusOut, statusErr := runner(4*time.Second, path, "status", "--json")
	if statusErr == nil && len(statusOut) > 0 {
		mergeTailscaleStatusMetadata(metadata, statusOut)
	}

	versionOut, versionErr := runner(3*time.Second, path, "version", "--json")
	if versionErr == nil && len(versionOut) > 0 {
		if version := parseTailscaleVersion(versionOut); version != "" {
			metadata["tailscale_version"] = version
		}
	}
	if metadata["tailscale_version"] == "" {
		versionOut, versionErr = runner(3*time.Second, path, "version")
		if versionErr == nil && len(versionOut) > 0 {
			if version := parseTailscaleVersion(versionOut); version != "" {
				metadata["tailscale_version"] = version
			}
		}
	}

	return metadata
}

func mergeTailscaleStatusMetadata(metadata map[string]string, raw []byte) {
	var status struct {
		BackendState   string `json:"BackendState"`
		ExitNodeStatus any    `json:"ExitNodeStatus"`
		CurrentTailnet struct {
			Name           string `json:"Name"`
			MagicDNSSuffix string `json:"MagicDNSSuffix"`
		} `json:"CurrentTailnet"`
		Self struct {
			DNSName        string   `json:"DNSName"`
			TailscaleIPs   []string `json:"TailscaleIPs"`
			ExitNode       bool     `json:"ExitNode"`
			ExitNodeOption bool     `json:"ExitNodeOption"`
		} `json:"Self"`
	}
	if err := json.Unmarshal(raw, &status); err != nil {
		return
	}

	if value := strings.TrimSpace(status.BackendState); value != "" {
		metadata["tailscale_backend_state"] = value
	}

	tailnet := strings.TrimSpace(status.CurrentTailnet.Name)
	if tailnet == "" {
		tailnet = strings.TrimSuffix(strings.TrimSpace(status.CurrentTailnet.MagicDNSSuffix), ".")
	}
	if tailnet != "" {
		metadata["tailscale_tailnet"] = tailnet
	}

	if dnsName := strings.TrimSpace(status.Self.DNSName); dnsName != "" {
		metadata["tailscale_self_dns_name"] = dnsName
	}
	if len(status.Self.TailscaleIPs) > 0 {
		metadata["tailscale_self_tailscale_ip"] = strings.Join(status.Self.TailscaleIPs, ",")
	}

	exitNode := status.Self.ExitNode || status.Self.ExitNodeOption
	if !exitNode {
		switch v := status.ExitNodeStatus.(type) {
		case nil:
		case string:
			exitNode = strings.TrimSpace(v) != ""
		default:
			exitNode = true
		}
	}
	metadata["tailscale_exit_node"] = strconv.FormatBool(exitNode)
}

var tailscaleVersionPattern = regexp.MustCompile(`(?i)\bv?\d+\.\d+\.\d+(?:[-+][0-9a-z._-]+)?\b`)

func parseTailscaleVersion(raw []byte) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return ""
	}

	if strings.HasPrefix(trimmed, "{") {
		var parsed map[string]any
		if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
			for _, key := range []string{"short", "long", "version"} {
				if value, ok := parsed[key].(string); ok {
					if version := extractTailscaleVersionToken(value); version != "" {
						return version
					}
				}
			}
		}
	}

	for _, line := range strings.Split(trimmed, "\n") {
		if version := extractTailscaleVersionToken(line); version != "" {
			return version
		}
	}
	return ""
}

func extractTailscaleVersionToken(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	match := tailscaleVersionPattern.FindString(value)
	if match == "" {
		return ""
	}
	if len(match) >= 2 && (match[0] == 'v' || match[0] == 'V') && match[1] >= '0' && match[1] <= '9' {
		return match[1:]
	}
	return match
}

// RunCommandOutput executes a command with a timeout and returns combined stdout+stderr.
func RunCommandOutput(timeout time.Duration, name string, args ...string) ([]byte, error) {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := securityruntime.CommandContextCombinedOutput(ctx, name, args...)
	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("%s timed out", name)
	}
	return out, err
}
