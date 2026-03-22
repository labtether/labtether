package backends

import (
	"reflect"
	"testing"
)

func TestParseBrewInstalledPackages(t *testing.T) {
	raw := []byte(`{
		"formulae": [
			{
				"name": "wget",
				"full_name": "wget",
				"installed": [{"version": "1.24.5"}],
				"versions": {"stable": "1.24.5"}
			},
			{
				"name": "jq",
				"full_name": "jq",
				"installed": [],
				"versions": {"stable": "1.7.1"}
			}
		],
		"casks": [
			{
				"token": "iterm2",
				"version": "3.5.0",
				"installed": ["3.5.0"]
			}
		]
	}`)

	packages, err := ParseBrewInstalledPackages(raw)
	if err != nil {
		t.Fatalf("ParseBrewInstalledPackages returned error: %v", err)
	}

	names := make([]string, 0, len(packages))
	for _, pkg := range packages {
		names = append(names, pkg.Name)
		if pkg.Status != "installed" {
			t.Fatalf("expected installed status, got %q for %s", pkg.Status, pkg.Name)
		}
	}

	wantNames := []string{"iterm2", "jq", "wget"}
	if !reflect.DeepEqual(names, wantNames) {
		t.Fatalf("package names=%v, want %v", names, wantNames)
	}
}

func TestParseBrewInstalledPackagesCaskInstalledAsString(t *testing.T) {
	raw := []byte(`{
		"formulae": [],
		"casks": [
			{
				"token": "visual-studio-code",
				"version": "1.98.0",
				"installed": "1.98.0"
			}
		]
	}`)

	packages, err := ParseBrewInstalledPackages(raw)
	if err != nil {
		t.Fatalf("ParseBrewInstalledPackages returned error: %v", err)
	}
	if len(packages) != 1 {
		t.Fatalf("expected 1 package, got %d", len(packages))
	}
	if packages[0].Name != "visual-studio-code" {
		t.Fatalf("package name=%q, want visual-studio-code", packages[0].Name)
	}
	if packages[0].Version != "1.98.0" {
		t.Fatalf("package version=%q, want 1.98.0", packages[0].Version)
	}
}

func TestParseBrewInstalledPackagesInvalidJSON(t *testing.T) {
	_, err := ParseBrewInstalledPackages([]byte(`not-json`))
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestBuildDarwinPackageActionArgs(t *testing.T) {
	tests := []struct {
		name     string
		action   string
		packages []string
		want     []string
		wantErr  bool
	}{
		{
			name:     "install",
			action:   "install",
			packages: []string{"wget", "jq"},
			want:     []string{"install", "wget", "jq"},
		},
		{
			name:     "remove",
			action:   "remove",
			packages: []string{"wget"},
			want:     []string{"uninstall", "wget"},
		},
		{
			name:   "upgrade-all",
			action: "upgrade",
			want:   []string{"upgrade"},
		},
		{
			name:     "upgrade-specific",
			action:   "upgrade",
			packages: []string{"wget"},
			want:     []string{"upgrade", "wget"},
		},
		{
			name:    "invalid",
			action:  "noop",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := BuildDarwinPackageActionArgs(tc.action, tc.packages)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("args=%v, want %v", got, tc.want)
			}
		})
	}
}
