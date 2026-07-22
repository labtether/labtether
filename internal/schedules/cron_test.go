package schedules

import (
	"strings"
	"testing"
	"time"
)

func TestNextRunSupportsFiveFieldAndDescriptors(t *testing.T) {
	base := time.Date(2026, time.July, 14, 3, 12, 45, 0, time.UTC)
	tests := []struct {
		expression string
		want       time.Time
	}{
		{expression: "*/15 * * * *", want: time.Date(2026, time.July, 14, 3, 15, 0, 0, time.UTC)},
		{expression: "@hourly", want: time.Date(2026, time.July, 14, 4, 0, 0, 0, time.UTC)},
		{expression: "CRON_TZ=Australia/Sydney 0 9 * * *", want: time.Date(2026, time.July, 14, 23, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.expression, func(t *testing.T) {
			got, err := NextRun(tt.expression, base)
			if err != nil {
				t.Fatalf("NextRun() error = %v", err)
			}
			if !got.Equal(tt.want) {
				t.Fatalf("NextRun() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestNextRunRejectsSecondsMalformedAndOversizedExpressions(t *testing.T) {
	for _, expression := range []string{
		"* * * * * *",
		"not a cron",
		"@every 30s",
		"CRON_TZ=UTC @every 1m",
		"CRON_TZ=UTC",
		"TZ= * * * * *",
		strings.Repeat("*", MaxCronExpressionLength+1),
	} {
		if _, err := NextRun(expression, time.Now()); err == nil {
			t.Fatalf("NextRun(%q) unexpectedly succeeded", expression)
		}
	}
}

func TestExecutionJobIDIsStableAndOccurrenceScoped(t *testing.T) {
	due := time.Date(2026, time.July, 14, 4, 0, 0, 0, time.UTC)
	first := ExecutionJobID("sched-1", due)
	if first != ExecutionJobID("sched-1", due) {
		t.Fatal("same occurrence produced different job IDs")
	}
	if first == ExecutionJobID("sched-1", due.Add(time.Hour)) {
		t.Fatal("different occurrences produced the same job ID")
	}
}
