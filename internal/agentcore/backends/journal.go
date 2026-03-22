package backends

import (
	"encoding/json"
	"log"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

// journalEntry represents the relevant fields from a journalctl --output=json line.
type journalEntry struct {
	RealtimeTimestamp string `json:"__REALTIME_TIMESTAMP"`
	SystemdUnit       string `json:"_SYSTEMD_UNIT"`
	Priority          string `json:"PRIORITY"`
	Message           string `json:"MESSAGE"`
	SyslogIdentifier  string `json:"SYSLOG_IDENTIFIER"`
}

// JournalManager handles historical journalctl queries from the hub.
type JournalManager struct {
	Backend LogBackend
}

// NewJournalManager creates a JournalManager with the OS-appropriate backend.
func NewJournalManager() *JournalManager {
	return &JournalManager{
		Backend: NewLogBackendForOS(),
	}
}

// CloseAll is a no-op for JournalManager — journal queries are stateless.
func (jm *JournalManager) CloseAll() {}

// HandleJournalQuery runs a historical journal query and sends entries to the hub.
func (jm *JournalManager) HandleJournalQuery(transport MessageSender, msg agentmgr.Message) {
	var req agentmgr.JournalQueryData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("journal: invalid journal.query request: %v", err)
		return
	}

	entries, err := jm.Backend.QueryEntries(req)
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
		log.Printf("journal: failed to query entries: %v", err)
	}

	data, marshalErr := json.Marshal(agentmgr.JournalEntriesData{
		RequestID: req.RequestID,
		Entries:   entries,
		Error:     errMsg,
	})
	if marshalErr != nil {
		log.Printf("journal: failed to marshal journal.entries response: %v", marshalErr)
		return
	}

	if sendErr := transport.Send(agentmgr.Message{
		Type: agentmgr.MsgJournalEntries,
		ID:   req.RequestID,
		Data: data,
	}); sendErr != nil {
		log.Printf("journal: failed to send journal.entries for request %s: %v", req.RequestID, sendErr)
	}
}

// ParseJournalLine decodes a single journalctl JSON line into a LogStreamData
// entry. Returns (entry, false) when the line is empty or unparseable.
func ParseJournalLine(raw []byte) (agentmgr.LogStreamData, bool) {
	if len(raw) == 0 {
		return agentmgr.LogStreamData{}, false
	}

	var je journalEntry
	if err := json.Unmarshal(raw, &je); err != nil {
		return agentmgr.LogStreamData{}, false
	}

	if je.Message == "" {
		return agentmgr.LogStreamData{}, false
	}

	// Convert microsecond epoch string to RFC3339.
	ts := journalTimestampToRFC3339(je.RealtimeTimestamp)

	// Derive a human-readable source: prefer SYSLOG_IDENTIFIER, fall back to
	// the systemd unit name (stripped of the ".service" suffix).
	source := je.SyslogIdentifier
	if source == "" {
		source = je.SystemdUnit
		// Strip ".service" suffix for brevity.
		if len(source) > 8 && source[len(source)-8:] == ".service" {
			source = source[:len(source)-8]
		}
	}

	return agentmgr.LogStreamData{
		Timestamp: ts,
		Level:     journalPriorityToLevel(je.Priority),
		Message:   je.Message,
		Source:    source,
	}, true
}

// journalPriorityToLevel maps syslog priority integers (as strings) to level names.
//
// Syslog priorities: 0=emerg, 1=alert, 2=crit, 3=err, 4=warning, 5=notice, 6=info, 7=debug.
func journalPriorityToLevel(priority string) string {
	switch priority {
	case "0", "1", "2", "3":
		return "error"
	case "4":
		return "warning"
	case "5":
		return "notice"
	case "7":
		return "debug"
	default:
		// Covers "6" (info) and any unrecognised value.
		return "info"
	}
}

// journalTimestampToRFC3339 converts a journalctl __REALTIME_TIMESTAMP (microseconds
// since Unix epoch as a decimal string) to an RFC3339 timestamp string.
// Returns the current time formatted as RFC3339 if conversion fails.
func journalTimestampToRFC3339(microsStr string) string {
	if microsStr == "" {
		return time.Now().UTC().Format(time.RFC3339)
	}

	var micros int64
	for _, c := range microsStr {
		if c < '0' || c > '9' {
			return time.Now().UTC().Format(time.RFC3339)
		}
		micros = micros*10 + int64(c-'0')
	}

	secs := micros / 1_000_000
	nanos := (micros % 1_000_000) * 1_000
	return time.Unix(secs, nanos).UTC().Format(time.RFC3339)
}
