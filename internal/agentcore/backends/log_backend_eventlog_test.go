package backends

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
)

func TestParseWevtutilOutputCount(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("testdata", "wevtutil_query.json"))
	if err != nil {
		t.Fatalf("failed to read testdata: %v", err)
	}

	entries, err := parseWevtutilOutput(raw)
	if err != nil {
		t.Fatalf("parseWevtutilOutput returned error: %v", err)
	}

	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}
}

func TestParseWevtutilOutputFields(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("testdata", "wevtutil_query.json"))
	if err != nil {
		t.Fatalf("failed to read testdata: %v", err)
	}

	entries, err := parseWevtutilOutput(raw)
	if err != nil {
		t.Fatalf("parseWevtutilOutput returned error: %v", err)
	}

	// First entry: Level 4 (info), Service Control Manager
	first := entries[0]
	if first.Message == "" {
		t.Fatal("expected non-empty message on first entry")
	}
	if !strings.Contains(first.Message, "Windows Update service") {
		t.Fatalf("message=%q, expected to contain 'Windows Update service'", first.Message)
	}
	if first.Level != "info" {
		t.Fatalf("level=%q, want info (level 4)", first.Level)
	}
	if first.Source != "Service Control Manager" {
		t.Fatalf("source=%q, want 'Service Control Manager'", first.Source)
	}
	if first.Timestamp == "" {
		t.Fatal("expected timestamp to be set")
	}
	// Timestamp should be RFC3339
	if !strings.Contains(first.Timestamp, "T") {
		t.Fatalf("timestamp=%q does not look like RFC3339", first.Timestamp)
	}
}

func TestParseWevtutilOutputLevelMapping(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("testdata", "wevtutil_query.json"))
	if err != nil {
		t.Fatalf("failed to read testdata: %v", err)
	}

	entries, err := parseWevtutilOutput(raw)
	if err != nil {
		t.Fatalf("parseWevtutilOutput returned error: %v", err)
	}

	// Build a map by Source+level for assertions.
	// entry[0]: Level 4 -> info  (Service Control Manager)
	// entry[1]: Level 0 -> info  (Security Auditing - 0 means success/info)
	// entry[2]: Level 1 -> critical (Kernel-Power)
	// entry[3]: Level 2 -> error  (WindowsUpdateClient)
	// entry[4]: Level 3 -> warning (Dhcp-Client)
	expected := []struct {
		levelNum  int
		wantLevel string
	}{
		{4, "info"},
		{0, "info"},
		{1, "critical"},
		{2, "error"},
		{3, "warning"},
	}

	for i, ex := range expected {
		if i >= len(entries) {
			t.Fatalf("entry[%d] not present", i)
		}
		if entries[i].Level != ex.wantLevel {
			t.Errorf("entry[%d] level=%q, want %q (numeric level %d)", i, entries[i].Level, ex.wantLevel, ex.levelNum)
		}
	}
}

func TestParseWevtutilOutputSources(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("testdata", "wevtutil_query.json"))
	if err != nil {
		t.Fatalf("failed to read testdata: %v", err)
	}

	entries, err := parseWevtutilOutput(raw)
	if err != nil {
		t.Fatalf("parseWevtutilOutput returned error: %v", err)
	}

	wantSources := []string{
		"Service Control Manager",
		"Microsoft-Windows-Security-Auditing",
		"Microsoft-Windows-Kernel-Power",
		"Microsoft-Windows-WindowsUpdateClient",
		"Microsoft-Windows-Dhcp-Client",
	}

	for i, want := range wantSources {
		if i >= len(entries) {
			t.Fatalf("entry[%d] not present", i)
		}
		if entries[i].Source != want {
			t.Errorf("entry[%d] source=%q, want %q", i, entries[i].Source, want)
		}
	}
}

func TestParseWevtutilOutputEmpty(t *testing.T) {
	t.Parallel()

	entries, err := parseWevtutilOutput([]byte(""))
	if err != nil {
		t.Fatalf("parseWevtutilOutput returned error on empty input: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries from empty input, got %d", len(entries))
	}
}

func TestParseWevtutilOutputSkipsInvalidLines(t *testing.T) {
	t.Parallel()

	input := []byte(`not json
{"Event":{"System":{"Provider":{"Name":"TestProvider"},"Level":"4","TimeCreated":{"SystemTime":"2026-03-21T08:00:00.000000000Z"}},"RenderingInfo":{"Message":"hello world","Level":"Information"}}}
also not json
`)
	entries, err := parseWevtutilOutput(input)
	if err != nil {
		t.Fatalf("parseWevtutilOutput returned error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 valid entry, got %d", len(entries))
	}
	if entries[0].Message != "hello world" {
		t.Fatalf("message=%q, want 'hello world'", entries[0].Message)
	}
}

func TestParseWevtutilOutputSkipsEmptyMessage(t *testing.T) {
	t.Parallel()

	// An event with an empty message should be skipped.
	input := []byte(`{"Event":{"System":{"Provider":{"Name":"TestProvider"},"Level":"4","TimeCreated":{"SystemTime":"2026-03-21T08:00:00.000000000Z"}},"RenderingInfo":{"Message":"","Level":"Information"}}}`)
	entries, err := parseWevtutilOutput(input)
	if err != nil {
		t.Fatalf("parseWevtutilOutput returned error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries from event with empty message, got %d", len(entries))
	}
}

func TestBuildWevtutilQueryArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		req      agentmgr.JournalQueryData
		channel  string
		wantArgs []string
	}{
		{
			name:     "basic-system-channel",
			req:      agentmgr.JournalQueryData{Limit: 50},
			channel:  "System",
			wantArgs: []string{"qe", "System", "/f:json", "/c:50"},
		},
		{
			name:     "with-since",
			req:      agentmgr.JournalQueryData{Limit: 10, Since: "2026-03-21T00:00:00Z"},
			channel:  "Application",
			wantArgs: []string{"qe", "Application", "/f:json", "/c:10", "/q:*[System[TimeCreated[@SystemTime>='2026-03-21T00:00:00Z']]]"},
		},
		{
			name:     "default-limit",
			req:      agentmgr.JournalQueryData{Limit: 0},
			channel:  "System",
			wantArgs: []string{"qe", "System", "/f:json", "/c:200"},
		},
		{
			name:     "clamped-limit",
			req:      agentmgr.JournalQueryData{Limit: 9999},
			channel:  "System",
			wantArgs: []string{"qe", "System", "/f:json", "/c:1000"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			args := BuildWevtutilQueryArgs(tc.req, tc.channel)
			if len(args) != len(tc.wantArgs) {
				t.Fatalf("args=%v, want %v", args, tc.wantArgs)
			}
			for i, want := range tc.wantArgs {
				if args[i] != want {
					t.Errorf("args[%d]=%q, want %q", i, args[i], want)
				}
			}
		})
	}
}

func TestWevtutilLevelToString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		level int
		want  string
	}{
		{0, "info"},
		{1, "critical"},
		{2, "error"},
		{3, "warning"},
		{4, "info"},
		{5, "info"},
		{99, "info"},
	}

	for _, tc := range tests {
		got := wevtutilLevelToString(tc.level)
		if got != tc.want {
			t.Errorf("wevtutilLevelToString(%d)=%q, want %q", tc.level, got, tc.want)
		}
	}
}
