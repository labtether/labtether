package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/labtether/labtether/internal/auth"
)

// TestTerminalRoutesReachHandlersThroughRouteTable sends one signed-in admin
// request to each terminal route through the real route table and auth, and
// checks the answer only that route's handler gives.
//
// The terminal handler tests live in internal/hubapi/terminal and build
// their Deps with testutil/hubdeps. This test catches a fault in cmd's own
// wiring that those tests cannot see: a route bound to the wrong handler, or
// buildTerminalDeps leaving out a store (sessions, saved sessions, bookmarks,
// known hosts), the policy state, the rate limiter or the stream tickets.
// Routes that need Postgres get a malformed body, which their handler
// refuses before it reads the database.
func TestTerminalRoutesReachHandlersThroughRouteTable(t *testing.T) {
	sut := newTestAPIServer(t)
	// Production fills the bookmark store from Postgres; the test server has none.
	sut.terminalBookmarkStore = newMemoryTerminalBookmarkStore()
	admin := createRoleSessionToken(t, sut, "admin-terminal-wiring", auth.RoleAdmin)
	call := routeWiringCaller(t, sut.buildHTTPHandlers(nil, nil, nil), admin)

	session := call("/terminal/sessions", http.MethodPost, "/terminal/sessions", `{"target":"lab-host-01","mode":"interactive"}`, http.StatusCreated, `"target":"lab-host-01"`)
	sessionID := routeWiringID(t, session, "session")
	call("/terminal/sessions/", http.MethodPost, "/terminal/sessions/"+sessionID+"/stream-ticket", "", http.StatusCreated, `"stream_path"`)
	call("/terminal/commands/recent", http.MethodGet, "/terminal/commands/recent", "", http.StatusOK, `"commands":[]`)
	call("/terminal/commands/", http.MethodDelete, "/terminal/commands/cmd-missing", "", http.StatusNotFound, "command not found")
	saved := call("/terminal/persistent-sessions", http.MethodPost, "/terminal/persistent-sessions", `{"target":"lab-host-01","title":"Shell"}`, http.StatusCreated, `"title":"Shell"`)
	call("/terminal/persistent-sessions/", http.MethodGet, "/terminal/persistent-sessions/"+routeWiringID(t, saved, "persistent_session"), "", http.StatusOK, `"title":"Shell"`)
	bookmark := call("/terminal/bookmarks", http.MethodPost, "/terminal/bookmarks", `{"title":"nas01","host":"192.0.2.10","port":22}`, http.StatusCreated, `"title":"nas01"`)
	call("/terminal/bookmarks/", http.MethodGet, "/terminal/bookmarks/"+routeWiringID(t, bookmark, "bookmark"), "", http.StatusOK, `"title":"nas01"`)
	call("/terminal/host-keys/", http.MethodPost, "/terminal/host-keys/forget", `{"host":"192.0.2.10","port":22}`, http.StatusOK, `"removed":false`)
	call("/terminal/quick-session", http.MethodPost, "/terminal/quick-session", `{"host":"192.0.2.10"}`, http.StatusBadRequest, "username is required")
	call("/terminal/preferences", http.MethodPut, "/terminal/preferences", `{`, http.StatusBadRequest, "invalid terminal preferences payload")
	call("/terminal/snippets", http.MethodPost, "/terminal/snippets", `{`, http.StatusBadRequest, "invalid snippet payload")
	call("/terminal/snippets/", http.MethodPut, "/terminal/snippets/snip-1", `{`, http.StatusBadRequest, "invalid snippet payload")
	call("/terminal/workspace/tabs", http.MethodPost, "/terminal/workspace/tabs", `{`, http.StatusBadRequest, "invalid workspace tab payload")
	call("/terminal/workspace/tabs/", http.MethodPut, "/terminal/workspace/tabs/tab-1", `{`, http.StatusBadRequest, "invalid workspace tab payload")
}

// routeWiringID reads {"<key>":{"id":...}} from a response body.
func routeWiringID(t *testing.T, body, key string) string {
	t.Helper()
	var payload map[string]struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil || payload[key].ID == "" {
		t.Fatalf("no %s id in %s (err %v)", key, body, err)
	}
	return payload[key].ID
}
