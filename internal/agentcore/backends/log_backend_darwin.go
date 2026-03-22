package backends

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/securityruntime"
)

const darwinLogCommandTimeout = 25 * time.Second

var darwinRelativeSincePattern = regexp.MustCompile(`^([0-9]+)\s*([smhdw])(?:\s*ago)?$`)

// DarwinLogBackend implements LogBackend using macOS `log` command.
type DarwinLogBackend struct{}

// QueryEntries queries historical log entries via `log show`.
func (DarwinLogBackend) QueryEntries(req agentmgr.JournalQueryData) ([]agentmgr.LogStreamData, error) {
	if _, err := exec.LookPath("log"); err != nil {
		return nil, fmt.Errorf("system log query is not available on this host")
	}

	ctx, cancel := context.WithTimeout(context.Background(), darwinLogCommandTimeout)
	defer cancel()

	args := BuildDarwinLogShowArgs(req)
	out, err := securityruntime.CommandContextCombinedOutput(ctx, "log", args...)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("system log query timed out")
		}
		trimmed := strings.TrimSpace(string(out))
		if trimmed != "" {
			return nil, fmt.Errorf("system log query failed: %s", trimmed)
		}
		return nil, fmt.Errorf("system log query failed: %w", err)
	}

	limit := NormalizedJournalLimit(req.Limit)
	buffer := make([]timedLogEntry, 0, limit)

	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(make([]byte, 256*1024), 256*1024)
	for scanner.Scan() {
		entry, ok := ParseOSLogLine(scanner.Bytes())
		if !ok {
			continue
		}
		if !EntryMatchesQuery(entry, req) {
			continue
		}
		buffer = append(buffer, timedLogEntry{
			entry: entry,
			when:  ParseLogEntryTime(entry.Timestamp),
		})
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return nil, fmt.Errorf("failed to parse system log output: %w", scanErr)
	}

	sort.Slice(buffer, func(i, j int) bool {
		return buffer[i].when.After(buffer[j].when)
	})

	if len(buffer) > limit {
		buffer = buffer[:limit]
	}

	entries := make([]agentmgr.LogStreamData, 0, len(buffer))
	for _, item := range buffer {
		entries = append(entries, item.entry)
	}
	return entries, nil
}

// StreamEntries streams log entries via `log stream`.
func (DarwinLogBackend) StreamEntries(ctx context.Context, emit func(agentmgr.LogStreamData)) error {
	if _, err := exec.LookPath("log"); err != nil {
		return ErrLogStreamingUnsupported
	}

	cmd, err := securityruntime.NewCommandContext(ctx, "log", BuildDarwinLogStreamArgs()...)
	if err != nil {
		return fmt.Errorf("failed to build system log stream command: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to open system log stream stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start system log stream: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			_ = cmd.Wait()
			return nil
		default:
		}

		entry, ok := ParseOSLogLineForStream(scanner.Bytes())
		if !ok {
			continue
		}
		emit(entry)
	}

	waitErr := cmd.Wait()
	if scanErr := scanner.Err(); scanErr != nil && ctx.Err() == nil {
		return fmt.Errorf("system log stream scanner error: %w", scanErr)
	}
	if waitErr != nil && ctx.Err() == nil {
		return fmt.Errorf("system log stream failed: %w", waitErr)
	}

	return nil
}

// BuildDarwinLogShowArgs builds the arguments for `log show`.
func BuildDarwinLogShowArgs(req agentmgr.JournalQueryData) []string {
	args := []string{"show", "--style", "ndjson", "--color", "none", "--debug"}
	start, end, last := ResolveDarwinLogRange(req.Since, req.Until)
	if last != "" {
		args = append(args, "--last", last)
		return args
	}
	if start != "" {
		args = append(args, "--start", start)
	}
	if end != "" {
		args = append(args, "--end", end)
	}
	if start == "" && end == "" {
		args = append(args, "--last", "1h")
	}
	return args
}

// BuildDarwinLogStreamArgs builds the arguments for `log stream`.
func BuildDarwinLogStreamArgs() []string {
	// Use newline-delimited JSON so scanner-based parsing handles one event per line.
	// Restrict to log events and default+ to keep sustained CPU/load bounded on macOS.
	return []string{"stream", "--style", "ndjson", "--color", "none", "--type", "log", "--level", "default"}
}

// ResolveDarwinLogRange resolves the --start/--end/--last arguments for macOS log.
func ResolveDarwinLogRange(sinceRaw, untilRaw string) (start string, end string, last string) {
	since := strings.TrimSpace(sinceRaw)
	until := strings.TrimSpace(untilRaw)

	if since == "" && until == "" {
		return "", "", "1h"
	}

	if until == "" || strings.EqualFold(until, "now") {
		if rel := parseDarwinRelativeSince(since); rel != "" {
			return "", "", rel
		}
	}

	return since, until, ""
}

func parseDarwinRelativeSince(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return ""
	}
	matches := darwinRelativeSincePattern.FindStringSubmatch(value)
	if len(matches) != 3 {
		return ""
	}
	if _, err := strconv.Atoi(matches[1]); err != nil {
		return ""
	}
	return matches[1] + matches[2]
}

type timedLogEntry struct {
	entry agentmgr.LogStreamData
	when  time.Time
}

type osLogEntry struct {
	Timestamp       string `json:"timestamp"`
	EventMessage    string `json:"eventMessage"`
	ComposedMessage string `json:"composedMessage"`
	MessageType     string `json:"messageType"`
	Process         string `json:"process"`
	Subsystem       string `json:"subsystem"`
	ProcessPath     string `json:"processImagePath"`
	SenderPath      string `json:"senderImagePath"`
}

// ParseOSLogLine parses a single macOS log JSON line with timestamps.
func ParseOSLogLine(raw []byte) (agentmgr.LogStreamData, bool) {
	return parseOSLogLineWithTimestamp(raw, true)
}

// ParseOSLogLineForStream parses a single macOS log JSON line without timestamps.
func ParseOSLogLineForStream(raw []byte) (agentmgr.LogStreamData, bool) {
	return parseOSLogLineWithTimestamp(raw, false)
}

func parseOSLogLineWithTimestamp(raw []byte, includeTimestamp bool) (agentmgr.LogStreamData, bool) {
	if len(raw) == 0 {
		return agentmgr.LogStreamData{}, false
	}

	var parsed osLogEntry
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return agentmgr.LogStreamData{}, false
	}

	message := firstNonEmptyTrimmed(parsed.EventMessage, parsed.ComposedMessage)
	if message == "" {
		return agentmgr.LogStreamData{}, false
	}

	source := firstNonEmptyTrimmed(parsed.Subsystem, parsed.Process)
	if source == "" {
		source = osLogSourceFromPath(parsed.ProcessPath)
	}
	if source == "" {
		source = osLogSourceFromPath(parsed.SenderPath)
	}
	if source == "" {
		source = "oslog"
	}

	entry := agentmgr.LogStreamData{
		Level:   osLogMessageTypeToLevel(parsed.MessageType),
		Message: message,
		Source:  source,
	}
	if includeTimestamp {
		entry.Timestamp = osLogTimestampToRFC3339(parsed.Timestamp)
	}
	return entry, true
}

func firstNonEmptyTrimmed(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func osLogSourceFromPath(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	lastSlash := strings.LastIndexByte(value, '/')
	switch {
	case lastSlash == -1:
		return value
	case lastSlash == len(value)-1:
		return ""
	default:
		return value[lastSlash+1:]
	}
}

func osLogTimestampToRFC3339(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Now().UTC().Format(time.RFC3339)
	}

	layouts := []string{
		"2006-01-02 15:04:05.000000-0700",
		"2006-01-02 15:04:05.000-0700",
		"2006-01-02 15:04:05-0700",
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.000000 MST",
		"2006-01-02 15:04:05 MST",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC().Format(time.RFC3339)
		}
	}

	return time.Now().UTC().Format(time.RFC3339)
}

func osLogMessageTypeToLevel(messageType string) string {
	switch strings.ToLower(strings.TrimSpace(messageType)) {
	case "debug":
		return "debug"
	case "warning":
		return "warning"
	case "error", "fault", "critical":
		return "error"
	default:
		return "info"
	}
}

// ParseLogEntryTime parses an RFC3339 timestamp string.
func ParseLogEntryTime(raw string) time.Time {
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed
	}
	if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return parsed
	}
	return time.Unix(0, 0)
}
