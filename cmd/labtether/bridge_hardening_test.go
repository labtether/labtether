package main

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	respkg "github.com/labtether/labtether/internal/hubapi/resources"
	"github.com/labtether/labtether/internal/hubapi/testutil"
)

func marshalAgentMessage(t *testing.T, payload any) agentmgr.Message {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return agentmgr.Message{Data: data}
}

func TestProcessAgentProcessListedRejectsMismatchedSender(t *testing.T) {
	var srv apiServer
	replies := testutil.AwaitReply(t, &srv.processReplies, "req-asset-bound", "node-1")

	payload := marshalAgentMessage(t, agentmgr.ProcessListedData{
		RequestID: "req-asset-bound",
		Processes: []agentmgr.ProcessInfo{{PID: 1, Name: "init"}},
	})

	srv.ensureResourcesDeps().ProcessAgentProcessListed(&agentmgr.AgentConn{AssetID: "node-2"}, payload)
	select {
	case <-replies:
		t.Fatal("expected mismatched sender process payload to be ignored")
	default:
	}

	srv.ensureResourcesDeps().ProcessAgentProcessListed(&agentmgr.AgentConn{AssetID: "node-1"}, payload)
	select {
	case <-replies:
	case <-time.After(time.Second):
		t.Fatal("expected matched sender process payload to be delivered")
	}
}

func TestProcessAgentProcessKillResultRejectsMismatchedSender(t *testing.T) {
	var srv apiServer
	replies := testutil.AwaitReply(t, &srv.processReplies, "req-kill-asset-bound", "node-1")

	payload := marshalAgentMessage(t, agentmgr.ProcessKillResultData{
		PID:     42,
		Success: true,
	})
	payload.Type = agentmgr.MsgProcessKillResult
	payload.ID = "req-kill-asset-bound"

	srv.ensureResourcesDeps().ProcessAgentProcessKillResult(&agentmgr.AgentConn{AssetID: "node-2"}, payload)
	select {
	case <-replies:
		t.Fatal("expected mismatched sender process kill payload to be ignored")
	default:
	}

	srv.ensureResourcesDeps().ProcessAgentProcessKillResult(&agentmgr.AgentConn{AssetID: "node-1"}, payload)
	select {
	case <-replies:
	case <-time.After(time.Second):
		t.Fatal("expected matched sender process kill payload to be delivered")
	}
}

func TestProcessAgentCronListedRejectsMismatchedSender(t *testing.T) {
	var srv apiServer
	replies := testutil.AwaitReply(t, &srv.cronReplies, "req-cron-asset-bound", "node-1")

	payload := marshalAgentMessage(t, agentmgr.CronListedData{
		RequestID: "req-cron-asset-bound",
		Entries:   []agentmgr.CronEntry{{Source: "crontab", Schedule: "* * * * *", Command: "true", User: "root"}},
	})

	srv.ensureResourcesDeps().ProcessAgentCronListed(&agentmgr.AgentConn{AssetID: "node-2"}, payload)
	select {
	case <-replies:
		t.Fatal("expected mismatched sender cron payload to be ignored")
	default:
	}

	srv.ensureResourcesDeps().ProcessAgentCronListed(&agentmgr.AgentConn{AssetID: "node-1"}, payload)
	select {
	case <-replies:
	case <-time.After(time.Second):
		t.Fatal("expected matched sender cron payload to be delivered")
	}
}

func TestProcessAgentUsersListedRejectsMismatchedSender(t *testing.T) {
	var srv apiServer
	replies := testutil.AwaitReply(t, &srv.usersReplies, "req-users-asset-bound", "node-1")

	payload := marshalAgentMessage(t, agentmgr.UsersListedData{
		RequestID: "req-users-asset-bound",
		Sessions:  []agentmgr.UserSession{{Username: "alice", Terminal: "pts/0", LoginTime: "2026-03-08T14:30:00Z"}},
	})

	srv.ensureResourcesDeps().ProcessAgentUsersListed(&agentmgr.AgentConn{AssetID: "node-2"}, payload)
	select {
	case <-replies:
		t.Fatal("expected mismatched sender users payload to be ignored")
	default:
	}

	srv.ensureResourcesDeps().ProcessAgentUsersListed(&agentmgr.AgentConn{AssetID: "node-1"}, payload)
	select {
	case <-replies:
	case <-time.After(time.Second):
		t.Fatal("expected matched sender users payload to be delivered")
	}
}

func TestProcessAgentDiskListedRejectsMismatchedSender(t *testing.T) {
	var srv apiServer
	replies := testutil.AwaitReply(t, &srv.diskReplies, "req-disk-asset-bound", "node-1")

	payload := marshalAgentMessage(t, agentmgr.DiskListedData{
		RequestID: "req-disk-asset-bound",
		Mounts:    []agentmgr.MountInfo{{Device: "/dev/sda1", MountPoint: "/", FSType: "ext4", Total: 100, Used: 40, Available: 60, UsePct: 40}},
	})

	srv.ensureResourcesDeps().ProcessAgentDiskListed(&agentmgr.AgentConn{AssetID: "node-2"}, payload)
	select {
	case <-replies:
		t.Fatal("expected mismatched sender disk payload to be ignored")
	default:
	}

	srv.ensureResourcesDeps().ProcessAgentDiskListed(&agentmgr.AgentConn{AssetID: "node-1"}, payload)
	select {
	case <-replies:
	case <-time.After(time.Second):
		t.Fatal("expected matched sender disk payload to be delivered")
	}
}

func TestProcessAgentServiceListedRejectsMismatchedSender(t *testing.T) {
	var srv apiServer
	replies := testutil.AwaitReply(t, &srv.serviceReplies, "req-svc-asset-bound", "node-1")

	payload := marshalAgentMessage(t, agentmgr.ServiceListedData{
		RequestID: "req-svc-asset-bound",
		Services:  []agentmgr.ServiceInfo{{Name: "sshd"}},
	})

	srv.ensureResourcesDeps().ProcessAgentServiceListed(&agentmgr.AgentConn{AssetID: "node-2"}, payload)
	select {
	case <-replies:
		t.Fatal("expected mismatched sender service list payload to be ignored")
	default:
	}

	srv.ensureResourcesDeps().ProcessAgentServiceListed(&agentmgr.AgentConn{AssetID: "node-1"}, payload)
	select {
	case <-replies:
	case <-time.After(time.Second):
		t.Fatal("expected matched sender service list payload to be delivered")
	}
}

func TestProcessAgentServiceResultRejectsMismatchedSender(t *testing.T) {
	var srv apiServer
	replies := testutil.AwaitReply(t, &srv.serviceReplies, "req-svc-result-asset-bound", "node-1")

	payload := marshalAgentMessage(t, agentmgr.ServiceResultData{
		RequestID: "req-svc-result-asset-bound",
		OK:        true,
	})

	srv.ensureResourcesDeps().ProcessAgentServiceResult(&agentmgr.AgentConn{AssetID: "node-2"}, payload)
	select {
	case <-replies:
		t.Fatal("expected mismatched sender service result payload to be ignored")
	default:
	}

	srv.ensureResourcesDeps().ProcessAgentServiceResult(&agentmgr.AgentConn{AssetID: "node-1"}, payload)
	select {
	case <-replies:
	case <-time.After(time.Second):
		t.Fatal("expected matched sender service result payload to be delivered")
	}
}

func TestProcessAgentNetworkListedRejectsMismatchedSender(t *testing.T) {
	var srv apiServer
	replies := testutil.AwaitReply(t, &srv.networkReplies, "req-net-asset-bound", "node-1")

	payload := marshalAgentMessage(t, agentmgr.NetworkListedData{
		RequestID: "req-net-asset-bound",
		Interfaces: []agentmgr.NetInterface{{
			Name: "eth0",
		}},
	})

	srv.ensureResourcesDeps().ProcessAgentNetworkListed(&agentmgr.AgentConn{AssetID: "node-2"}, payload)
	select {
	case <-replies:
		t.Fatal("expected mismatched sender network list payload to be ignored")
	default:
	}

	srv.ensureResourcesDeps().ProcessAgentNetworkListed(&agentmgr.AgentConn{AssetID: "node-1"}, payload)
	select {
	case <-replies:
	case <-time.After(time.Second):
		t.Fatal("expected matched sender network list payload to be delivered")
	}
}

func TestProcessAgentNetworkResultRejectsMismatchedSender(t *testing.T) {
	var srv apiServer
	replies := testutil.AwaitReply(t, &srv.networkReplies, "req-net-result-asset-bound", "node-1")

	payload := marshalAgentMessage(t, agentmgr.NetworkResultData{
		RequestID: "req-net-result-asset-bound",
		OK:        true,
	})

	srv.ensureResourcesDeps().ProcessAgentNetworkResult(&agentmgr.AgentConn{AssetID: "node-2"}, payload)
	select {
	case <-replies:
		t.Fatal("expected mismatched sender network result payload to be ignored")
	default:
	}

	srv.ensureResourcesDeps().ProcessAgentNetworkResult(&agentmgr.AgentConn{AssetID: "node-1"}, payload)
	select {
	case <-replies:
	case <-time.After(time.Second):
		t.Fatal("expected matched sender network result payload to be delivered")
	}
}

func TestProcessAgentPackageListedRejectsMismatchedSender(t *testing.T) {
	var srv apiServer
	replies := testutil.AwaitReply(t, &srv.packageReplies, "req-pkg-asset-bound", "node-1")

	payload := marshalAgentMessage(t, agentmgr.PackageListedData{
		RequestID: "req-pkg-asset-bound",
		Packages:  []agentmgr.PackageInfo{{Name: "jq"}},
	})

	srv.ensureResourcesDeps().ProcessAgentPackageListed(&agentmgr.AgentConn{AssetID: "node-2"}, payload)
	select {
	case <-replies:
		t.Fatal("expected mismatched sender package list payload to be ignored")
	default:
	}

	payload.ID = "different-request"
	srv.ensureResourcesDeps().ProcessAgentPackageListed(&agentmgr.AgentConn{AssetID: "node-1"}, payload)
	select {
	case <-replies:
		t.Fatal("expected mismatched envelope/package request id to be ignored")
	default:
	}

	payload.ID = "req-pkg-asset-bound"
	srv.ensureResourcesDeps().ProcessAgentPackageListed(&agentmgr.AgentConn{AssetID: "node-1"}, payload)
	select {
	case <-replies:
	case <-time.After(time.Second):
		t.Fatal("expected matched sender package list payload to be delivered")
	}
}

func TestProcessAgentPackageResultRejectsMismatchedSender(t *testing.T) {
	var srv apiServer
	replies := testutil.AwaitReply(t, &srv.packageReplies, "req-pkg-result-asset-bound", "node-1")

	payload := marshalAgentMessage(t, agentmgr.PackageResultData{
		RequestID: "req-pkg-result-asset-bound",
		OK:        true,
	})

	srv.ensureResourcesDeps().ProcessAgentPackageResult(&agentmgr.AgentConn{AssetID: "node-2"}, payload)
	select {
	case <-replies:
		t.Fatal("expected mismatched sender package result payload to be ignored")
	default:
	}

	payload.ID = "different-request"
	srv.ensureResourcesDeps().ProcessAgentPackageResult(&agentmgr.AgentConn{AssetID: "node-1"}, payload)
	select {
	case <-replies:
		t.Fatal("expected mismatched envelope/package result id to be ignored")
	default:
	}

	payload.ID = "req-pkg-result-asset-bound"
	srv.ensureResourcesDeps().ProcessAgentPackageResult(&agentmgr.AgentConn{AssetID: "node-1"}, payload)
	select {
	case <-replies:
	case <-time.After(time.Second):
		t.Fatal("expected matched sender package result payload to be delivered")
	}
}

func TestProcessAgentDesktopDisplaysRejectsMismatchedSender(t *testing.T) {
	var srv apiServer
	replies := testutil.AwaitReply(t, &srv.displayReplies, "req-display-asset-bound", "node-1")

	payload := marshalAgentMessage(t, agentmgr.DisplayListData{
		RequestID: "req-display-asset-bound",
		Displays:  []agentmgr.DisplayInfo{{Name: "display-1", Width: 1920, Height: 1080}},
	})

	srv.ensureDesktopDeps().ProcessAgentDesktopDisplays(&agentmgr.AgentConn{AssetID: "node-2"}, payload)
	select {
	case <-replies:
		t.Fatal("expected mismatched sender display payload to be ignored")
	default:
	}

	srv.ensureDesktopDeps().ProcessAgentDesktopDisplays(&agentmgr.AgentConn{AssetID: "node-1"}, payload)
	select {
	case <-replies:
	case <-time.After(time.Second):
		t.Fatal("expected matched sender display payload to be delivered")
	}
}

func TestProcessAgentFileDataRejectsMismatchedSender(t *testing.T) {
	var srv apiServer
	bridge := respkg.NewFileBridge(1, "node-1")
	defer bridge.Close()
	srv.fileBridges.Store("req-file-asset-bound", bridge)

	payload := marshalAgentMessage(t, agentmgr.FileDataPayload{
		RequestID: "req-file-asset-bound",
		Data:      base64.StdEncoding.EncodeToString([]byte("chunk")),
	})

	srv.ensureResourcesDeps().ProcessAgentFileData(&agentmgr.AgentConn{AssetID: "node-2"}, payload)
	select {
	case <-bridge.Ch:
		t.Fatal("expected mismatched sender file payload to be ignored")
	default:
	}

	srv.ensureResourcesDeps().ProcessAgentFileData(&agentmgr.AgentConn{AssetID: "node-1"}, payload)
	select {
	case <-bridge.Ch:
	case <-time.After(time.Second):
		t.Fatal("expected matched sender file payload to be delivered")
	}
}

func TestProcessAgentJournalEntriesRejectsMismatchedSender(t *testing.T) {
	var srv apiServer
	replies := testutil.AwaitReply(t, &srv.journalReplies, "req-journal-bound", "node-1")

	payload := marshalAgentMessage(t, agentmgr.JournalEntriesData{
		RequestID: "req-journal-bound",
	})

	srv.ensureResourcesDeps().ProcessAgentJournalEntries(&agentmgr.AgentConn{AssetID: "node-2"}, payload)
	select {
	case <-replies:
		t.Fatal("expected mismatched sender journal payload to be ignored")
	default:
	}

	srv.ensureResourcesDeps().ProcessAgentJournalEntries(&agentmgr.AgentConn{AssetID: "node-1"}, payload)
	select {
	case <-replies:
	case <-time.After(time.Second):
		t.Fatal("expected matched sender journal payload to be delivered")
	}
}

func TestParseProcessListLimitClamp(t *testing.T) {
	if got := respkg.ParseProcessListLimit(""); got != respkg.DefaultProcessListLimit {
		t.Fatalf("empty limit should default to %d, got %d", respkg.DefaultProcessListLimit, got)
	}
	if got := respkg.ParseProcessListLimit("-5"); got != respkg.DefaultProcessListLimit {
		t.Fatalf("negative limit should default to %d, got %d", respkg.DefaultProcessListLimit, got)
	}
	if got := respkg.ParseProcessListLimit("9999"); got != respkg.MaxProcessListLimit {
		t.Fatalf("oversized limit should clamp to %d, got %d", respkg.MaxProcessListLimit, got)
	}
	if got := respkg.ParseProcessListLimit("30"); got != 30 {
		t.Fatalf("expected explicit valid limit, got %d", got)
	}
}

func TestProcessAgentNodeOpsHandlersIgnoreNonBridgeEntries(t *testing.T) {
	// The reply registries are typed now, so a non-bridge entry can no
	// longer be stored; each handler still gets a reply with no waiter and
	// no connection and must drop it without a panic.
	t.Run("cron listed", func(t *testing.T) {
		var srv apiServer
		srv.ensureResourcesDeps().ProcessAgentCronListed(nil, marshalAgentMessage(t, agentmgr.CronListedData{RequestID: "req-cron"}))
	})

	t.Run("process listed", func(t *testing.T) {
		var srv apiServer
		srv.ensureResourcesDeps().ProcessAgentProcessListed(nil, marshalAgentMessage(t, agentmgr.ProcessListedData{RequestID: "req-proc"}))
	})

	t.Run("process kill result", func(t *testing.T) {
		var srv apiServer
		msg := marshalAgentMessage(t, agentmgr.ProcessKillResultData{PID: 42, Success: true})
		msg.ID = "req-proc-kill"
		srv.ensureResourcesDeps().ProcessAgentProcessKillResult(nil, msg)
	})

	t.Run("service listed", func(t *testing.T) {
		var srv apiServer
		srv.ensureResourcesDeps().ProcessAgentServiceListed(nil, marshalAgentMessage(t, agentmgr.ServiceListedData{RequestID: "req-svc-list"}))
	})

	t.Run("service result", func(t *testing.T) {
		var srv apiServer
		srv.ensureResourcesDeps().ProcessAgentServiceResult(nil, marshalAgentMessage(t, agentmgr.ServiceResultData{RequestID: "req-svc-result"}))
	})

	t.Run("network listed", func(t *testing.T) {
		var srv apiServer
		srv.ensureResourcesDeps().ProcessAgentNetworkListed(nil, marshalAgentMessage(t, agentmgr.NetworkListedData{RequestID: "req-net-list"}))
	})

	t.Run("network result", func(t *testing.T) {
		var srv apiServer
		srv.ensureResourcesDeps().ProcessAgentNetworkResult(nil, marshalAgentMessage(t, agentmgr.NetworkResultData{RequestID: "req-net-result"}))
	})

	t.Run("disk listed", func(t *testing.T) {
		var srv apiServer
		srv.ensureResourcesDeps().ProcessAgentDiskListed(nil, marshalAgentMessage(t, agentmgr.DiskListedData{RequestID: "req-disk"}))
	})

	t.Run("package listed", func(t *testing.T) {
		var srv apiServer
		srv.ensureResourcesDeps().ProcessAgentPackageListed(nil, marshalAgentMessage(t, agentmgr.PackageListedData{RequestID: "req-pkg-list"}))
	})

	t.Run("package result", func(t *testing.T) {
		var srv apiServer
		srv.ensureResourcesDeps().ProcessAgentPackageResult(nil, marshalAgentMessage(t, agentmgr.PackageResultData{RequestID: "req-pkg-result"}))
	})

	t.Run("users listed", func(t *testing.T) {
		var srv apiServer
		srv.ensureResourcesDeps().ProcessAgentUsersListed(nil, marshalAgentMessage(t, agentmgr.UsersListedData{RequestID: "req-users"}))
	})

	t.Run("journal entries", func(t *testing.T) {
		var srv apiServer
		srv.ensureResourcesDeps().ProcessAgentJournalEntries(nil, marshalAgentMessage(t, agentmgr.JournalEntriesData{RequestID: "req-journal"}))
	})

	t.Run("desktop displays", func(t *testing.T) {
		var srv apiServer
		srv.ensureDesktopDeps().ProcessAgentDesktopDisplays(nil, marshalAgentMessage(t, agentmgr.DisplayListData{RequestID: "req-display"}))
	})
}

func TestProcessAgentFileHandlersIgnoreNonBridgeEntries(t *testing.T) {
	var srv apiServer
	srv.fileBridges.Store("req-file", "ignore-me")

	srv.ensureResourcesDeps().ProcessAgentFileListed(nil, marshalAgentMessage(t, agentmgr.FileListedData{RequestID: "req-file"}))
	srv.ensureResourcesDeps().ProcessAgentFileData(nil, marshalAgentMessage(t, agentmgr.FileDataPayload{
		RequestID: "req-file",
		Data:      base64.StdEncoding.EncodeToString([]byte("chunk")),
	}))
	srv.ensureResourcesDeps().ProcessAgentFileWritten(nil, marshalAgentMessage(t, agentmgr.FileWrittenData{RequestID: "req-file"}))
	srv.ensureResourcesDeps().ProcessAgentFileResult(nil, marshalAgentMessage(t, agentmgr.FileResultData{RequestID: "req-file"}))
}
