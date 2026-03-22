package backends

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// parseGetHotFixOutput
// ---------------------------------------------------------------------------

func TestParseGetHotFixOutputFromFixture(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("testdata", "get_hotfix.json"))
	if err != nil {
		t.Fatalf("failed to read testdata: %v", err)
	}

	updates, err := parseGetHotFixOutput(raw)
	if err != nil {
		t.Fatalf("parseGetHotFixOutput returned error: %v", err)
	}

	if len(updates) != 6 {
		t.Fatalf("expected 6 updates, got %d", len(updates))
	}
}

func TestParseGetHotFixOutputFields(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("testdata", "get_hotfix.json"))
	if err != nil {
		t.Fatalf("failed to read testdata: %v", err)
	}

	updates, err := parseGetHotFixOutput(raw)
	if err != nil {
		t.Fatalf("parseGetHotFixOutput returned error: %v", err)
	}

	byID := make(map[string]WindowsUpdateInfo, len(updates))
	for _, u := range updates {
		byID[u.HotFixID] = u
	}

	tests := []struct {
		hotfixID    string
		description string
		installedOn string
		installedBy string
	}{
		{
			hotfixID:    "KB5034441",
			description: "Update",
			installedOn: "2024-01-15",
			installedBy: `NT AUTHORITY\SYSTEM`,
		},
		{
			hotfixID:    "KB5033372",
			description: "Security Update",
			installedOn: "2023-12-12",
			installedBy: `NT AUTHORITY\SYSTEM`,
		},
		{
			hotfixID:    "KB5012170",
			description: "Update",
			installedOn: "2022-08-09",
			installedBy: "",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.hotfixID, func(t *testing.T) {
			t.Parallel()
			u, ok := byID[tc.hotfixID]
			if !ok {
				t.Fatalf("update %q not found in output", tc.hotfixID)
			}
			if u.Description != tc.description {
				t.Errorf("description=%q, want %q", u.Description, tc.description)
			}
			if u.InstalledOn != tc.installedOn {
				t.Errorf("installed_on=%q, want %q", u.InstalledOn, tc.installedOn)
			}
			if u.InstalledBy != tc.installedBy {
				t.Errorf("installed_by=%q, want %q", u.InstalledBy, tc.installedBy)
			}
		})
	}
}

func TestParseGetHotFixOutputEmpty(t *testing.T) {
	t.Parallel()

	updates, err := parseGetHotFixOutput([]byte(""))
	if err != nil {
		t.Fatalf("parseGetHotFixOutput returned error on empty input: %v", err)
	}
	if len(updates) != 0 {
		t.Fatalf("expected 0 updates from empty input, got %d", len(updates))
	}
}

func TestParseGetHotFixOutputWhitespaceOnly(t *testing.T) {
	t.Parallel()

	updates, err := parseGetHotFixOutput([]byte("   \n\t  "))
	if err != nil {
		t.Fatalf("unexpected error on whitespace-only input: %v", err)
	}
	if len(updates) != 0 {
		t.Fatalf("expected 0 updates from whitespace-only input, got %d", len(updates))
	}
}

func TestParseGetHotFixOutputSingleObject(t *testing.T) {
	t.Parallel()

	// PowerShell emits a bare object (not an array) when only one hotfix is installed.
	input := `{"Source":"WIN-SERVER01","Description":"Security Update","HotFixID":"KB5034441","InstalledBy":"NT AUTHORITY\\SYSTEM","InstalledOn":"1/15/2024 12:00:00 AM"}`

	updates, err := parseGetHotFixOutput([]byte(input))
	if err != nil {
		t.Fatalf("parseGetHotFixOutput returned error for single object: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}
	if updates[0].HotFixID != "KB5034441" {
		t.Errorf("hotfix_id=%q, want KB5034441", updates[0].HotFixID)
	}
}

func TestParseGetHotFixOutputBOM(t *testing.T) {
	t.Parallel()

	// PowerShell on some Windows versions prepends a UTF-8 BOM.
	bom := "\xef\xbb\xbf"
	input := bom + `[{"Source":"WIN-SERVER01","Description":"Update","HotFixID":"KB5034441","InstalledBy":"","InstalledOn":"1/15/2024 12:00:00 AM"}]`

	updates, err := parseGetHotFixOutput([]byte(input))
	if err != nil {
		t.Fatalf("parseGetHotFixOutput returned error with BOM: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("expected 1 update with BOM input, got %d", len(updates))
	}
}

func TestParseGetHotFixOutputSkipsEmptyHotFixID(t *testing.T) {
	t.Parallel()

	input := `[
		{"Source":"WIN-SERVER01","Description":"Update","HotFixID":"KB5034441","InstalledBy":"","InstalledOn":"1/15/2024 12:00:00 AM"},
		{"Source":"WIN-SERVER01","Description":"Update","HotFixID":"","InstalledBy":"","InstalledOn":""}
	]`

	updates, err := parseGetHotFixOutput([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("expected 1 update (entry with empty HotFixID skipped), got %d", len(updates))
	}
}

func TestParseGetHotFixOutputDateNormalisation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw  string
		want string
	}{
		{"1/15/2024 12:00:00 AM", "2024-01-15"},
		{"12/12/2023 12:00:00 AM", "2023-12-12"},
		{"8/9/2022 12:00:00 AM", "2022-08-09"},
		{"", ""},
		{"not-a-date", "not-a-date"}, // passthrough on parse failure
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.raw, func(t *testing.T) {
			t.Parallel()
			got := normaliseHotFixDate(tc.raw)
			if got != tc.want {
				t.Errorf("normaliseHotFixDate(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseRebootRequiredOutput
// ---------------------------------------------------------------------------

func TestParseRebootRequiredOutputTrue(t *testing.T) {
	t.Parallel()

	cases := [][]byte{
		[]byte("True"),
		[]byte("True\r\n"),
		[]byte("  True  \n"),
	}
	for _, c := range cases {
		if !parseRebootRequiredOutput(c) {
			t.Errorf("parseRebootRequiredOutput(%q) = false, want true", c)
		}
	}
}

func TestParseRebootRequiredOutputFalse(t *testing.T) {
	t.Parallel()

	cases := [][]byte{
		[]byte("False"),
		[]byte("False\r\n"),
		[]byte("  False  \n"),
		[]byte(""),
		[]byte("true"),  // PowerShell always capitalises; lower-case must not match
		[]byte("false"),
	}
	for _, c := range cases {
		if parseRebootRequiredOutput(c) {
			t.Errorf("parseRebootRequiredOutput(%q) = true, want false", c)
		}
	}
}
