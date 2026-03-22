package backends

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/securityruntime"
)

const launchdPlistCommandTimeout = 5 * time.Second

// DarwinCronBackend implements CronBackend using launchd and crontabs.
type DarwinCronBackend struct{}

// ListEntries lists launchd entries and crontab entries.
func (DarwinCronBackend) ListEntries() ([]agentmgr.CronEntry, error) {
	entries := make([]agentmgr.CronEntry, 0)

	launchdEntries, launchdErr := collectLaunchdEntries()
	entries = append(entries, launchdEntries...)

	crontabEntries, crontabErr := CollectCrontabs()
	if crontabErr == nil {
		entries = append(entries, crontabEntries...)
	}

	if len(entries) == 0 {
		if launchdErr != nil {
			return nil, launchdErr
		}
		if crontabErr != nil {
			return nil, fmt.Errorf("crontabs: %w", crontabErr)
		}
	}

	return entries, nil
}

func collectLaunchdEntries() ([]agentmgr.CronEntry, error) {
	if _, err := exec.LookPath("plutil"); err != nil {
		return nil, fmt.Errorf("plutil is not available on this host")
	}

	currentUser := strings.TrimSpace(os.Getenv("USER"))
	homeDir, _ := os.UserHomeDir()
	directories := []struct {
		path string
		user string
	}{
		{path: "/System/Library/LaunchDaemons", user: "root"},
		{path: "/Library/LaunchDaemons", user: "root"},
		{path: "/Library/LaunchAgents", user: currentUser},
	}
	if strings.TrimSpace(homeDir) != "" {
		directories = append(directories, struct {
			path string
			user string
		}{
			path: filepath.Join(homeDir, "Library", "LaunchAgents"),
			user: currentUser,
		})
	}

	entries := make([]agentmgr.CronEntry, 0)
	var firstErr error
	for _, directory := range directories {
		dirEntries, err := os.ReadDir(directory.path)
		if err != nil {
			continue
		}
		for _, dirEntry := range dirEntries {
			if dirEntry.IsDir() {
				continue
			}
			if !strings.HasSuffix(strings.ToLower(dirEntry.Name()), ".plist") {
				continue
			}

			path := filepath.Join(directory.path, dirEntry.Name())
			entry, ok, parseErr := parseLaunchdPlist(path, directory.user)
			if parseErr != nil {
				if firstErr == nil {
					firstErr = parseErr
				}
				continue
			}
			if !ok {
				continue
			}
			entries = append(entries, entry)
		}
	}

	if len(entries) == 0 && firstErr != nil {
		return nil, firstErr
	}

	return entries, nil
}

func parseLaunchdPlist(path, user string) (agentmgr.CronEntry, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), launchdPlistCommandTimeout)
	defer cancel()

	out, err := securityruntime.CommandContextCombinedOutput(ctx, "plutil", "-convert", "json", "-o", "-", path)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return agentmgr.CronEntry{}, false, fmt.Errorf("launchd plist parse timed out: %s", path)
		}
		trimmed := strings.TrimSpace(string(out))
		if trimmed != "" {
			return agentmgr.CronEntry{}, false, fmt.Errorf("launchd plist parse failed for %s: %s", path, trimmed)
		}
		return agentmgr.CronEntry{}, false, fmt.Errorf("launchd plist parse failed for %s: %w", path, err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		return agentmgr.CronEntry{}, false, fmt.Errorf("launchd plist JSON decode failed for %s: %w", path, err)
	}

	entry, ok := BuildLaunchdCronEntry(payload, user)
	return entry, ok, nil
}

// BuildLaunchdCronEntry builds a CronEntry from a launchd plist payload.
func BuildLaunchdCronEntry(payload map[string]any, user string) (agentmgr.CronEntry, bool) {
	label := strings.TrimSpace(asLaunchdString(payload["Label"]))
	command := buildLaunchdCommand(payload)
	if label == "" && command == "" {
		return agentmgr.CronEntry{}, false
	}
	if command == "" {
		command = label
	}
	if strings.TrimSpace(user) == "" {
		user = "unknown"
	}
	schedule := BuildLaunchdSchedule(payload)
	if schedule == "" {
		schedule = "on-demand"
	}
	return agentmgr.CronEntry{
		Source:   "launchd",
		Schedule: schedule,
		Command:  command,
		User:     strings.TrimSpace(user),
	}, true
}

func buildLaunchdCommand(payload map[string]any) string {
	if args, ok := payload["ProgramArguments"].([]any); ok && len(args) > 0 {
		parts := make([]string, 0, len(args))
		for _, arg := range args {
			value := strings.TrimSpace(asLaunchdString(arg))
			if value == "" {
				continue
			}
			parts = append(parts, value)
		}
		if len(parts) > 0 {
			return strings.Join(parts, " ")
		}
	}
	return strings.TrimSpace(asLaunchdString(payload["Program"]))
}

// BuildLaunchdSchedule builds the schedule string from a launchd plist payload.
func BuildLaunchdSchedule(payload map[string]any) string {
	parts := make([]string, 0, 3)

	if interval, ok := asLaunchdInt(payload["StartInterval"]); ok && interval > 0 {
		parts = append(parts, fmt.Sprintf("every %ds", interval))
	}

	if calendarRaw, ok := payload["StartCalendarInterval"]; ok {
		if calendar := buildLaunchdCalendarSchedule(calendarRaw); calendar != "" {
			parts = append(parts, calendar)
		}
	}

	if runAtLoad, ok := payload["RunAtLoad"].(bool); ok && runAtLoad {
		parts = append(parts, "@reboot")
	}

	return strings.Join(parts, " | ")
}

func buildLaunchdCalendarSchedule(raw any) string {
	switch value := raw.(type) {
	case map[string]any:
		return launchdCalendarMapToCron(value)
	case []any:
		calendars := make([]string, 0, len(value))
		for _, item := range value {
			itemMap, ok := item.(map[string]any)
			if !ok {
				continue
			}
			cron := launchdCalendarMapToCron(itemMap)
			if cron != "" {
				calendars = append(calendars, cron)
			}
		}
		if len(calendars) == 0 {
			return ""
		}
		return strings.Join(calendars, " | ")
	default:
		return ""
	}
}

func launchdCalendarMapToCron(value map[string]any) string {
	minute := launchdCronField(value, "Minute")
	hour := launchdCronField(value, "Hour")
	day := launchdCronField(value, "Day")
	month := launchdCronField(value, "Month")
	weekday := launchdCronField(value, "Weekday")
	return strings.Join([]string{minute, hour, day, month, weekday}, " ")
}

func launchdCronField(value map[string]any, key string) string {
	if parsed, ok := asLaunchdInt(value[key]); ok {
		return strconv.Itoa(parsed)
	}
	return "*"
}

func asLaunchdInt(value any) (int, bool) {
	switch parsed := value.(type) {
	case int:
		return parsed, true
	case int32:
		return int(parsed), true
	case int64:
		return int(parsed), true
	case float64:
		return int(parsed), true
	case float32:
		return int(parsed), true
	case json.Number:
		n, err := strconv.Atoi(parsed.String())
		return n, err == nil
	default:
		return 0, false
	}
}

func asLaunchdString(value any) string {
	switch parsed := value.(type) {
	case string:
		return parsed
	case []byte:
		return string(parsed)
	case fmt.Stringer:
		return parsed.String()
	default:
		return ""
	}
}
