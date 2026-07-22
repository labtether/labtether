package schedulespkg

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/apikeys"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/schedules"
)

func TestRestrictedSchedulesFilterCollectionsAndRejectCrossAssetWrites(t *testing.T) {
	store := persistence.NewMemoryScheduleStore()
	d := &Deps{ScheduleStore: store}
	for _, task := range []schedules.ScheduledTask{
		{ID: "allowed", Name: "allowed", CronExpr: "0 * * * *", Command: "uptime", Targets: []string{"asset-a"}, CreatedAt: time.Now()},
		{ID: "secret", Name: "secret", CronExpr: "0 * * * *", Command: "uptime", Targets: []string{"asset-b"}, CreatedAt: time.Now()},
	} {
		if err := store.CreateScheduledTask(context.Background(), task); err != nil {
			t.Fatal(err)
		}
	}
	ctx := apiv2.ContextWithAllowedAssets(context.Background(), []string{"asset-a"})

	listReq := httptest.NewRequest(http.MethodGet, "/api/v2/schedules", nil).WithContext(ctx)
	listRec := httptest.NewRecorder()
	d.V2ListSchedules(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var envelope struct {
		Data []schedules.ScheduledTask `json:"data"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Data) != 1 || envelope.Data[0].ID != "allowed" {
		t.Fatalf("unexpected filtered schedules: %#v", envelope.Data)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v2/schedules", bytes.NewReader([]byte(`{
		"name":"mixed","cron_expr":"0 * * * *","command":"uptime","targets":["asset-a","asset-b"]
	}`))).WithContext(ctx)
	createRec := httptest.NewRecorder()
	d.V2CreateSchedule(createRec, createReq)
	if createRec.Code != http.StatusForbidden {
		t.Fatalf("create: expected 403, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v2/schedules/secret", nil).WithContext(ctx)
	deleteRec := httptest.NewRecorder()
	d.V2DeleteSchedule(deleteRec, deleteReq, "secret")
	if deleteRec.Code != http.StatusForbidden {
		t.Fatalf("delete: expected 403, got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}
	if _, ok, err := store.GetScheduledTask(context.Background(), "secret"); err != nil || !ok {
		t.Fatalf("forbidden delete reached store: ok=%v err=%v", ok, err)
	}
}

func TestAuthorizeExecutionTargetRevalidatesUserAndAPIKeyAuthority(t *testing.T) {
	authStore := persistence.NewMemoryAuthStore()
	operator, err := authStore.CreateUserWithRole("operator", "hash", auth.RoleOperator, "local", "")
	if err != nil {
		t.Fatalf("create operator: %v", err)
	}
	viewer, err := authStore.CreateUserWithRole("viewer", "hash", auth.RoleViewer, "local", "")
	if err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	keyStore := persistence.NewMemoryAPIKeyStore()
	now := time.Now().UTC()
	keys := []apikeys.APIKey{
		{ID: "allowed", Role: auth.RoleOperator, Scopes: []string{"actions:exec"}, AllowedAssets: []string{"asset-a"}, CreatedAt: now},
		{ID: "under-scoped", Role: auth.RoleOperator, Scopes: []string{"schedules:write"}, CreatedAt: now},
		{ID: "viewer", Role: auth.RoleViewer, Scopes: []string{"actions:exec"}, CreatedAt: now},
		{ID: "owner", Role: auth.RoleOwner, Scopes: []string{"actions:exec"}, CreatedAt: now},
	}
	for _, key := range keys {
		if err := keyStore.CreateAPIKey(context.Background(), key); err != nil {
			t.Fatalf("create key %s: %v", key.ID, err)
		}
	}
	deps := &Deps{AuthStore: authStore, APIKeyStore: keyStore}

	for _, tc := range []struct {
		name    string
		actorID string
		assetID string
		wantErr bool
	}{
		{name: "owner token", actorID: "owner", assetID: "asset-a"},
		{name: "operator user", actorID: operator.ID, assetID: "asset-a"},
		{name: "viewer user", actorID: viewer.ID, assetID: "asset-a", wantErr: true},
		{name: "missing user", actorID: "usr-missing", assetID: "asset-a", wantErr: true},
		{name: "allowed key", actorID: "apikey:allowed", assetID: "asset-a"},
		{name: "key asset denied", actorID: "apikey:allowed", assetID: "asset-b", wantErr: true},
		{name: "key scope denied", actorID: "apikey:under-scoped", assetID: "asset-a", wantErr: true},
		{name: "key role denied", actorID: "apikey:viewer", assetID: "asset-a", wantErr: true},
		{name: "reserved owner key denied", actorID: "apikey:owner", assetID: "asset-a", wantErr: true},
		{name: "missing key", actorID: "apikey:missing", assetID: "asset-a", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := deps.AuthorizeExecutionTarget(context.Background(), tc.actorID, tc.assetID)
			if (err != nil) != tc.wantErr {
				t.Fatalf("AuthorizeExecutionTarget() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
