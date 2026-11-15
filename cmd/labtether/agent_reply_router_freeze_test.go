package main

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/testutil"
)

// TestKnownMessageTypes checks only the router's keys. This checks that each
// reply type of a request that waits for one agent reply, put through the
// live router, answers the request that is waiting for it.
func TestWSRouterDeliversEachReplyToItsWaitingRequest(t *testing.T) {
	// The display answer also asks for the RDP gateway; a closed loopback
	// port makes that answer come at once.
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	t.Setenv("GUACD_HOST", "127.0.0.1")
	t.Setenv("GUACD_PORT", "9")

	routed := func(name, method, path, body, sends, reply string, build func(id, marker string) any,
		handle func(*apiServer) http.HandlerFunc, waiters func(*apiServer) *sync.Map) testutil.ReplySite {
		return testutil.ReplySite{
			Name: name, Method: method, Path: path, Body: body, Sends: sends, ReplyType: reply, Reply: build,
			ByEnvelope: reply == agentmgr.MsgProcessKillResult,
			Bind: func(t *testing.T, manager *agentmgr.AgentManager) testutil.ReplyHarness {
				srv := newTestAPIServer(t)
				srv.agentMgr = manager
				router := srv.buildWSRouter()
				route, ok := router[reply]
				if !ok {
					t.Fatalf("router has no entry for %q", reply)
				}
				return testutil.ReplyHarness{
					Handle:  handle(srv),
					Deliver: route,
					Pending: func() int {
						n := 0
						waiters(srv).Range(func(any, any) bool { n++; return true })
						return n
					},
					SetTimeout: func(func(time.Duration) time.Duration) {},
				}
			},
		}
	}
	get, post := http.MethodGet, http.MethodPost
	sites := []testutil.ReplySite{
		routed("disk listed", get, "/disks/nas01", "", agentmgr.MsgDiskList, agentmgr.MsgDiskListed,
			func(id, m string) any { return agentmgr.DiskListedData{RequestID: id, Error: m} },
			func(s *apiServer) http.HandlerFunc { return s.ensureResourcesDeps().HandleDisks },
			func(s *apiServer) *sync.Map { return &s.diskBridges }),
		routed("cron listed", get, "/cron/nas01", "", agentmgr.MsgCronList, agentmgr.MsgCronListed,
			func(id, m string) any { return agentmgr.CronListedData{RequestID: id, Error: m} },
			func(s *apiServer) http.HandlerFunc { return s.ensureResourcesDeps().HandleCrons },
			func(s *apiServer) *sync.Map { return &s.cronBridges }),
		routed("users listed", get, "/users/nas01", "", agentmgr.MsgUsersList, agentmgr.MsgUsersListed,
			func(id, m string) any { return agentmgr.UsersListedData{RequestID: id, Error: m} },
			func(s *apiServer) http.HandlerFunc { return s.ensureResourcesDeps().HandleUsers },
			func(s *apiServer) *sync.Map { return &s.usersBridges }),
		routed("journal entries", get, "/logs/journal/nas01", "", agentmgr.MsgJournalQuery, agentmgr.MsgJournalEntries,
			func(id, m string) any { return agentmgr.JournalEntriesData{RequestID: id, Error: m} },
			func(s *apiServer) http.HandlerFunc { return s.ensureResourcesDeps().HandleJournalLogs },
			func(s *apiServer) *sync.Map { return &s.journalBridges }),
		routed("process listed", get, "/processes/nas01", "", agentmgr.MsgProcessList, agentmgr.MsgProcessListed,
			func(id, m string) any { return agentmgr.ProcessListedData{RequestID: id, Error: m} },
			func(s *apiServer) http.HandlerFunc { return s.ensureResourcesDeps().HandleProcesses },
			func(s *apiServer) *sync.Map { return &s.processBridges }),
		routed("process kill result", post, "/processes/nas01/kill", `{"pid":42}`, agentmgr.MsgProcessKill, agentmgr.MsgProcessKillResult,
			func(_, m string) any { return agentmgr.ProcessKillResultData{PID: 42, Error: m} },
			func(s *apiServer) http.HandlerFunc { return s.ensureResourcesDeps().HandleProcesses },
			func(s *apiServer) *sync.Map { return &s.processBridges }),
		routed("service listed", get, "/services/nas01", "", agentmgr.MsgServiceList, agentmgr.MsgServiceListed,
			func(id, m string) any { return agentmgr.ServiceListedData{RequestID: id, Error: m} },
			func(s *apiServer) http.HandlerFunc { return s.ensureResourcesDeps().HandleServices },
			func(s *apiServer) *sync.Map { return &s.serviceBridges }),
		routed("service result", post, "/services/nas01/restart", `{"service":"sshd"}`, agentmgr.MsgServiceAction, agentmgr.MsgServiceResult,
			func(id, m string) any { return agentmgr.ServiceResultData{RequestID: id, Error: m} },
			func(s *apiServer) http.HandlerFunc { return s.ensureResourcesDeps().HandleServices },
			func(s *apiServer) *sync.Map { return &s.serviceBridges }),
		routed("network listed", get, "/network/nas01", "", agentmgr.MsgNetworkList, agentmgr.MsgNetworkListed,
			func(id, m string) any { return agentmgr.NetworkListedData{RequestID: id, Error: m} },
			func(s *apiServer) http.HandlerFunc { return s.ensureResourcesDeps().HandleNetworks },
			func(s *apiServer) *sync.Map { return &s.networkBridges }),
		routed("network result", post, "/network/nas01/snapshot", `{}`, agentmgr.MsgNetworkAction, agentmgr.MsgNetworkResult,
			func(id, m string) any { return agentmgr.NetworkResultData{RequestID: id, Error: m} },
			func(s *apiServer) http.HandlerFunc { return s.ensureResourcesDeps().HandleNetworks },
			func(s *apiServer) *sync.Map { return &s.networkBridges }),
		routed("package listed", get, "/packages/nas01", "", agentmgr.MsgPackageList, agentmgr.MsgPackageListed,
			func(id, m string) any { return agentmgr.PackageListedData{RequestID: id, Error: m} },
			func(s *apiServer) http.HandlerFunc { return s.ensureResourcesDeps().HandlePackages },
			func(s *apiServer) *sync.Map { return &s.packageBridges }),
		routed("package result", post, "/packages/nas01/install", `{"packages":["jq"]}`, agentmgr.MsgPackageAction, agentmgr.MsgPackageResult,
			func(id, m string) any { return agentmgr.PackageResultData{RequestID: id, Error: m} },
			func(s *apiServer) http.HandlerFunc { return s.ensureResourcesDeps().HandlePackages },
			func(s *apiServer) *sync.Map { return &s.packageBridges }),
		routed("desktop displays", get, "/api/v1/nodes/nas01/displays", "", agentmgr.MsgDesktopListDisplays, agentmgr.MsgDesktopDisplays,
			func(id, m string) any {
				return agentmgr.DisplayListData{RequestID: id, Displays: []agentmgr.DisplayInfo{{Name: m, Width: 1920, Height: 1080}}}
			},
			func(s *apiServer) http.HandlerFunc { return s.ensureDesktopDeps().HandleNodeSubRoutes },
			func(s *apiServer) *sync.Map { return &s.displayBridges }),
		routed("desktop diagnosed", post, "/desktop/diagnose/nas01", "", agentmgr.MsgDesktopDiagnose, agentmgr.MsgDesktopDiagnosed,
			func(id, m string) any { return agentmgr.DesktopDiagnosticData{RequestID: id, DesktopUser: m} },
			func(s *apiServer) http.HandlerFunc { return s.ensureDesktopDeps().HandleDesktopDiagnoseRequest },
			func(s *apiServer) *sync.Map { return &s.desktopDiagnosticWaiters }),
		routed("clipboard data", post, "/api/v1/nodes/nas01/clipboard/get", `{}`, agentmgr.MsgClipboardGet, agentmgr.MsgClipboardData,
			func(id, m string) any { return agentmgr.ClipboardDataPayload{RequestID: id, Format: "text", Error: m} },
			func(s *apiServer) http.HandlerFunc { return s.ensureDesktopDeps().HandleClipboardRoutes },
			func(s *apiServer) *sync.Map { return &s.clipboardBridges }),
		routed("clipboard set ack", post, "/api/v1/nodes/nas01/clipboard/set", `{"format":"text","text":"x"}`, agentmgr.MsgClipboardSet, agentmgr.MsgClipboardSetAck,
			func(id, m string) any { return agentmgr.ClipboardSetAckData{RequestID: id, Error: m} },
			func(s *apiServer) http.HandlerFunc { return s.ensureDesktopDeps().HandleClipboardRoutes },
			func(s *apiServer) *sync.Map { return &s.clipboardBridges }),
	}
	testutil.RunReplyRouting(t, sites)
}
