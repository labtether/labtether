package actionspkg

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/savedactions"
)

func savedActionRequest(method, path, body string, allowedAssets []string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	ctx := apiv2.ContextWithPrincipal(req.Context(), "actor-a", "operator")
	ctx = apiv2.ContextWithScopes(ctx, []string{"actions:read", "actions:write", "actions:exec", "assets:exec"})
	ctx = apiv2.ContextWithAllowedAssets(ctx, allowedAssets)
	return req.WithContext(ctx)
}

func seedSavedActionTestAsset(t *testing.T, store *persistence.MemoryAssetStore, id string) {
	t.Helper()
	if _, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: id,
		Name:    id,
		Source:  "agent",
		Type:    "host",
		Status:  "online",
	}); err != nil {
		t.Fatalf("seed asset %q: %v", id, err)
	}
}

func TestSavedActionRestrictedCreateRejectsAllTargetsAtomically(t *testing.T) {
	assetStore := persistence.NewMemoryAssetStore()
	seedSavedActionTestAsset(t, assetStore, "allowed")
	seedSavedActionTestAsset(t, assetStore, "denied")
	actionStore := persistence.NewMemorySavedActionStore()
	deps := Deps{SavedActionStore: actionStore, AssetStore: assetStore}

	req := savedActionRequest(http.MethodPost, "/api/v2/actions",
		`{"name":"mixed","steps":[{"name":"one","command":"echo allowed","target":"allowed"},{"name":"two","command":"echo denied","target":"denied"}]}`,
		[]string{"allowed"})
	rec := httptest.NewRecorder()
	deps.HandleV2SavedActions(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", rec.Code, rec.Body.String())
	}
	list, total, err := actionStore.ListSavedActions(context.Background(), "actor-a", 100, 0)
	if err != nil || total != 0 || len(list) != 0 {
		t.Fatalf("rejected create persisted state: total=%d list=%v err=%v", total, list, err)
	}
}

func TestSavedActionCreateRejectsMissingTargetAtomically(t *testing.T) {
	assetStore := persistence.NewMemoryAssetStore()
	actionStore := persistence.NewMemorySavedActionStore()
	deps := Deps{SavedActionStore: actionStore, AssetStore: assetStore}
	req := savedActionRequest(http.MethodPost, "/api/v2/actions",
		`{"name":"missing","steps":[{"command":"uptime","target":"missing"}]}`, nil)
	rec := httptest.NewRecorder()
	deps.HandleV2SavedActions(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	list, total, err := actionStore.ListSavedActions(context.Background(), "actor-a", 100, 0)
	if err != nil || total != 0 || len(list) != 0 {
		t.Fatalf("missing-target create persisted state: total=%d list=%v err=%v", total, list, err)
	}
}

func TestSavedActionVisibilityAndExecutionAreAllOrNothing(t *testing.T) {
	assetStore := persistence.NewMemoryAssetStore()
	seedSavedActionTestAsset(t, assetStore, "allowed")
	seedSavedActionTestAsset(t, assetStore, "denied")
	actionStore := persistence.NewMemorySavedActionStore()
	now := time.Now().UTC()
	fullyAllowed := savedactions.SavedAction{
		ID: "act_allowed", Name: "allowed action", CreatedBy: "actor-a", CreatedAt: now,
		Steps: []savedactions.ActionStep{{Name: "one", Command: "echo ok", Target: "allowed"}},
	}
	mixed := savedactions.SavedAction{
		ID: "act_mixed", Name: "mixed action", CreatedBy: "actor-a", CreatedAt: now.Add(-time.Second),
		Steps: []savedactions.ActionStep{
			{Name: "one", Command: "echo ok", Target: "allowed"},
			{Name: "two", Command: "echo secret", Target: "denied"},
		},
	}
	missing := savedactions.SavedAction{
		ID: "act_missing", Name: "missing action", CreatedBy: "actor-a", CreatedAt: now.Add(-2 * time.Second),
		Steps: []savedactions.ActionStep{{Name: "gone", Command: "echo gone", Target: "missing"}},
	}
	for _, action := range []savedactions.SavedAction{fullyAllowed, mixed, missing} {
		if err := actionStore.CreateSavedAction(context.Background(), action); err != nil {
			t.Fatalf("seed action %q: %v", action.ID, err)
		}
	}
	execCalls := 0
	deps := Deps{
		SavedActionStore: actionStore,
		AssetStore:       assetStore,
		ExecOnAsset: func(*http.Request, string, string, int) ExecResult {
			execCalls++
			return ExecResult{ExitCode: 0, Stdout: "ok"}
		},
	}

	listRec := httptest.NewRecorder()
	deps.HandleV2SavedActions(listRec, savedActionRequest(http.MethodGet, "/api/v2/actions", "", []string{"allowed"}))
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", listRec.Code, listRec.Body.String())
	}
	var listEnvelope struct {
		Data []savedactions.SavedAction `json:"data"`
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listEnvelope); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listEnvelope.Data) != 1 || listEnvelope.Data[0].ID != fullyAllowed.ID || listEnvelope.Meta.Total != 1 {
		t.Fatalf("restricted list leaked mixed action: %+v", listEnvelope)
	}

	for _, action := range []savedactions.SavedAction{mixed, missing} {
		for _, methodAndPath := range [][2]string{
			{http.MethodGet, "/api/v2/actions/" + action.ID},
			{http.MethodDelete, "/api/v2/actions/" + action.ID},
			{http.MethodPost, "/api/v2/actions/" + action.ID + "/run"},
		} {
			rec := httptest.NewRecorder()
			deps.HandleV2SavedActionActions(rec, savedActionRequest(methodAndPath[0], methodAndPath[1], "", []string{"allowed"}))
			if rec.Code != http.StatusNotFound {
				t.Errorf("%s %s status = %d, want 404: %s", methodAndPath[0], methodAndPath[1], rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), action.ID) || strings.Contains(rec.Body.String(), action.Steps[0].Target) {
				t.Errorf("%s %s leaked inaccessible action details: %s", methodAndPath[0], methodAndPath[1], rec.Body.String())
			}
		}
	}
	if execCalls != 0 {
		t.Fatalf("inaccessible action dispatched %d commands", execCalls)
	}
	if _, ok, err := actionStore.GetSavedAction(context.Background(), "actor-a", mixed.ID); err != nil || !ok {
		t.Fatalf("inaccessible delete mutated action: ok=%v err=%v", ok, err)
	}
	if _, ok, err := actionStore.GetSavedAction(context.Background(), "actor-a", missing.ID); err != nil || !ok {
		t.Fatalf("missing-target delete mutated action: ok=%v err=%v", ok, err)
	}
}

func TestSavedActionRunResultsAndAuditsDoNotEchoCommands(t *testing.T) {
	assetStore := persistence.NewMemoryAssetStore()
	seedSavedActionTestAsset(t, assetStore, "allowed")
	actionStore := persistence.NewMemorySavedActionStore()
	secretCommand := "echo do-not-leak-this-command"
	action := savedactions.SavedAction{
		ID: "act_redacted", Name: "private action name", CreatedBy: "actor-a", CreatedAt: time.Now().UTC(),
		Steps: []savedactions.ActionStep{{Name: "private step name", Command: secretCommand, Target: "allowed"}},
	}
	if err := actionStore.CreateSavedAction(context.Background(), action); err != nil {
		t.Fatalf("seed action: %v", err)
	}
	var events []audit.Event
	deps := Deps{
		SavedActionStore: actionStore,
		AssetStore:       assetStore,
		ExecOnAsset: func(*http.Request, string, string, int) ExecResult {
			return ExecResult{Error: "exec_failed", Message: "command failed"}
		},
		AppendAuditEventBestEffort: func(event audit.Event, _ string) { events = append(events, event) },
	}
	rec := httptest.NewRecorder()
	deps.HandleV2SavedActionActions(rec, savedActionRequest(http.MethodPost, "/api/v2/actions/"+action.ID+"/run", "", []string{"allowed"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("run status = %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), secretCommand) {
		t.Fatalf("run result echoed command: %s", rec.Body.String())
	}
	encodedEvents, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal events: %v", err)
	}
	if strings.Contains(string(encodedEvents), secretCommand) || strings.Contains(string(encodedEvents), action.Name) || strings.Contains(string(encodedEvents), action.Steps[0].Name) {
		t.Fatalf("audit events contained saved-action content: %s", encodedEvents)
	}
}

type capacitySavedActionStore struct {
	persistence.SavedActionStore
}

func (capacitySavedActionStore) CreateSavedAction(context.Context, savedactions.SavedAction) error {
	return savedactions.ErrCapacity
}

func TestSavedActionCapacityReturnsConflict(t *testing.T) {
	assetStore := persistence.NewMemoryAssetStore()
	seedSavedActionTestAsset(t, assetStore, "allowed")
	deps := Deps{
		SavedActionStore: capacitySavedActionStore{SavedActionStore: persistence.NewMemorySavedActionStore()},
		AssetStore:       assetStore,
	}
	rec := httptest.NewRecorder()
	deps.HandleV2SavedActions(rec, savedActionRequest(http.MethodPost, "/api/v2/actions",
		`{"name":"at capacity","steps":[{"command":"uptime","target":"allowed"}]}`, nil))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "capacity_reached") {
		t.Fatalf("capacity response = %d: %s", rec.Code, rec.Body.String())
	}
}
