package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/apikeys"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/persistence"
)

// An admin must not be able to make or change another admin, whether by
// creating a user, promoting one, pointing SSO at groups or an IdP that grant
// admin, or editing another user's admin API key. Admins still manage
// operators and viewers, and the owner can do all of it.
func TestOnlyOwnerCanMakeOrManageAdmins(t *testing.T) {
	deps, store := newTestAuthDeps(t)
	deps.SettingsStore = &oidcSettingsStoreStub{}
	deps.APIKeyStore = persistence.NewMemoryAPIKeyStore()
	callerRole := auth.RoleAdmin
	deps.UserRoleFromContext = func(context.Context) string { return callerRole }

	hash, _ := auth.HashPassword("testpassword123")
	operator, _ := store.CreateUserWithRole("ops-user", hash, auth.RoleOperator, "local", "")
	if err := deps.APIKeyStore.CreateAPIKey(context.Background(), apikeys.APIKey{
		ID: "key_other_admin", Name: "automation", Role: auth.RoleAdmin, Scopes: []string{"*"}, CreatedBy: "other-admin",
	}); err != nil {
		t.Fatal(err)
	}

	call := func(handler http.HandlerFunc, method, path, body string) int {
		rec := httptest.NewRecorder()
		handler(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rec.Code
	}
	newAdmin := `{"username":"new-admin","password":"LongEnoughPassword123!","role":"admin"}`
	refused := []struct {
		name    string
		handler http.HandlerFunc
		method  string
		path    string
		body    string
	}{
		{"create admin", deps.HandleAuthUsers, http.MethodPost, "/auth/users", newAdmin},
		{"promote to admin", deps.HandleAuthUserActions, http.MethodPatch, "/auth/users/" + operator.ID, `{"role":"admin"}`},
		{"change SSO settings", deps.HandleOIDCSettingsPut, http.MethodPut, "/settings/oidc", `{"enabled":false,"admin_role_values":"staff"}`},
		{"change another admin's key", deps.HandleAPIKeyActions, http.MethodPatch, "/api/v2/keys/key_other_admin", `{"scopes":["assets:read"]}`},
		{"delete another admin's key", deps.HandleAPIKeyActions, http.MethodDelete, "/api/v2/keys/key_other_admin", ""},
	}
	for _, tc := range refused {
		if code := call(tc.handler, tc.method, tc.path, tc.body); code != http.StatusForbidden {
			t.Fatalf("admin %s: status=%d, want 403", tc.name, code)
		}
	}
	if _, exists, _ := store.GetUserByUsername("new-admin"); exists {
		t.Fatal("an admin created another admin")
	}
	if got, _, _ := store.GetUserByID(operator.ID); got.Role != auth.RoleOperator {
		t.Fatalf("an admin promoted a user to %q", got.Role)
	}
	if key, found, _ := deps.APIKeyStore.GetAPIKey(context.Background(), "key_other_admin"); !found || len(key.Scopes) != 1 || key.Scopes[0] != "*" {
		t.Fatalf("an admin changed another admin's key: found=%v key=%+v", found, key)
	}

	newOperator := `{"username":"new-operator","password":"LongEnoughPassword123!","role":"operator"}`
	if code := call(deps.HandleAuthUsers, http.MethodPost, "/auth/users", newOperator); code != http.StatusCreated {
		t.Fatalf("admin creating an operator: status=%d, want 201", code)
	}

	callerRole = auth.RoleOwner
	if code := call(deps.HandleAuthUsers, http.MethodPost, "/auth/users", newAdmin); code != http.StatusCreated {
		t.Fatalf("owner creating an admin: status=%d, want 201", code)
	}
	if code := call(deps.HandleAuthUserActions, http.MethodPatch, "/auth/users/"+operator.ID, `{"role":"admin"}`); code != http.StatusOK {
		t.Fatalf("owner promoting to admin: status=%d, want 200", code)
	}
	if code := call(deps.HandleAPIKeyActions, http.MethodDelete, "/api/v2/keys/key_other_admin", ""); code != http.StatusOK {
		t.Fatalf("owner deleting an admin key: status=%d, want 200", code)
	}
}
