package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTerminalCommandDeleteRoutesRequireAuthenticationAndWriteScope(t *testing.T) {
	sut := newTestAPIServer(t)
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	legacy := handlers["/terminal/commands/"]
	if legacy == nil {
		t.Fatal("legacy command-delete route is not registered")
	}
	unauthenticated := httptest.NewRecorder()
	legacy(unauthenticated, httptest.NewRequest(http.MethodDelete, "/terminal/commands/cmd-missing", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", unauthenticated.Code)
	}

	readKey := createLegacyRouteAPIKey(t, sut, []string{"terminal:read"}, nil)
	readOnly := invokeLegacyRoute(
		t,
		legacy,
		http.MethodDelete,
		"/terminal/commands/cmd-missing",
		readKey,
		"",
	)
	if readOnly.Code != http.StatusForbidden {
		t.Fatalf("read-only status = %d, want 403: %s", readOnly.Code, readOnly.Body.String())
	}

	writeKey := createLegacyRouteAPIKey(t, sut, []string{"terminal:write"}, nil)
	missing := invokeLegacyRoute(
		t,
		legacy,
		http.MethodDelete,
		"/terminal/commands/cmd-missing",
		writeKey,
		"",
	)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("write-scoped missing status = %d, want 404: %s", missing.Code, missing.Body.String())
	}

	v2 := handlers["/api/v2/terminal/history/"]
	if v2 == nil {
		t.Fatal("v2 command-delete route is not registered")
	}
	v2ReadOnly := invokeLegacyRoute(
		t,
		v2,
		http.MethodDelete,
		"/api/v2/terminal/history/cmd-missing",
		readKey,
		"",
	)
	if v2ReadOnly.Code != http.StatusForbidden {
		t.Fatalf("v2 read-only status = %d, want 403: %s", v2ReadOnly.Code, v2ReadOnly.Body.String())
	}
}
