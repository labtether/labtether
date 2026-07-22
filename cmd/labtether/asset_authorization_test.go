package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
)

func TestGuardAssetRestrictedGlobalAPIFailsClosed(t *testing.T) {
	called := false
	handler := guardAssetRestrictedGlobalAPI("global settings", func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})
	ctx := apiv2.ContextWithAllowedAssets(context.Background(), []string{"asset-a"})
	req := httptest.NewRequest(http.MethodGet, "/settings/runtime", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	handler(rec, req)
	if called {
		t.Fatal("global handler must not run for an asset-restricted key")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGuardAssetRestrictedGlobalAPIAllowsUnrestrictedPrincipal(t *testing.T) {
	called := false
	handler := guardAssetRestrictedGlobalAPI("global settings", func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/settings/runtime", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)
	if !called || rec.Code != http.StatusNoContent {
		t.Fatalf("unrestricted handler call failed: called=%v status=%d", called, rec.Code)
	}
}
