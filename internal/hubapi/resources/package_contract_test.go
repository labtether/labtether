package resources

import (
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
)

func TestValidatePackageListedResponseAcceptsInstalledCompatibilityAndUpgradableContract(t *testing.T) {
	t.Run("legacy installed", func(t *testing.T) {
		err := validatePackageListedResponse(packageListedResponseWire{
			RequestID: "req-installed",
			Packages: []packageInfoWire{{
				Name:    "jq",
				Version: "1.7",
				Status:  "installed",
			}},
		}, "req-installed", packageInventoryInstalled)
		if err != nil {
			t.Fatalf("legacy installed response rejected: %v", err)
		}
	})

	t.Run("installed registry entry without version", func(t *testing.T) {
		err := validatePackageListedResponse(packageListedResponseWire{
			RequestID: "req-installed",
			Packages:  []packageInfoWire{{Name: "Vendor Utility", Status: "installed"}},
		}, "req-installed", packageInventoryInstalled)
		if err != nil {
			t.Fatalf("installed package without registry version rejected: %v", err)
		}
	})

	t.Run("upgradable", func(t *testing.T) {
		err := validatePackageListedResponse(packageListedResponseWire{
			RequestID: "req-upgradable",
			Inventory: packageInventoryUpgradable,
			Packages: []packageInfoWire{{
				Name:             "curl",
				Version:          "8.5.0",
				AvailableVersion: "8.6.0",
				Status:           packageInventoryUpgradable,
			}},
		}, "req-upgradable", packageInventoryUpgradable)
		if err != nil {
			t.Fatalf("upgradable response rejected: %v", err)
		}
	})
}

func TestValidatePackageActionResultBoundsAndCorrelation(t *testing.T) {
	valid := agentmgr.PackageResultData{RequestID: "req-action", OK: true, Output: "updated"}
	if err := validatePackageActionResult(valid, "req-action"); err != nil {
		t.Fatalf("valid result rejected: %v", err)
	}
	for _, result := range []agentmgr.PackageResultData{
		{RequestID: "other", OK: true},
		{RequestID: "req-action", Output: strings.Repeat("x", maxPackageActionOutputBytes+1)},
		{RequestID: "req-action", Error: strings.Repeat("x", maxPackageInventoryErrorBytes+1)},
	} {
		if err := validatePackageActionResult(result, "req-action"); err == nil {
			t.Fatalf("malformed action result accepted: %+v", result)
		}
	}
}

func TestValidatePackageListedResponseRejectsMalformedAndOversized(t *testing.T) {
	base := packageListedResponseWire{
		RequestID: "req-upgradable",
		Inventory: packageInventoryUpgradable,
		Packages: []packageInfoWire{{
			Name:             "curl",
			Version:          "8.5.0",
			AvailableVersion: "8.6.0",
			Status:           packageInventoryUpgradable,
		}},
	}
	tests := []struct {
		name     string
		mutate   func(*packageListedResponseWire)
		contains string
	}{
		{name: "request mismatch", mutate: func(r *packageListedResponseWire) { r.RequestID = "other" }, contains: "request id mismatch"},
		{name: "missing discriminator", mutate: func(r *packageListedResponseWire) { r.Inventory = "" }, contains: "does not support"},
		{name: "unknown discriminator", mutate: func(r *packageListedResponseWire) { r.Inventory = "updates" }, contains: "does not support"},
		{name: "missing target", mutate: func(r *packageListedResponseWire) { r.Packages[0].AvailableVersion = "" }, contains: "incomplete"},
		{name: "wrong status", mutate: func(r *packageListedResponseWire) { r.Packages[0].Status = "installed" }, contains: "incomplete"},
		{name: "unsafe package", mutate: func(r *packageListedResponseWire) { r.Packages[0].Name = "--source" }, contains: "not actionable"},
		{name: "control character", mutate: func(r *packageListedResponseWire) { r.Packages[0].Version = "1\n2" }, contains: "control"},
		{name: "long version", mutate: func(r *packageListedResponseWire) {
			r.Packages[0].Version = strings.Repeat("1", maxPackageInventoryVersionBytes+1)
		}, contains: "exceeds"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := base
			response.Packages = append([]packageInfoWire(nil), base.Packages...)
			test.mutate(&response)
			err := validatePackageListedResponse(response, "req-upgradable", packageInventoryUpgradable)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("error = %v, want substring %q", err, test.contains)
			}
		})
	}

	oversized := base
	oversized.Packages = make([]packageInfoWire, maxPackageInventoryItems+1)
	if err := validatePackageListedResponse(oversized, "req-upgradable", packageInventoryUpgradable); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized inventory error = %v", err)
	}
}
