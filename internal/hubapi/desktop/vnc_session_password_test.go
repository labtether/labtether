package desktop

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/terminal"
)

// The Hub makes a session password for every agent VNC session and the viewer
// sends it without asking. Only the Linux agent starts its VNC server with
// that password. On Windows and macOS the agent tunnels to the
// VNC server the person runs, so the password was a failed login there
// ("VNC security failure: 1 - authentication rejected") and the person was
// never asked for their own. The ticket must keep it from those viewers.
func TestStreamTicketOffersTheSessionPasswordOnlyWhereTheAgentStartedVNCWithIt(t *testing.T) {
	manager := agentmgr.NewManager()
	for id, platform := range map[string]string{"linux01": "linux", "other01": "plan9", "win01": "windows", "mac01": "darwin"} {
		manager.Register(&agentmgr.AgentConn{AssetID: id, Platform: platform})
	}
	var optionsMu sync.RWMutex
	options := map[string]DesktopSessionOptions{}
	deps := &Deps{
		AgentMgr:         manager,
		DesktopSessionMu: &optionsMu, DesktopSessionOpts: &options,
		EnforceRateLimit: func(http.ResponseWriter, *http.Request, string, int, time.Duration) bool { return true },
		IssueStreamTicket: func(context.Context, string) (string, time.Time, error) {
			return "ticket", time.Now().Add(time.Minute), nil
		},
	}

	for target, offered := range map[string]bool{"linux01": true, "other01": true, "win01": false, "mac01": false} {
		session := terminal.Session{ID: "sess-" + target, Target: target, Mode: "desktop"}
		deps.SetDesktopSessionOptions(session.ID, DesktopSessionOptions{Protocol: "vnc"})
		rec := httptest.NewRecorder()
		deps.HandleDesktopStreamTicket(rec, httptest.NewRequest(http.MethodPost, "/desktop/sessions/"+session.ID+"/stream-ticket", nil), session)
		if rec.Code != http.StatusCreated {
			t.Fatalf("%s: stream ticket %d %s", target, rec.Code, rec.Body.String())
		}
		var ticket struct {
			VNCPassword string `json:"vnc_password"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &ticket); err != nil {
			t.Fatalf("%s: decode ticket: %v", target, err)
		}
		if got := ticket.VNCPassword != ""; got != offered {
			t.Fatalf("%s: session password offered to the viewer = %t, want %t", target, got, offered)
		}
	}
}
