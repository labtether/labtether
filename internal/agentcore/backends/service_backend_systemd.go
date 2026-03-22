package backends

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/securityruntime"
)

const (
	linuxServiceActionTimeout = 30 * time.Second
	linuxServiceListTimeout   = 15 * time.Second
)

// LinuxServiceBackend implements ServiceBackend using systemd.
type LinuxServiceBackend struct{}

// RunLinuxServiceCommand is the function used to run systemctl commands. Overridable for tests.
var RunLinuxServiceCommand = securityruntime.CommandContextCombinedOutput

// ListServices lists systemd services.
func (LinuxServiceBackend) ListServices() ([]agentmgr.ServiceInfo, error) {
	enabledMap, err := collectEnabledStatesLinux()
	if err != nil {
		enabledMap = map[string]string{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), linuxServiceListTimeout)
	defer cancel()

	out, err := RunLinuxServiceCommand(
		ctx,
		"systemctl", "list-units",
		"--type=service", "--all",
		"--no-pager", "--plain", "--no-legend",
	)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("systemctl list-units timed out")
		}
		return nil, fmt.Errorf("systemctl list-units: %w", err)
	}

	var services []agentmgr.ServiceInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// UNIT LOAD ACTIVE SUB DESCRIPTION...
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		unit := fields[0]
		loadState := fields[1]
		activeState := fields[2]
		subState := fields[3]
		description := ""
		if len(fields) > 4 {
			description = strings.Join(fields[4:], " ")
		}
		name := strings.TrimSuffix(unit, ".service")

		services = append(services, agentmgr.ServiceInfo{
			Name:        name,
			Description: description,
			ActiveState: activeState,
			SubState:    subState,
			Enabled:     enabledMap[unit],
			LoadState:   loadState,
		})
	}

	return services, nil
}

// PerformAction performs a systemctl action on a named service.
func (LinuxServiceBackend) PerformAction(action, service string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), linuxServiceActionTimeout)
	defer cancel()

	out, err := RunLinuxServiceCommand(ctx, "systemctl", action, service)
	output := strings.TrimSpace(string(out))
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded {
			return output, fmt.Errorf("systemctl timed out")
		}
		return output, err
	}
	return output, nil
}

func collectEnabledStatesLinux() (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := RunLinuxServiceCommand(
		ctx,
		"systemctl", "list-unit-files",
		"--type=service",
		"--no-pager", "--plain", "--no-legend",
	)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("systemctl list-unit-files timed out")
		}
		return nil, fmt.Errorf("systemctl list-unit-files: %w", err)
	}

	result := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		result[fields[0]] = fields[1]
	}
	return result, nil
}
