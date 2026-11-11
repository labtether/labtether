package terminal

import (
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/persistence"
	terminalmodel "github.com/labtether/labtether/internal/terminal"
)

// A saved shell stayed under Running sessions as "Detached" after the user
// typed exit, although its tmux session was gone. It now moves to Ended, but
// only when the agent can tell an exit from a detach: a session that is still
// running on the device must stay attachable.
func TestSavedShellMovesToEndedOnlyWhenItsShellExited(t *testing.T) {
	cases := []struct {
		name          string
		reportsDetach string
		reason        string
		want          string
	}{
		{"shell exited", "true", "shell exited", "archived"},
		{"tmux client detached", "true", "tmux client detached", "detached"},
		{"older agent cannot tell", "", "shell exited", "detached"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := persistence.NewMemoryTerminalStore()
			saved, err := store.CreateOrUpdatePersistentSession(terminalmodel.CreatePersistentSessionRequest{ActorID: "owner", Target: "node-1", Title: "shell"})
			if err != nil {
				t.Fatal(err)
			}
			session := terminalmodel.Session{ID: "public-saved", Target: "node-1", PersistentSessionID: saved.ID, TmuxSessionName: saved.TmuxSessionName}
			d, conn, peer, url, finished := newAgentTerminalHarness(t, session)
			d.TerminalPersistentStore = store
			conn.SetMeta(agentmgr.MetaTerminalReportsTmuxDetach, tc.reportsDetach)

			browser := dialAgentTerminalTestBrowser(t, url+"?request=saved")
			defer browser.Close()
			start := readAgentTerminalStart(t, readAgentTerminalWire(t, peer))
			d.ProcessAgentTerminalStarted(conn, agentTerminalMessage(t, agentmgr.TerminalStartedData{SessionID: start.SessionID, TmuxAttached: true}))
			readAgentTerminalReady(t, browser)

			d.ProcessAgentTerminalClosed(conn, agentTerminalMessage(t, agentmgr.TerminalCloseData{SessionID: start.SessionID, Reason: tc.reason}))
			select {
			case <-finished:
			case <-time.After(3 * time.Second):
				t.Fatal("terminal handler did not end")
			}

			got, ok, err := store.GetPersistentSession(saved.ID)
			if err != nil || !ok {
				t.Fatalf("saved shell is missing: ok=%v err=%v", ok, err)
			}
			if got.Status != tc.want {
				t.Fatalf("saved shell status = %q, want %q", got.Status, tc.want)
			}
		})
	}
}
