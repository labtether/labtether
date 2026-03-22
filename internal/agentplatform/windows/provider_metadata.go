package windows

import (
	"os/exec"
	"strings"
)

func commandExists(lookPath func(string) (string, error), name string) bool {
	path, err := lookPath(name)
	return err == nil && strings.TrimSpace(path) != ""
}

func readCapabilityMetadata() map[string]string {
	return readCapabilityMetadataWith(exec.LookPath)
}

func readCapabilityMetadataWith(lookPath func(string) (string, error)) map[string]string {
	metadata := map[string]string{
		"cap_services":    "list,action",
		"service_backend": "scm",
		"cap_logs":        "stored,query,stream",
		"log_backend":     "eventlog",
		"cap_schedules":   "list",
		"cron_backend":    "taskscheduler",
		"cap_network":     "list,action",
		"network_backend": "netsh",
		"cap_packages":    "",
		"package_backend": "none",
	}

	if commandExists(lookPath, "winget") || commandExists(lookPath, "winget.exe") {
		metadata["cap_packages"] = "list,action"
		metadata["package_backend"] = "winget"
	} else if commandExists(lookPath, "choco") || commandExists(lookPath, "choco.exe") {
		metadata["cap_packages"] = "list,action"
		metadata["package_backend"] = "choco"
	}

	return metadata
}
