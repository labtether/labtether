package resources

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentcaps"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/servicehttp"
)

const unsupportedDevice = "node01"

// deviceRoute is one device route that sends one request to the agent.
type deviceRoute struct {
	name, method, target, body, sends string
	handle                            func(*Deps, http.ResponseWriter, *http.Request)
}

func onAsset(handle func(*Deps, http.ResponseWriter, *http.Request, string)) func(*Deps, http.ResponseWriter, *http.Request) {
	return func(d *Deps, w http.ResponseWriter, r *http.Request) { handle(d, w, r, unsupportedDevice) }
}

var gatedDeviceRoutes = []deviceRoute{
	{"disks", "GET", "/disks/node01", "", agentmgr.MsgDiskList, (*Deps).HandleDisks},
	{"process list", "GET", "/processes/node01", "", agentmgr.MsgProcessList, (*Deps).HandleProcesses},
	{"process kill", "POST", "/processes/node01/kill", `{"pid":4242,"signal":"SIGTERM"}`, agentmgr.MsgProcessKill, (*Deps).HandleProcesses},
	{"service list", "GET", "/services/node01", "", agentmgr.MsgServiceList, (*Deps).HandleServices},
	{"service action", "POST", "/services/node01/restart", `{"service":"nginx"}`, agentmgr.MsgServiceAction, (*Deps).HandleServices},
	{"network action", "POST", "/network/node01/apply", `{}`, agentmgr.MsgNetworkAction, (*Deps).HandleNetworks},
	{"package list", "GET", "/packages/node01", "", agentmgr.MsgPackageList, (*Deps).HandlePackages},
	{"package action", "POST", "/packages/node01/install", `{"packages":["htop"]}`, agentmgr.MsgPackageAction, (*Deps).HandlePackages},
	{"cron", "GET", "/cron/node01", "", agentmgr.MsgCronList, (*Deps).HandleCrons},
	{"users", "GET", "/users/node01", "", agentmgr.MsgUsersList, (*Deps).HandleUsers},
	{"journal", "GET", "/logs/journal/node01", "", agentmgr.MsgJournalQuery, (*Deps).HandleJournalLogs},
	{"file list", "GET", "/files/node01/list?path=/etc", "", agentmgr.MsgFileList, onAsset((*Deps).HandleFileList)},
	{"file download", "GET", "/files/node01/download?path=/etc/hosts", "", agentmgr.MsgFileRead, onAsset((*Deps).HandleFileDownload)},
	{"file upload", "POST", "/files/node01/upload?path=/tmp/a.txt", "hello", agentmgr.MsgFileWrite, onAsset((*Deps).HandleFileUpload)},
	{"file mkdir", "POST", "/files/node01/mkdir?path=/tmp/new", "", agentmgr.MsgFileMkdir, onAsset((*Deps).HandleFileMkdir)},
	{"file delete", "DELETE", "/files/node01/delete?path=/tmp/a.txt", "", agentmgr.MsgFileDelete, onAsset((*Deps).HandleFileDelete)},
	{"file rename", "POST", "/files/node01/rename?old_path=/tmp/a&new_path=/tmp/b", "", agentmgr.MsgFileRename, onAsset((*Deps).HandleFileRename)},
	{"file copy", "POST", "/files/node01/copy?src_path=/tmp/a&dst_path=/tmp/b", "", agentmgr.MsgFileCopy, onAsset((*Deps).HandleFileCopy)},
}

func routeDeps(peer *testutil.AgentPeer) *Deps {
	return &Deps{
		AgentMgr:    peer.Manager,
		FileBridges: &sync.Map{},
		DiskReplies: &AgentReplies{}, CronReplies: &AgentReplies{}, UsersReplies: &AgentReplies{},
		JournalReplies: &AgentReplies{}, ProcessReplies: &AgentReplies{}, ServiceReplies: &AgentReplies{},
		NetworkReplies: &AgentReplies{}, PackageReplies: &AgentReplies{},
		replyTimeout: func(time.Duration) time.Duration { return 100 * time.Millisecond },
	}
}

func routeRequest(route deviceRoute) *http.Request {
	return httptest.NewRequest(route.method, route.target, strings.NewReader(route.body))
}

// Fault: a device route sends a request the explicit agent never advertised
// (it times out), or refuses it with words or a status other than the one
// fixed answer.
func TestExplicitAgentRoutesRefuseWithoutSending(t *testing.T) {
	for _, route := range gatedDeviceRoutes {
		t.Run(route.name, func(t *testing.T) {
			peer := testutil.NewAgentPeer(t, unsupportedDevice)
			agentcaps.Record(peer.Conn, map[string]string{agentcaps.MetaMode: agentcaps.ModeExplicit, "cap_network": "list"})
			rec := httptest.NewRecorder()
			route.handle(routeDeps(peer), rec, routeRequest(route))
			var body struct{ Error string }
			_ = json.Unmarshal(rec.Body.Bytes(), &body)
			if rec.Code != http.StatusUnprocessableEntity || body.Error != servicehttp.AgentUnsupportedMessage {
				t.Fatalf("got %d %s, want 422 %q", rec.Code, rec.Body.String(), servicehttp.AgentUnsupportedMessage)
			}
			peer.Silent(t, 150*time.Millisecond)
		})
	}
}

// Fault: the gate also stops an agent without the signal (every Go agent),
// or an explicit agent's advertised request.
func TestRoutesStillSendWhenOffered(t *testing.T) {
	for _, route := range gatedDeviceRoutes {
		for name, metadata := range map[string]map[string]string{
			"no signal":     nil,
			"unknown value": {agentcaps.MetaMode: "complete"},
		} {
			t.Run(route.name+"/"+name, func(t *testing.T) {
				peer := testutil.NewAgentPeer(t, unsupportedDevice)
				if metadata != nil {
					agentcaps.Record(peer.Conn, metadata)
				}
				d := routeDeps(peer)
				go route.handle(d, httptest.NewRecorder(), routeRequest(route))
				if got := peer.Next(t); got.Type != route.sends {
					t.Fatalf("sent %s, want %s", got.Type, route.sends)
				}
			})
		}
	}
	t.Run("explicit network list", func(t *testing.T) {
		peer := testutil.NewAgentPeer(t, unsupportedDevice)
		agentcaps.Record(peer.Conn, map[string]string{agentcaps.MetaMode: agentcaps.ModeExplicit, "cap_network": "list"})
		go (*Deps).HandleNetworks(routeDeps(peer), httptest.NewRecorder(), httptest.NewRequest("GET", "/network/node01", nil))
		if got := peer.Next(t); got.Type != agentmgr.MsgNetworkList {
			t.Fatalf("sent %s, want %s", got.Type, agentmgr.MsgNetworkList)
		}
	})
}
