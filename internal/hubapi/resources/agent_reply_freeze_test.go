package resources

import (
	"net/http"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/testutil"
)

// These pin how the device requests that send one message to the agent and
// wait for one reply behave (see testutil/agent_reply_freeze.go). The waits
// and key rules differ per request; the device, connection and reply-type
// rules are the same for all.

type depsHandler func(*Deps, http.ResponseWriter, *http.Request)
type depsDeliver func(*Deps, *agentmgr.AgentConn, agentmgr.Message)

func bindDeps(site testutil.ReplySite, handle depsHandler, deliver depsDeliver, waiters func(*Deps) *AgentReplies) testutil.ReplySite {
	site.Bind = func(_ *testing.T, manager *agentmgr.AgentManager) testutil.ReplyHarness {
		d := &Deps{
			AgentMgr:       manager,
			DiskReplies:    &AgentReplies{},
			CronReplies:    &AgentReplies{},
			UsersReplies:   &AgentReplies{},
			JournalReplies: &AgentReplies{},
			ProcessReplies: &AgentReplies{},
			ServiceReplies: &AgentReplies{},
			NetworkReplies: &AgentReplies{},
			PackageReplies: &AgentReplies{},
		}
		return testutil.ReplyHarness{
			Handle:     func(w http.ResponseWriter, r *http.Request) { handle(d, w, r) },
			Deliver:    func(conn *agentmgr.AgentConn, msg agentmgr.Message) { deliver(d, conn, msg) },
			Route:      func(conn *agentmgr.AgentConn, msg agentmgr.Message) { resourceReplyHandlers[msg.Type](d, conn, msg) },
			Pending:    func() int { return waiters(d).Len() },
			SetTimeout: func(pick func(time.Duration) time.Duration) { d.replyTimeout = pick },
		}
	}
	return site
}

// The reply handlers the agent router calls, by message type.
var resourceReplyHandlers = map[string]depsDeliver{
	agentmgr.MsgDiskListed: (*Deps).ProcessAgentDiskListed, agentmgr.MsgCronListed: (*Deps).ProcessAgentCronListed,
	agentmgr.MsgUsersListed: (*Deps).ProcessAgentUsersListed, agentmgr.MsgJournalEntries: (*Deps).ProcessAgentJournalEntries,
	agentmgr.MsgProcessListed: (*Deps).ProcessAgentProcessListed, agentmgr.MsgProcessKillResult: (*Deps).ProcessAgentProcessKillResult,
	agentmgr.MsgServiceListed: (*Deps).ProcessAgentServiceListed, agentmgr.MsgServiceResult: (*Deps).ProcessAgentServiceResult,
	agentmgr.MsgNetworkListed: (*Deps).ProcessAgentNetworkListed, agentmgr.MsgNetworkResult: (*Deps).ProcessAgentNetworkResult,
	agentmgr.MsgPackageListed: (*Deps).ProcessAgentPackageListed, agentmgr.MsgPackageResult: (*Deps).ProcessAgentPackageResult,
}

// Replies of the other type in each shared registry, for RunReplyForeignTypes.
func foreign(site testutil.ReplySite, replyType string, build func(id, marker string) any) testutil.ReplySite {
	site.Foreign = []testutil.ForeignReply{{Type: replyType, Reply: build}}
	return site
}

var (
	processListedReply = func(id, m string) any { return agentmgr.ProcessListedData{RequestID: id, Error: m} }
	processKillReply   = func(_, m string) any { return agentmgr.ProcessKillResultData{PID: 42, Error: m} }
	serviceListedReply = func(id, m string) any { return agentmgr.ServiceListedData{RequestID: id, Error: m} }
	serviceResultReply = func(id, m string) any { return agentmgr.ServiceResultData{RequestID: id, Error: m} }
	networkListedReply = func(id, m string) any { return agentmgr.NetworkListedData{RequestID: id, Error: m} }
	networkResultReply = func(id, m string) any { return agentmgr.NetworkResultData{RequestID: id, Error: m} }
	packageListedReply = func(id, m string) any { return packageListedResponseWire{RequestID: id, Error: m} }
	packageResultReply = func(id, m string) any { return agentmgr.PackageResultData{RequestID: id, Error: m} }
)

// Accepted-reply rules, one per group of requests.
var (
	// Disk, cron, users, journal, process list, service and network.
	matchesPayloadID = map[string]bool{
		testutil.ReplyWithoutConn: false, testutil.ReplyFromOtherCase: false,
		testutil.ReplyOtherEnvelopeID: true, testutil.ReplyNoEnvelopeID: true, testutil.ReplyOtherPayloadID: false,
	}
	// Process kill results carry no request_id; the envelope ID is the key.
	matchesEnvelopeID = map[string]bool{
		testutil.ReplyWithoutConn: false, testutil.ReplyFromOtherCase: false,
		testutil.ReplyOtherEnvelopeID: false, testutil.ReplyNoEnvelopeID: false,
	}
	// Package replies with another envelope ID are pinned by
	// TestProcessAgentPackage{Listed,Result}RejectsMismatchedSender.
	matchesPackageIDs = map[string]bool{
		testutil.ReplyWithoutConn: false, testutil.ReplyFromOtherCase: false,
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
	disks := func(d *Deps) *AgentReplies { return d.DiskReplies }
	crons := func(d *Deps) *AgentReplies { return d.CronReplies }
	users := func(d *Deps) *AgentReplies { return d.UsersReplies }
	journal := func(d *Deps) *AgentReplies { return d.JournalReplies }
	processes := func(d *Deps) *AgentReplies { return d.ProcessReplies }
	services := func(d *Deps) *AgentReplies { return d.ServiceReplies }
	networks := func(d *Deps) *AgentReplies { return d.NetworkReplies }
	packages := func(d *Deps) *AgentReplies { return d.PackageReplies }
	return []testutil.ReplySite{
		bindDeps(with(get("disk list", "/disks/nas01", agentmgr.MsgDiskList, agentmgr.MsgDiskListed, 10*time.Second), matchesPayloadID, offlinePinned,
			func(id, m string) any { return agentmgr.DiskListedData{RequestID: id, Error: m} }),
			(*Deps).HandleDisks, (*Deps).ProcessAgentDiskListed, disks),
		bindDeps(with(get("cron list", "/cron/nas01", agentmgr.MsgCronList, agentmgr.MsgCronListed, 10*time.Second), matchesPayloadID, "",
			func(id, m string) any { return agentmgr.CronListedData{RequestID: id, Error: m} }),
			(*Deps).HandleCrons, (*Deps).ProcessAgentCronListed, crons),
		bindDeps(with(get("users list", "/users/nas01", agentmgr.MsgUsersList, agentmgr.MsgUsersListed, 10*time.Second), matchesPayloadID, "",
			func(id, m string) any { return agentmgr.UsersListedData{RequestID: id, Error: m} }),
			(*Deps).HandleUsers, (*Deps).ProcessAgentUsersListed, users),
		bindDeps(with(get("journal query", "/logs/journal/nas01", agentmgr.MsgJournalQuery, agentmgr.MsgJournalEntries, 20*time.Second), matchesPayloadID, "",
			func(id, m string) any { return agentmgr.JournalEntriesData{RequestID: id, Error: m} }),
			(*Deps).HandleJournalLogs, (*Deps).ProcessAgentJournalEntries, journal),
		bindDeps(foreign(with(get("process list", "/processes/nas01", agentmgr.MsgProcessList, agentmgr.MsgProcessListed, 10*time.Second), matchesPayloadID, "",
			processListedReply), agentmgr.MsgProcessKillResult, processKillReply),
			(*Deps).HandleProcesses, (*Deps).ProcessAgentProcessListed, processes),
		bindDeps(func() testutil.ReplySite {
			site := foreign(with(post("process kill", "/processes/nas01/kill", `{"pid":42}`, agentmgr.MsgProcessKill, agentmgr.MsgProcessKillResult, 30*time.Second), matchesEnvelopeID, "",
				processKillReply), agentmgr.MsgProcessListed, processListedReply)
			site.ByEnvelope = true
			return site
		}(), (*Deps).HandleProcesses, (*Deps).ProcessAgentProcessKillResult, processes),
		bindDeps(foreign(with(get("service list", "/services/nas01", agentmgr.MsgServiceList, agentmgr.MsgServiceListed, 10*time.Second), matchesPayloadID, offlinePinned,
			serviceListedReply), agentmgr.MsgServiceResult, serviceResultReply),
			(*Deps).HandleServices, (*Deps).ProcessAgentServiceListed, services),
		bindDeps(foreign(with(post("service action", "/services/nas01/restart", `{"service":"sshd"}`, agentmgr.MsgServiceAction, agentmgr.MsgServiceResult, 30*time.Second), matchesPayloadID, "",
			serviceResultReply), agentmgr.MsgServiceListed, serviceListedReply),
			(*Deps).HandleServices, (*Deps).ProcessAgentServiceResult, services),
		bindDeps(foreign(with(get("network list", "/network/nas01", agentmgr.MsgNetworkList, agentmgr.MsgNetworkListed, 10*time.Second), matchesPayloadID, offlinePinned,
			networkListedReply), agentmgr.MsgNetworkResult, networkResultReply),
			(*Deps).HandleNetworks, (*Deps).ProcessAgentNetworkListed, networks),
		bindDeps(foreign(with(post("network action", "/network/nas01/snapshot", `{}`, agentmgr.MsgNetworkAction, agentmgr.MsgNetworkResult, 90*time.Second), matchesPayloadID, "",
			networkResultReply), agentmgr.MsgNetworkListed, networkListedReply),
			(*Deps).HandleNetworks, (*Deps).ProcessAgentNetworkResult, networks),
		bindDeps(foreign(with(get("package list", "/packages/nas01", agentmgr.MsgPackageList, agentmgr.MsgPackageListed, 30*time.Second), matchesPackageIDs, offlinePinned,
			packageListedReply), agentmgr.MsgPackageResult, packageResultReply),
			(*Deps).HandlePackages, (*Deps).ProcessAgentPackageListed, packages),
		bindDeps(foreign(with(post("package action", "/packages/nas01/install", `{"packages":["jq"]}`, agentmgr.MsgPackageAction, agentmgr.MsgPackageResult, 10*time.Minute), matchesPackageIDs, offlinePinned,
			packageResultReply), agentmgr.MsgPackageListed, packageListedReply),
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

func TestResourceAgentRequestDropsRepliesOfAnotherType(t *testing.T) {
	testutil.RunReplyForeignTypes(t, resourceReplySites())
}

func TestResourceAgentRequestEndsWhenTheAgentDisconnects(t *testing.T) {
	testutil.RunReplyDisconnects(t, resourceReplySites())
}
