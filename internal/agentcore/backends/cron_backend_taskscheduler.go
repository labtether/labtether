package backends

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/securityruntime"
)

const schtasksCommandTimeout = 30 * time.Second

// RunSchtasksCommand is the function used to execute schtasks.exe. Overridable for tests.
var RunSchtasksCommand = securityruntime.CommandContextCombinedOutput

// WindowsCronBackend implements CronBackend using Windows Task Scheduler (schtasks.exe).
// It has no build tags so that parser tests can run on any platform.
type WindowsCronBackend struct{}

// ListEntries lists scheduled tasks via schtasks.exe, filtering out Microsoft internal tasks.
func (WindowsCronBackend) ListEntries() ([]agentmgr.CronEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), schtasksCommandTimeout)
	defer cancel()

	out, err := RunSchtasksCommand(ctx, "schtasks.exe", "/Query", "/FO", "CSV", "/V")
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("schtasks query timed out")
		}
		trimmed := strings.TrimSpace(string(out))
		if trimmed != "" {
			return nil, fmt.Errorf("schtasks query failed: %s", trimmed)
		}
		return nil, fmt.Errorf("schtasks query failed: %w", err)
	}

	return parseSchtasksCSV(out)
}

// parseSchtasksCSV parses the CSV output of `schtasks.exe /Query /FO CSV /V`.
//
// The output has a header row followed by one row per task. Tasks whose
// TaskName starts with \Microsoft\ are filtered out as OS maintenance tasks.
func parseSchtasksCSV(raw []byte) ([]agentmgr.CronEntry, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}

	r := csv.NewReader(bytes.NewReader(raw))
	r.LazyQuotes = true
	r.TrimLeadingSpace = true

	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("schtasks CSV parse failed: %w", err)
	}
	if len(records) < 2 {
		// Header only or empty — no tasks.
		return nil, nil
	}

	// Build a column index from the header row.
	header := records[0]
	colIndex := make(map[string]int, len(header))
	for i, h := range header {
		colIndex[strings.TrimSpace(h)] = i
	}

	col := func(row []string, name string) string {
		i, ok := colIndex[name]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}

	var entries []agentmgr.CronEntry
	for _, row := range records[1:] {
		taskName := col(row, "TaskName")
		if taskName == "" {
			continue
		}
		// Filter OS maintenance tasks under \Microsoft\.
		if strings.HasPrefix(taskName, `\Microsoft\`) {
			continue
		}

		schedType := col(row, "Schedule Type")
		startTime := col(row, "Start Time")
		schedule := buildSchtasksSchedule(schedType, startTime)

		nextRun := parseSchtasksTime(col(row, "Next Run Time"))
		lastRun := parseSchtasksTime(col(row, "Last Run Time"))

		// Run As User may be a qualified "HOST\user" form; strip the host prefix.
		user := col(row, "Run As User")
		if idx := strings.LastIndex(user, `\`); idx >= 0 {
			user = user[idx+1:]
		}

		entries = append(entries, agentmgr.CronEntry{
			Source:   "task-scheduler",
			Schedule: schedule,
			Command:  taskName,
			User:     user,
			NextRun:  nextRun,
			LastRun:  lastRun,
		})
	}

	return entries, nil
}

// buildSchtasksSchedule constructs a human-readable schedule expression from
// the Schedule Type and Start Time columns of schtasks CSV output.
func buildSchtasksSchedule(schedType, startTime string) string {
	parts := make([]string, 0, 2)
	if schedType != "" && schedType != "Scheduling data is not available in this format." {
		parts = append(parts, schedType)
	}
	if startTime != "" && startTime != "N/A" {
		parts = append(parts, "at "+startTime)
	}
	if len(parts) == 0 {
		return "on-demand"
	}
	return strings.Join(parts, " ")
}

// schtasksTimeLayouts lists the date/time formats schtasks.exe may produce.
// schtasks emits locale-dependent timestamps; we cover the common US en-US format.
var schtasksTimeLayouts = []string{
	"1/2/2006 3:04:05 PM",
	"1/2/2006 3:04:05 AM",
	"1/2/2006 15:04:05",
}

// parseSchtasksTime converts a schtasks timestamp string to RFC3339.
// Returns empty string if the value is "N/A", empty, or unparseable.
func parseSchtasksTime(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" || strings.EqualFold(v, "N/A") {
		return ""
	}
	for _, layout := range schtasksTimeLayouts {
		if t, err := time.ParseInLocation(layout, v, time.Local); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	}
	// Return empty rather than a misleading value if we cannot parse.
	return ""
}
