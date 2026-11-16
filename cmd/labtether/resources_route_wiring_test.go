package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/groupfailover"
	"github.com/labtether/labtether/internal/groupprofiles"
	respkg "github.com/labtether/labtether/internal/hubapi/resources"
	"github.com/labtether/labtether/internal/persistence"
)

// TestResourcesRoutesReachHandlersThroughRouteTable sends one signed-in
// admin request to each resources route that needs no agent through the
// real route table and auth, and checks the answer only that route's
// handler gives. TestResourcesAgentRoutesReachHandlersThroughRouteTable
// covers the nine agent routes.
//
// The resources handler tests live in internal/hubapi/resources and build
// their Deps with testutil/hubdeps. This test catches a fault in cmd's own
// wiring that those tests cannot see: a route bound to the wrong handler,
// or buildResourcesDeps leaving out a store, the audit callback, the Hub
// SSH key check or cmd's canonical heartbeat writer. The test Hub has no
// database, so the routes whose store is the database (file connections,
// remote bookmarks, push devices) get a request their handler refuses
// before it reads the store.
func TestResourcesRoutesReachHandlersThroughRouteTable(t *testing.T) {
	sut := newTestAPIServer(t)
	seedResourcesWiringDevice(t, sut)
	sut.edgeStore = newMockEdgeStore()
	sut.dependencyStore = newStubDependencyStore()
	sut.groupProfileStore = resourcesWiringProfiles{}
	sut.failoverStore = resourcesWiringFailoverPairs{}
	sut.fileTransferStore = resourcesWiringTransfers{}
	awaitRestart := stubHubRestartForWiring(t)
	admin := createRoleSessionToken(t, sut, "admin-resources-wiring", auth.RoleAdmin)
	call := routeWiringCaller(t, sut.buildHTTPHandlers(nil, nil, nil), admin)

	groupID := routeWiringID(t, call("/groups", http.MethodPost, "/groups", `{"name":"Wiring group"}`, http.StatusCreated, `"name":"Wiring group"`), "group")
	call("/groups/", http.MethodGet, "/groups/"+groupID, "", http.StatusOK, `"name":"Wiring group"`)
	call("/group-profiles", http.MethodGet, "/group-profiles", "", http.StatusOK, `"name":"Wiring profile"`)
	call("/group-profiles/", http.MethodGet, "/group-profiles/gp-wiring", "", http.StatusOK, `"name":"Wiring profile"`)
	call("/group-failover-pairs", http.MethodGet, "/group-failover-pairs", "", http.StatusOK, `"name":"Wiring pair"`)
	call("/group-failover-pairs/", http.MethodGet, "/group-failover-pairs/fp-wiring", "", http.StatusOK, `"name":"Wiring pair"`)

	call("/assets", http.MethodGet, "/assets", "", http.StatusOK, `"assets":[{"id":"node-1"`)
	call("/assets/manual", http.MethodPost, "/assets/manual", `{"name":"Wiring manual","host":"192.0.2.10"}`, http.StatusCreated, `"source":"manual"`)
	call("/assets/heartbeat", http.MethodPost, "/assets/heartbeat",
		`{"asset_id":"node-hb","type":"host","name":"node-hb","source":"lab-script","status":"online","platform":"linux"}`, http.StatusAccepted, `"status":"recorded"`)
	if refs, err := sut.canonicalStore.ListResourceExternalRefs("node-hb"); err != nil || len(refs) == 0 {
		t.Fatalf("heartbeat wrote no canonical record: refs=%v err=%v", refs, err)
	}
	call("/assets/", http.MethodGet, "/assets/node-1", "", http.StatusOK, `"hub_ssh_key_enabled":false`)
	call("/metrics/overview", http.MethodGet, "/metrics/overview", "", http.StatusOK, `"asset_id":"node-1"`)
	call("/metrics/assets/", http.MethodGet, "/metrics/assets/node-1", "", http.StatusOK, `"series":[`)
	call("/telemetry/frontend/perf", http.MethodPost, "/telemetry/frontend/perf", `{"route":"/x","metric":"lcp","value":1}`, http.StatusBadRequest, "route must be one of dashboard")
	call("/telemetry/mobile/client", http.MethodPost, "/telemetry/mobile/client", `{"events":[{"route":"dashboard"}]}`, http.StatusBadRequest, "metric is required")

	call("/api/v1/file-connections", http.MethodPost, "/api/v1/file-connections", `{"name":"x","protocol":"bogus"}`, http.StatusBadRequest, "host is required")
	call("/api/v1/file-connections/", http.MethodPost, "/api/v1/file-connections/test", `{"protocol":"bogus"}`, http.StatusBadRequest, "protocol and host are required")
	bookmark := `{"label":"x","protocol":"ssh","host":"192.0.2.10"}`
	call("/api/v1/remote-bookmarks", http.MethodPost, "/api/v1/remote-bookmarks", bookmark, http.StatusBadRequest, "protocol must be one of: vnc, rdp, spice, ard")
	call("/api/v1/remote-bookmarks/", http.MethodPost, "/api/v1/remote-bookmarks/", bookmark, http.StatusBadRequest, "protocol must be one of: vnc, rdp, spice, ard")
	call("/api/v1/file-transfers", http.MethodGet, "/api/v1/file-transfers", "", http.StatusOK, `"id":"ft-wiring"`)
	call("/api/v1/file-transfers/", http.MethodGet, "/api/v1/file-transfers/ft-missing", "", http.StatusNotFound, "file transfer not found")

	call("/settings/retention", http.MethodGet, "/settings/retention", "", http.StatusOK, `"presets"`)
	call("/settings/restart", http.MethodPost, "/settings/restart", "", http.StatusAccepted, "Backend restart requested")
	awaitRestart()
	call("/audit/events", http.MethodGet, "/audit/events", "", http.StatusOK, `"type":"settings.restart.requested"`)

	depID := routeWiringID(t, call("/dependencies", http.MethodPost, "/dependencies",
		`{"source_asset_id":"node-1","target_asset_id":"node-2","relationship_type":"depends_on"}`, http.StatusCreated, `"relationship_type":"depends_on"`), "dependency")
	call("/dependencies/", http.MethodGet, "/dependencies/"+depID, "", http.StatusOK, `"relationship_type":"depends_on"`)
	call("/links/suggestions", http.MethodGet, "/links/suggestions", "", http.StatusOK, `"suggestions":[`)
	call("/links/suggestions/", http.MethodPut, "/links/suggestions/nope", `{"status":"accepted"}`, http.StatusNotFound, "link suggestion not found")
	call("/links/manual", http.MethodPost, "/links/manual", `{"source_id":"node-1","target_id":"node-2"}`, http.StatusCreated, `"relationship_type":"contains"`)
	edgeID := routeWiringID(t, call("/edges", http.MethodPost, "/edges",
		`{"source_asset_id":"node-1","target_asset_id":"node-2","relationship_type":"runs_on"}`, http.StatusCreated, `"relationship_type":"runs_on"`), "edge")
	call("/edges/", http.MethodGet, "/edges/"+edgeID, "", http.StatusOK, `"relationship_type":"runs_on"`)
	call("/composites", http.MethodPost, "/composites", `{"primary_asset_id":"node-1","facet_asset_ids":["node-2"]}`, http.StatusCreated, `"composite":{"composite_id"`)
	call("/composites/", http.MethodGet, "/composites/nope", "", http.StatusNotFound, "composite not found")
	call("/discovery/proposals", http.MethodGet, "/discovery/proposals", "", http.StatusOK, `"proposals"`)
	call("/discovery/proposals/", http.MethodPost, "/discovery/proposals/nope/accept", "", http.StatusNotFound, "proposal not found")
	call("/assets/bulk/move", http.MethodPut, "/assets/bulk/move", `{"asset_ids":[]}`, http.StatusBadRequest, "asset_ids is required")

	checkID := routeWiringID(t, call("/synthetic-checks", http.MethodPost, "/synthetic-checks",
		`{"name":"Wiring check","check_type":"http","target":"http://192.0.2.10/"}`, http.StatusCreated, `"name":"Wiring check"`), "check")
	call("/synthetic-checks/", http.MethodGet, "/synthetic-checks/"+checkID, "", http.StatusOK, `"name":"Wiring check"`)
	call("/api/v1/devices/register", http.MethodPost, "/api/v1/devices/register", `{}`, http.StatusBadRequest, "device_id and push_token")
	call("/api/v1/devices/deregister", http.MethodPost, "/api/v1/devices/deregister", `{}`, http.StatusBadRequest, "device_id is required")
}

// TestResourcesAgentRoutesReachHandlersThroughRouteTable sends one
// signed-in admin request to each of the nine resources routes that ask the
// device's agent, through the real route table and auth. The agent is a
// test WebSocket that answers each list request with one marked item, and
// its answers go through cmd's own agent message router. This catches a
// route bound to the wrong handler, and buildResourcesDeps leaving out the
// agent manager or a reply registry the router delivers to.
func TestResourcesAgentRoutesReachHandlersThroughRouteTable(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()
	seedResourcesWiringDevice(t, sut)
	serverConn, agentConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()
	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-1", "linux"))
	defer sut.agentMgr.Unregister("node-1")
	router := sut.buildWSRouter()
	go func() {
		from := &agentmgr.AgentConn{AssetID: "node-1"}
		for {
			var msg agentmgr.Message
			if agentConn.ReadJSON(&msg) != nil {
				return
			}
			var req struct {
				RequestID string `json:"request_id"`
			}
			reply, ok := resourcesWiringAgentReplies[msg.Type]
			if !ok || json.Unmarshal(msg.Data, &req) != nil {
				continue
			}
			sut.dispatchAgentWebSocketMessage(router, "node-1", from, agentmgr.Message{
				Type: reply[0], ID: req.RequestID, Data: json.RawMessage(fmt.Sprintf(reply[1], req.RequestID)),
			})
		}
	}()
	admin := createRoleSessionToken(t, sut, "admin-resources-agent-wiring", auth.RoleAdmin)
	call := routeWiringCaller(t, sut.buildHTTPHandlers(nil, nil, nil), admin)

	call("/files/", http.MethodGet, "/files/node-1/list?path=%2Ftmp", "", http.StatusOK, "wiring-file")
	call("/processes/", http.MethodGet, "/processes/node-1", "", http.StatusOK, "wiring-proc")
	call("/disks/", http.MethodGet, "/disks/node-1", "", http.StatusOK, "wiring-disk")
	call("/network/", http.MethodGet, "/network/node-1", "", http.StatusOK, "wiring-nic")
	call("/packages/", http.MethodGet, "/packages/node-1", "", http.StatusOK, "wiring-pkg")
	call("/cron/", http.MethodGet, "/cron/node-1", "", http.StatusOK, "wiring-cron")
	call("/users/", http.MethodGet, "/users/node-1", "", http.StatusOK, "wiring-user")
	call("/services/", http.MethodGet, "/services/node-1", "", http.StatusOK, "wiring-svc")
	call("/logs/journal/", http.MethodGet, "/logs/journal/node-1", "", http.StatusOK, "wiring-journal")
}

// resourcesWiringAgentReplies maps each list request an agent route sends
// to the reply type and body (with the request ID) the test agent sends.
var resourcesWiringAgentReplies = map[string][2]string{
	agentmgr.MsgFileList:     {agentmgr.MsgFileListed, `{"request_id":%q,"path":"/tmp","entries":[{"name":"wiring-file"}]}`},
	agentmgr.MsgProcessList:  {agentmgr.MsgProcessListed, `{"request_id":%q,"processes":[{"pid":7,"name":"wiring-proc"}]}`},
	agentmgr.MsgDiskList:     {agentmgr.MsgDiskListed, `{"request_id":%q,"mounts":[{"device":"wiring-disk","mount_point":"/"}]}`},
	agentmgr.MsgNetworkList:  {agentmgr.MsgNetworkListed, `{"request_id":%q,"interfaces":[{"name":"wiring-nic","state":"up"}]}`},
	agentmgr.MsgPackageList:  {agentmgr.MsgPackageListed, `{"request_id":%q,"packages":[{"name":"wiring-pkg","version":"1.0","status":"installed"}]}`},
	agentmgr.MsgCronList:     {agentmgr.MsgCronListed, `{"request_id":%q,"entries":[{"source":"crontab","command":"wiring-cron"}]}`},
	agentmgr.MsgUsersList:    {agentmgr.MsgUsersListed, `{"request_id":%q,"sessions":[{"username":"wiring-user"}]}`},
	agentmgr.MsgServiceList:  {agentmgr.MsgServiceListed, `{"request_id":%q,"services":[{"name":"wiring-svc"}]}`},
	agentmgr.MsgJournalQuery: {agentmgr.MsgJournalEntries, `{"request_id":%q,"entries":[{"message":"wiring-journal"}]}`},
}

func seedResourcesWiringDevice(t *testing.T, sut *apiServer) {
	t.Helper()
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "node-1", Type: "host", Name: "node-1", Source: "agent", Status: "online", Platform: "linux",
	}); err != nil {
		t.Fatalf("seed device: %v", err)
	}
}

// stubHubRestartForWiring replaces the restart route's signal to the Hub
// process, which would end the test binary, with a stand-in. The route
// sends it 300 ms after it answers; the returned function waits for that
// call. The real one is put back only once the stand-in was called. If it
// was not called, a restart may still be on its way (the test may have
// stopped between the request and the wait), so the stand-in stays.
func stubHubRestartForWiring(t *testing.T) func() {
	t.Helper()
	orig := respkg.HubRestartSelf
	restarted := make(chan struct{})
	var once sync.Once
	respkg.HubRestartSelf = func() error {
		once.Do(func() { close(restarted) })
		return nil
	}
	var asked bool
	t.Cleanup(func() {
		select {
		case <-restarted:
			respkg.HubRestartSelf = orig
		case <-time.After(time.Second):
			if asked {
				t.Error("the restart stand-in was never called; it stays in place")
			}
		}
	})
	return func() {
		asked = true
		select {
		case <-restarted:
		case <-time.After(5 * time.Second):
			t.Fatal("the restart route did not ask the Hub to restart")
		}
	}
}

// Stores the test Hub leaves nil, each with the one record the routes read.
type resourcesWiringProfiles struct{ persistence.GroupProfileStore }

func (resourcesWiringProfiles) ListGroupProfiles(int) ([]groupprofiles.Profile, error) {
	return []groupprofiles.Profile{{ID: "gp-wiring", Name: "Wiring profile"}}, nil
}

func (resourcesWiringProfiles) GetGroupProfile(id string) (groupprofiles.Profile, bool, error) {
	return groupprofiles.Profile{ID: id, Name: "Wiring profile"}, id == "gp-wiring", nil
}

type resourcesWiringFailoverPairs struct{ persistence.FailoverStore }

func (resourcesWiringFailoverPairs) ListFailoverPairs(int) ([]groupfailover.FailoverPair, error) {
	return []groupfailover.FailoverPair{{ID: "fp-wiring", Name: "Wiring pair"}}, nil
}

func (resourcesWiringFailoverPairs) GetFailoverPair(id string) (groupfailover.FailoverPair, bool, error) {
	return groupfailover.FailoverPair{ID: id, Name: "Wiring pair"}, id == "fp-wiring", nil
}

type resourcesWiringTransfers struct{ persistence.FileTransferStore }

func (resourcesWiringTransfers) ListFileTransfers(_ context.Context, actorID, _ string, _, _ int) ([]persistence.FileTransfer, int, error) {
	return []persistence.FileTransfer{{ID: "ft-wiring", ActorID: actorID}}, 1, nil
}

func (resourcesWiringTransfers) GetFileTransfer(context.Context, string) (*persistence.FileTransfer, error) {
	return nil, persistence.ErrNotFound
}

// TestResourcesRoutesBuildDepsPerRequest catches a route bound to a
// resources Deps built once, when the route table was built. cmd builds the
// resources Deps for each request (ensureResourcesDeps), so a store set on
// the server after start is the one a request reads. A route that took
// s.ensureResourcesDeps().H as a method value would keep the first store.
func TestResourcesRoutesBuildDepsPerRequest(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.groupProfileStore = resourcesWiringProfiles{}
	admin := createRoleSessionToken(t, sut, "admin-resources-per-request", auth.RoleAdmin)
	call := routeWiringCaller(t, sut.buildHTTPHandlers(nil, nil, nil), admin)

	call("/group-profiles", http.MethodGet, "/group-profiles", "", http.StatusOK, `"name":"Wiring profile"`)
	sut.groupProfileStore = resourcesLaterProfiles{}
	call("/group-profiles", http.MethodGet, "/group-profiles", "", http.StatusOK, `"name":"Later profile"`)
	call("/group-profiles/", http.MethodGet, "/group-profiles/gp-later", "", http.StatusOK, `"name":"Later profile"`)
}

type resourcesLaterProfiles struct{ persistence.GroupProfileStore }

func (resourcesLaterProfiles) ListGroupProfiles(int) ([]groupprofiles.Profile, error) {
	return []groupprofiles.Profile{{ID: "gp-later", Name: "Later profile"}}, nil
}

func (resourcesLaterProfiles) GetGroupProfile(id string) (groupprofiles.Profile, bool, error) {
	return groupprofiles.Profile{ID: id, Name: "Later profile"}, id == "gp-later", nil
}
