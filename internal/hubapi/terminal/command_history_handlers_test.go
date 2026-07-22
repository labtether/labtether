package terminal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/persistence"
	terminalmodel "github.com/labtether/labtether/internal/terminal"
)

func TestHandleCommandActionsDeletesOwnedCompletedHistoryWithoutAuditingBody(t *testing.T) {
	store, command := seedHistoryCommand(t, "actor-a", "asset-a", "succeeded")
	var capturedAudit audit.Event
	var rateBucket string
	var rateLimit int
	deps := historyDeleteTestDeps(store, "actor-a", false)
	deps.EnforceRateLimit = func(_ http.ResponseWriter, _ *http.Request, bucket string, limit int, window time.Duration) bool {
		rateBucket, rateLimit = bucket, limit
		if window != time.Minute {
			t.Fatalf("rate window = %s, want 1m", window)
		}
		return true
	}
	deps.AppendAuditEventBestEffort = func(event audit.Event, _ string) { capturedAudit = event }

	req := httptest.NewRequest(http.MethodDelete, "/terminal/commands/"+command.ID, nil)
	rec := httptest.NewRecorder()
	deps.HandleCommandActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Deleted   bool   `json:"deleted"`
		CommandID string `json:"command_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Deleted || response.CommandID != command.ID {
		t.Fatalf("unexpected response: %+v", response)
	}
	if _, ok, err := store.GetCommand(command.ID); err != nil || ok {
		t.Fatalf("command still exists: ok=%t err=%v", ok, err)
	}
	if rateBucket != "terminal.command.delete" || rateLimit != terminalHistoryDeleteLimitPerMinute {
		t.Fatalf("rate policy = %q/%d", rateBucket, rateLimit)
	}
	if capturedAudit.Type != "terminal.command.deleted" || capturedAudit.ActorID != "actor-a" || capturedAudit.CommandID != command.ID {
		t.Fatalf("unexpected audit event: %+v", capturedAudit)
	}
	encodedAudit, err := json.Marshal(capturedAudit)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedAudit), command.Body) {
		t.Fatalf("deletion audit exposed command body: %s", encodedAudit)
	}
}

func TestHandleCommandActionsEnforcesOwnershipAssetScopeAndTerminalState(t *testing.T) {
	for _, tc := range []struct {
		name          string
		commandActor  string
		requestActor  string
		owner         bool
		status        string
		allowedAssets []string
		wantStatus    int
		wantDeleted   bool
	}{
		{name: "different actor denied", commandActor: "actor-a", requestActor: "actor-b", status: "succeeded", wantStatus: http.StatusForbidden},
		{name: "owner may delete another actor record", commandActor: "actor-a", requestActor: "owner", owner: true, status: "failed", wantStatus: http.StatusOK, wantDeleted: true},
		{name: "asset restricted key denied", commandActor: "actor-a", requestActor: "actor-a", status: "succeeded", allowedAssets: []string{"asset-b"}, wantStatus: http.StatusForbidden},
		{name: "queued command remains durable", commandActor: "actor-a", requestActor: "actor-a", status: "queued", wantStatus: http.StatusConflict},
		{name: "unknown legacy status fails closed", commandActor: "actor-a", requestActor: "actor-a", status: "mystery", wantStatus: http.StatusConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, command := seedHistoryCommand(t, tc.commandActor, "asset-a", tc.status)
			deps := historyDeleteTestDeps(store, tc.requestActor, tc.owner)
			req := httptest.NewRequest(http.MethodDelete, "/terminal/commands/"+command.ID, nil)
			role := "admin"
			if tc.owner {
				role = "owner"
			}
			req = req.WithContext(apiv2.ContextWithPrincipal(req.Context(), tc.requestActor, role))
			if tc.allowedAssets != nil {
				req = req.WithContext(apiv2.ContextWithAllowedAssets(req.Context(), tc.allowedAssets))
			}
			rec := httptest.NewRecorder()
			deps.HandleCommandActions(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			_, exists, err := store.GetCommand(command.ID)
			if err != nil {
				t.Fatal(err)
			}
			if exists == tc.wantDeleted {
				t.Fatalf("command existence = %t, want deleted=%t", exists, tc.wantDeleted)
			}
		})
	}
}

func TestHandleCommandActionsReturnsHonestNotFoundAndRejectsInvalidPath(t *testing.T) {
	store := persistence.NewMemoryTerminalStore()
	deps := historyDeleteTestDeps(store, "actor-a", false)

	for _, path := range []string{
		"/terminal/commands/missing",
		"/terminal/commands/",
		"/terminal/commands/nested/value",
	} {
		req := httptest.NewRequest(http.MethodDelete, path, nil)
		rec := httptest.NewRecorder()
		deps.HandleCommandActions(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("path %q status = %d, want 404: %s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestHandleCommandActionsHonorsRateLimiterBeforeLookup(t *testing.T) {
	store := persistence.NewMemoryTerminalStore()
	deps := historyDeleteTestDeps(store, "actor-a", false)
	deps.EnforceRateLimit = func(w http.ResponseWriter, _ *http.Request, _ string, _ int, _ time.Duration) bool {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return false
	}

	req := httptest.NewRequest(http.MethodDelete, "/terminal/commands/not-present", nil)
	rec := httptest.NewRecorder()
	deps.HandleCommandActions(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
}

func seedHistoryCommand(t *testing.T, actorID, target, status string) (*persistence.MemoryTerminalStore, terminalmodel.Command) {
	t.Helper()
	store := persistence.NewMemoryTerminalStore()
	session, err := store.CreateSession(terminalmodel.CreateSessionRequest{
		ActorID: actorID,
		Target:  target,
		Mode:    "interactive",
	})
	if err != nil {
		t.Fatal(err)
	}
	command, err := store.AddCommand(session.ID, terminalmodel.CreateCommandRequest{
		ActorID: actorID,
		Command: "printf 'secret command text'",
	}, target, session.Mode)
	if err != nil {
		t.Fatal(err)
	}
	if status != "queued" {
		if err := store.UpdateCommandResult(session.ID, command.ID, status, "output"); err != nil {
			t.Fatal(err)
		}
		command.Status = status
	}
	return store, command
}

func historyDeleteTestDeps(store persistence.TerminalStore, actorID string, _ bool) *Deps {
	return &Deps{
		TerminalStore: store,
		EnforceRateLimit: func(http.ResponseWriter, *http.Request, string, int, time.Duration) bool {
			return true
		},
		PrincipalActorID: func(context.Context) string { return actorID },
		AppendAuditEventBestEffort: func(audit.Event, string) {
		},
		MaxTargetLength: 255,
	}
}
