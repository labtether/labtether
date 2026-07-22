package resources

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/persistence"
)

func restrictedRequest(method, target string, body []byte, allowed ...string) *http.Request {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	ctx := apiv2.ContextWithAllowedAssets(context.Background(), allowed)
	return req.WithContext(ctx)
}

func createAssetInGroup(t *testing.T, d *Deps, assetID, groupID string) {
	t.Helper()
	if _, err := d.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: assetID, Type: "server", Name: assetID, Source: "test"}); err != nil {
		t.Fatalf("create asset: %v", err)
	}
	if _, err := d.AssetStore.UpdateAsset(assetID, assets.UpdateRequest{GroupID: &groupID}); err != nil {
		t.Fatalf("assign asset to group: %v", err)
	}
}

func TestRestrictedGroupListExcludesMixedAndSecretSubtrees(t *testing.T) {
	d := newTestResourcesDeps(t)
	root := mustCreateGroup(t, d, "Root", "")
	allowedGroup := mustCreateGroup(t, d, "Allowed", root.ID)
	secretGroup := mustCreateGroup(t, d, "Secret", root.ID)
	createAssetInGroup(t, d, "asset-allowed", allowedGroup.ID)
	createAssetInGroup(t, d, "asset-secret", secretGroup.ID)

	rec := httptest.NewRecorder()
	d.HandleGroups(rec, restrictedRequest(http.MethodGet, "/groups", nil, "asset-allowed"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Groups []groups.Group `json:"groups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Groups) != 1 || response.Groups[0].ID != allowedGroup.ID {
		t.Fatalf("expected only allowed child group, got %#v", response.Groups)
	}
	if strings.Contains(rec.Body.String(), root.ID) || strings.Contains(rec.Body.String(), secretGroup.ID) {
		t.Fatalf("response leaked mixed or secret group: %s", rec.Body.String())
	}
}

func TestRestrictedGroupMutationFailsBeforeStoreWrite(t *testing.T) {
	d := newTestResourcesDeps(t)
	group := mustCreateGroup(t, d, "Allowed", "")
	createAssetInGroup(t, d, "asset-allowed", group.ID)

	rec := httptest.NewRecorder()
	d.HandleGroupActions(rec, restrictedRequest(http.MethodPatch, "/groups/"+group.ID, []byte(`{"name":"Compromised"}`), "asset-allowed"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	stored, ok, err := d.GroupStore.GetGroup(group.ID)
	if err != nil || !ok {
		t.Fatalf("reload group: ok=%v err=%v", ok, err)
	}
	if stored.Name != "Allowed" {
		t.Fatalf("forbidden mutation reached store: name=%q", stored.Name)
	}
}

func TestRestrictedLinkSuggestionUsesStoredAssetsAndCannotBeForged(t *testing.T) {
	d := newTestResourcesDeps(t)
	suggestion, err := d.LinkSuggestionStore.CreateLinkSuggestion("asset-allowed", "asset-secret", "test", 0.9)
	if err != nil {
		t.Fatalf("create suggestion: %v", err)
	}

	rec := httptest.NewRecorder()
	d.HandleLinkSuggestionActions(rec, restrictedRequest(
		http.MethodPut,
		"/links/suggestions/"+suggestion.ID,
		[]byte(`{"status":"dismissed","source_asset_id":"asset-allowed","target_asset_id":"asset-allowed"}`),
		"asset-allowed",
	))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	stored, ok, err := d.LinkSuggestionStore.GetLinkSuggestion(suggestion.ID)
	if err != nil || !ok {
		t.Fatalf("reload suggestion: ok=%v err=%v", ok, err)
	}
	if stored.Status != "pending" {
		t.Fatalf("forbidden suggestion was mutated: status=%q", stored.Status)
	}
}

func TestRestrictedSyntheticChecksFailClosed(t *testing.T) {
	d := newTestResourcesDeps(t)
	rec := httptest.NewRecorder()
	d.HandleSyntheticChecks(rec, restrictedRequest(http.MethodGet, "/synthetic-checks", nil, "asset-allowed"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRestrictedBulkMoveFailsClosedWithoutDestinationGroupAuthorization(t *testing.T) {
	d := newTestResourcesDeps(t)
	createAssetInGroup(t, d, "asset-allowed", "")
	d.GroupStore = nil

	rec := httptest.NewRecorder()
	d.HandleAssetBulkMove(rec, restrictedRequest(
		http.MethodPut,
		"/assets/bulk/move",
		[]byte(`{"asset_ids":["asset-allowed"],"group_id":"group-unverified"}`),
		"asset-allowed",
	))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rec.Code, rec.Body.String())
	}
	asset, ok, err := d.AssetStore.GetAsset("asset-allowed")
	if err != nil || !ok {
		t.Fatalf("reload asset: ok=%v err=%v", ok, err)
	}
	if asset.GroupID != "" {
		t.Fatalf("failed-closed bulk move mutated asset group to %q", asset.GroupID)
	}
}

var _ persistence.LinkSuggestionStore = (*persistence.MemoryLinkSuggestionStore)(nil)
