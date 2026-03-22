package backends

import (
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
)

func TestParseOSLogLine(t *testing.T) {
	raw := []byte(`{
		"timestamp":"2026-02-24 11:12:13.123456-0800",
		"eventMessage":"network changed",
		"messageType":"Error",
		"process":"configd",
		"subsystem":"com.apple.network"
	}`)

	entry, ok := ParseOSLogLine(raw)
	if !ok {
		t.Fatal("expected ParseOSLogLine to parse valid line")
	}
	if entry.Message != "network changed" {
		t.Fatalf("message=%q, want network changed", entry.Message)
	}
	if entry.Level != "error" {
		t.Fatalf("level=%q, want error", entry.Level)
	}
	if entry.Source != "com.apple.network" {
		t.Fatalf("source=%q, want subsystem", entry.Source)
	}
	if entry.Timestamp == "" {
		t.Fatal("expected timestamp to be set")
	}
}

func TestParseOSLogLineForStreamSkipsTimestampParsing(t *testing.T) {
	raw := []byte(`{
		"timestamp":"2026-02-24 11:12:13.123456-0800",
		"eventMessage":"network changed",
		"messageType":"Info",
		"process":"configd"
	}`)

	entry, ok := ParseOSLogLineForStream(raw)
	if !ok {
		t.Fatal("expected ParseOSLogLineForStream to parse valid line")
	}
	if entry.Timestamp != "" {
		t.Fatalf("timestamp=%q, want empty for stream fast path", entry.Timestamp)
	}
}

func TestParseOSLogLineSourceFallbackToProcessPathBase(t *testing.T) {
	raw := []byte(`{
		"eventMessage":"daemon event",
		"messageType":"Info",
		"processImagePath":"/usr/libexec/some-daemon"
	}`)

	entry, ok := ParseOSLogLineForStream(raw)
	if !ok {
		t.Fatal("expected ParseOSLogLineForStream to parse valid line")
	}
	if entry.Source != "some-daemon" {
		t.Fatalf("source=%q, want process path basename", entry.Source)
	}
}

func TestParseOSLogLineSourceFallbackToSenderPathBase(t *testing.T) {
	raw := []byte(`{
		"eventMessage":"sender event",
		"messageType":"Info",
		"senderImagePath":"/System/Library/CoreServices/Finder.app/Contents/MacOS/Finder"
	}`)

	entry, ok := ParseOSLogLineForStream(raw)
	if !ok {
		t.Fatal("expected ParseOSLogLineForStream to parse valid line")
	}
	if entry.Source != "Finder" {
		t.Fatalf("source=%q, want sender path basename", entry.Source)
	}
}

func TestResolveDarwinLogRange(t *testing.T) {
	tests := []struct {
		name      string
		since     string
		until     string
		wantStart string
		wantEnd   string
		wantLast  string
	}{
		{
			name:     "default-last-hour",
			wantLast: "1h",
		},
		{
			name:     "relative-since",
			since:    "30m ago",
			wantLast: "30m",
		},
		{
			name:      "absolute-range",
			since:     "2026-02-24 00:00:00",
			until:     "2026-02-24 01:00:00",
			wantStart: "2026-02-24 00:00:00",
			wantEnd:   "2026-02-24 01:00:00",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			start, end, last := ResolveDarwinLogRange(tc.since, tc.until)
			if start != tc.wantStart {
				t.Fatalf("start=%q, want %q", start, tc.wantStart)
			}
			if end != tc.wantEnd {
				t.Fatalf("end=%q, want %q", end, tc.wantEnd)
			}
			if last != tc.wantLast {
				t.Fatalf("last=%q, want %q", last, tc.wantLast)
			}
		})
	}
}

func TestBuildDarwinLogShowArgs(t *testing.T) {
	args := BuildDarwinLogShowArgs(agentmgr.JournalQueryData{
		Since: "1h ago",
	})

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--last 1h") {
		t.Fatalf("expected --last 1h in args, got %v", args)
	}
	if !strings.Contains(joined, "--style ndjson") {
		t.Fatalf("expected --style ndjson in args, got %v", args)
	}
}

func TestBuildDarwinLogStreamArgs(t *testing.T) {
	args := BuildDarwinLogStreamArgs()
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "--style ndjson") {
		t.Fatalf("expected --style ndjson in stream args, got %v", args)
	}
	if !strings.Contains(joined, "--type log") {
		t.Fatalf("expected --type log in stream args, got %v", args)
	}
	if !strings.Contains(joined, "--level default") {
		t.Fatalf("expected --level default in stream args, got %v", args)
	}
}

func TestEntryMatchesQuery(t *testing.T) {
	entry := agentmgr.LogStreamData{
		Level:   "warning",
		Source:  "com.apple.network",
		Message: "interface en0 changed state",
	}

	if !EntryMatchesQuery(entry, agentmgr.JournalQueryData{Priority: "info"}) {
		t.Fatal("expected warning entry to match info priority ceiling")
	}
	if EntryMatchesQuery(entry, agentmgr.JournalQueryData{Priority: "err"}) {
		t.Fatal("expected warning entry to not match err priority")
	}
	if !EntryMatchesQuery(entry, agentmgr.JournalQueryData{Unit: "network"}) {
		t.Fatal("expected source filter to match")
	}
	if EntryMatchesQuery(entry, agentmgr.JournalQueryData{Search: "cpu"}) {
		t.Fatal("expected search filter miss")
	}
}
