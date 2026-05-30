package resources

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/dependencies"
	"github.com/labtether/labtether/internal/edges"
)

func TestLegacyTopologyHandlersRequireAPIKeyScopes(t *testing.T) {
	store := &authzEdgeStore{}
	deps := &Deps{
		EdgeStore: store,
		DecodeJSONBody: func(_ http.ResponseWriter, r *http.Request, dst any) error {
			return jsonDecode(r, dst)
		},
		EnforceRateLimit: func(http.ResponseWriter, *http.Request, string, int, time.Duration) bool {
			return true
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/discovery/proposals", nil)
	req = req.WithContext(apiv2.ContextWithScopes(req.Context(), []string{"assets:read"}))
	rec := httptest.NewRecorder()
	deps.HandleProposals(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for discovery proposals without discovery:read, got %d: %s", rec.Code, rec.Body.String())
	}
	if store.listProposalsCalls != 0 {
		t.Fatalf("proposal store was called despite missing scope")
	}

	body := bytes.NewBufferString(`{"primary_asset_id":"asset-a","facet_asset_ids":["asset-b"]}`)
	req = httptest.NewRequest(http.MethodPost, "/composites", body)
	req = req.WithContext(apiv2.ContextWithScopes(req.Context(), []string{"topology:read"}))
	rec = httptest.NewRecorder()
	deps.HandleComposites(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for composite create without topology:write, got %d: %s", rec.Code, rec.Body.String())
	}
	if store.createCompositeCalls != 0 {
		t.Fatalf("composite store was called despite missing scope")
	}
}

func TestTopologyHandlersApplyAPIKeyAssetAllowlist(t *testing.T) {
	store := &authzEdgeStore{
		edges: []edges.Edge{{
			ID:               "proposal-1",
			SourceAssetID:    "allowed",
			TargetAssetID:    "secret",
			RelationshipType: edges.RelRunsOn,
			Origin:           edges.OriginSuggested,
		}},
	}
	deps := &Deps{
		EdgeStore: store,
		EnforceRateLimit: func(http.ResponseWriter, *http.Request, string, int, time.Duration) bool {
			return true
		},
	}

	ctx := apiv2.ContextWithScopes(context.Background(), []string{"topology:read"})
	ctx = apiv2.ContextWithAllowedAssets(ctx, []string{"allowed"})
	req := httptest.NewRequest(http.MethodGet, "/edges?asset_id=secret", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	deps.HandleEdges(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for disallowed queried asset, got %d: %s", rec.Code, rec.Body.String())
	}
	if store.listEdgesByAssetCalls != 0 {
		t.Fatalf("edge store was called despite disallowed queried asset")
	}

	req = httptest.NewRequest(http.MethodGet, "/edges?asset_id=allowed", nil).WithContext(ctx)
	rec = httptest.NewRecorder()
	deps.HandleEdges(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for allowed queried asset, got %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("response leaked disallowed target asset: %s", rec.Body.String())
	}

	ctx = apiv2.ContextWithScopes(context.Background(), []string{"discovery:write"})
	ctx = apiv2.ContextWithAllowedAssets(ctx, []string{"allowed"})
	req = httptest.NewRequest(http.MethodPost, "/discovery/proposals/proposal-1/accept", nil).WithContext(ctx)
	rec = httptest.NewRecorder()
	deps.HandleProposalActions(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for proposal action touching disallowed asset, got %d: %s", rec.Code, rec.Body.String())
	}
	if store.acceptProposalCalls != 0 {
		t.Fatalf("proposal was accepted despite disallowed target asset")
	}
}

func TestDependencyHandlersApplyAPIKeyAssetAllowlist(t *testing.T) {
	store := newTestDependencyStore()
	dep, err := store.CreateAssetDependency(dependencies.CreateDependencyRequest{
		SourceAssetID:    "allowed",
		TargetAssetID:    "secret",
		RelationshipType: dependencies.RelationshipRunsOn,
	})
	if err != nil {
		t.Fatalf("create dependency: %v", err)
	}
	deps := &Deps{DependencyStore: store}

	ctx := apiv2.ContextWithScopes(context.Background(), []string{"topology:read"})
	ctx = apiv2.ContextWithAllowedAssets(ctx, []string{"allowed"})
	req := httptest.NewRequest(http.MethodGet, "/dependencies/"+dep.ID, nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	deps.HandleDependencyActions(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for dependency touching disallowed asset, got %d: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/dependencies?asset_id=allowed", nil).WithContext(ctx)
	rec = httptest.NewRecorder()
	deps.HandleDependencies(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for allowed dependency list, got %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("dependency list leaked disallowed target asset: %s", rec.Body.String())
	}
}

func jsonDecode(r *http.Request, dst any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
}

type authzEdgeStore struct {
	edges                 []edges.Edge
	composites            map[string]edges.Composite
	listProposalsCalls    int
	acceptProposalCalls   int
	createCompositeCalls  int
	listEdgesByAssetCalls int
}

func (s *authzEdgeStore) CreateEdge(req edges.CreateEdgeRequest) (edges.Edge, error) {
	edge := edges.Edge{
		ID:               "edge-created",
		SourceAssetID:    req.SourceAssetID,
		TargetAssetID:    req.TargetAssetID,
		RelationshipType: req.RelationshipType,
	}
	return edge, nil
}

func (s *authzEdgeStore) GetEdge(id string) (edges.Edge, bool, error) {
	for _, edge := range s.edges {
		if edge.ID == id {
			return edge, true, nil
		}
	}
	return edges.Edge{}, false, nil
}

func (s *authzEdgeStore) UpdateEdge(string, string, string) error { return nil }
func (s *authzEdgeStore) DeleteEdge(string) error                 { return nil }

func (s *authzEdgeStore) ListEdgesByAsset(assetID string, _ int) ([]edges.Edge, error) {
	s.listEdgesByAssetCalls++
	out := make([]edges.Edge, 0)
	for _, edge := range s.edges {
		if edge.SourceAssetID == assetID || edge.TargetAssetID == assetID {
			out = append(out, edge)
		}
	}
	return out, nil
}

func (s *authzEdgeStore) ListEdgesBatch(assetIDs []string, limit int) ([]edges.Edge, error) {
	out := make([]edges.Edge, 0)
	for _, assetID := range assetIDs {
		edgesForAsset, err := s.ListEdgesByAsset(assetID, limit)
		if err != nil {
			return nil, err
		}
		out = append(out, edgesForAsset...)
	}
	return out, nil
}

func (s *authzEdgeStore) Descendants(string, int) ([]edges.TreeNode, error) { return nil, nil }
func (s *authzEdgeStore) Ancestors(string, int) ([]edges.TreeNode, error)   { return nil, nil }

func (s *authzEdgeStore) ListProposals() ([]edges.Edge, error) {
	s.listProposalsCalls++
	return append([]edges.Edge(nil), s.edges...), nil
}

func (s *authzEdgeStore) AcceptProposal(string) error {
	s.acceptProposalCalls++
	return nil
}

func (s *authzEdgeStore) DismissProposal(string) error { return nil }

func (s *authzEdgeStore) CreateComposite(req edges.CreateCompositeRequest) (edges.Composite, error) {
	s.createCompositeCalls++
	return edges.Composite{
		CompositeID: "composite-created",
		Members: []edges.CompositeMember{
			{AssetID: req.PrimaryAssetID, Role: "primary"},
		},
	}, nil
}

func (s *authzEdgeStore) GetComposite(id string) (edges.Composite, bool, error) {
	if s.composites == nil {
		return edges.Composite{}, false, nil
	}
	composite, ok := s.composites[id]
	return composite, ok, nil
}

func (s *authzEdgeStore) ChangePrimary(string, string) error { return nil }
func (s *authzEdgeStore) DetachMember(string, string) error  { return nil }
func (s *authzEdgeStore) ListCompositesByAssets([]string) ([]edges.Composite, error) {
	return nil, nil
}
func (s *authzEdgeStore) ResolveCompositeID(string) (string, bool, error) { return "", false, nil }
