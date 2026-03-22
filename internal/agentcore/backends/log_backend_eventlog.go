package backends

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/securityruntime"
)

const (
	wevtutilQueryCommandTimeout  = 20 * time.Second
	wevtutilStreamPollInterval   = 5 * time.Second
	wevtutilDefaultChannels      = "System,Application"
)

// WindowsLogBackend implements LogBackend using wevtutil on Windows.
// It has no build tags so that parser tests can run on any platform.
type WindowsLogBackend struct {
	// Channels is the comma-separated list of Event Log channels to query.
	// Defaults to "System,Application" if empty.
	Channels string
}

// RunWevtutilCommand is the function used to execute wevtutil. Overridable for tests.
var RunWevtutilCommand = securityruntime.CommandContextCombinedOutput

// QueryEntries queries Windows Event Log entries via wevtutil across configured channels.
func (b WindowsLogBackend) QueryEntries(req agentmgr.JournalQueryData) ([]agentmgr.LogStreamData, error) {
	channels := b.resolvedChannels()

	limit := NormalizedJournalLimit(req.Limit)
	perChannel := limit
	if len(channels) > 1 {
		// Distribute the limit across channels; we will sort and trim after merging.
		perChannel = limit
	}

	var all []agentmgr.LogStreamData
	for _, ch := range channels {
		// Filter by unit: skip channels that don't match when a unit filter is set.
		if unit := strings.TrimSpace(req.Unit); unit != "" {
			if !strings.EqualFold(ch, unit) {
				continue
			}
		}

		entries, err := b.queryChannel(req, ch, perChannel)
		if err != nil {
			// Non-fatal: skip channels that fail (e.g. missing permissions).
			continue
		}
		all = append(all, entries...)
	}

	// Apply search filter and trim to limit.
	filtered := make([]agentmgr.LogStreamData, 0, len(all))
	for _, e := range all {
		if EntryMatchesQuery(e, req) {
			filtered = append(filtered, e)
		}
	}
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	return filtered, nil
}

// StreamEntries polls wevtutil periodically to simulate streaming (Event Log has no follow mode).
func (b WindowsLogBackend) StreamEntries(ctx context.Context, emit func(agentmgr.LogStreamData)) error {
	channels := b.resolvedChannels()

	// Track the latest timestamp seen per channel to use as a pagination cursor.
	cursors := make(map[string]string, len(channels))
	for _, ch := range channels {
		cursors[ch] = time.Now().UTC().Format(time.RFC3339)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(wevtutilStreamPollInterval):
		}

		for _, ch := range channels {
			since := cursors[ch]
			req := agentmgr.JournalQueryData{
				Limit: 100,
				Since: since,
			}
			entries, err := b.queryChannel(req, ch, 100)
			if err != nil {
				continue
			}
			for _, e := range entries {
				emit(e)
				if e.Timestamp > cursors[ch] {
					cursors[ch] = e.Timestamp
				}
			}
		}
	}
}

func (b WindowsLogBackend) resolvedChannels() []string {
	raw := strings.TrimSpace(b.Channels)
	if raw == "" {
		raw = wevtutilDefaultChannels
	}
	parts := strings.Split(raw, ",")
	channels := make([]string, 0, len(parts))
	for _, p := range parts {
		ch := strings.TrimSpace(p)
		if ch != "" {
			channels = append(channels, ch)
		}
	}
	if len(channels) == 0 {
		return []string{"System", "Application"}
	}
	return channels
}

func (b WindowsLogBackend) queryChannel(req agentmgr.JournalQueryData, channel string, limit int) ([]agentmgr.LogStreamData, error) {
	overrideReq := req
	overrideReq.Limit = limit

	ctx, cancel := context.WithTimeout(context.Background(), wevtutilQueryCommandTimeout)
	defer cancel()

	args := BuildWevtutilQueryArgs(overrideReq, channel)
	out, err := RunWevtutilCommand(ctx, "wevtutil", args...)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("wevtutil query timed out for channel %s", channel)
		}
		trimmed := strings.TrimSpace(string(out))
		if trimmed != "" {
			return nil, fmt.Errorf("wevtutil query failed for channel %s: %s", channel, trimmed)
		}
		return nil, fmt.Errorf("wevtutil query failed for channel %s: %w", channel, err)
	}

	return parseWevtutilOutput(out)
}

// BuildWevtutilQueryArgs builds the argument list for a wevtutil qe command.
func BuildWevtutilQueryArgs(req agentmgr.JournalQueryData, channel string) []string {
	limit := NormalizedJournalLimit(req.Limit)
	args := []string{
		"qe", channel,
		"/f:json",
		"/c:" + strconv.Itoa(limit),
	}
	if since := strings.TrimSpace(req.Since); since != "" {
		query := "*[System[TimeCreated[@SystemTime>='" + since + "']]]"
		args = append(args, "/q:"+query)
	}
	return args
}

// wevtutilEvent is the JSON structure produced by wevtutil qe /f:json.
// Each event is a single JSON object on its own line.
type wevtutilEvent struct {
	Event struct {
		System struct {
			Provider struct {
				Name string `json:"Name"`
			} `json:"Provider"`
			Level       string `json:"Level"`
			TimeCreated struct {
				SystemTime string `json:"SystemTime"`
			} `json:"TimeCreated"`
		} `json:"System"`
		RenderingInfo struct {
			Message string `json:"Message"`
			Level   string `json:"Level"`
		} `json:"RenderingInfo"`
	} `json:"Event"`
}

// parseWevtutilOutput parses newline-delimited JSON from wevtutil qe /f:json.
// Each line is an independent JSON object representing one event.
func parseWevtutilOutput(raw []byte) ([]agentmgr.LogStreamData, error) {
	var entries []agentmgr.LogStreamData

	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 256*1024), 256*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var ev wevtutilEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			// Skip malformed lines — wevtutil output may include BOM or preamble.
			continue
		}

		message := strings.TrimSpace(ev.Event.RenderingInfo.Message)
		if message == "" {
			continue
		}

		source := strings.TrimSpace(ev.Event.System.Provider.Name)
		if source == "" {
			source = "eventlog"
		}

		level := wevtutilLevelToString(parseWevtutilLevelNum(ev.Event.System.Level))
		timestamp := wevtutilSystemTimeToRFC3339(ev.Event.System.TimeCreated.SystemTime)

		entries = append(entries, agentmgr.LogStreamData{
			Timestamp: timestamp,
			Level:     level,
			Message:   message,
			Source:    source,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to parse wevtutil output: %w", err)
	}

	return entries, nil
}

// parseWevtutilLevelNum converts the Level string from wevtutil JSON to an integer.
// Windows Event Log levels are: 0=LogAlways/Info, 1=Critical, 2=Error, 3=Warning, 4=Info, 5=Verbose.
func parseWevtutilLevelNum(raw string) int {
	v := strings.TrimSpace(raw)
	if v == "" {
		return 4
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 4
	}
	return n
}

// wevtutilLevelToString maps Windows Event Log numeric levels to LabTether log level strings.
// Windows levels: 0=LogAlways(info), 1=Critical, 2=Error, 3=Warning, 4=Information, 5=Verbose(info).
func wevtutilLevelToString(level int) string {
	switch level {
	case 1:
		return "critical"
	case 2:
		return "error"
	case 3:
		return "warning"
	default:
		// 0 (LogAlways/success), 4 (Information), 5 (Verbose), unknown
		return "info"
	}
}

// wevtutilSystemTimeToRFC3339 converts the wevtutil SystemTime format to RFC3339.
// wevtutil emits timestamps like "2026-03-21T08:12:34.000000000Z".
func wevtutilSystemTimeToRFC3339(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Now().UTC().Format(time.RFC3339)
	}

	layouts := []string{
		"2006-01-02T15:04:05.000000000Z",
		"2006-01-02T15:04:05.000000000Z07:00",
		time.RFC3339Nano,
		time.RFC3339,
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC().Format(time.RFC3339)
		}
	}

	return time.Now().UTC().Format(time.RFC3339)
}
