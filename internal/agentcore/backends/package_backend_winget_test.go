package backends

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseWinGetListOutput(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("testdata", "winget_list.txt"))
	if err != nil {
		t.Fatalf("failed to read testdata: %v", err)
	}

	pkgs, err := parseWinGetListOutput(raw)
	if err != nil {
		t.Fatalf("parseWinGetListOutput returned error: %v", err)
	}

	if len(pkgs) != 8 {
		t.Fatalf("expected 8 packages, got %d", len(pkgs))
	}
}

func TestParseWinGetListOutputFields(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("testdata", "winget_list.txt"))
	if err != nil {
		t.Fatalf("failed to read testdata: %v", err)
	}

	pkgs, err := parseWinGetListOutput(raw)
	if err != nil {
		t.Fatalf("parseWinGetListOutput returned error: %v", err)
	}

	tests := []struct {
		name      string
		id        string
		version   string
		available string
	}{
		{
			name:      "Microsoft Visual C++ 2015-2022 Redist",
			id:        "Microsoft.VCRedist.2015+.x64",
			version:   "14.38.33135",
			available: "",
		},
		{
			name:      "Google Chrome",
			id:        "Google.Chrome",
			version:   "120.0.6099.130",
			available: "121.0.6167.85",
		},
		{
			name:      "Git",
			id:        "Git.Git",
			version:   "2.43.0",
			available: "2.44.0",
		},
		{
			name:      "Windows Terminal",
			id:        "Microsoft.WindowsTerminal",
			version:   "1.19.10821.0",
			available: "",
		},
	}

	byID := make(map[string]wingetPackageRow, len(pkgs))
	for _, p := range pkgs {
		byID[p.id] = p
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			p, ok := byID[tc.id]
			if !ok {
				t.Fatalf("package %q not found in output", tc.id)
			}
			if p.name != tc.name {
				t.Errorf("name=%q, want %q", p.name, tc.name)
			}
			if p.version != tc.version {
				t.Errorf("version=%q, want %q", p.version, tc.version)
			}
			if p.available != tc.available {
				t.Errorf("available=%q, want %q", p.available, tc.available)
			}
		})
	}
}

func TestParseWinGetListOutputAvailableDetected(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("testdata", "winget_list.txt"))
	if err != nil {
		t.Fatalf("failed to read testdata: %v", err)
	}

	pkgs, err := parseWinGetListOutput(raw)
	if err != nil {
		t.Fatalf("parseWinGetListOutput returned error: %v", err)
	}

	withUpdate := 0
	for _, p := range pkgs {
		if p.available != "" {
			withUpdate++
		}
	}

	// Chrome, Git, VSCode, Python all have Available versions in the fixture.
	if withUpdate != 4 {
		t.Fatalf("expected 4 packages with available updates, got %d", withUpdate)
	}
}

func TestParseWinGetListOutputEmpty(t *testing.T) {
	t.Parallel()

	pkgs, err := parseWinGetListOutput([]byte(""))
	if err != nil {
		t.Fatalf("parseWinGetListOutput returned error on empty input: %v", err)
	}
	if len(pkgs) != 0 {
		t.Fatalf("expected 0 packages from empty input, got %d", len(pkgs))
	}
}

func TestParseWinGetListOutputHeaderOnly(t *testing.T) {
	t.Parallel()

	input := "Name   Id   Version   Available   Source\n" +
		"--------------------------------------------------\n"

	pkgs, err := parseWinGetListOutput([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pkgs) != 0 {
		t.Fatalf("expected 0 packages from header-only input, got %d", len(pkgs))
	}
}

func TestParseChocoListOutput(t *testing.T) {
	t.Parallel()

	input := `Chocolatey v1.4.0
git 2.43.0
googlechrome 121.0.6167.85
7zip 23.01
notepadplusplus 8.6.2
4 packages installed.
`
	pkgs, err := parseChocoListOutput([]byte(input))
	if err != nil {
		t.Fatalf("parseChocoListOutput returned error: %v", err)
	}
	if len(pkgs) != 4 {
		t.Fatalf("expected 4 packages, got %d", len(pkgs))
	}

	byName := make(map[string]string)
	for _, p := range pkgs {
		byName[p.Name] = p.Version
	}

	if byName["git"] != "2.43.0" {
		t.Errorf("git version=%q, want 2.43.0", byName["git"])
	}
	if byName["googlechrome"] != "121.0.6167.85" {
		t.Errorf("googlechrome version=%q, want 121.0.6167.85", byName["googlechrome"])
	}
}

func TestParseChocoListOutputEmpty(t *testing.T) {
	t.Parallel()

	pkgs, err := parseChocoListOutput([]byte(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pkgs) != 0 {
		t.Fatalf("expected 0 packages, got %d", len(pkgs))
	}
}
