package dockerpkg

import (
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/connectors/docker"
)

func requireDockerScope(w http.ResponseWriter, r *http.Request, scope string) bool {
	if apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), scope) {
		return true
	}
	apiv2.WriteScopeForbidden(w, scope)
	return false
}

func dockerScopeForMethod(method string) string {
	if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
		return "docker:read"
	}
	return "docker:write"
}

func requireDockerAssetAccess(w http.ResponseWriter, r *http.Request, assetIDs ...string) bool {
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

func dockerHostAllowed(r *http.Request, host *docker.DockerHost, routeID string) bool {
	allowed := apiv2.AllowedAssetsFromContext(r.Context())
	if len(allowed) == 0 || host == nil {
		return true
	}
	for _, assetID := range dockerHostAssetIDs(host, routeID) {
		if apiv2.AssetCheck(allowed, assetID) {
			return true
		}
	}
	return false
}

func requireDockerHostAccess(w http.ResponseWriter, r *http.Request, host *docker.DockerHost, routeID string) bool {
	if dockerHostAllowed(r, host, routeID) {
		return true
	}
	assetID := strings.TrimSpace(routeID)
	if assetID == "" && host != nil {
		assetID = host.AgentID
	}
	apiv2.WriteAssetForbidden(w, assetID)
	return false
}

func dockerHostAssetIDs(host *docker.DockerHost, routeID string) []string {
	ids := make([]string, 0, 4)
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		for _, existing := range ids {
			if existing == id {
				return
			}
		}
		ids = append(ids, id)
	}

	normalizedRoute := NormalizeDockerHostLookupID(routeID)
	add(routeID)
	add(normalizedRoute)
	if host != nil {
		normalizedAgent := NormalizeDockerHostLookupID(host.AgentID)
		add(host.AgentID)
		add(normalizedAgent)
		add("docker-host-" + normalizedAgent)
	}
	if normalizedRoute != "" && !strings.HasPrefix(normalizedRoute, "docker-host-") {
		add("docker-host-" + normalizedRoute)
	}
	return ids
}
