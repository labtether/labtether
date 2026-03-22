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
	hyperVListTimeout   = 30 * time.Second
	hyperVActionTimeout = 60 * time.Second
)

// RunHyperVCommand is the function used to run PowerShell commands for Hyper-V.
// Overridable for tests.
var RunHyperVCommand = securityruntime.CommandContextCombinedOutput

// HyperVVM holds information about a single Hyper-V virtual machine.
type HyperVVM struct {
	Name             string `json:"name"`
	State            string `json:"state"`
	CPUUsage         int    `json:"cpu_usage"`
	MemoryAssignedMB int64  `json:"memory_assigned_mb"`
	Generation       int    `json:"generation"`
	Uptime           string `json:"uptime"`
	VMId             string `json:"vm_id"`
}

// HyperVBackend manages Hyper-V virtual machines via PowerShell.
// It has no build tags so that parser tests can run on any platform.
type HyperVBackend struct{}

// ListVMs returns all Hyper-V VMs on the local host via `Get-VM | ConvertTo-Json`.
func (HyperVBackend) ListVMs() ([]HyperVVM, error) {
	ctx, cancel := context.WithTimeout(context.Background(), hyperVListTimeout)
	defer cancel()

	out, err := RunHyperVCommand(ctx, "powershell.exe",
		"-NonInteractive", "-NoProfile", "-Command",
		"Get-VM | ConvertTo-Json -Depth 3")
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("Get-VM timed out")
		}
		trimmed := strings.TrimSpace(string(out))
		if trimmed != "" {
			return nil, fmt.Errorf("Get-VM failed: %s", trimmed)
		}
		return nil, fmt.Errorf("Get-VM failed: %w", err)
	}

	return parseGetVMOutput(out)
}

// PerformVMAction runs a lifecycle action against the named VM.
// Supported actions: "start", "stop", "restart", "checkpoint".
func (HyperVBackend) PerformVMAction(action, vmName string) error {
	if strings.TrimSpace(vmName) == "" {
		return fmt.Errorf("PerformVMAction: vmName must not be empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), hyperVActionTimeout)
	defer cancel()

	var psCmd string
	switch strings.ToLower(action) {
	case "start":
		psCmd = fmt.Sprintf("Start-VM -Name %q", vmName)
	case "stop":
		psCmd = fmt.Sprintf("Stop-VM -Name %q -Force", vmName)
	case "restart":
		psCmd = fmt.Sprintf("Restart-VM -Name %q -Force", vmName)
	case "checkpoint":
		psCmd = fmt.Sprintf("Checkpoint-VM -Name %q", vmName)
	default:
		return fmt.Errorf("PerformVMAction: unsupported action %q", action)
	}

	out, err := RunHyperVCommand(ctx, "powershell.exe",
		"-NonInteractive", "-NoProfile", "-Command", psCmd)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("VM action %q on %q timed out", action, vmName)
		}
		trimmed := strings.TrimSpace(string(out))
		if trimmed != "" {
			return fmt.Errorf("VM action %q on %q failed: %s", action, vmName, trimmed)
		}
		return fmt.Errorf("VM action %q on %q failed: %w", action, vmName, err)
	}

	return nil
}

// vmUptimeJSON is the nested Uptime shape in Get-VM | ConvertTo-Json output.
type vmUptimeJSON struct {
	Days         int     `json:"Days"`
	Hours        int     `json:"Hours"`
	Minutes      int     `json:"Minutes"`
	Seconds      int     `json:"Seconds"`
	TotalSeconds float64 `json:"TotalSeconds"`
}

// vmJSON is the JSON shape emitted by Get-VM | ConvertTo-Json.
type vmJSON struct {
	Name           string       `json:"Name"`
	State          int          `json:"State"`
	CPUUsage       int          `json:"CPUUsage"`
	MemoryAssigned int64        `json:"MemoryAssigned"`
	MemoryStartup  int64        `json:"MemoryStartup"`
	Generation     int          `json:"Generation"`
	Uptime         vmUptimeJSON `json:"Uptime"`
	Status         string       `json:"Status"`
	Path           string       `json:"Path"`
	VMId           string       `json:"VMId"`
}

// hyperVStateString converts the integer State field from Get-VM into a
// human-readable string. The numeric values match the Hyper-V VMState enum:
// Running=2, Off=3, Saved=6, Paused=9.
func hyperVStateString(state int) string {
	switch state {
	case 2:
		return "Running"
	case 3:
		return "Off"
	case 6:
		return "Saved"
	case 9:
		return "Paused"
	default:
		return fmt.Sprintf("Unknown(%d)", state)
	}
}

// formatVMUptime formats a vmUptimeJSON into a compact human-readable string
// of the form "3d 14h 22m 7s", omitting leading zero components.
func formatVMUptime(u vmUptimeJSON) string {
	if u.TotalSeconds == 0 {
		return "0s"
	}
	parts := []string{}
	if u.Days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", u.Days))
	}
	if u.Hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", u.Hours))
	}
	if u.Minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", u.Minutes))
	}
	if u.Seconds > 0 {
		parts = append(parts, fmt.Sprintf("%ds", u.Seconds))
	}
	if len(parts) == 0 {
		return "0s"
	}
	return strings.Join(parts, " ")
}

// parseGetVMOutput parses the JSON emitted by `Get-VM | ConvertTo-Json`.
// PowerShell emits a single JSON object when only one VM exists; both
// the array and single-object forms are handled.
func parseGetVMOutput(raw []byte) ([]HyperVVM, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, nil
	}

	// Strip UTF-8 BOM emitted by some PowerShell versions.
	trimmed = strings.TrimPrefix(trimmed, "\xef\xbb\xbf")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return nil, nil
	}

	data := []byte(trimmed)

	var arr []vmJSON
	if err := json.Unmarshal(data, &arr); err != nil {
		// Fall back to single-object parse (one VM on the host).
		var single vmJSON
		if err2 := json.Unmarshal(data, &single); err2 != nil {
			return nil, fmt.Errorf("parseGetVMOutput: %w", err2)
		}
		arr = []vmJSON{single}
	}

	vms := make([]HyperVVM, 0, len(arr))
	for _, v := range arr {
		name := strings.TrimSpace(v.Name)
		if name == "" {
			continue
		}
		vms = append(vms, HyperVVM{
			Name:             name,
			State:            hyperVStateString(v.State),
			CPUUsage:         v.CPUUsage,
			MemoryAssignedMB: v.MemoryAssigned / (1024 * 1024),
			Generation:       v.Generation,
			Uptime:           formatVMUptime(v.Uptime),
			VMId:             strings.TrimSpace(v.VMId),
		})
	}

	return vms, nil
}
