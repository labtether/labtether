package resources

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/hubapi/testutil"
)

func newTestResourcesDeps(t *testing.T) *Deps {
	t.Helper()
	return &Deps{
		AssetStore:          testutil.NewAssetStore(),
		GroupStore:          testutil.NewGroupStore(),
		TelemetryStore:      testutil.NewTelemetryStore(),
		LogStore:            testutil.NewLogStore(),
		SyntheticStore:      testutil.NewSyntheticStore(),
		LinkSuggestionStore: testutil.NewLinkSuggestionStore(),
		CredentialStore:     testutil.NewCredentialStore(),
		AuditStore:          testutil.NewAuditStore(),
		RetentionStore:      testutil.NewRetentionStore(),
		FileBridges:         &sync.Map{},
		ProcessBridges:      &sync.Map{},
		ServiceBridges:      &sync.Map{},
		JournalBridges:      &sync.Map{},
		DiskBridges:         &sync.Map{},
		NetworkBridges:      &sync.Map{},
		PackageBridges:      &sync.Map{},
		CronBridges:         &sync.Map{},
		UsersBridges:        &sync.Map{},
		WrapAuth:            testutil.NoopAuth,
		DecodeJSONBody:      testutil.DecodeJSONBody,
		EnforceRateLimit:    testutil.NoopRateLimit,
		PrincipalActorID:    testutil.TestActorID,
		UserIDFromContext:    testutil.TestUserID,
		SecretsManager:      testutil.TestSecretsManager(t),
		AppendAuditEventBestEffort: testutil.NoopAudit,
	}
}

func mustCreateGroup(t *testing.T, deps *Deps, name string, parentID string) groups.Group {
	t.Helper()
	body := map[string]any{"name": name}
	if parentID != "" {
		body["parent_group_id"] = parentID
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/groups", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	deps.HandleGroups(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating group %q, got %d body=%s", name, rec.Code, rec.Body.String())
	}
	var resp struct {
		Group groups.Group `json:"group"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode group response: %v", err)
	}
	return resp.Group
}

func TestHandleGroupsListReturnsEmpty(t *testing.T) {
	deps := newTestResourcesDeps(t)
	req := httptest.NewRequest(http.MethodGet, "/groups", nil)
	rec := httptest.NewRecorder()
	deps.HandleGroups(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Groups []groups.Group `json:"groups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Groups) != 0 {
		t.Fatalf("expected 0 groups, got %d", len(resp.Groups))
	}
}

func TestHandleGroupsCreate(t *testing.T) {
	deps := newTestResourcesDeps(t)
	g := mustCreateGroup(t, deps, "Lab Rack A", "")

	if g.Name != "Lab Rack A" {
		t.Fatalf("expected name 'Lab Rack A', got %q", g.Name)
	}
	if g.ID == "" {
		t.Fatal("expected group to have an ID")
	}
}

func TestHandleGroupsCreateNameRequired(t *testing.T) {
	deps := newTestResourcesDeps(t)
	payload := []byte(`{"name":""}`)
	req := httptest.NewRequest(http.MethodPost, "/groups", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	deps.HandleGroups(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty name, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleGroupsList(t *testing.T) {
	deps := newTestResourcesDeps(t)
	mustCreateGroup(t, deps, "Group A", "")
	mustCreateGroup(t, deps, "Group B", "")

	req := httptest.NewRequest(http.MethodGet, "/groups", nil)
	rec := httptest.NewRecorder()
	deps.HandleGroups(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Groups []groups.Group `json:"groups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(resp.Groups))
	}
}

func TestHandleGroupsTreeFormat(t *testing.T) {
	deps := newTestResourcesDeps(t)
	parent := mustCreateGroup(t, deps, "Parent", "")
	mustCreateGroup(t, deps, "Child", parent.ID)

	req := httptest.NewRequest(http.MethodGet, "/groups?format=tree", nil)
	rec := httptest.NewRecorder()
	deps.HandleGroups(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Tree []groups.TreeNode `json:"tree"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode tree response: %v", err)
	}
	if len(resp.Tree) != 1 {
		t.Fatalf("expected 1 root tree node, got %d", len(resp.Tree))
	}
	if resp.Tree[0].Group.Name != "Parent" {
		t.Fatalf("expected root node 'Parent', got %q", resp.Tree[0].Group.Name)
	}
	if len(resp.Tree[0].Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(resp.Tree[0].Children))
	}
	if resp.Tree[0].Children[0].Group.Name != "Child" {
		t.Fatalf("expected child 'Child', got %q", resp.Tree[0].Children[0].Group.Name)
	}
}

func TestHandleGroupsHasLocationFilter(t *testing.T) {
	deps := newTestResourcesDeps(t)

	payload := []byte(`{"name":"Lab NYC","location":"New York","timezone":"America/New_York"}`)
	req := httptest.NewRequest(http.MethodPost, "/groups", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	deps.HandleGroups(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}

	mustCreateGroup(t, deps, "No Location", "")

	req = httptest.NewRequest(http.MethodGet, "/groups?has_location=true", nil)
	rec = httptest.NewRecorder()
	deps.HandleGroups(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Groups []groups.Group `json:"groups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Groups) != 1 {
		t.Fatalf("expected 1 location-bearing group, got %d", len(resp.Groups))
	}
	if resp.Groups[0].Name != "Lab NYC" {
		t.Fatalf("expected 'Lab NYC', got %q", resp.Groups[0].Name)
	}
}

func TestHandleGroupActionsGet(t *testing.T) {
	deps := newTestResourcesDeps(t)
	created := mustCreateGroup(t, deps, "Test Group", "")

	req := httptest.NewRequest(http.MethodGet, "/groups/"+created.ID, nil)
	rec := httptest.NewRecorder()
	deps.HandleGroupActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Group groups.Group `json:"group"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Group.ID != created.ID {
		t.Fatalf("expected group ID %q, got %q", created.ID, resp.Group.ID)
	}
}

func TestHandleGroupActionsGetNotFound(t *testing.T) {
	deps := newTestResourcesDeps(t)
	req := httptest.NewRequest(http.MethodGet, "/groups/nonexistent", nil)
	rec := httptest.NewRecorder()
	deps.HandleGroupActions(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestHandleGroupActionsUpdate(t *testing.T) {
	deps := newTestResourcesDeps(t)
	created := mustCreateGroup(t, deps, "Original", "")

	payload := []byte(`{"name":"Renamed"}`)
	req := httptest.NewRequest(http.MethodPut, "/groups/"+created.ID, bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	deps.HandleGroupActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Group groups.Group `json:"group"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Group.Name != "Renamed" {
		t.Fatalf("expected name 'Renamed', got %q", resp.Group.Name)
	}
}

func TestHandleGroupActionsDelete(t *testing.T) {
	deps := newTestResourcesDeps(t)
	parent := mustCreateGroup(t, deps, "Parent", "")
	child := mustCreateGroup(t, deps, "Child", parent.ID)

	req := httptest.NewRequest(http.MethodDelete, "/groups/"+parent.ID, nil)
	rec := httptest.NewRecorder()
	deps.HandleGroupActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/groups/"+child.ID, nil)
	getRec := httptest.NewRecorder()
	deps.HandleGroupActions(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for child lookup, got %d", getRec.Code)
	}

	var resp struct {
		Group groups.Group `json:"group"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Group.ParentGroupID != "" {
		t.Fatalf("expected child reparented to root (empty parent), got %q", resp.Group.ParentGroupID)
	}
}

func TestHandleGroupActionsDeleteNotFound(t *testing.T) {
	deps := newTestResourcesDeps(t)
	req := httptest.NewRequest(http.MethodDelete, "/groups/nonexistent", nil)
	rec := httptest.NewRecorder()
	deps.HandleGroupActions(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestHandleGroupMoveCycleDetection(t *testing.T) {
	deps := newTestResourcesDeps(t)
	grandparent := mustCreateGroup(t, deps, "Grandparent", "")
	parent := mustCreateGroup(t, deps, "Parent", grandparent.ID)
	child := mustCreateGroup(t, deps, "Child", parent.ID)

	payload, _ := json.Marshal(map[string]string{"parent_group_id": child.ID})
	req := httptest.NewRequest(http.MethodPut, "/groups/"+grandparent.ID+"/move", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	deps.HandleGroupActions(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for cycle detection, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleGroupMoveSelfParent(t *testing.T) {
	deps := newTestResourcesDeps(t)
	g := mustCreateGroup(t, deps, "Self", "")

	payload, _ := json.Marshal(map[string]string{"parent_group_id": g.ID})
	req := httptest.NewRequest(http.MethodPut, "/groups/"+g.ID+"/move", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	deps.HandleGroupActions(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for self-parent, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleGroupMoveValid(t *testing.T) {
	deps := newTestResourcesDeps(t)
	a := mustCreateGroup(t, deps, "A", "")
	b := mustCreateGroup(t, deps, "B", "")

	payload, _ := json.Marshal(map[string]string{"parent_group_id": a.ID})
	req := httptest.NewRequest(http.MethodPut, "/groups/"+b.ID+"/move", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	deps.HandleGroupActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Group groups.Group `json:"group"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Group.ParentGroupID != a.ID {
		t.Fatalf("expected parent %q, got %q", a.ID, resp.Group.ParentGroupID)
	}
}

func TestHandleGroupMoveToRoot(t *testing.T) {
	deps := newTestResourcesDeps(t)
	parent := mustCreateGroup(t, deps, "Parent", "")
	child := mustCreateGroup(t, deps, "Child", parent.ID)

	payload := []byte(`{"parent_group_id":""}`)
	req := httptest.NewRequest(http.MethodPut, "/groups/"+child.ID+"/move", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	deps.HandleGroupActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Group groups.Group `json:"group"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Group.ParentGroupID != "" {
		t.Fatalf("expected empty parent for root, got %q", resp.Group.ParentGroupID)
	}
}

func TestHandleGroupsStoreUnavailable(t *testing.T) {
	deps := newTestResourcesDeps(t)
	deps.GroupStore = nil

	req := httptest.NewRequest(http.MethodGet, "/groups", nil)
	rec := httptest.NewRecorder()
	deps.HandleGroups(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestHandleGroupReorder(t *testing.T) {
	deps := newTestResourcesDeps(t)
	g := mustCreateGroup(t, deps, "Reorder Me", "")

	payload := []byte(`{"sort_order":5}`)
	req := httptest.NewRequest(http.MethodPut, "/groups/"+g.ID+"/reorder", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	deps.HandleGroupActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Group groups.Group `json:"group"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Group.SortOrder != 5 {
		t.Fatalf("expected sort_order 5, got %d", resp.Group.SortOrder)
	}
}
