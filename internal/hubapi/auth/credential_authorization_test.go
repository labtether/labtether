package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/persistence"
)

func TestRestrictedAPIKeyCannotAccessGlobalCredentialProfiles(t *testing.T) {
	store := persistence.NewMemoryCredentialStore()
	if _, err := store.CreateCredentialProfile(credentials.Profile{ID: "cred-secret", Name: "secret", Kind: "password"}); err != nil {
		t.Fatal(err)
	}
	d := &Deps{CredentialStore: store}
	ctx := apiv2.ContextWithAllowedAssets(context.Background(), []string{"asset-a"})

	for _, tc := range []struct {
		name   string
		method string
		path   string
	}{
		{name: "list", method: http.MethodGet, path: "/credentials/profiles"},
		{name: "create", method: http.MethodPost, path: "/credentials/profiles"},
		{name: "get", method: http.MethodGet, path: "/credentials/profiles/cred-secret"},
		{name: "delete", method: http.MethodDelete, path: "/credentials/profiles/cred-secret"},
		{name: "rotate", method: http.MethodPost, path: "/credentials/profiles/cred-secret/rotate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil).WithContext(ctx)
			rec := httptest.NewRecorder()
			if tc.path == "/credentials/profiles" {
				d.HandleCredentialProfiles(rec, req)
			} else {
				d.HandleCredentialProfileActions(rec, req)
			}
			if rec.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
			}
		})
	}

	if _, ok, err := store.GetCredentialProfile("cred-secret"); err != nil || !ok {
		t.Fatalf("restricted mutations changed credential store: ok=%v err=%v", ok, err)
	}
}

func TestInteractiveCredentialProfileMutationsRequireAdmin(t *testing.T) {
	d := &Deps{
		CredentialStore:     persistence.NewMemoryCredentialStore(),
		UserRoleFromContext: apiv2.UserRoleFromContext,
	}

	viewerContext := apiv2.ContextWithPrincipal(context.Background(), "usr-viewer", "viewer")
	for _, test := range []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/credentials/profiles"},
		{method: http.MethodDelete, path: "/credentials/profiles/cred-any"},
		{method: http.MethodPost, path: "/credentials/profiles/cred-any/rotate"},
	} {
		req := httptest.NewRequest(test.method, test.path, nil).WithContext(viewerContext)
		rec := httptest.NewRecorder()
		if test.path == "/credentials/profiles" {
			d.HandleCredentialProfiles(rec, req)
		} else {
			d.HandleCredentialProfileActions(rec, req)
		}
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s status=%d body=%s", test.method, test.path, rec.Code, rec.Body.String())
		}
	}

	readReq := httptest.NewRequest(http.MethodGet, "/credentials/profiles", nil).WithContext(viewerContext)
	readRec := httptest.NewRecorder()
	d.HandleCredentialProfiles(readRec, readReq)
	if readRec.Code != http.StatusOK {
		t.Fatalf("viewer list status=%d body=%s", readRec.Code, readRec.Body.String())
	}

	apiKeyContext := apiv2.ContextWithScopes(viewerContext, []string{"credentials:write"})
	apiKeyReq := httptest.NewRequest(http.MethodPost, "/credentials/profiles/cred-missing/rotate", nil).WithContext(apiKeyContext)
	apiKeyRec := httptest.NewRecorder()
	d.HandleCredentialProfileActions(apiKeyRec, apiKeyReq)
	if apiKeyRec.Code == http.StatusForbidden {
		t.Fatalf("explicitly scoped API key was rejected as an interactive viewer: %s", apiKeyRec.Body.String())
	}
}
