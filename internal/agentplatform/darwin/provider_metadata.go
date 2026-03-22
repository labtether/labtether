package darwin

import (
	"os/exec"
	"strings"

	"github.com/labtether/labtether/internal/agentplatform/tailscale"
)

func readCapabilityMetadata() map[string]string {
	return readCapabilityMetadataWith(exec.LookPath)
}

func readCapabilityMetadataWith(lookPath func(string) (string, error)) map[string]string {
	metadata := map[string]string{
		"cap_services":           "",
		"cap_packages":           "",
		"cap_logs":               "stored",
		"cap_schedules":          "list",
		"cap_network":            "list",
		"service_backend":        "none",
		"package_backend":        "none",
		"log_backend":            "none",
		"network_backend":        "ifconfig",
		"network_action_backend": "none",
	}

	if commandExists(lookPath, "launchctl") {
		metadata["cap_services"] = "list,action"
		metadata["service_backend"] = "launchd"
	}

	if commandExists(lookPath, "brew") {
		metadata["cap_packages"] = "list,action"
		metadata["package_backend"] = "brew"
	}

	if commandExists(lookPath, "log") {
		metadata["cap_logs"] = "stored,query,stream"
		metadata["log_backend"] = "oslog"
	}
	if commandExists(lookPath, "networksetup") {
		metadata["cap_network"] = "list,action"
		metadata["network_backend"] = "networksetup"
		metadata["network_action_backend"] = "networksetup"
	}

	return metadata
}

func commandExists(lookPath func(string) (string, error), name string) bool {
	path, err := lookPath(name)
	return err == nil && strings.TrimSpace(path) != ""
}

func readTailscaleMetadata() map[string]string {
	return tailscale.ReadMetadata()
}
