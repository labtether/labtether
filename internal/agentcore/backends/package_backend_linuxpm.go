package backends

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/securityruntime"
)

// LinuxPackageBackend implements PackageBackend using dpkg/rpm and apt/dnf/yum/etc.
type LinuxPackageBackend struct{}

// PackageActionCommand represents a single package manager command to run.
type PackageActionCommand struct {
	Name string
	Args []string
}

var (
	// LinuxPackageLookPath is the function used to find package managers. Overridable for tests.
	LinuxPackageLookPath = exec.LookPath
	// LinuxPackageDpkgLister collects packages via dpkg. Overridable for tests.
	LinuxPackageDpkgLister = collectLinuxPackagesDpkg
	// LinuxPackageRPMLister collects packages via rpm. Overridable for tests.
	LinuxPackageRPMLister = collectLinuxPackagesRPM
	// DetectLinuxPackageManagerFn detects the available package manager. Overridable for tests.
	DetectLinuxPackageManagerFn = DetectLinuxPackageManager
	// BuildLinuxPackageActionCommandsFn builds the command list for a package action. Overridable for tests.
	BuildLinuxPackageActionCommandsFn = BuildLinuxPackageActionCommands
	// RunLinuxPackageCommand runs a package command. Overridable for tests.
	RunLinuxPackageCommand = securityruntime.CommandContextCombinedOutput
	// DetectLinuxRebootRequiredFn checks if a reboot is required. Overridable for tests.
	DetectLinuxRebootRequiredFn = DetectLinuxRebootRequired
	// LinuxPackageStat is the function used to stat files. Overridable for tests.
	LinuxPackageStat = os.Stat
	// NewLinuxPackageCommand creates a new command. Overridable for tests.
	NewLinuxPackageCommand = securityruntime.NewCommand
)

// ListPackages lists installed Linux packages using dpkg or rpm.
func (LinuxPackageBackend) ListPackages() ([]agentmgr.PackageInfo, error) {
	if path, err := LinuxPackageLookPath("dpkg-query"); err == nil && path != "" {
		return LinuxPackageDpkgLister()
	}
	if path, err := LinuxPackageLookPath("rpm"); err == nil && path != "" {
		return LinuxPackageRPMLister()
	}
	return nil, ErrNoLinuxPackageManager
}

// PerformAction performs a Linux package action (install, remove, upgrade).
func (LinuxPackageBackend) PerformAction(action string, packages []string) (PackageActionResult, error) {
	pkgManager, err := DetectLinuxPackageManagerFn()
	if err != nil {
		return PackageActionResult{}, err
	}
	commands, err := BuildLinuxPackageActionCommandsFn(pkgManager, action, packages)
	if err != nil {
		return PackageActionResult{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), PackageActionCommandTimeout)
	defer cancel()

	var combined bytes.Buffer
	for _, command := range commands {
		out, runErr := RunLinuxPackageCommand(ctx, command.Name, command.Args...)
		if combined.Len() > 0 && len(out) > 0 {
			combined.WriteByte('\n')
		}
		combined.Write(out)
		result := PackageActionResult{
			Output:         TruncateCommandOutput(combined.Bytes(), MaxCommandOutputBytes),
			RebootRequired: DetectLinuxRebootRequiredFn(),
		}
		if runErr != nil {
			if errors.Is(runErr, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded {
				return result, fmt.Errorf("package action timed out")
			}
			return result, runErr
		}
	}

	return PackageActionResult{
		Output:         TruncateCommandOutput(combined.Bytes(), MaxCommandOutputBytes),
		RebootRequired: DetectLinuxRebootRequiredFn(),
	}, nil
}

// BuildLinuxPackageActionCommands builds the list of commands for a package action.
func BuildLinuxPackageActionCommands(manager, action string, packages []string) ([]PackageActionCommand, error) {
	args, err := buildLinuxPackageActionArgs(manager, action, packages)
	if err != nil {
		return nil, err
	}

	commands := []PackageActionCommand{{
		Name: manager,
		Args: args,
	}}
	if manager == "apt-get" && (action == "install" || action == "upgrade") {
		commands = append([]PackageActionCommand{{
			Name: manager,
			Args: []string{"update"},
		}}, commands...)
	}
	return commands, nil
}

// DetectLinuxPackageManager detects the available Linux package manager.
func DetectLinuxPackageManager() (string, error) {
	for _, candidate := range []string{"apt-get", "dnf", "yum", "zypper", "pacman", "apk"} {
		if path, err := LinuxPackageLookPath(candidate); err == nil && path != "" {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no supported package manager found")
}

func buildLinuxPackageActionArgs(manager, action string, packages []string) ([]string, error) {
	switch manager {
	case "apt-get":
		switch action {
		case "install":
			return append([]string{"-y", "install"}, packages...), nil
		case "remove":
			return append([]string{"-y", "remove"}, packages...), nil
		case "upgrade":
			if len(packages) > 0 {
				return append([]string{"-y", "install", "--only-upgrade"}, packages...), nil
			}
			return []string{"-y", "upgrade"}, nil
		}
	case "dnf", "yum":
		switch action {
		case "install":
			return append([]string{"-y", "install"}, packages...), nil
		case "remove":
			return append([]string{"-y", "remove"}, packages...), nil
		case "upgrade":
			if len(packages) > 0 {
				return append([]string{"-y", "upgrade"}, packages...), nil
			}
			return []string{"-y", "upgrade"}, nil
		}
	case "zypper":
		switch action {
		case "install":
			return append([]string{"--non-interactive", "install"}, packages...), nil
		case "remove":
			return append([]string{"--non-interactive", "remove"}, packages...), nil
		case "upgrade":
			if len(packages) > 0 {
				return append([]string{"--non-interactive", "update"}, packages...), nil
			}
			return []string{"--non-interactive", "update"}, nil
		}
	case "pacman":
		switch action {
		case "install":
			return append([]string{"--noconfirm", "-S"}, packages...), nil
		case "remove":
			return append([]string{"--noconfirm", "-R"}, packages...), nil
		case "upgrade":
			if len(packages) > 0 {
				return append([]string{"--noconfirm", "-S"}, packages...), nil
			}
			return []string{"--noconfirm", "-Syu"}, nil
		}
	case "apk":
		switch action {
		case "install":
			return append([]string{"add"}, packages...), nil
		case "remove":
			return append([]string{"del"}, packages...), nil
		case "upgrade":
			if len(packages) > 0 {
				return append([]string{"add", "--upgrade"}, packages...), nil
			}
			return []string{"upgrade"}, nil
		}
	}
	return nil, fmt.Errorf("unsupported package action for %s", manager)
}

// DetectLinuxRebootRequired checks if a reboot is required on Linux.
func DetectLinuxRebootRequired() bool {
	if _, err := LinuxPackageStat("/run/reboot-required"); err == nil {
		return true
	}
	if _, err := LinuxPackageStat("/var/run/reboot-required"); err == nil {
		return true
	}

	needsRestartPath, err := LinuxPackageLookPath("needs-restarting")
	if err == nil && needsRestartPath != "" {
		// Exit code 1 indicates reboot required.
		cmd, cmdErr := NewLinuxPackageCommand(needsRestartPath, "-r")
		if cmdErr != nil {
			return false
		}
		if runErr := cmd.Run(); runErr != nil {
			if exitErr, ok := runErr.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
				return true
			}
		}
	}

	return false
}

// ErrNoLinuxPackageManager is returned when no supported package manager is found.
var ErrNoLinuxPackageManager = &PackageError{Msg: "no supported package manager"}

// PackageError represents a package manager error.
type PackageError struct {
	Msg string
}

func (e *PackageError) Error() string { return e.Msg }

func collectLinuxPackagesDpkg() ([]agentmgr.PackageInfo, error) {
	out, err := securityruntime.CommandCombinedOutput("dpkg-query", "-W", "-f", "${Package}\t${Version}\t${Status}\n")
	if err != nil {
		return nil, err
	}

	var pkgs []agentmgr.PackageInfo
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) < 3 {
			continue
		}

		// Status field from dpkg is like "install ok installed" — extract last word.
		statusParts := strings.Fields(fields[2])
		status := fields[2]
		if len(statusParts) > 0 {
			status = statusParts[len(statusParts)-1]
		}

		pkgs = append(pkgs, agentmgr.PackageInfo{
			Name:    fields[0],
			Version: fields[1],
			Status:  status,
		})
	}

	return pkgs, nil
}

func collectLinuxPackagesRPM() ([]agentmgr.PackageInfo, error) {
	out, err := securityruntime.CommandCombinedOutput("rpm", "-qa", "--queryformat", "%{NAME}\t%{VERSION}-%{RELEASE}\tinstalled\n")
	if err != nil {
		return nil, err
	}

	var pkgs []agentmgr.PackageInfo
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) < 3 {
			continue
		}
		pkgs = append(pkgs, agentmgr.PackageInfo{
			Name:    fields[0],
			Version: fields[1],
			Status:  fields[2],
		})
	}

	return pkgs, nil
}
