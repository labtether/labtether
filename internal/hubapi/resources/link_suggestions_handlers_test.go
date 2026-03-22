package resources

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/dependencies"
	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/persistence"
)

func createTestAssetForLinks(t *testing.T, store persistence.AssetStore, id, assetType, name string) {
	t.Helper()
	testutil.CreateTestAsset(t, store, id, assetType, name)
}

// ---------------------------------------------------------------------------
// GET /links/suggestions
// ---------------------------------------------------------------------------

func TestLinkSuggestionsGetEmpty(t *testing.T) {
	deps := newTestResourcesDeps(t)

	req := httptest.NewRequest(http.MethodGet, "/links/suggestions", nil)
	rec := httptest.NewRecorder()
	deps.HandleLinkSuggestions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Suggestions []persistence.LinkSuggestion `json:"suggestions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Suggestions) != 0 {
		t.Errorf("expected 0 suggestions, got %d", len(resp.Suggestions))
	}
}

func TestLinkSuggestionsGetPopulated(t *testing.T) {
	deps := newTestResourcesDeps(t)

	createTestAssetForLinks(t, deps.AssetStore, "host-1", "server", "Host 1")
	createTestAssetForLinks(t, deps.AssetStore, "vm-1", "vm", "VM 1")

	sug, err := deps.LinkSuggestionStore.CreateLinkSuggestion("host-1", "vm-1", "ip_match", 0.85)
	if err != nil {
		t.Fatalf("create suggestion: %v", err)
	}
	if sug.ID == "" {
		t.Fatal("expected suggestion ID")
	}

	req := httptest.NewRequest(http.MethodGet, "/links/suggestions", nil)
	rec := httptest.NewRecorder()
	deps.HandleLinkSuggestions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Suggestions []persistence.LinkSuggestion `json:"suggestions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(resp.Suggestions))
	}
	if resp.Suggestions[0].SourceAssetID != "host-1" {
		t.Errorf("expected source host-1, got %s", resp.Suggestions[0].SourceAssetID)
	}
	if resp.Suggestions[0].TargetAssetID != "vm-1" {
		t.Errorf("expected target vm-1, got %s", resp.Suggestions[0].TargetAssetID)
	}
	if resp.Suggestions[0].MatchReason != "ip_match" {
		t.Errorf("expected reason ip_match, got %s", resp.Suggestions[0].MatchReason)
	}
}

// ---------------------------------------------------------------------------
// PUT /links/suggestions/{id} -- accept
// ---------------------------------------------------------------------------

func TestLinkSuggestionAcceptCreatesDependency(t *testing.T) {
	deps := newTestResourcesDeps(t)
	depStore := newTestDependencyStore()
	deps.DependencyStore = depStore

	createTestAssetForLinks(t, deps.AssetStore, "host-1", "server", "Host 1")
	createTestAssetForLinks(t, deps.AssetStore, "vm-1", "vm", "VM 1")

	sug, err := deps.LinkSuggestionStore.CreateLinkSuggestion("host-1", "vm-1", "ip_match", 0.85)
	if err != nil {
		t.Fatalf("create suggestion: %v", err)
	}

	body := fmt.Sprintf(`{"status":"accepted","source_asset_id":"host-1","target_asset_id":"vm-1"}`)
	req := httptest.NewRequest(http.MethodPut, "/links/suggestions/"+sug.ID, strings.NewReader(body))
	rec := httptest.NewRecorder()
	deps.HandleLinkSuggestionActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	pending, err := deps.LinkSuggestionStore.ListPendingLinkSuggestions()
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("expected 0 pending after accept, got %d", len(pending))
	}

	allDeps, err := depStore.ListAssetDependencies("host-1", 100)
	if err != nil {
		t.Fatalf("list deps: %v", err)
	}
	if len(allDeps) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(allDeps))
	}
	if allDeps[0].RelationshipType != dependencies.RelationshipContains {
		t.Errorf("expected contains relationship, got %s", allDeps[0].RelationshipType)
	}
}

// ---------------------------------------------------------------------------
// PUT /links/suggestions/{id} -- dismiss
// ---------------------------------------------------------------------------

func TestLinkSuggestionDismissNoDependency(t *testing.T) {
	deps := newTestResourcesDeps(t)
	depStore := newTestDependencyStore()
	deps.DependencyStore = depStore

	createTestAssetForLinks(t, deps.AssetStore, "host-1", "server", "Host 1")
	createTestAssetForLinks(t, deps.AssetStore, "vm-1", "vm", "VM 1")

	sug, err := deps.LinkSuggestionStore.CreateLinkSuggestion("host-1", "vm-1", "ip_match", 0.85)
	if err != nil {
		t.Fatalf("create suggestion: %v", err)
	}

	body := `{"status":"dismissed"}`
	req := httptest.NewRequest(http.MethodPut, "/links/suggestions/"+sug.ID, strings.NewReader(body))
	rec := httptest.NewRecorder()
	deps.HandleLinkSuggestionActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	pending, err := deps.LinkSuggestionStore.ListPendingLinkSuggestions()
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("expected 0 pending after dismiss, got %d", len(pending))
	}

	allDeps, err := depStore.ListAssetDependencies("host-1", 100)
	if err != nil {
		t.Fatalf("list deps: %v", err)
	}
	if len(allDeps) != 0 {
		t.Errorf("expected 0 dependencies after dismiss, got %d", len(allDeps))
	}
}

// ---------------------------------------------------------------------------
// POST /links/manual
// ---------------------------------------------------------------------------

func TestManualLinkCreatesContainsDependency(t *testing.T) {
	deps := newTestResourcesDeps(t)
	depStore := newTestDependencyStore()
	deps.DependencyStore = depStore

	createTestAssetForLinks(t, deps.AssetStore, "host-1", "server", "Host 1")
	createTestAssetForLinks(t, deps.AssetStore, "vm-1", "vm", "VM 1")

	body := `{"source_id":"host-1","target_id":"vm-1"}`
	req := httptest.NewRequest(http.MethodPost, "/links/manual", strings.NewReader(body))
	rec := httptest.NewRecorder()
	deps.HandleManualLink(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	allDeps, err := depStore.ListAssetDependencies("host-1", 100)
	if err != nil {
		t.Fatalf("list deps: %v", err)
	}
	if len(allDeps) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(allDeps))
	}
	if allDeps[0].RelationshipType != dependencies.RelationshipContains {
		t.Errorf("expected contains relationship, got %s", allDeps[0].RelationshipType)
	}
	if allDeps[0].SourceAssetID != "host-1" {
		t.Errorf("expected source host-1, got %s", allDeps[0].SourceAssetID)
	}
	if allDeps[0].TargetAssetID != "vm-1" {
		t.Errorf("expected target vm-1, got %s", allDeps[0].TargetAssetID)
	}
}

func TestManualLinkRejectsSameAsset(t *testing.T) {
	deps := newTestResourcesDeps(t)
	depStore := newTestDependencyStore()
	deps.DependencyStore = depStore

	body := `{"source_id":"host-1","target_id":"host-1"}`
	req := httptest.NewRequest(http.MethodPost, "/links/manual", strings.NewReader(body))
	rec := httptest.NewRecorder()
	deps.HandleManualLink(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// PUT /assets/bulk/move
// ---------------------------------------------------------------------------

func TestBulkMoveAssetsToGroup(t *testing.T) {
	deps := newTestResourcesDeps(t)

	grp, err := deps.GroupStore.CreateGroup(groups.CreateRequest{
		Name: "Lab Rack 1",
		Slug: "lab-rack-1",
	})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}

	createTestAssetForLinks(t, deps.AssetStore, "host-1", "server", "Host 1")
	createTestAssetForLinks(t, deps.AssetStore, "host-2", "server", "Host 2")
	createTestAssetForLinks(t, deps.AssetStore, "host-3", "server", "Host 3")

	payload, _ := json.Marshal(map[string]any{
		"asset_ids": []string{"host-1", "host-2", "host-3"},
		"group_id":  grp.ID,
	})
	req := httptest.NewRequest(http.MethodPut, "/assets/bulk/move", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	deps.HandleAssetBulkMove(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Updated int `json:"updated"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Updated != 3 {
		t.Errorf("expected 3 updated, got %d", resp.Updated)
	}

	for _, id := range []string{"host-1", "host-2", "host-3"} {
		a, ok, err := deps.AssetStore.GetAsset(id)
		if err != nil {
			t.Fatalf("get asset %s: %v", id, err)
		}
		if !ok {
			t.Fatalf("asset %s not found", id)
		}
		if a.GroupID != grp.ID {
			t.Errorf("asset %s group_id = %q, want %q", id, a.GroupID, grp.ID)
		}
	}
}

func TestBulkMoveRejectsNonExistentGroup(t *testing.T) {
	deps := newTestResourcesDeps(t)

	createTestAssetForLinks(t, deps.AssetStore, "host-1", "server", "Host 1")

	payload, _ := json.Marshal(map[string]any{
		"asset_ids": []string{"host-1"},
		"group_id":  "grp_nonexistent",
	})
	req := httptest.NewRequest(http.MethodPut, "/assets/bulk/move", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	deps.HandleAssetBulkMove(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestBulkMoveSkipsNonExistentAssets(t *testing.T) {
	deps := newTestResourcesDeps(t)

	grp, err := deps.GroupStore.CreateGroup(groups.CreateRequest{
		Name: "Lab Rack 1",
		Slug: "lab-rack-1",
	})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}

	createTestAssetForLinks(t, deps.AssetStore, "host-1", "server", "Host 1")

	payload, _ := json.Marshal(map[string]any{
		"asset_ids": []string{"host-1", "host-nonexistent"},
		"group_id":  grp.ID,
	})
	req := httptest.NewRequest(http.MethodPut, "/assets/bulk/move", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	deps.HandleAssetBulkMove(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Updated int `json:"updated"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Updated != 1 {
		t.Errorf("expected 1 updated (skip nonexistent), got %d", resp.Updated)
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

type testDependencyStore struct {
	mu   sync.Mutex
	deps map[string]dependencies.Dependency
	next int
}

func newTestDependencyStore() *testDependencyStore {
	return &testDependencyStore{
		deps: make(map[string]dependencies.Dependency),
	}
}

func (s *testDependencyStore) CreateAssetDependency(req dependencies.CreateDependencyRequest) (dependencies.Dependency, error) {
	source := strings.TrimSpace(req.SourceAssetID)
	target := strings.TrimSpace(req.TargetAssetID)
	relType := strings.TrimSpace(req.RelationshipType)

	if source == target {
		return dependencies.Dependency{}, dependencies.ErrSelfReference
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, existing := range s.deps {
		if existing.SourceAssetID == source && existing.TargetAssetID == target && existing.RelationshipType == relType {
			return dependencies.Dependency{}, dependencies.ErrDuplicateDependency
		}
	}

	s.next++
	now := time.Now().UTC()
	dep := dependencies.Dependency{
		ID:               fmt.Sprintf("dep_%d", s.next),
		SourceAssetID:    source,
		TargetAssetID:    target,
		RelationshipType: relType,
		Direction:        req.Direction,
		Criticality:      req.Criticality,
		Metadata:         req.Metadata,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	s.deps[dep.ID] = dep
	return dep, nil
}

func (s *testDependencyStore) ListAssetDependencies(assetID string, limit int) ([]dependencies.Dependency, error) {
	if limit <= 0 {
		limit = 50
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]dependencies.Dependency, 0)
	for _, dep := range s.deps {
		if dep.SourceAssetID == assetID || dep.TargetAssetID == assetID {
			out = append(out, dep)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (s *testDependencyStore) GetAssetDependency(id string) (dependencies.Dependency, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dep, ok := s.deps[id]
	return dep, ok, nil
}

func (s *testDependencyStore) DeleteAssetDependency(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.deps[id]; !ok {
		return dependencies.ErrDependencyNotFound
	}
	delete(s.deps, id)
	return nil
}

func (s *testDependencyStore) BlastRadius(_ string, _ int) ([]dependencies.ImpactNode, error) {
	return nil, nil
}

func (s *testDependencyStore) UpstreamCauses(_ string, _ int) ([]dependencies.ImpactNode, error) {
	return nil, nil
}

func (s *testDependencyStore) LinkIncidentAsset(_ string, _ incidents.LinkAssetRequest) (incidents.IncidentAsset, error) {
	return incidents.IncidentAsset{}, nil
}

func (s *testDependencyStore) ListIncidentAssets(_ string, _ int) ([]incidents.IncidentAsset, error) {
	return nil, nil
}

func (s *testDependencyStore) UnlinkIncidentAsset(_, _ string) error {
	return nil
}
