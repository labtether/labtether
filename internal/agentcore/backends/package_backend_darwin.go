package backends

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/securityruntime"
)

const darwinPackageListTimeout = 45 * time.Second

// DarwinPackageBackend implements PackageBackend using Homebrew.
type DarwinPackageBackend struct{}

// ListPackages lists installed Homebrew packages.
func (DarwinPackageBackend) ListPackages() ([]agentmgr.PackageInfo, error) {
	if _, err := exec.LookPath("brew"); err != nil {
		return nil, fmt.Errorf("brew is not available on this host")
	}

	ctx, cancel := context.WithTimeout(context.Background(), darwinPackageListTimeout)
	defer cancel()

	out, err := securityruntime.CommandContextCombinedOutput(ctx, "brew", "info", "--json=v2", "--installed")
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("brew package listing timed out")
		}
		trimmed := strings.TrimSpace(string(out))
		if trimmed != "" {
			return nil, fmt.Errorf("brew package listing failed: %s", trimmed)
		}
		return nil, fmt.Errorf("brew package listing failed: %w", err)
	}

	packages, parseErr := ParseBrewInstalledPackages(out)
	if parseErr != nil {
		return nil, parseErr
	}
	return packages, nil
}

// PerformAction performs a Homebrew package action (install, remove, upgrade).
func (DarwinPackageBackend) PerformAction(action string, packages []string) (PackageActionResult, error) {
	if _, err := exec.LookPath("brew"); err != nil {
		return PackageActionResult{}, fmt.Errorf("brew is not available on this host")
	}

	args, err := BuildDarwinPackageActionArgs(action, packages)
	if err != nil {
		return PackageActionResult{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), PackageActionCommandTimeout)
	defer cancel()

	out, runErr := securityruntime.CommandContextCombinedOutput(ctx, "brew", args...)
	result := PackageActionResult{
		Output:         TruncateCommandOutput(out, MaxCommandOutputBytes),
		RebootRequired: false,
	}
	if runErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return result, fmt.Errorf("package action timed out")
		}
		return result, runErr
	}

	return result, nil
}

// BuildDarwinPackageActionArgs builds the Homebrew arguments for a package action.
func BuildDarwinPackageActionArgs(action string, packages []string) ([]string, error) {
	switch action {
	case "install":
		return append([]string{"install"}, packages...), nil
	case "remove":
		return append([]string{"uninstall"}, packages...), nil
	case "upgrade":
		if len(packages) > 0 {
			return append([]string{"upgrade"}, packages...), nil
		}
		return []string{"upgrade"}, nil
	default:
		return nil, fmt.Errorf("unsupported package action %q", action)
	}
}

type brewInstalledJSON struct {
	Formulae []brewFormulaInfo `json:"formulae"`
	Casks    []brewCaskInfo    `json:"casks"`
}

type brewFormulaInfo struct {
	Name      string `json:"name"`
	FullName  string `json:"full_name"`
	Installed []struct {
		Version string `json:"version"`
	} `json:"installed"`
	Versions struct {
		Stable string `json:"stable"`
	} `json:"versions"`
}

type brewCaskInfo struct {
	Token     string                    `json:"token"`
	FullToken string                    `json:"full_token"`
	Version   string                    `json:"version"`
	Installed brewCaskInstalledVersions `json:"installed"`
}

type brewCaskInstalledVersions []string

func (v *brewCaskInstalledVersions) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		*v = nil
		return nil
	}

	var array []string
	if err := json.Unmarshal(data, &array); err == nil {
		*v = array
		return nil
	}

	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		single = strings.TrimSpace(single)
		if single == "" {
			*v = nil
			return nil
		}
		*v = []string{single}
		return nil
	}

	return fmt.Errorf("unsupported brew cask installed field format")
}

// ParseBrewInstalledPackages parses the output of `brew info --json=v2 --installed`.
func ParseBrewInstalledPackages(raw []byte) ([]agentmgr.PackageInfo, error) {
	var payload brewInstalledJSON
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse brew package output: %w", err)
	}

	packages := make([]agentmgr.PackageInfo, 0, len(payload.Formulae)+len(payload.Casks))
	seen := make(map[string]struct{}, len(payload.Formulae)+len(payload.Casks))

	for _, formula := range payload.Formulae {
		name := strings.TrimSpace(formula.Name)
		if name == "" {
			name = strings.TrimSpace(formula.FullName)
		}
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}

		version := strings.TrimSpace(formula.Versions.Stable)
		for _, installed := range formula.Installed {
			if v := strings.TrimSpace(installed.Version); v != "" {
				version = v
				break
			}
		}

		packages = append(packages, agentmgr.PackageInfo{
			Name:    name,
			Version: version,
			Status:  "installed",
		})
	}

	for _, cask := range payload.Casks {
		name := strings.TrimSpace(cask.Token)
		if name == "" {
			name = strings.TrimSpace(cask.FullToken)
		}
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}

		version := strings.TrimSpace(cask.Version)
		for _, installed := range cask.Installed {
			if v := strings.TrimSpace(installed); v != "" {
				version = v
				break
			}
		}

		packages = append(packages, agentmgr.PackageInfo{
			Name:    name,
			Version: version,
			Status:  "installed",
		})
	}

	sort.Slice(packages, func(i, j int) bool {
		return packages[i].Name < packages[j].Name
	})

	return packages, nil
}
