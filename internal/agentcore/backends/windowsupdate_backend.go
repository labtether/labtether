package backends

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/securityruntime"
)

const (
	windowsUpdateListTimeout   = 60 * time.Second
	windowsUpdateRebootTimeout = 10 * time.Second
)

// RunWindowsUpdateCommand is the function used to run PowerShell commands for Windows Update.
// Overridable for tests.
var RunWindowsUpdateCommand = securityruntime.CommandContextCombinedOutput

// WindowsUpdateInfo holds information about a single installed Windows update (hotfix).
type WindowsUpdateInfo struct {
	HotFixID    string `json:"hotfix_id"`
	Description string `json:"description"`
	InstalledBy string `json:"installed_by,omitempty"`
	InstalledOn string `json:"installed_on,omitempty"`
	Source      string `json:"source,omitempty"`
}

// WindowsUpdateBackend queries installed Windows updates and reboot status via PowerShell.
// It has no build tags so that parser tests can run on any platform.
type WindowsUpdateBackend struct{}

// ListInstalledUpdates returns installed hotfixes via Get-HotFix | ConvertTo-Json.
func (WindowsUpdateBackend) ListInstalledUpdates() ([]WindowsUpdateInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), windowsUpdateListTimeout)
	defer cancel()

	out, err := RunWindowsUpdateCommand(ctx, "powershell.exe",
		"-NonInteractive", "-NoProfile", "-Command",
		"Get-HotFix | ConvertTo-Json -Depth 2")
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("Get-HotFix timed out")
		}
		trimmed := strings.TrimSpace(string(out))
		if trimmed != "" {
			return nil, fmt.Errorf("Get-HotFix failed: %s", trimmed)
		}
		return nil, fmt.Errorf("Get-HotFix failed: %w", err)
	}

	return parseGetHotFixOutput(out)
}

// CheckRebootRequired reports whether a Windows Update reboot is pending by
// checking the registry key
// HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired.
func (WindowsUpdateBackend) CheckRebootRequired() (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), windowsUpdateRebootTimeout)
	defer cancel()

	// Test-Path returns "True" or "False" and exits 0 in both cases.
	out, err := RunWindowsUpdateCommand(ctx, "powershell.exe",
		"-NonInteractive", "-NoProfile", "-Command",
		`Test-Path "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired"`)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return false, fmt.Errorf("reboot-required check timed out")
		}
		trimmed := strings.TrimSpace(string(out))
		if trimmed != "" {
			return false, fmt.Errorf("reboot-required check failed: %s", trimmed)
		}
		return false, fmt.Errorf("reboot-required check failed: %w", err)
	}

	return parseRebootRequiredOutput(out), nil
}

// hotfixJSON is the JSON shape emitted by Get-HotFix | ConvertTo-Json.
// PowerShell serialises WMI Win32_QuickFixEngineering objects.
type hotfixJSON struct {
	Source      string `json:"Source"`
	Description string `json:"Description"`
	HotFixID    string `json:"HotFixID"`
	InstalledBy string `json:"InstalledBy"`
	InstalledOn string `json:"InstalledOn"`
}

// parseGetHotFixOutput parses the JSON array produced by
// `Get-HotFix | ConvertTo-Json`. When only one hotfix is installed
// PowerShell emits a single JSON object rather than an array; both
// forms are handled.
func parseGetHotFixOutput(raw []byte) ([]WindowsUpdateInfo, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, nil
	}

	// PowerShell emits a BOM on some versions; strip it.
	trimmed = strings.TrimPrefix(trimmed, "\xef\xbb\xbf")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return nil, nil
	}

	data := []byte(trimmed)

	// Attempt array parse first.
	var arr []hotfixJSON
	if err := json.Unmarshal(data, &arr); err != nil {
		// Fall back to single-object parse (one hotfix installed).
		var single hotfixJSON
		if err2 := json.Unmarshal(data, &single); err2 != nil {
			return nil, fmt.Errorf("parseGetHotFixOutput: %w", err2)
		}
		arr = []hotfixJSON{single}
	}

	updates := make([]WindowsUpdateInfo, 0, len(arr))
	for _, h := range arr {
		id := strings.TrimSpace(h.HotFixID)
		if id == "" {
			continue
		}
		updates = append(updates, WindowsUpdateInfo{
			HotFixID:    id,
			Description: strings.TrimSpace(h.Description),
			InstalledBy: strings.TrimSpace(h.InstalledBy),
			InstalledOn: normaliseHotFixDate(strings.TrimSpace(h.InstalledOn)),
			Source:      strings.TrimSpace(h.Source),
		})
	}

	return updates, nil
}

// parseRebootRequiredOutput interprets the stdout of a PowerShell Test-Path
// command. Returns true only when the output contains "True".
func parseRebootRequiredOutput(raw []byte) bool {
	return strings.Contains(strings.TrimSpace(string(raw)), "True")
}

// normaliseHotFixDate converts the M/D/YYYY h:mm:ss AM/PM format that
// Get-HotFix emits into a date-only string (YYYY-MM-DD). If parsing fails
// the original value is returned unchanged so no data is silently dropped.
func normaliseHotFixDate(raw string) string {
	if raw == "" {
		return ""
	}

	layouts := []string{
		"1/2/2006 3:04:05 PM",
		"1/2/2006 15:04:05",
		"1/2/2006",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.Format("2006-01-02")
		}
	}

	return raw
}
