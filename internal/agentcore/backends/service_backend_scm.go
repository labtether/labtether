package backends

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/securityruntime"
)

const (
	windowsServiceListTimeout   = 15 * time.Second
	windowsServiceActionTimeout = 30 * time.Second
)

// RunWindowsServiceCommand is the function used to run sc.exe commands. Overridable for tests.
var RunWindowsServiceCommand = securityruntime.CommandContextCombinedOutput

// WindowsServiceBackend implements ServiceBackend using sc.exe (Service Control Manager).
type WindowsServiceBackend struct{}

// ListServices lists Windows services via `sc.exe query type= service state= all`.
func (WindowsServiceBackend) ListServices() ([]agentmgr.ServiceInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), windowsServiceListTimeout)
	defer cancel()

	out, err := RunWindowsServiceCommand(ctx, "sc.exe", "query", "type=", "service", "state=", "all")
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("sc.exe query timed out")
		}
		return nil, fmt.Errorf("sc.exe query: %w", err)
	}

	return parseSCQueryOutput(string(out)), nil
}

// PerformAction performs a service action using sc.exe.
// Supported actions: start, stop, restart, enable, disable.
func (WindowsServiceBackend) PerformAction(action, service string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), windowsServiceActionTimeout)
	defer cancel()

	var args []string
	switch action {
	case "start":
		args = []string{"start", service}
	case "stop":
		args = []string{"stop", service}
	case "restart":
		// Stop first, then start. Ignore stop errors (service may already be stopped).
		stopCtx, stopCancel := context.WithTimeout(context.Background(), windowsServiceActionTimeout)
		defer stopCancel()
		_, _ = RunWindowsServiceCommand(stopCtx, "sc.exe", "stop", service)

		startCtx, startCancel := context.WithTimeout(context.Background(), windowsServiceActionTimeout)
		defer startCancel()
		out, err := RunWindowsServiceCommand(startCtx, "sc.exe", "start", service)
		output := strings.TrimSpace(string(out))
		if err != nil {
			if startCtx.Err() == context.DeadlineExceeded {
				return output, fmt.Errorf("sc.exe start timed out during restart")
			}
			return output, fmt.Errorf("sc.exe start (restart): %w", err)
		}
		return output, nil
	case "enable":
		args = []string{"config", service, "start=", "auto"}
	case "disable":
		args = []string{"config", service, "start=", "disabled"}
	default:
		return "", fmt.Errorf("unsupported service action: %s", action)
	}

	out, err := RunWindowsServiceCommand(ctx, "sc.exe", args...)
	output := strings.TrimSpace(string(out))
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return output, fmt.Errorf("sc.exe %s timed out", action)
		}
		return output, fmt.Errorf("sc.exe %s: %w", action, err)
	}
	return output, nil
}

// parseSCQueryOutput parses the text output of `sc.exe query type= service state= all`
// into a slice of ServiceInfo. No build tags — runs on all platforms.
func parseSCQueryOutput(raw string) []agentmgr.ServiceInfo {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	var services []agentmgr.ServiceInfo

	// Split on blank lines to get per-service blocks.
	// Windows line endings (\r\n) are normalised to \n first.
	normalised := strings.ReplaceAll(raw, "\r\n", "\n")
	blocks := strings.Split(normalised, "\n\n")

	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}

		var name, displayName, stateWord string

		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)

			if strings.HasPrefix(line, "SERVICE_NAME:") {
				name = strings.TrimSpace(strings.TrimPrefix(line, "SERVICE_NAME:"))
				continue
			}
			if strings.HasPrefix(line, "DISPLAY_NAME:") {
				displayName = strings.TrimSpace(strings.TrimPrefix(line, "DISPLAY_NAME:"))
				continue
			}
			// STATE line looks like:  STATE              : 4  RUNNING
			if strings.HasPrefix(line, "STATE") && strings.Contains(line, ":") {
				after := line[strings.Index(line, ":")+1:]
				after = strings.TrimSpace(after)
				// after is now "4  RUNNING" or "2  START_PENDING"
				fields := strings.Fields(after)
				if len(fields) >= 2 {
					stateWord = fields[1]
				} else if len(fields) == 1 {
					stateWord = fields[0]
				}
				continue
			}
		}

		if name == "" {
			continue
		}

		activeState, subState := parseSCState(stateWord)

		services = append(services, agentmgr.ServiceInfo{
			Name:        name,
			Description: displayName,
			ActiveState: activeState,
			SubState:    subState,
			Enabled:     "unknown",
			LoadState:   "loaded",
		})
	}

	return services
}

// parseSCState maps an sc.exe STATE word to (activeState, subState).
// activeState matches the systemd vocabulary used by the rest of the agent:
//
//	active, inactive, activating, deactivating, failed
func parseSCState(stateWord string) (activeState, subState string) {
	switch strings.ToUpper(stateWord) {
	case "RUNNING":
		return "active", "running"
	case "STOPPED":
		return "inactive", "dead"
	case "START_PENDING", "CONTINUE_PENDING":
		return "activating", "start-pending"
	case "STOP_PENDING", "PAUSE_PENDING":
		return "deactivating", "stop-pending"
	case "PAUSED":
		return "inactive", "paused"
	default:
		return "inactive", "unknown"
	}
}
