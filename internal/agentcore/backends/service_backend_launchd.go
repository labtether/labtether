package backends

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/securityruntime"
)

const darwinServiceActionTimeout = 30 * time.Second

// DarwinServiceBackend implements ServiceBackend using launchctl.
type DarwinServiceBackend struct{}

// ListServices lists launchd services.
func (DarwinServiceBackend) ListServices() ([]agentmgr.ServiceInfo, error) {
	if _, err := exec.LookPath("launchctl"); err != nil {
		return nil, fmt.Errorf("launchctl is not available on this host")
	}

	ctx, cancel := context.WithTimeout(context.Background(), linuxServiceListTimeout)
	defer cancel()

	out, err := securityruntime.CommandContextCombinedOutput(ctx, "launchctl", "list")
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("launchctl list timed out")
		}
		return nil, fmt.Errorf("launchctl list: %w", err)
	}

	services := ParseLaunchctlListOutput(string(out))
	sort.Slice(services, func(i, j int) bool {
		return services[i].Name < services[j].Name
	})
	return services, nil
}

// PerformAction performs a launchctl action on a named service.
func (DarwinServiceBackend) PerformAction(action, service string) (string, error) {
	if _, err := exec.LookPath("launchctl"); err != nil {
		return "", fmt.Errorf("launchctl is not available on this host")
	}

	candidates := BuildLaunchctlActionCandidates(action, service)
	return runLaunchctlActionCandidates(candidates)
}

// ParseLaunchctlListOutput parses the output of `launchctl list`.
func ParseLaunchctlListOutput(raw string) []agentmgr.ServiceInfo {
	var services []agentmgr.ServiceInfo
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "PID") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		pid := strings.TrimSpace(fields[0])
		status := strings.TrimSpace(fields[1])
		label := strings.TrimSpace(fields[2])
		if label == "" {
			continue
		}

		activeState := "inactive"
		subState := "stopped"
		if pid != "-" && pid != "0" {
			activeState = "active"
			subState = "running"
		} else if status != "0" && status != "-" {
			subState = "failed"
		}

		services = append(services, agentmgr.ServiceInfo{
			Name:        label,
			Description: label,
			ActiveState: activeState,
			SubState:    subState,
			Enabled:     "unknown",
			LoadState:   "loaded",
		})
	}
	return services
}

// BuildLaunchctlActionCandidates builds the launchctl command candidates.
func BuildLaunchctlActionCandidates(action, service string) [][]string {
	targets := launchdTargets(service)
	candidates := make([][]string, 0, len(targets)+1)

	switch action {
	case "start":
		for _, target := range targets {
			candidates = append(candidates, []string{"kickstart", "-k", target})
		}
		candidates = append(candidates, []string{"start", service})
	case "restart":
		for _, target := range targets {
			candidates = append(candidates, []string{"kickstart", "-k", target})
		}
		candidates = append(candidates, []string{"stop", service}, []string{"start", service})
	case "stop":
		candidates = append(candidates, []string{"stop", service})
		for _, target := range targets {
			candidates = append(candidates, []string{"kill", "SIGTERM", target})
		}
	case "enable":
		for _, target := range targets {
			candidates = append(candidates, []string{"enable", target})
		}
	case "disable":
		for _, target := range targets {
			candidates = append(candidates, []string{"disable", target})
		}
	}

	return candidates
}

func launchdTargets(service string) []string {
	uid := os.Getuid()
	return []string{
		fmt.Sprintf("gui/%d/%s", uid, service),
		fmt.Sprintf("user/%d/%s", uid, service),
		fmt.Sprintf("system/%s", service),
	}
}

func runLaunchctlActionCandidates(candidates [][]string) (string, error) {
	if len(candidates) == 0 {
		return "", fmt.Errorf("no launchctl command candidates available")
	}

	failures := make([]string, 0, len(candidates))
	lastOutput := ""
	for _, args := range candidates {
		output, err := runLaunchctl(args...)
		if strings.TrimSpace(output) != "" {
			lastOutput = output
		}
		if err == nil {
			return output, nil
		}
		failures = append(failures, fmt.Sprintf("launchctl %s: %v", strings.Join(args, " "), err))
	}

	return lastOutput, errors.New(strings.Join(failures, "; "))
}

func runLaunchctl(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), darwinServiceActionTimeout)
	defer cancel()

	out, err := securityruntime.CommandContextCombinedOutput(ctx, "launchctl", args...)
	output := strings.TrimSpace(string(out))
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return output, fmt.Errorf("launchctl timed out")
		}
		if output == "" {
			return output, err
		}
		return output, fmt.Errorf("%s: %w", output, err)
	}
	return output, nil
}
