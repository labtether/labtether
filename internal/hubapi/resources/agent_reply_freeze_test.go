package resources

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/testutil"
)

// These pin today's behaviour of the device requests that send one message
// to the agent and wait for one reply (see testutil/agent_reply_freeze.go).
// The waits, answers and reply matching differ per request on purpose or by
// history; moving them onto a shared helper must keep each one as listed.

type depsHandler func(*Deps, http.ResponseWriter, *http.Request)
type depsDeliver func(*Deps, *agentmgr.AgentConn, agentmgr.Message)

func bindDeps(site testutil.ReplySite, handle depsHandler, deliver depsDeliver, waiters func(*Deps) *sync.Map) testutil.ReplySite {
	site.Bind = func(_ *testing.T, manager *agentmgr.AgentManager) testutil.ReplyHarness {
		d := &Deps{
			AgentMgr:       manager,
			DiskBridges:    &sync.Map{},
			CronBridges:    &sync.Map{},
			UsersBridges:   &sync.Map{},
			JournalBridges: &sync.Map{},
			ProcessBridges: &sync.Map{},
			ServiceBridges: &sync.Map{},
			NetworkBridges: &sync.Map{},
			PackageBridges: &sync.Map{},
		}
		return testutil.ReplyHarness{
			Handle:  func(w http.ResponseWriter, r *http.Request) { handle(d, w, r) },
			Deliver: func(conn *agentmgr.AgentConn, msg agentmgr.Message) { deliver(d, conn, msg) },
			Pending: func() int {
				n := 0
				waiters(d).Range(func(any, any) bool { n++; return true })
				return n
			},
			SetTimeout: func(pick func(time.Duration) time.Duration) { d.replyTimeout = pick },
		}
	}
	return site
}

// Accepted-reply rules, one per group of requests.
var (
	// Disk, cron and users skip the device check for a reply with no connection.
	acceptsNilConn = map[string]bool{
		testutil.ReplyWithoutConn: true, testutil.ReplyFromOtherCase: true,
		testutil.ReplyOtherEnvelopeID: true, testutil.ReplyNoEnvelopeID: true, testutil.ReplyOtherPayloadID: false,
	}
	matchesPayloadID = map[string]bool{
		testutil.ReplyWithoutConn: false, testutil.ReplyFromOtherCase: true,
		testutil.ReplyOtherEnvelopeID: true, testutil.ReplyNoEnvelopeID: true, testutil.ReplyOtherPayloadID: false,
	}
	// Process kill results carry no request_id; the envelope ID is the key.
	matchesEnvelopeID = map[string]bool{
		testutil.ReplyWithoutConn: false, testutil.ReplyFromOtherCase: true,
		testutil.ReplyOtherEnvelopeID: false, testutil.ReplyNoEnvelopeID: false,
	}
	// Package replies with another envelope ID are pinned by
	// TestProcessAgentPackage{Listed,Result}RejectsMismatchedSender.
	matchesPackageIDs = map[string]bool{
		testutil.ReplyWithoutConn: false, testutil.ReplyFromOtherCase: true,
		testutil.ReplyNoEnvelopeID: true, testutil.ReplyOtherPayloadID: false,
	}
)

// Replies from another device are pinned for every site below by the
// ...RejectsMismatchedSender tests in cmd/labtether/bridge_hardening_test.go.
const offlinePinned = "TestDeviceRoutesSayAgentIsNotConnected"

func resourceReplySites() []testutil.ReplySite {
	get := func(name, path, sends, reply string, wait time.Duration) testutil.ReplySite {
		return testutil.ReplySite{Name: name, Method: http.MethodGet, Path: path, Sends: sends, ReplyType: reply, Wait: wait}
	}
	post := func(name, path, body, sends, reply string, wait time.Duration) testutil.ReplySite {
		return testutil.ReplySite{Name: name, Method: http.MethodPost, Path: path, Body: body, Sends: sends, ReplyType: reply, Wait: wait}
	}
	with := func(site testutil.ReplySite, accepts map[string]bool, offline string, build func(id, marker string) any) testutil.ReplySite {
		site.Accepts, site.OfflinePinnedBy, site.Reply = accepts, offline, build
		return site
	}
	disks := func(d *Deps) *sync.Map { return d.DiskBridges }
	crons := func(d *Deps) *sync.Map { return d.CronBridges }
	users := func(d *Deps) *sync.Map { return d.UsersBridges }
	journal := func(d *Deps) *sync.Map { return d.JournalBridges }
	processes := func(d *Deps) *sync.Map { return d.ProcessBridges }
	services := func(d *Deps) *sync.Map { return d.ServiceBridges }
	networks := func(d *Deps) *sync.Map { return d.NetworkBridges }
	packages := func(d *Deps) *sync.Map { return d.PackageBridges }
	return []testutil.ReplySite{
		bindDeps(with(get("disk list", "/disks/nas01", agentmgr.MsgDiskList, agentmgr.MsgDiskListed, 10*time.Second), acceptsNilConn, offlinePinned,
			func(id, m string) any { return agentmgr.DiskListedData{RequestID: id, Error: m} }),
			(*Deps).HandleDisks, (*Deps).ProcessAgentDiskListed, disks),
		bindDeps(with(get("cron list", "/cron/nas01", agentmgr.MsgCronList, agentmgr.MsgCronListed, 10*time.Second), acceptsNilConn, "",
			func(id, m string) any { return agentmgr.CronListedData{RequestID: id, Error: m} }),
			(*Deps).HandleCrons, (*Deps).ProcessAgentCronListed, crons),
		bindDeps(with(get("users list", "/users/nas01", agentmgr.MsgUsersList, agentmgr.MsgUsersListed, 10*time.Second), acceptsNilConn, "",
			func(id, m string) any { return agentmgr.UsersListedData{RequestID: id, Error: m} }),
			(*Deps).HandleUsers, (*Deps).ProcessAgentUsersListed, users),
		bindDeps(with(get("journal query", "/logs/journal/nas01", agentmgr.MsgJournalQuery, agentmgr.MsgJournalEntries, 20*time.Second), matchesPayloadID, "",
			func(id, m string) any { return agentmgr.JournalEntriesData{RequestID: id, Error: m} }),
			(*Deps).HandleJournalLogs, (*Deps).ProcessAgentJournalEntries, journal),
		bindDeps(with(get("process list", "/processes/nas01", agentmgr.MsgProcessList, agentmgr.MsgProcessListed, 10*time.Second), matchesPayloadID, "",
			func(id, m string) any { return agentmgr.ProcessListedData{RequestID: id, Error: m} }),
			(*Deps).HandleProcesses, (*Deps).ProcessAgentProcessListed, processes),
		bindDeps(func() testutil.ReplySite {
			site := with(post("process kill", "/processes/nas01/kill", `{"pid":42}`, agentmgr.MsgProcessKill, agentmgr.MsgProcessKillResult, 30*time.Second), matchesEnvelopeID, "",
				func(_, m string) any { return agentmgr.ProcessKillResultData{PID: 42, Error: m} })
			site.ByEnvelope = true
			return site
		}(), (*Deps).HandleProcesses, (*Deps).ProcessAgentProcessKillResult, processes),
		bindDeps(with(get("service list", "/services/nas01", agentmgr.MsgServiceList, agentmgr.MsgServiceListed, 10*time.Second), matchesPayloadID, offlinePinned,
			func(id, m string) any { return agentmgr.ServiceListedData{RequestID: id, Error: m} }),
			(*Deps).HandleServices, (*Deps).ProcessAgentServiceListed, services),
		bindDeps(with(post("service action", "/services/nas01/restart", `{"service":"sshd"}`, agentmgr.MsgServiceAction, agentmgr.MsgServiceResult, 30*time.Second), matchesPayloadID, "",
			func(id, m string) any { return agentmgr.ServiceResultData{RequestID: id, Error: m} }),
			(*Deps).HandleServices, (*Deps).ProcessAgentServiceResult, services),
		bindDeps(with(get("network list", "/network/nas01", agentmgr.MsgNetworkList, agentmgr.MsgNetworkListed, 10*time.Second), matchesPayloadID, offlinePinned,
			func(id, m string) any { return agentmgr.NetworkListedData{RequestID: id, Error: m} }),
			(*Deps).HandleNetworks, (*Deps).ProcessAgentNetworkListed, networks),
		bindDeps(with(post("network action", "/network/nas01/snapshot", `{}`, agentmgr.MsgNetworkAction, agentmgr.MsgNetworkResult, 90*time.Second), matchesPayloadID, "",
			func(id, m string) any { return agentmgr.NetworkResultData{RequestID: id, Error: m} }),
			(*Deps).HandleNetworks, (*Deps).ProcessAgentNetworkResult, networks),
		bindDeps(with(get("package list", "/packages/nas01", agentmgr.MsgPackageList, agentmgr.MsgPackageListed, 30*time.Second), matchesPackageIDs, offlinePinned,
			func(id, m string) any { return packageListedResponseWire{RequestID: id, Error: m} }),
			(*Deps).HandlePackages, (*Deps).ProcessAgentPackageListed, packages),
		bindDeps(with(post("package action", "/packages/nas01/install", `{"packages":["jq"]}`, agentmgr.MsgPackageAction, agentmgr.MsgPackageResult, 10*time.Minute), matchesPackageIDs, offlinePinned,
			func(id, m string) any { return agentmgr.PackageResultData{RequestID: id, Error: m} }),
			(*Deps).HandlePackages, (*Deps).ProcessAgentPackageResult, packages),
	}
}

func TestResourceAgentRequestTimeouts(t *testing.T) {
	testutil.RunReplyTimeouts(t, resourceReplySites())
}

func TestResourceAgentRequestSendFailures(t *testing.T) {
	testutil.RunReplySendFailures(t, resourceReplySites())
}

func TestResourceAgentRequestOffline(t *testing.T) {
	testutil.RunReplyOffline(t, resourceReplySites())
}

func TestResourceAgentRequestDuplicateReplies(t *testing.T) {
	testutil.RunReplyDuplicates(t, resourceReplySites())
}

func TestResourceAgentRequestReplyMatching(t *testing.T) {
	testutil.RunReplyVariants(t, resourceReplySites())
}
