package system

import (
	"bufio"
	"bytes"
	"encoding/json"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/securityruntime"
)

// CollectUserSessionsFn is the function used to collect active user sessions.
// It can be overridden in tests.
var CollectUserSessionsFn = CollectUserSessions

// UsersManager handles active user session requests from the hub.
// It carries no persistent state; the struct exists for consistency
// with the other manager types.
type UsersManager struct{}

// NewUsersManager creates a new UsersManager.
func NewUsersManager() *UsersManager { return &UsersManager{} }

// CloseAll is a no-op for UsersManager -- user session requests are stateless
// and require no cleanup.
func (um *UsersManager) CloseAll() {}

// HandleUsersList collects active user sessions and sends them to the hub.
func (um *UsersManager) HandleUsersList(transport MessageSender, msg agentmgr.Message) {
	var req agentmgr.UsersListData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("users: invalid users.list request: %v", err)
		return
	}

	sessions, err := CollectUserSessionsFn()

	var errMsg string
	if err != nil {
		errMsg = err.Error()
		log.Printf("users: failed to collect user sessions: %v", err)
	}

	data, marshalErr := json.Marshal(agentmgr.UsersListedData{
		RequestID: req.RequestID,
		Sessions:  sessions,
		Error:     errMsg,
	})
	if marshalErr != nil {
		log.Printf("users: failed to marshal users.listed response: %v", marshalErr)
		return
	}

	if sendErr := transport.Send(agentmgr.Message{
		Type: agentmgr.MsgUsersListed,
		ID:   req.RequestID,
		Data: data,
	}); sendErr != nil {
		log.Printf("users: failed to send users.listed for request %s: %v", req.RequestID, sendErr)
	}
}

// CollectUserSessions runs `who` and parses the output to extract active user sessions.
// who output format: username tty    YYYY-MM-DD HH:MM (remote_host)
func CollectUserSessions() ([]agentmgr.UserSession, error) {
	whoBin, err := exec.LookPath("who")
	if err != nil {
		return nil, err
	}

	out, err := securityruntime.CommandCombinedOutput(whoBin)
	if err != nil {
		return nil, err
	}

	return ParseUserSessionsOutput(out), nil
}

// ParseUserSessionsOutput parses `who` command output into user sessions.
func ParseUserSessionsOutput(out []byte) []agentmgr.UserSession {
	var sessions []agentmgr.UserSession
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		session := agentmgr.UserSession{
			Username: fields[0],
			Terminal: fields[1],
		}

		loginFields := fields[2:]
		if n := len(loginFields); n > 0 {
			last := loginFields[n-1]
			if strings.HasPrefix(last, "(") && strings.HasSuffix(last, ")") {
				loginFields = loginFields[:n-1]
			}
		}

		// Try to parse ISO-style login times first.
		if len(loginFields) >= 2 {
			dateStr := loginFields[0] + " " + loginFields[1]
			if t, parseErr := time.Parse("2006-01-02 15:04", dateStr); parseErr == nil {
				session.LoginTime = t.Format(time.RFC3339)
			} else {
				session.LoginTime = strings.Join(loginFields, " ")
			}
		} else if len(loginFields) == 1 {
			session.LoginTime = loginFields[0]
		}

		// Remote host is in parentheses at end of line, if present.
		if idx := strings.LastIndex(line, "("); idx >= 0 {
			if endIdx := strings.LastIndex(line, ")"); endIdx > idx {
				host := line[idx+1 : endIdx]
				if host != "" {
					session.RemoteHost = host
				}
			}
		}

		sessions = append(sessions, session)
	}

	return sessions
}
