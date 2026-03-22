package windows

import (
	"os/exec"
	"testing"
)

func TestCapabilityMetadata_AllToolsPresent(t *testing.T) {
	lookPath := func(name string) (string, error) {
		return "/usr/bin/" + name, nil
	}
	meta := readCapabilityMetadataWith(lookPath)

	// Services always available on Windows
	if meta["cap_services"] != "list,action" {
		t.Errorf("expected cap_services=list,action, got %s", meta["cap_services"])
	}
	if meta["service_backend"] != "scm" {
		t.Errorf("expected service_backend=scm, got %s", meta["service_backend"])
	}
	// WinGet detected
	if meta["cap_packages"] != "list,action" {
		t.Errorf("expected cap_packages=list,action, got %s", meta["cap_packages"])
	}
	if meta["package_backend"] != "winget" {
		t.Errorf("expected package_backend=winget, got %s", meta["package_backend"])
	}
	// Logs always available
	if meta["cap_logs"] != "stored,query,stream" {
		t.Errorf("expected cap_logs=stored,query,stream, got %s", meta["cap_logs"])
	}
}

func TestCapabilityMetadata_NoWinGetHasChoco(t *testing.T) {
	lookPath := func(name string) (string, error) {
		if name == "winget" || name == "winget.exe" {
			return "", exec.ErrNotFound
		}
		return "/usr/bin/" + name, nil
	}
	meta := readCapabilityMetadataWith(lookPath)
	if meta["package_backend"] != "choco" {
		t.Errorf("expected package_backend=choco, got %s", meta["package_backend"])
	}
}

func TestCapabilityMetadata_NoPackageManagers(t *testing.T) {
	lookPath := func(name string) (string, error) {
		if name == "winget" || name == "winget.exe" || name == "choco" || name == "choco.exe" {
			return "", exec.ErrNotFound
		}
		return "/usr/bin/" + name, nil
	}
	meta := readCapabilityMetadataWith(lookPath)
	if meta["cap_packages"] != "" {
		t.Errorf("expected empty cap_packages, got %s", meta["cap_packages"])
	}
	if meta["package_backend"] != "none" {
		t.Errorf("expected package_backend=none, got %s", meta["package_backend"])
	}
}
