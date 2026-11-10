package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/audit"
)

// GET /audit/events returned every row to any signed-in user or API key, so a
// viewer could read what the owner and admins did. The owner, admins and
// admin API keys read the whole log; everyone else reads only their own rows
// (owner decision, 2026-11-10), by every route that returns audit rows: both
// audit endpoints and the status call. Paging is done over their own rows.
func TestAuditEventsReadScopeFollowsRole(t *testing.T) {
	s := newTestAPIServer(t)
	base := time.Now().UTC().Add(-time.Hour)
	for i, actor := range []string{"usr-owner", "usr-viewer", "usr-admin", "apikey:key-limited", "usr-viewer", "apikey:key-admin", "usr-operator"} {
		if err := s.auditStore.Append(audit.Event{ID: "audit-scope-" + string(rune('a'+i)), Type: "webhook.created", ActorID: actor,
			Target: "wh-" + string(rune('a'+i)), Timestamp: base.Add(time.Duration(i) * time.Minute)}); err != nil {
			t.Fatalf("seed audit row: %v", err)
		}
	}
	read := func(handler http.HandlerFunc, path, actorID, role string) []string {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		ctx := contextWithPrincipal(req.Context(), actorID, role)
		if keyID, isKey := strings.CutPrefix(actorID, "apikey:"); isKey {
			ctx = contextWithAPIKeyID(ctx, keyID)
			ctx = contextWithScopes(ctx, []string{"audit:read"})
		}
		rec := httptest.NewRecorder()
		handler(rec, req.WithContext(ctx))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s as %s = %d: %s", path, actorID, rec.Code, rec.Body.String())
		}
		var body struct {
			Events      []audit.Event `json:"events"`
			RecentAudit []audit.Event `json:"recentAudit"`
			Data        struct {
				Events []audit.Event `json:"events"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		actors := []string{}
		for _, event := range append(append(body.Events, body.Data.Events...), body.RecentAudit...) {
			actors = append(actors, event.ActorID)
		}
		sort.Strings(actors)
		return actors
	}
	const everyone = "apikey:key-admin apikey:key-limited usr-admin usr-operator usr-owner usr-viewer usr-viewer"
	cases := []struct{ actor, role, want string }{
		{"usr-owner", "owner", everyone},
		{"usr-admin", "admin", everyone},
		{"apikey:key-admin", "admin", everyone},
		{"usr-viewer", "viewer", "usr-viewer usr-viewer"},
		{"usr-operator", "operator", "usr-operator"},
		{"apikey:key-limited", "operator", "apikey:key-limited"},
		{"usr-new", "viewer", ""},
	}
	for _, tc := range cases {
		if got := strings.Join(read(s.handleAuditEvents, "/audit/events", tc.actor, tc.role), " "); got != tc.want {
			t.Errorf("/audit/events as %s (%s) shows rows by [%s], want [%s]", tc.actor, tc.role, got, tc.want)
		}
		if got := strings.Join(read(s.handleV2AuditEvents, "/api/v2/audit/events", tc.actor, tc.role), " "); got != tc.want {
			t.Errorf("/api/v2/audit/events as %s (%s) shows rows by [%s], want [%s]", tc.actor, tc.role, got, tc.want)
		}
	}
	// The console's status call carries the newest audit rows; same rule there.
	status := s.handleStatusAggregate(nil, nil)
	if got := strings.Join(read(status, "/status/aggregate", "usr-viewer", "viewer"), " "); got != "usr-viewer usr-viewer" {
		t.Errorf("status recentAudit as the viewer shows rows by [%s], want only the viewer's", got)
	}
	if got := strings.Join(read(status, "/status/aggregate", "usr-admin", "admin"), " "); got != everyone {
		t.Errorf("status recentAudit as an admin shows rows by [%s], want everyone's", got)
	}
	// Paging counts the reader's own rows: the viewer's second row is on page 2.
	if got := read(s.handleAuditEvents, "/audit/events?limit=1&offset=1", "usr-viewer", "viewer"); len(got) != 1 || got[0] != "usr-viewer" {
		t.Errorf("viewer page 2 of own rows = %v, want one own row", got)
	}
}
