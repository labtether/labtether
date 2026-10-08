package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

// ─── GET /api/v2/openapi.json ─────────────────────────────────────────────

func TestHandleV2OpenAPI_OK(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/openapi.json", nil)
	// No auth required.
	rec := httptest.NewRecorder()
	s.handleV2OpenAPI(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
	// Verify the response is valid JSON with openapi field.
	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("openapi spec is not valid JSON: %v", err)
	}
	if doc["openapi"] != "3.0.3" {
		t.Errorf("expected openapi 3.0.3, got %v", doc["openapi"])
	}
	if doc["paths"] == nil {
		t.Error("expected paths object in spec")
	}
}

func TestHandleV2OpenAPIAdvertisesImplementedAssetSubpaths(t *testing.T) {
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal([]byte(v2OpenAPISpec), &doc); err != nil {
		t.Fatalf("openapi spec is not valid JSON: %v", err)
	}

	expected := map[string][]string{
		"/api/v2/assets/{id}/exec":                    {"post"},
		"/api/v2/assets/{id}/files":                   {"get", "delete"},
		"/api/v2/assets/{id}/files/read":              {"get"},
		"/api/v2/assets/{id}/files/write":             {"post"},
		"/api/v2/assets/{id}/files/mkdir":             {"post"},
		"/api/v2/assets/{id}/files/rename":            {"post"},
		"/api/v2/assets/{id}/files/copy":              {"post"},
		"/api/v2/assets/{id}/processes":               {"get"},
		"/api/v2/assets/{id}/processes/kill":          {"post"},
		"/api/v2/assets/{id}/services":                {"get"},
		"/api/v2/assets/{id}/services/{name}/start":   {"post"},
		"/api/v2/assets/{id}/services/{name}/stop":    {"post"},
		"/api/v2/assets/{id}/services/{name}/restart": {"post"},
		"/api/v2/assets/{id}/network":                 {"get"},
		"/api/v2/assets/{id}/disks":                   {"get"},
		"/api/v2/assets/{id}/packages":                {"get"},
		"/api/v2/assets/{id}/packages/upgradable":     {"get"},
		"/api/v2/assets/{id}/packages/install":        {"post"},
		"/api/v2/assets/{id}/packages/update":         {"post"},
		"/api/v2/assets/{id}/packages/upgrade":        {"post"},
		"/api/v2/assets/{id}/cron":                    {"get"},
		"/api/v2/assets/{id}/users":                   {"get"},
		"/api/v2/assets/{id}/logs":                    {"get"},
	}
	for path, methods := range expected {
		pathItem, ok := doc.Paths[path]
		if !ok {
			t.Errorf("implemented asset path %q is absent from OpenAPI", path)
			continue
		}
		for _, method := range methods {
			if _, ok := pathItem[method]; !ok {
				t.Errorf("implemented asset operation %s %s is absent from OpenAPI", method, path)
			}
		}
	}
}

func TestHandleV2OpenAPIMatchesImplementedAdvancedMethods(t *testing.T) {
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal([]byte(v2OpenAPISpec), &doc); err != nil {
		t.Fatalf("openapi spec is not valid JSON: %v", err)
	}

	expected := map[string][]string{
		"/api/v2/assets/{id}":                       {"delete", "get", "patch", "put"},
		"/api/v2/groups/{id}":                       {"delete", "get", "patch", "put"},
		"/api/v2/docker/hosts":                      {"get"},
		"/api/v2/docker/hosts/{id}":                 {"get"},
		"/api/v2/docker/containers/{id}/action":     {"post"},
		"/api/v2/docker/stacks/{id}/action":         {"post"},
		"/api/v2/updates/plans/{id}":                {"delete", "get"},
		"/api/v2/updates/plans/{id}/execute":        {"post"},
		"/api/v2/updates/runs/{id}":                 {"delete", "get"},
		"/api/v2/alerts/{id}":                       {"delete", "get"},
		"/api/v2/alerts/{id}/ack":                   {"post"},
		"/api/v2/alerts/{id}/resolve":               {"post"},
		"/api/v2/alerts/rules/{id}":                 {"delete", "get", "patch", "put"},
		"/api/v2/incidents/{id}":                    {"delete", "get", "patch", "put"},
		"/api/v2/connectors/{id}/test":              {"post"},
		"/api/v2/connectors/{id}/discover":          {"get"},
		"/api/v2/connectors/{id}/health":            {"get"},
		"/api/v2/connectors/{id}/actions":           {"get"},
		"/api/v2/credentials/profiles/{id}":         {"delete", "get"},
		"/api/v2/credentials/profiles/{id}/rotate":  {"post"},
		"/api/v2/terminal/snippets/{id}":            {"delete", "get", "put"},
		"/api/v2/agents/{id}/settings":              {"get", "patch"},
		"/api/v2/hub/tailscale":                     {"get", "post"},
		"/api/v2/web-services":                      {"get", "post"},
		"/api/v2/web-services/{id}":                 {"delete", "patch", "put"},
		"/api/v2/collectors/{id}":                   {"delete", "get", "patch", "put"},
		"/api/v2/collectors/{id}/run":               {"post"},
		"/api/v2/notifications/channels":            {"get", "post"},
		"/api/v2/synthetic-checks/{id}":             {"delete", "get", "patch", "put"},
		"/api/v2/discovery/proposals/{id}/accept":   {"post"},
		"/api/v2/discovery/proposals/{id}/dismiss":  {"post"},
		"/api/v2/dependencies/{id}":                 {"delete", "get"},
		"/api/v2/dependencies/batch":                {"get"},
		"/api/v2/dependencies/graph":                {"get"},
		"/api/v2/edges":                             {"get", "post"},
		"/api/v2/edges/{id}":                        {"delete", "get", "patch"},
		"/api/v2/edges/tree":                        {"get"},
		"/api/v2/edges/ancestors":                   {"get"},
		"/api/v2/composites":                        {"post"},
		"/api/v2/composites/{id}":                   {"get", "patch"},
		"/api/v2/composites/{id}/members/{assetId}": {"delete"},
		"/api/v2/topology/zones":                    {"post"},
		"/api/v2/topology/zones/{id}":               {"delete", "put"},
		"/api/v2/topology/zones/{id}/members":       {"put"},
		"/api/v2/topology/zones/reorder":            {"put"},
		"/api/v2/topology/connections":              {"post"},
		"/api/v2/topology/connections/{id}":         {"delete", "put"},
		"/api/v2/topology/viewport":                 {"put"},
		"/api/v2/failover-pairs/{id}":               {"delete", "get", "patch", "put"},
		"/api/v2/logs/views/{id}":                   {"delete", "get", "patch", "put"},
		"/api/v2/settings/prometheus":               {"get", "patch"},
		"/api/v2/keys/{id}":                         {"delete", "get", "patch"},
	}

	for path, want := range expected {
		pathItem, ok := doc.Paths[path]
		if !ok {
			t.Errorf("implemented path %q is absent from OpenAPI", path)
			continue
		}
		got := make([]string, 0, len(pathItem))
		for key := range pathItem {
			if key != "parameters" {
				got = append(got, key)
			}
		}
		sort.Strings(got)
		sort.Strings(want)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("OpenAPI methods for %s = %v, want %v", path, got, want)
		}
	}

	for _, path := range []string{
		"/api/v2/agents/{id}",
		"/api/v2/connectors/{id}",
		"/api/v2/discovery/proposals/{id}",
		"/api/v2/docker/stacks/{id}",
	} {
		if _, ok := doc.Paths[path]; ok {
			t.Errorf("OpenAPI still advertises unimplemented path %q", path)
		}
	}
}

func TestHandleV2WebServicesPostCreatesManualService(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/web-services", strings.NewReader(
		`{"name":"Lab UI","category":"Infrastructure","url":"https://lab.example.test"}`,
	))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()

	s.handleV2WebServices(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"name":"Lab UI"`) {
		t.Fatalf("created service missing from response: %s", rec.Body.String())
	}
}

func TestHandleV2WebServicesRejectsUnadvertisedMethods(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v2/web-services", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()

	s.handleV2WebServices(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2HubStatusRejectsMutatingMethods(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/hub/status", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()

	s.handleV2HubStatus(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2OpenAPI_MethodNotAllowed(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/openapi.json", nil)
	rec := httptest.NewRecorder()
	s.handleV2OpenAPI(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}
