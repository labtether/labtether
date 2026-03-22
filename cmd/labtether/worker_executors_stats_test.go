package main

import (
	"errors"
	"testing"

	"github.com/labtether/labtether/internal/persistence"
)

type fakeWorkerQueryStatsReader struct {
	stats []persistence.QueryStat
	err   error
}

func (f fakeWorkerQueryStatsReader) TopQueryStats(limit int) ([]persistence.QueryStat, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.stats, nil
}

func TestWorkerPerformanceSnapshotWithoutReader(t *testing.T) {
	snapshot := workerPerformanceSnapshot(nil, 5)
	if enabled, _ := snapshot["pg_stat_statements_enabled"].(bool); enabled {
		t.Fatalf("expected pg_stat_statements_enabled=false without reader")
	}
}

func TestWorkerPerformanceSnapshotWithQueryStats(t *testing.T) {
	snapshot := workerPerformanceSnapshot(fakeWorkerQueryStatsReader{stats: []persistence.QueryStat{{QueryID: "123", Calls: 10}}}, 50)
	if enabled, _ := snapshot["pg_stat_statements_enabled"].(bool); !enabled {
		t.Fatalf("expected pg_stat_statements_enabled=true when stats are available")
	}
	if limit, _ := snapshot["query_limit"].(int); limit != 50 {
		t.Fatalf("expected query_limit=50, got %d", limit)
	}
	entries, ok := snapshot["top_queries"].([]persistence.QueryStat)
	if !ok {
		t.Fatalf("expected top_queries to be []persistence.QueryStat")
	}
	if len(entries) != 1 || entries[0].QueryID != "123" {
		t.Fatalf("unexpected top query payload: %+v", entries)
	}
}

func TestWorkerPerformanceSnapshotHandlesUnavailableExtension(t *testing.T) {
	snapshot := workerPerformanceSnapshot(fakeWorkerQueryStatsReader{err: persistence.ErrPGStatStatementsUnavailable}, 5)
	if enabled, _ := snapshot["pg_stat_statements_enabled"].(bool); enabled {
		t.Fatalf("expected pg_stat_statements_enabled=false when extension unavailable")
	}
}

func TestWorkerPerformanceSnapshotSurfacesUnexpectedErrors(t *testing.T) {
	snapshot := workerPerformanceSnapshot(fakeWorkerQueryStatsReader{err: errors.New("boom")}, 5)
	if enabled, _ := snapshot["pg_stat_statements_enabled"].(bool); enabled {
		t.Fatalf("expected pg_stat_statements_enabled=false on reader error")
	}
	if snapshot["pg_stat_statements_error"] == nil {
		t.Fatalf("expected error details for unexpected reader failure")
	}
}

func TestParseWorkerQueryStatsLimit(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{input: "", want: 5},
		{input: "all", want: 0},
		{input: "0", want: 0},
		{input: "25", want: 25},
		{input: "5001", want: 5000},
		{input: "-1", want: 5},
		{input: "nope", want: 5},
	}
	for _, tc := range cases {
		if got := parseWorkerQueryStatsLimit(tc.input); got != tc.want {
			t.Fatalf("parseWorkerQueryStatsLimit(%q)=%d, want %d", tc.input, got, tc.want)
		}
	}
}
