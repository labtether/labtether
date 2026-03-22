package backends

import (
	"bufio"
	"bytes"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

// CronManager handles cron/timer visibility requests from the hub.
type CronManager struct {
	Backend CronBackend
}

// NewCronManager creates a CronManager with the OS-appropriate backend.
func NewCronManager() *CronManager {
	return &CronManager{
		Backend: NewCronBackendForOS(),
	}
}

// CloseAll is a no-op for CronManager — cron requests are stateless
// and require no cleanup.
func (cm *CronManager) CloseAll() {}

// HandleCronList collects cron jobs and systemd timers and sends them to the hub.
func (cm *CronManager) HandleCronList(transport MessageSender, msg agentmgr.Message) {
	var req agentmgr.CronListData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("cron: invalid cron.list request: %v", err)
		return
	}

	entries, collectErr := cm.Backend.ListEntries()
	if collectErr != nil {
		log.Printf("cron: failed to collect schedules: %v", collectErr)
	}

	var errMsg string
	if collectErr != nil && len(entries) == 0 {
		errMsg = collectErr.Error()
	}

	data, marshalErr := json.Marshal(agentmgr.CronListedData{
		RequestID: req.RequestID,
		Entries:   entries,
		Error:     errMsg,
	})
	if marshalErr != nil {
		log.Printf("cron: failed to marshal cron.listed response: %v", marshalErr)
		return
	}

	if sendErr := transport.Send(agentmgr.Message{
		Type: agentmgr.MsgCronListed,
		ID:   req.RequestID,
		Data: data,
	}); sendErr != nil {
		log.Printf("cron: failed to send cron.listed for request %s: %v", req.RequestID, sendErr)
	}
}

// CollectSystemdTimers runs `systemctl list-timers --all --no-pager --plain`
// and parses the output to extract timer entries.
func CollectSystemdTimers() ([]agentmgr.CronEntry, error) {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return nil, nil // systemd not available, skip silently
	}

	out, err := exec.Command("systemctl", "list-timers", "--all", "--no-pager", "--plain").CombinedOutput()
	if err != nil {
		return nil, err
	}

	var entries []agentmgr.CronEntry
	scanner := bufio.NewScanner(bytes.NewReader(out))
	headerSkipped := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// Skip the header line and the summary line at the end.
		if !headerSkipped {
			if strings.HasPrefix(line, "NEXT") {
				headerSkipped = true
			}
			continue
		}
		// The summary line starts with a digit (e.g., "13 timers listed.")
		if len(line) > 0 && line[0] >= '0' && line[0] <= '9' {
			break
		}

		// --plain format columns: NEXT LEFT LAST PASSED UNIT ACTIVATES
		// NEXT and LAST are multi-word timestamps (e.g., "Sun 2026-02-23 12:00:00 UTC")
		// We parse by splitting on whitespace and working with the known field count.
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}

		// The UNIT is the second-to-last field, ACTIVATES is the last field.
		unit := fields[len(fields)-2]
		activates := fields[len(fields)-1]

		// Try to parse NEXT — first 4 fields might be the timestamp (day, date, time, tz)
		// or it could be "n/a" if no next run.
		var nextRun string
		nextStr := strings.Join(fields[:4], " ")
		if !strings.Contains(nextStr, "n/a") {
			if t, parseErr := ParseSystemdTime(nextStr); parseErr == nil {
				nextRun = t.Format(time.RFC3339)
			}
		}

		entries = append(entries, agentmgr.CronEntry{
			Source:   "systemd-timer",
			Schedule: unit,
			Command:  activates,
			User:     "root",
			NextRun:  nextRun,
		})
	}
	return entries, nil
}

// ParseSystemdTime attempts to parse a systemd timer timestamp like
// "Sun 2026-02-23 12:00:00 UTC".
func ParseSystemdTime(s string) (time.Time, error) {
	// Try common formats.
	layouts := []string{
		"Mon 2006-01-02 15:04:05 MST",
		"Mon 2006-01-02 15:04:05 -0700",
		"2006-01-02 15:04:05 MST",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, &CronError{"unparseable time: " + s}
}

// CronError represents a cron parsing error.
type CronError struct{ Msg string }

func (e *CronError) Error() string { return e.Msg }

// CollectCrontabs reads user crontab files and /etc/cron.d/* entries.
func CollectCrontabs() ([]agentmgr.CronEntry, error) {
	return CollectCrontabsFromPaths(
		[]string{"/var/spool/cron/crontabs", "/var/spool/cron"},
		"/etc/cron.d",
		"/etc/crontab",
	)
}

// CollectCrontabsFromPaths reads crontab files from the given paths.
func CollectCrontabsFromPaths(userDirs []string, cronDDir, systemCrontabPath string) ([]agentmgr.CronEntry, error) {
	var entries []agentmgr.CronEntry

	// User crontabs from /var/spool/cron/crontabs/ (Debian/Ubuntu)
	// or /var/spool/cron/ (RHEL/CentOS).
	for _, dir := range userDirs {
		dirEntries, err := os.ReadDir(dir)
		if err != nil {
			continue // permission denied or doesn't exist — skip
		}
		for _, de := range dirEntries {
			if de.IsDir() {
				continue
			}
			user := de.Name()
			parsed := ParseCrontabFile(filepath.Join(dir, user), user, false)
			entries = append(entries, parsed...)
		}
	}

	// System crontabs from /etc/cron.d/
	dirEntries, err := os.ReadDir(cronDDir)
	if err == nil {
		for _, de := range dirEntries {
			if de.IsDir() {
				continue
			}
			parsed := ParseCrontabFile(filepath.Join(cronDDir, de.Name()), "", true)
			entries = append(entries, parsed...)
		}
	}

	// System crontab from /etc/crontab
	if strings.TrimSpace(systemCrontabPath) != "" {
		entries = append(entries, ParseCrontabFile(systemCrontabPath, "", true)...)
	}

	return entries, nil
}

// ParseCrontabFile reads a crontab file and extracts cron entries.
// If systemStyle is true, the 6th field is the user (as in /etc/cron.d/* files).
func ParseCrontabFile(path, defaultUser string, systemStyle bool) []agentmgr.CronEntry {
	data, err := os.ReadFile(path) // #nosec G304 -- Path comes from enumerated cron directories under controlled system locations.
	if err != nil {
		return nil // permission denied — skip
	}

	var entries []agentmgr.CronEntry
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Skip environment variable assignments (e.g., SHELL=/bin/bash).
		if strings.Contains(line, "=") && !strings.HasPrefix(line, "*") &&
			!strings.HasPrefix(line, "@") && (len(line) < 2 || line[0] < '0' || line[0] > '9') {
			continue
		}

		// Handle @reboot, @hourly, etc. shortcuts.
		if strings.HasPrefix(line, "@") {
			fields := strings.Fields(line)
			minFields := 2
			if systemStyle {
				minFields = 3
			}
			if len(fields) < minFields {
				continue
			}
			schedule := fields[0]
			user := defaultUser
			cmdStart := 1
			if systemStyle {
				user = fields[1]
				cmdStart = 2
			}
			rest := strings.Join(fields[cmdStart:], " ")
			entries = append(entries, agentmgr.CronEntry{
				Source:   "crontab",
				Schedule: schedule,
				Command:  rest,
				User:     user,
			})
			continue
		}

		// Standard 5-field cron schedule.
		fields := strings.Fields(line)
		minFields := 6 // 5 schedule + 1 command
		if systemStyle {
			minFields = 7 // 5 schedule + 1 user + 1 command
		}
		if len(fields) < minFields {
			continue
		}

		schedule := strings.Join(fields[:5], " ")
		user := defaultUser
		cmdStart := 5
		if systemStyle {
			user = fields[5]
			cmdStart = 6
		}
		command := strings.Join(fields[cmdStart:], " ")

		entries = append(entries, agentmgr.CronEntry{
			Source:   "crontab",
			Schedule: schedule,
			Command:  command,
			User:     user,
		})
	}
	return entries
}
