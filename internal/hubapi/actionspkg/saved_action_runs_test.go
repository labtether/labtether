package actionspkg

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/savedactions"
)

// Running a saved action never reached Recent runs: the steps ran, but no
// action run was stored. Each step must leave one run, refused steps too,
// and only the person who ran it (or the owner) may read those runs.
func TestSavedActionRunStepsAppearInRunHistoryForTheirRunner(t *testing.T) {
	assetStore := persistence.NewMemoryAssetStore()
	seedSavedActionTestAsset(t, assetStore, "allowed")
	savedStore := persistence.NewMemorySavedActionStore()
	action := savedactions.SavedAction{
		ID: "act_history", Name: "history", CreatedBy: "actor-a", CreatedAt: time.Now().UTC(),
		Steps: []savedactions.ActionStep{
			{Name: "one", Command: "uptime", Target: "allowed"},
			{Name: "two", Command: "uname -a", Target: "allowed"},
		},
	}
	if err := savedStore.CreateSavedAction(context.Background(), action); err != nil {
		t.Fatalf("seed action: %v", err)
	}
	currentPolicy := policy.DefaultEvaluatorConfig()
	runStore := persistence.NewMemoryActionStore()
	deps := Deps{
		SavedActionStore: savedStore,
		AssetStore:       assetStore,
		ActionStore:      runStore,
		GetPolicyConfig:  func() policy.EvaluatorConfig { return currentPolicy },
		ExecOnAsset: func(*http.Request, string, string, int) ExecResult {
			currentPolicy.StructuredEnabled = false // the second step is refused
			return ExecResult{Status: actions.StatusSucceeded, ExitCode: new(0), Stdout: "up 3 days"}
		},
	}

	rec := httptest.NewRecorder()
	deps.HandleV2SavedActionActions(rec, savedActionRequest(http.MethodPost, "/api/v2/actions/"+action.ID+"/run", "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("run status = %d: %s", rec.Code, rec.Body.String())
	}

	listAs := func(actorID, role string) []actions.Run {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/actions/runs", nil)
		req = req.WithContext(apiv2.ContextWithPrincipal(req.Context(), actorID, role))
		listRec := httptest.NewRecorder()
		deps.HandleActionRuns(listRec, req)
		if listRec.Code != http.StatusOK {
			t.Fatalf("list runs as %s = %d: %s", actorID, listRec.Code, listRec.Body.String())
		}
		var body struct {
			Runs []actions.Run `json:"runs"`
		}
		if err := json.Unmarshal(listRec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode runs: %v", err)
		}
		return body.Runs
	}

	runs := listAs("actor-a", "operator")
	if len(runs) != 2 {
		t.Fatalf("runner sees %d runs, want one per step: %+v", len(runs), runs)
	}
	byCommand := map[string]actions.Run{}
	for _, run := range runs {
		byCommand[run.Command] = run
	}
	if got := byCommand["uptime"]; got.Status != actions.StatusSucceeded || got.Target != "allowed" || got.Output != "up 3 days" || got.CompletedAt == nil {
		t.Fatalf("dispatched step run = %+v, want succeeded on allowed with its output", got)
	}
	if got := byCommand["uname -a"]; got.Status != actions.StatusFailed || got.Error == "" {
		t.Fatalf("refused step run = %+v, want failed with the reason", got)
	}

	if others := listAs("actor-b", "operator"); len(others) != 0 {
		t.Fatalf("another operator sees %d saved-action runs, want none", len(others))
	}
	if owner := listAs("owner-1", "owner"); len(owner) != 2 {
		t.Fatalf("owner sees %d runs, want 2", len(owner))
	}
}
