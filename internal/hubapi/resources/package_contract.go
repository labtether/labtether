package resources

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/labtether/labtether/internal/agentmgr"
)

const (
	packageInventoryInstalled  = "installed"
	packageInventoryUpgradable = "upgradable"

	maxPackageInventoryItems        = 10_000
	maxPackageInventoryNameBytes    = 1_024
	maxPackageInventoryVersionBytes = 1_024
	maxPackageInventoryStatusBytes  = 256
	maxPackageInventoryErrorBytes   = 4_096
	maxPackageRequestIDBytes        = 512
	maxPackageActionOutputBytes     = 128 * 1024
)

// These additive wire fields mirror the protocol repository's package
// contract while the hub remains pinned to the last published protocol tag.
type packageListRequestWire struct {
	RequestID string `json:"request_id"`
	Inventory string `json:"inventory,omitempty"`
}

type packageInfoWire struct {
	Name             string `json:"name"`
	Version          string `json:"version"`
	AvailableVersion string `json:"available_version,omitempty"`
	Status           string `json:"status"`
}

type packageListedResponseWire struct {
	RequestID string            `json:"request_id"`
	Inventory string            `json:"inventory,omitempty"`
	Packages  []packageInfoWire `json:"packages"`
	Error     string            `json:"error,omitempty"`
}

func validatePackageActionResult(result agentmgr.PackageResultData, requestID string) error {
	if strings.TrimSpace(result.RequestID) != requestID {
		return fmt.Errorf("package response request id mismatch")
	}
	if !utf8.ValidString(result.Output) || len(result.Output) > maxPackageActionOutputBytes {
		return fmt.Errorf("package action output is malformed or exceeds %d bytes", maxPackageActionOutputBytes)
	}
	if !utf8.ValidString(result.Error) || len(result.Error) > maxPackageInventoryErrorBytes {
		return fmt.Errorf("package action error is malformed or exceeds %d bytes", maxPackageInventoryErrorBytes)
	}
	return nil
}

func validatePackageListedResponse(response packageListedResponseWire, requestID, expectedInventory string) error {
	if strings.TrimSpace(response.RequestID) != requestID {
		return fmt.Errorf("package response request id mismatch")
	}
	inventory := strings.ToLower(strings.TrimSpace(response.Inventory))
	switch expectedInventory {
	case packageInventoryUpgradable:
		if inventory != packageInventoryUpgradable {
			return fmt.Errorf("agent does not support upgradable package inventory")
		}
	default:
		if inventory != "" && inventory != packageInventoryInstalled {
			return fmt.Errorf("unexpected package inventory %q", inventory)
		}
	}
	if len(response.Error) > maxPackageInventoryErrorBytes || !utf8.ValidString(response.Error) {
		return fmt.Errorf("package response error is malformed")
	}
	if len(response.Packages) > maxPackageInventoryItems {
		return fmt.Errorf("package inventory exceeds %d entries", maxPackageInventoryItems)
	}
	for index, pkg := range response.Packages {
		if err := validatePackageInventoryField(pkg.Name, maxPackageInventoryNameBytes, true); err != nil {
			return fmt.Errorf("package entry %d name: %w", index+1, err)
		}
		requireUpdateFields := expectedInventory == packageInventoryUpgradable
		if err := validatePackageInventoryField(pkg.Version, maxPackageInventoryVersionBytes, requireUpdateFields); err != nil {
			return fmt.Errorf("package entry %d version: %w", index+1, err)
		}
		if err := validatePackageInventoryField(pkg.Status, maxPackageInventoryStatusBytes, requireUpdateFields); err != nil {
			return fmt.Errorf("package entry %d status: %w", index+1, err)
		}
		if pkg.AvailableVersion != "" {
			if err := validatePackageInventoryField(pkg.AvailableVersion, maxPackageInventoryVersionBytes, true); err != nil {
				return fmt.Errorf("package entry %d available version: %w", index+1, err)
			}
		}
		if expectedInventory == packageInventoryUpgradable {
			if pkg.AvailableVersion == "" || pkg.Status != packageInventoryUpgradable {
				return fmt.Errorf("upgradable package entry %d is incomplete", index+1)
			}
			if _, err := normalizeAndValidatePackageTokens([]string{pkg.Name}); err != nil {
				return fmt.Errorf("upgradable package entry %d is not actionable", index+1)
			}
		}
	}
	return nil
}

func validatePackageInventoryField(value string, maxBytes int, required bool) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("is not valid UTF-8")
	}
	trimmed := strings.TrimSpace(value)
	if required && trimmed == "" {
		return fmt.Errorf("is required")
	}
	if len(value) > maxBytes {
		return fmt.Errorf("exceeds %d bytes", maxBytes)
	}
	if strings.IndexFunc(value, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return fmt.Errorf("contains a control character")
	}
	return nil
}
