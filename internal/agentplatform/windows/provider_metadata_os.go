//go:build windows

package windows

import (
	"os"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentplatform/tailscale"
)

func readWindowsOSMetadata() map[string]string {
	metadata := map[string]string{}
	if name, err := os.Hostname(); err == nil {
		metadata["computer_name"] = name
	}
	if out, err := tailscale.RunCommandOutput(5*time.Second, "powershell", "-NoProfile", "-Command",
		`(Get-CimInstance Win32_OperatingSystem).Caption + '|' + (Get-CimInstance Win32_OperatingSystem).BuildNumber`); err == nil {
		parts := strings.SplitN(strings.TrimSpace(string(out)), "|", 2)
		if len(parts) == 2 {
			metadata["os_edition"] = strings.TrimSpace(parts[0])
			metadata["os_build"] = strings.TrimSpace(parts[1])
		}
	}
	if out, err := tailscale.RunCommandOutput(5*time.Second, "powershell", "-NoProfile", "-Command",
		`(Get-CimInstance Win32_ComputerSystem).PartOfDomain`); err == nil {
		metadata["domain_joined"] = strings.TrimSpace(strings.ToLower(string(out)))
	}
	return metadata
}
