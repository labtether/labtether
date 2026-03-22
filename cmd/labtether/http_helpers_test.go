package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/updates"
)

// Tests exercise the local aliases defined in http_helpers.go, which delegate
// to internal/hubapi/shared. These cover pure, request-level helpers that have
// no apiServer dependency.

// ---------------------------------------------------------------------------
// mapCommandLevel
// ---------------------------------------------------------------------------

func TestHTTPHelperMapCommandLevel(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"succeeded", "info"},
		{"success", "info"},
		{"SUCCEEDED", "info"},
		{"SUCCESS", "info"},
		{"queued", "debug"},
		{"running", "debug"},
		{"RUNNING", "debug"},
		{"failed", "error"},
		{"unknown", "error"},
		{"", "error"},
		{"  succeeded  ", "info"}, // trimmed before lower-casing
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			if got := mapCommandLevel(tc.input); got != tc.want {
				t.Fatalf("mapCommandLevel(%q): expected %q, got %q", tc.input, tc.want, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseLimit
// ---------------------------------------------------------------------------

func TestHTTPHelperParseLimit(t *testing.T) {
	cases := []struct {
		name     string
		query    string
		fallback int
		want     int
	}{
		{"no param uses fallback", "", 25, 25},
		{"valid limit", "?limit=50", 25, 50},
		{"limit capped at 1000", "?limit=9999", 25, 1000},
		{"negative limit uses fallback", "?limit=-1", 25, 25},
		{"zero limit uses fallback", "?limit=0", 25, 25},
		{"non-numeric uses fallback", "?limit=abc", 25, 25},
		{"exact boundary 1000 accepted", "?limit=1000", 25, 1000},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test"+tc.query, nil)
			if got := parseLimit(req, tc.fallback); got != tc.want {
				t.Fatalf("parseLimit(%q, %d): expected %d, got %d", tc.query, tc.fallback, tc.want, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseOffset
// ---------------------------------------------------------------------------

func TestHTTPHelperParseOffset(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  int
	}{
		{"no param returns 0", "", 0},
		{"valid offset", "?offset=10", 10},
		{"zero offset", "?offset=0", 0},
		{"negative offset returns 0", "?offset=-5", 0},
		{"non-numeric returns 0", "?offset=xyz", 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test"+tc.query, nil)
			if got := parseOffset(req); got != tc.want {
				t.Fatalf("parseOffset(%q): expected %d, got %d", tc.query, tc.want, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// groupIDQueryParam
// ---------------------------------------------------------------------------

func TestHTTPHelperGroupIDQueryParam(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  string
	}{
		{"no param returns empty string", "", ""},
		{"valid group_id", "?group_id=grp-01", "grp-01"},
		{"whitespace trimmed", "?group_id=+grp-01+", "grp-01"},
		{"nil request returns empty", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test"+tc.query, nil)
			if got := groupIDQueryParam(req); got != tc.want {
				t.Fatalf("groupIDQueryParam(%q): expected %q, got %q", tc.query, tc.want, got)
			}
		})
	}

	t.Run("nil request returns empty", func(t *testing.T) {
		if got := groupIDQueryParam(nil); got != "" {
			t.Fatalf("expected empty string for nil request, got %q", got)
		}
	})
}

// ---------------------------------------------------------------------------
// parseTimestampParam
// ---------------------------------------------------------------------------

func TestHTTPHelperParseTimestampParam(t *testing.T) {
	fallback := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("empty string returns fallback", func(t *testing.T) {
		if got := parseTimestampParam("", fallback); !got.Equal(fallback) {
			t.Fatalf("expected fallback %v, got %v", fallback, got)
		}
	})

	t.Run("valid RFC3339 parsed and returned as UTC", func(t *testing.T) {
		raw := "2025-06-15T10:30:00Z"
		want := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
		if got := parseTimestampParam(raw, fallback); !got.Equal(want) {
			t.Fatalf("expected %v, got %v", want, got)
		}
	})

	t.Run("invalid string returns fallback", func(t *testing.T) {
		if got := parseTimestampParam("not-a-date", fallback); !got.Equal(fallback) {
			t.Fatalf("expected fallback %v, got %v", fallback, got)
		}
	})

	t.Run("whitespace-only returns fallback", func(t *testing.T) {
		if got := parseTimestampParam("   ", fallback); !got.Equal(fallback) {
			t.Fatalf("expected fallback %v, got %v", fallback, got)
		}
	})

	t.Run("non-UTC offset converted to UTC", func(t *testing.T) {
		raw := "2025-06-15T12:00:00+02:00"
		want := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
		got := parseTimestampParam(raw, fallback)
		if !got.Equal(want) {
			t.Fatalf("expected UTC %v, got %v", want, got)
		}
		if got.Location() != time.UTC {
			t.Fatalf("expected UTC location, got %v", got.Location())
		}
	})
}

// ---------------------------------------------------------------------------
// parseDurationParam
// ---------------------------------------------------------------------------

func TestHTTPHelperParseDurationParam(t *testing.T) {
	fallback := time.Hour
	min := 5 * time.Minute
	max := 24 * time.Hour

	cases := []struct {
		name string
		raw  string
		want time.Duration
	}{
		{"empty returns fallback", "", fallback},
		{"valid in range", "30m", 30 * time.Minute},
		{"below min returns fallback", "1m", fallback},
		{"above max returns fallback", "48h", fallback},
		{"at min boundary accepted", "5m", 5 * time.Minute},
		{"at max boundary accepted", "24h", 24 * time.Hour},
		{"invalid string returns fallback", "notaduration", fallback},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseDurationParam(tc.raw, fallback, min, max); got != tc.want {
				t.Fatalf("parseDurationParam(%q): expected %v, got %v", tc.raw, tc.want, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// defaultStepForWindow
// ---------------------------------------------------------------------------

func TestHTTPHelperDefaultStepForWindow(t *testing.T) {
	cases := []struct {
		name   string
		window time.Duration
		want   time.Duration
	}{
		{"1 hour window → 15s step", time.Hour, 15 * time.Second},
		{"exactly 1 hour → 15s step", time.Hour, 15 * time.Second},
		{"3 hour window → 30s step", 3 * time.Hour, 30 * time.Second},
		{"6 hour window → 30s step", 6 * time.Hour, 30 * time.Second},
		{"12 hour window → 1m step", 12 * time.Hour, time.Minute},
		{"24 hour window → 1m step", 24 * time.Hour, time.Minute},
		{"1 minute window → 15s step", time.Minute, 15 * time.Second},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := defaultStepForWindow(tc.window); got != tc.want {
				t.Fatalf("defaultStepForWindow(%v): expected %v, got %v", tc.window, tc.want, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// requestClientKey
// ---------------------------------------------------------------------------

func TestHTTPHelperRequestClientKey(t *testing.T) {
	t.Run("extracts host from RemoteAddr", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.168.1.10:54321"
		if got := requestClientKey(req); got != "192.168.1.10" {
			t.Fatalf("expected 192.168.1.10, got %q", got)
		}
	})

	t.Run("empty RemoteAddr returns unknown", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = ""
		if got := requestClientKey(req); got != "unknown" {
			t.Fatalf("expected unknown, got %q", got)
		}
	})

	t.Run("RemoteAddr without port returned as-is", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.1"
		if got := requestClientKey(req); got != "10.0.0.1" {
			t.Fatalf("expected 10.0.0.1, got %q", got)
		}
	})
}

// ---------------------------------------------------------------------------
// validateMaxLen
// ---------------------------------------------------------------------------

func TestHTTPHelperValidateMaxLen(t *testing.T) {
	t.Run("value within limit", func(t *testing.T) {
		if err := validateMaxLen("field", "hello", 10); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("value at exact limit", func(t *testing.T) {
		if err := validateMaxLen("field", "hello", 5); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("value exceeds limit", func(t *testing.T) {
		if err := validateMaxLen("field", "toolongvalue", 5); err == nil {
			t.Fatal("expected error for value exceeding maxLen")
		}
	})

	t.Run("zero maxLen skips validation", func(t *testing.T) {
		if err := validateMaxLen("field", "anything", 0); err != nil {
			t.Fatalf("unexpected error with maxLen=0: %v", err)
		}
	})

	t.Run("negative maxLen skips validation", func(t *testing.T) {
		if err := validateMaxLen("field", "anything", -1); err != nil {
			t.Fatalf("unexpected error with maxLen=-1: %v", err)
		}
	})

	t.Run("empty value within any positive limit", func(t *testing.T) {
		if err := validateMaxLen("field", "", 1); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// actionRunMatchesGroup
// ---------------------------------------------------------------------------

func TestHTTPHelperActionRunMatchesGroup(t *testing.T) {
	assetGroup := map[string]string{
		"asset-1": "group-A",
		"asset-2": "group-B",
	}

	t.Run("empty groupID always matches", func(t *testing.T) {
		run := actions.Run{Target: "asset-1"}
		if !actionRunMatchesGroup(run, "", assetGroup) {
			t.Fatal("expected true for empty groupID")
		}
	})

	t.Run("target in matching group", func(t *testing.T) {
		run := actions.Run{Target: "asset-1"}
		if !actionRunMatchesGroup(run, "group-A", assetGroup) {
			t.Fatal("expected true for matching group")
		}
	})

	t.Run("target in different group", func(t *testing.T) {
		run := actions.Run{Target: "asset-1"}
		if actionRunMatchesGroup(run, "group-B", assetGroup) {
			t.Fatal("expected false for non-matching group")
		}
	})

	t.Run("target not in any group", func(t *testing.T) {
		run := actions.Run{Target: "asset-unknown"}
		if actionRunMatchesGroup(run, "group-A", assetGroup) {
			t.Fatal("expected false for unknown asset")
		}
	})
}

// ---------------------------------------------------------------------------
// filterLogEventsByGroup
// ---------------------------------------------------------------------------

func TestHTTPHelperFilterLogEventsByGroup(t *testing.T) {
	assetGroup := map[string]string{
		"asset-1": "group-A",
		"asset-2": "group-B",
	}

	events := []logs.Event{
		{ID: "e1", AssetID: "asset-1", Fields: map[string]string{}},
		{ID: "e2", AssetID: "asset-2", Fields: map[string]string{}},
		{ID: "e3", AssetID: "asset-3", Fields: map[string]string{"group_id": "group-A"}},
		{ID: "e4", AssetID: "", Fields: map[string]string{}},
	}

	t.Run("empty groupID returns all events", func(t *testing.T) {
		got := filterLogEventsByGroup(events, "", assetGroup)
		if len(got) != len(events) {
			t.Fatalf("expected %d events, got %d", len(events), len(got))
		}
	})

	t.Run("filters to group-A members", func(t *testing.T) {
		got := filterLogEventsByGroup(events, "group-A", assetGroup)
		// e1 matches via assetGroup; e3 matches via fields["group_id"]
		if len(got) != 2 {
			t.Fatalf("expected 2 events for group-A, got %d", len(got))
		}
		ids := map[string]bool{}
		for _, e := range got {
			ids[e.ID] = true
		}
		if !ids["e1"] || !ids["e3"] {
			t.Fatalf("expected e1 and e3 in group-A filter result, got %+v", ids)
		}
	})

	t.Run("filters to group-B members", func(t *testing.T) {
		got := filterLogEventsByGroup(events, "group-B", assetGroup)
		if len(got) != 1 || got[0].ID != "e2" {
			t.Fatalf("expected only e2 for group-B, got %+v", got)
		}
	})

	t.Run("unknown group returns empty", func(t *testing.T) {
		got := filterLogEventsByGroup(events, "group-Z", assetGroup)
		if len(got) != 0 {
			t.Fatalf("expected 0 events for unknown group, got %d", len(got))
		}
	})
}

// ---------------------------------------------------------------------------
// groupAssetIDsForGroup
// ---------------------------------------------------------------------------

func TestHTTPHelperGroupAssetIDsForGroup(t *testing.T) {
	assetGroup := map[string]string{
		"asset-1": "group-A",
		"asset-2": "group-A",
		"asset-3": "group-B",
	}

	t.Run("empty groupID returns nil", func(t *testing.T) {
		got := groupAssetIDsForGroup("", assetGroup)
		if got != nil {
			t.Fatalf("expected nil for empty groupID, got %v", got)
		}
	})

	t.Run("returns assets for matching group", func(t *testing.T) {
		got := groupAssetIDsForGroup("group-A", assetGroup)
		if len(got) != 2 {
			t.Fatalf("expected 2 assets for group-A, got %d", len(got))
		}
		ids := map[string]bool{}
		for _, id := range got {
			ids[id] = true
		}
		if !ids["asset-1"] || !ids["asset-2"] {
			t.Fatalf("expected asset-1 and asset-2, got %v", got)
		}
	})

	t.Run("unknown group returns empty slice", func(t *testing.T) {
		got := groupAssetIDsForGroup("group-Z", assetGroup)
		if len(got) != 0 {
			t.Fatalf("expected empty slice for unknown group, got %v", got)
		}
	})
}

// ---------------------------------------------------------------------------
// updatePlanTouchesGroup
// ---------------------------------------------------------------------------

func TestHTTPHelperUpdatePlanTouchesGroup(t *testing.T) {
	assetGroup := map[string]string{
		"asset-1": "group-A",
		"asset-2": "group-B",
	}

	t.Run("empty groupID always touches", func(t *testing.T) {
		plan := updates.Plan{Targets: []string{"asset-1"}}
		if !updatePlanTouchesGroup(plan, "", assetGroup) {
			t.Fatal("expected true for empty groupID")
		}
	})

	t.Run("plan target in matching group", func(t *testing.T) {
		plan := updates.Plan{Targets: []string{"asset-1", "asset-2"}}
		if !updatePlanTouchesGroup(plan, "group-A", assetGroup) {
			t.Fatal("expected true when at least one target in group")
		}
	})

	t.Run("plan targets not in group", func(t *testing.T) {
		plan := updates.Plan{Targets: []string{"asset-2"}}
		if updatePlanTouchesGroup(plan, "group-A", assetGroup) {
			t.Fatal("expected false when no targets in group-A")
		}
	})

	t.Run("plan with no targets returns false for non-empty group", func(t *testing.T) {
		plan := updates.Plan{Targets: []string{}}
		if updatePlanTouchesGroup(plan, "group-A", assetGroup) {
			t.Fatal("expected false for plan with no targets")
		}
	})
}
