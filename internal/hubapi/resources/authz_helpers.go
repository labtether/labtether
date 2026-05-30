package resources

import (
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/dependencies"
	"github.com/labtether/labtether/internal/edges"
)

func requireAPIScope(w http.ResponseWriter, r *http.Request, scope string) bool {
	if apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), scope) {
		return true
	}
	apiv2.WriteScopeForbidden(w, scope)
	return false
}

func requireAssetAccess(w http.ResponseWriter, r *http.Request, assetIDs ...string) bool {
	allowed := apiv2.AllowedAssetsFromContext(r.Context())
	for _, assetID := range assetIDs {
		assetID = strings.TrimSpace(assetID)
		if assetID == "" {
			continue
		}
		if !apiv2.AssetCheck(allowed, assetID) {
			apiv2.WriteAssetForbidden(w, assetID)
			return false
		}
	}
	return true
}

func filterDependenciesByAssetAccess(r *http.Request, deps []dependencies.Dependency) []dependencies.Dependency {
	allowed := apiv2.AllowedAssetsFromContext(r.Context())
	if len(allowed) == 0 {
		return deps
	}
	filtered := make([]dependencies.Dependency, 0, len(deps))
	for _, dep := range deps {
		if apiv2.AssetCheck(allowed, dep.SourceAssetID) && apiv2.AssetCheck(allowed, dep.TargetAssetID) {
			filtered = append(filtered, dep)
		}
	}
	if filtered == nil {
		return []dependencies.Dependency{}
	}
	return filtered
}

func filterImpactNodesByAssetAccess(r *http.Request, nodes []dependencies.ImpactNode) []dependencies.ImpactNode {
	allowed := apiv2.AllowedAssetsFromContext(r.Context())
	if len(allowed) == 0 {
		return nodes
	}
	filtered := make([]dependencies.ImpactNode, 0, len(nodes))
	for _, node := range nodes {
		if apiv2.AssetCheck(allowed, node.AssetID) {
			filtered = append(filtered, node)
		}
	}
	if filtered == nil {
		return []dependencies.ImpactNode{}
	}
	return filtered
}

func filterEdgesByAssetAccess(r *http.Request, edgeList []edges.Edge) []edges.Edge {
	allowed := apiv2.AllowedAssetsFromContext(r.Context())
	if len(allowed) == 0 {
		return edgeList
	}
	filtered := make([]edges.Edge, 0, len(edgeList))
	for _, edge := range edgeList {
		if apiv2.AssetCheck(allowed, edge.SourceAssetID) && apiv2.AssetCheck(allowed, edge.TargetAssetID) {
			filtered = append(filtered, edge)
		}
	}
	if filtered == nil {
		return []edges.Edge{}
	}
	return filtered
}

func filterTreeNodesByAssetAccess(r *http.Request, nodes []edges.TreeNode) []edges.TreeNode {
	allowed := apiv2.AllowedAssetsFromContext(r.Context())
	if len(allowed) == 0 {
		return nodes
	}
	filtered := make([]edges.TreeNode, 0, len(nodes))
	for _, node := range nodes {
		if apiv2.AssetCheck(allowed, node.AssetID) {
			filtered = append(filtered, node)
		}
	}
	if filtered == nil {
		return []edges.TreeNode{}
	}
	return filtered
}

func requireCompositeAccess(w http.ResponseWriter, r *http.Request, composite edges.Composite) bool {
	allowed := apiv2.AllowedAssetsFromContext(r.Context())
	for _, member := range composite.Members {
		if !apiv2.AssetCheck(allowed, member.AssetID) {
			apiv2.WriteAssetForbidden(w, member.AssetID)
			return false
		}
	}
	return true
}
