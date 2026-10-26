package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/auth"
)

func postRetentionSettings(t *testing.T, handler http.HandlerFunc, sessionToken, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/settings/retention", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: sessionToken})
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

// Operators could shrink the audit window and wipe the audit trail. Retention
// changes are admin-only, and even admins cannot go below the audit floor.
func TestRetentionChangesAreAdminOnlyAndKeepAuditFloor(t *testing.T) {
	sut := newTestAPIServer(t)
	handler := sut.buildHTTPHandlers(nil, nil, nil)["/settings/retention"]
	operator := createRoleSessionToken(t, sut, "operator-retention", auth.RoleOperator)
	admin := createRoleSessionToken(t, sut, "admin-retention", auth.RoleAdmin)
	viewer := createRoleSessionToken(t, sut, "viewer-retention", auth.RoleViewer)

	if rec := postRetentionSettings(t, handler, operator, `{"audit_window":"1h"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("operator retention change status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := postRetentionSettings(t, handler, admin, `{"audit_window":"1d"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("admin sub-floor audit window status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := postRetentionSettings(t, handler, admin, `{"audit_window":"30d"}`); rec.Code != http.StatusOK {
		t.Fatalf("admin audit window change status=%d body=%s", rec.Code, rec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/settings/retention", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: viewer})
	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("viewer retention read status=%d body=%s", rec.Code, rec.Body.String())
	}
}
