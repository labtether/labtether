package apiv2

import (
	"net/http/httptest"
	"testing"
)

func TestScopeCheck_Allowed(t *testing.T) {
	if !ScopeCheck([]string{"assets:read", "files:*"}, "assets:read") {
		t.Error("should allow exact scope match")
	}
}

func TestScopeCheck_Denied(t *testing.T) {
	if ScopeCheck([]string{"assets:read"}, "docker:write") {
		t.Error("should deny scope not in list")
	}
}

func TestScopeCheck_NilScopes_Allows(t *testing.T) {
	if !ScopeCheck(nil, "anything:here") {
		t.Error("nil scopes (session auth) should allow everything")
	}
}

func TestAssetCheck(t *testing.T) {
	if !AssetCheck(nil, "server1") {
		t.Error("nil allowlist should permit all")
	}
	if !AssetCheck([]string{}, "server1") {
		t.Error("empty allowlist should permit all")
	}
	if !AssetCheck([]string{"server1"}, "server1") {
		t.Error("should allow listed asset")
	}
	if AssetCheck([]string{"server1"}, "server2") {
		t.Error("should deny unlisted asset")
	}
}

func TestWriteScopeForbidden(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteScopeForbidden(rec, "docker:write")
	if rec.Code != 403 {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestWriteAssetForbidden(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteAssetForbidden(rec, "server1")
	if rec.Code != 403 {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}
