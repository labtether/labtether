package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// With no Proxmox connection the CLI's cluster-status, resources, nodes and
// ceph-status answered 502, which every client shows as "An internal error
// occurred." They must say Proxmox is not set up, with a 4xx.
func TestProxmoxClusterRoutesSayProxmoxIsNotSetUp(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.hubCollectorStore = &stubHubCollectorStore{}
	routes := map[string]http.HandlerFunc{
		"/api/v2/proxmox/cluster/status":    sut.handleV2ProxmoxClusterStatus,
		"/api/v2/proxmox/cluster/resources": sut.handleV2ProxmoxClusterResources,
		"/api/v2/proxmox/ceph/status":       sut.handleV2ProxmoxCephStatus,
	}
	for path, handler := range routes {
		rec := httptest.NewRecorder()
		handler(rec, httptest.NewRequest(http.MethodGet, path, nil))
		var body struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		text := body.Message + " " + body.Error
		if rec.Code != http.StatusNotFound || !strings.Contains(text, "Proxmox is not set up.") {
			t.Fatalf("%s: got %d %s, want 404 saying Proxmox is not set up", path, rec.Code, rec.Body.String())
		}
	}
}
