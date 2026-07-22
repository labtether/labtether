package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestV2AlertRuleObjectRouteDispatchesToRuleHandler(t *testing.T) {
	s := &apiServer{}
	ctx := contextWithScopes(context.Background(), []string{"alerts:read"})
	req := httptest.NewRequest(http.MethodGet, "/api/v2/alerts/rules/rule-1", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	s.handleV2AlertActions(rec, req)

	if req.URL.Path != "/alerts/rules/rule-1" {
		t.Fatalf("expected rule action rewrite, got %q", req.URL.Path)
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected rule handler to report missing store, got %d body=%s", rec.Code, rec.Body.String())
	}
}
