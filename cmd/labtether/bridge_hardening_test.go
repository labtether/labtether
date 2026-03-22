package main

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
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
	bridge := &processBridge{
		Ch:              make(chan agentmgr.Message, 1),
		ExpectedAssetID: "node-1",
	}
	srv.processBridges.Store("req-asset-bound", bridge)

	payload := marshalAgentMessage(t, agentmgr.ProcessListedData{
		RequestID: "req-asset-bound",
		Processes: []agentmgr.ProcessInfo{{PID: 1, Name: "init"}},
	})

	srv.processAgentProcessListed(&agentmgr.AgentConn{AssetID: "node-2"}, payload)
	select {
	case <-bridge.Ch:
		t.Fatal("expected mismatched sender process payload to be ignored")
	default:
	}

	srv.processAgentProcessListed(&agentmgr.AgentConn{AssetID: "node-1"}, payload)
	select {
	case <-bridge.Ch:
	case <-time.After(time.Second):
		t.Fatal("expected matched sender process payload to be delivered")
	}
}

func TestProcessAgentProcessKillResultRejectsMismatchedSender(t *testing.T) {
	var srv apiServer
	bridge := &processBridge{
		Ch:              make(chan agentmgr.Message, 1),
		ExpectedAssetID: "node-1",
	}
	srv.processBridges.Store("req-kill-asset-bound", bridge)

	payload := marshalAgentMessage(t, agentmgr.ProcessKillResultData{
		PID:     42,
		Success: true,
	})
	payload.Type = agentmgr.MsgProcessKillResult
	payload.ID = "req-kill-asset-bound"

	srv.processAgentProcessKillResult(&agentmgr.AgentConn{AssetID: "node-2"}, payload)
	select {
	case <-bridge.Ch:
		t.Fatal("expected mismatched sender process kill payload to be ignored")
	default:
	}

	srv.processAgentProcessKillResult(&agentmgr.AgentConn{AssetID: "node-1"}, payload)
	select {
	case <-bridge.Ch:
	case <-time.After(time.Second):
		t.Fatal("expected matched sender process kill payload to be delivered")
	}
}

func TestProcessAgentCronListedRejectsMismatchedSender(t *testing.T) {
	var srv apiServer
	bridge := &cronBridge{
		Ch:              make(chan agentmgr.Message, 1),
		ExpectedAssetID: "node-1",
	}
	srv.cronBridges.Store("req-cron-asset-bound", bridge)

	payload := marshalAgentMessage(t, agentmgr.CronListedData{
		RequestID: "req-cron-asset-bound",
		Entries:   []agentmgr.CronEntry{{Source: "crontab", Schedule: "* * * * *", Command: "true", User: "root"}},
	})

	srv.processAgentCronListed(&agentmgr.AgentConn{AssetID: "node-2"}, payload)
	select {
	case <-bridge.Ch:
		t.Fatal("expected mismatched sender cron payload to be ignored")
	default:
	}

	srv.processAgentCronListed(&agentmgr.AgentConn{AssetID: "node-1"}, payload)
	select {
	case <-bridge.Ch:
	case <-time.After(time.Second):
		t.Fatal("expected matched sender cron payload to be delivered")
	}
}

func TestProcessAgentUsersListedRejectsMismatchedSender(t *testing.T) {
	var srv apiServer
	bridge := &usersBridge{
		Ch:              make(chan agentmgr.Message, 1),
		ExpectedAssetID: "node-1",
	}
	srv.usersBridges.Store("req-users-asset-bound", bridge)

	payload := marshalAgentMessage(t, agentmgr.UsersListedData{
		RequestID: "req-users-asset-bound",
		Sessions:  []agentmgr.UserSession{{Username: "alice", Terminal: "pts/0", LoginTime: "2026-03-08T14:30:00Z"}},
	})

	srv.processAgentUsersListed(&agentmgr.AgentConn{AssetID: "node-2"}, payload)
	select {
	case <-bridge.Ch:
		t.Fatal("expected mismatched sender users payload to be ignored")
	default:
	}

	srv.processAgentUsersListed(&agentmgr.AgentConn{AssetID: "node-1"}, payload)
	select {
	case <-bridge.Ch:
	case <-time.After(time.Second):
		t.Fatal("expected matched sender users payload to be delivered")
	}
}

func TestProcessAgentDiskListedRejectsMismatchedSender(t *testing.T) {
	var srv apiServer
	bridge := &diskBridge{
		Ch:              make(chan agentmgr.Message, 1),
		ExpectedAssetID: "node-1",
	}
	srv.diskBridges.Store("req-disk-asset-bound", bridge)

	payload := marshalAgentMessage(t, agentmgr.DiskListedData{
		RequestID: "req-disk-asset-bound",
		Mounts:    []agentmgr.MountInfo{{Device: "/dev/sda1", MountPoint: "/", FSType: "ext4", Total: 100, Used: 40, Available: 60, UsePct: 40}},
	})

	srv.processAgentDiskListed(&agentmgr.AgentConn{AssetID: "node-2"}, payload)
	select {
	case <-bridge.Ch:
		t.Fatal("expected mismatched sender disk payload to be ignored")
	default:
	}

	srv.processAgentDiskListed(&agentmgr.AgentConn{AssetID: "node-1"}, payload)
	select {
	case <-bridge.Ch:
	case <-time.After(time.Second):
		t.Fatal("expected matched sender disk payload to be delivered")
	}
}

func TestProcessAgentServiceListedRejectsMismatchedSender(t *testing.T) {
	var srv apiServer
	bridge := &serviceBridge{
		Ch:              make(chan agentmgr.Message, 1),
		ExpectedAssetID: "node-1",
	}
	srv.serviceBridges.Store("req-svc-asset-bound", bridge)

	payload := marshalAgentMessage(t, agentmgr.ServiceListedData{
		RequestID: "req-svc-asset-bound",
		Services:  []agentmgr.ServiceInfo{{Name: "sshd"}},
	})

	srv.processAgentServiceListed(&agentmgr.AgentConn{AssetID: "node-2"}, payload)
	select {
	case <-bridge.Ch:
		t.Fatal("expected mismatched sender service list payload to be ignored")
	default:
	}

	srv.processAgentServiceListed(&agentmgr.AgentConn{AssetID: "node-1"}, payload)
	select {
	case <-bridge.Ch:
	case <-time.After(time.Second):
		t.Fatal("expected matched sender service list payload to be delivered")
	}
}

func TestProcessAgentServiceResultRejectsMismatchedSender(t *testing.T) {
	var srv apiServer
	bridge := &serviceBridge{
		Ch:              make(chan agentmgr.Message, 1),
		ExpectedAssetID: "node-1",
	}
	srv.serviceBridges.Store("req-svc-result-asset-bound", bridge)

	payload := marshalAgentMessage(t, agentmgr.ServiceResultData{
		RequestID: "req-svc-result-asset-bound",
		OK:        true,
	})

	srv.processAgentServiceResult(&agentmgr.AgentConn{AssetID: "node-2"}, payload)
	select {
	case <-bridge.Ch:
		t.Fatal("expected mismatched sender service result payload to be ignored")
	default:
	}

	srv.processAgentServiceResult(&agentmgr.AgentConn{AssetID: "node-1"}, payload)
	select {
	case <-bridge.Ch:
	case <-time.After(time.Second):
		t.Fatal("expected matched sender service result payload to be delivered")
	}
}

func TestProcessAgentNetworkListedRejectsMismatchedSender(t *testing.T) {
	var srv apiServer
	bridge := &networkBridge{
		Ch:              make(chan agentmgr.Message, 1),
		ExpectedAssetID: "node-1",
	}
	srv.networkBridges.Store("req-net-asset-bound", bridge)

	payload := marshalAgentMessage(t, agentmgr.NetworkListedData{
		RequestID: "req-net-asset-bound",
		Interfaces: []agentmgr.NetInterface{{
			Name: "eth0",
		}},
	})

	srv.processAgentNetworkListed(&agentmgr.AgentConn{AssetID: "node-2"}, payload)
	select {
	case <-bridge.Ch:
		t.Fatal("expected mismatched sender network list payload to be ignored")
	default:
	}

	srv.processAgentNetworkListed(&agentmgr.AgentConn{AssetID: "node-1"}, payload)
	select {
	case <-bridge.Ch:
	case <-time.After(time.Second):
		t.Fatal("expected matched sender network list payload to be delivered")
	}
}

func TestProcessAgentNetworkResultRejectsMismatchedSender(t *testing.T) {
	var srv apiServer
	bridge := &networkBridge{
		Ch:              make(chan agentmgr.Message, 1),
		ExpectedAssetID: "node-1",
	}
	srv.networkBridges.Store("req-net-result-asset-bound", bridge)

	payload := marshalAgentMessage(t, agentmgr.NetworkResultData{
		RequestID: "req-net-result-asset-bound",
		OK:        true,
	})

	srv.processAgentNetworkResult(&agentmgr.AgentConn{AssetID: "node-2"}, payload)
	select {
	case <-bridge.Ch:
		t.Fatal("expected mismatched sender network result payload to be ignored")
	default:
	}

	srv.processAgentNetworkResult(&agentmgr.AgentConn{AssetID: "node-1"}, payload)
	select {
	case <-bridge.Ch:
	case <-time.After(time.Second):
		t.Fatal("expected matched sender network result payload to be delivered")
	}
}

func TestProcessAgentPackageListedRejectsMismatchedSender(t *testing.T) {
	var srv apiServer
	bridge := &packageBridge{
		Ch:              make(chan agentmgr.Message, 1),
		ExpectedAssetID: "node-1",
	}
	srv.packageBridges.Store("req-pkg-asset-bound", bridge)

	payload := marshalAgentMessage(t, agentmgr.PackageListedData{
		RequestID: "req-pkg-asset-bound",
		Packages:  []agentmgr.PackageInfo{{Name: "jq"}},
	})

	srv.processAgentPackageListed(&agentmgr.AgentConn{AssetID: "node-2"}, payload)
	select {
	case <-bridge.Ch:
		t.Fatal("expected mismatched sender package list payload to be ignored")
	default:
	}

	srv.processAgentPackageListed(&agentmgr.AgentConn{AssetID: "node-1"}, payload)
	select {
	case <-bridge.Ch:
	case <-time.After(time.Second):
		t.Fatal("expected matched sender package list payload to be delivered")
	}
}

func TestProcessAgentPackageResultRejectsMismatchedSender(t *testing.T) {
	var srv apiServer
	bridge := &packageBridge{
		Ch:              make(chan agentmgr.Message, 1),
		ExpectedAssetID: "node-1",
	}
	srv.packageBridges.Store("req-pkg-result-asset-bound", bridge)

	payload := marshalAgentMessage(t, agentmgr.PackageResultData{
		RequestID: "req-pkg-result-asset-bound",
		OK:        true,
	})

	srv.processAgentPackageResult(&agentmgr.AgentConn{AssetID: "node-2"}, payload)
	select {
	case <-bridge.Ch:
		t.Fatal("expected mismatched sender package result payload to be ignored")
	default:
	}

	srv.processAgentPackageResult(&agentmgr.AgentConn{AssetID: "node-1"}, payload)
	select {
	case <-bridge.Ch:
	case <-time.After(time.Second):
		t.Fatal("expected matched sender package result payload to be delivered")
	}
}

func TestProcessAgentDesktopDisplaysRejectsMismatchedSender(t *testing.T) {
	var srv apiServer
	bridge := &displayBridge{
		Ch:              make(chan agentmgr.DisplayListData, 1),
		ExpectedAssetID: "node-1",
	}
	srv.displayBridges.Store("req-display-asset-bound", bridge)

	payload := marshalAgentMessage(t, agentmgr.DisplayListData{
		RequestID: "req-display-asset-bound",
		Displays:  []agentmgr.DisplayInfo{{Name: "display-1", Width: 1920, Height: 1080}},
	})

	srv.processAgentDesktopDisplays(&agentmgr.AgentConn{AssetID: "node-2"}, payload)
	select {
	case <-bridge.Ch:
		t.Fatal("expected mismatched sender display payload to be ignored")
	default:
	}

	srv.processAgentDesktopDisplays(&agentmgr.AgentConn{AssetID: "node-1"}, payload)
	select {
	case <-bridge.Ch:
	case <-time.After(time.Second):
		t.Fatal("expected matched sender display payload to be delivered")
	}
}

func TestProcessAgentFileDataRejectsMismatchedSender(t *testing.T) {
	var srv apiServer
	bridge := newFileBridge(1, "node-1")
	defer bridge.Close()
	srv.fileBridges.Store("req-file-asset-bound", bridge)

	payload := marshalAgentMessage(t, agentmgr.FileDataPayload{
		RequestID: "req-file-asset-bound",
		Data:      base64.StdEncoding.EncodeToString([]byte("chunk")),
	})

	srv.processAgentFileData(&agentmgr.AgentConn{AssetID: "node-2"}, payload)
	select {
	case <-bridge.Ch:
		t.Fatal("expected mismatched sender file payload to be ignored")
	default:
	}

	srv.processAgentFileData(&agentmgr.AgentConn{AssetID: "node-1"}, payload)
	select {
	case <-bridge.Ch:
	case <-time.After(time.Second):
		t.Fatal("expected matched sender file payload to be delivered")
	}
}

func TestProcessAgentJournalEntriesRejectsMismatchedSender(t *testing.T) {
	var srv apiServer
	bridge := &journalBridge{
		Ch:              make(chan agentmgr.Message, 1),
		ExpectedAssetID: "node-1",
	}
	srv.journalBridges.Store("req-journal-bound", bridge)

	payload := marshalAgentMessage(t, agentmgr.JournalEntriesData{
		RequestID: "req-journal-bound",
	})

	srv.processAgentJournalEntries(&agentmgr.AgentConn{AssetID: "node-2"}, payload)
	select {
	case <-bridge.Ch:
		t.Fatal("expected mismatched sender journal payload to be ignored")
	default:
	}

	srv.processAgentJournalEntries(&agentmgr.AgentConn{AssetID: "node-1"}, payload)
	select {
	case <-bridge.Ch:
	case <-time.After(time.Second):
		t.Fatal("expected matched sender journal payload to be delivered")
	}
}

func TestParseProcessListLimitClamp(t *testing.T) {
	if got := parseProcessListLimit(""); got != defaultProcessListLimit {
		t.Fatalf("empty limit should default to %d, got %d", defaultProcessListLimit, got)
	}
	if got := parseProcessListLimit("-5"); got != defaultProcessListLimit {
		t.Fatalf("negative limit should default to %d, got %d", defaultProcessListLimit, got)
	}
	if got := parseProcessListLimit("9999"); got != maxProcessListLimit {
		t.Fatalf("oversized limit should clamp to %d, got %d", maxProcessListLimit, got)
	}
	if got := parseProcessListLimit("30"); got != 30 {
		t.Fatalf("expected explicit valid limit, got %d", got)
	}
}

func TestProcessAgentNodeOpsHandlersIgnoreNonBridgeEntries(t *testing.T) {
	t.Run("cron listed", func(t *testing.T) {
		var srv apiServer
		srv.cronBridges.Store("req-cron", "ignore-me")
		srv.processAgentCronListed(nil, marshalAgentMessage(t, agentmgr.CronListedData{RequestID: "req-cron"}))
	})

	t.Run("process listed", func(t *testing.T) {
		var srv apiServer
		srv.processBridges.Store("req-proc", "ignore-me")
		srv.processAgentProcessListed(nil, marshalAgentMessage(t, agentmgr.ProcessListedData{RequestID: "req-proc"}))
	})

	t.Run("process kill result", func(t *testing.T) {
		var srv apiServer
		srv.processBridges.Store("req-proc-kill", "ignore-me")
		msg := marshalAgentMessage(t, agentmgr.ProcessKillResultData{PID: 42, Success: true})
		msg.ID = "req-proc-kill"
		srv.processAgentProcessKillResult(nil, msg)
	})

	t.Run("service listed", func(t *testing.T) {
		var srv apiServer
		srv.serviceBridges.Store("req-svc-list", "ignore-me")
		srv.processAgentServiceListed(nil, marshalAgentMessage(t, agentmgr.ServiceListedData{RequestID: "req-svc-list"}))
	})

	t.Run("service result", func(t *testing.T) {
		var srv apiServer
		srv.serviceBridges.Store("req-svc-result", "ignore-me")
		srv.processAgentServiceResult(nil, marshalAgentMessage(t, agentmgr.ServiceResultData{RequestID: "req-svc-result"}))
	})

	t.Run("network listed", func(t *testing.T) {
		var srv apiServer
		srv.networkBridges.Store("req-net-list", "ignore-me")
		srv.processAgentNetworkListed(nil, marshalAgentMessage(t, agentmgr.NetworkListedData{RequestID: "req-net-list"}))
	})

	t.Run("network result", func(t *testing.T) {
		var srv apiServer
		srv.networkBridges.Store("req-net-result", "ignore-me")
		srv.processAgentNetworkResult(nil, marshalAgentMessage(t, agentmgr.NetworkResultData{RequestID: "req-net-result"}))
	})

	t.Run("disk listed", func(t *testing.T) {
		var srv apiServer
		srv.diskBridges.Store("req-disk", "ignore-me")
		srv.processAgentDiskListed(nil, marshalAgentMessage(t, agentmgr.DiskListedData{RequestID: "req-disk"}))
	})

	t.Run("package listed", func(t *testing.T) {
		var srv apiServer
		srv.packageBridges.Store("req-pkg-list", "ignore-me")
		srv.processAgentPackageListed(nil, marshalAgentMessage(t, agentmgr.PackageListedData{RequestID: "req-pkg-list"}))
	})

	t.Run("package result", func(t *testing.T) {
		var srv apiServer
		srv.packageBridges.Store("req-pkg-result", "ignore-me")
		srv.processAgentPackageResult(nil, marshalAgentMessage(t, agentmgr.PackageResultData{RequestID: "req-pkg-result"}))
	})

	t.Run("users listed", func(t *testing.T) {
		var srv apiServer
		srv.usersBridges.Store("req-users", "ignore-me")
		srv.processAgentUsersListed(nil, marshalAgentMessage(t, agentmgr.UsersListedData{RequestID: "req-users"}))
	})

	t.Run("journal entries", func(t *testing.T) {
		var srv apiServer
		srv.journalBridges.Store("req-journal", "ignore-me")
		srv.processAgentJournalEntries(nil, marshalAgentMessage(t, agentmgr.JournalEntriesData{RequestID: "req-journal"}))
	})

	t.Run("desktop displays", func(t *testing.T) {
		var srv apiServer
		srv.displayBridges.Store("req-display", "ignore-me")
		srv.processAgentDesktopDisplays(nil, marshalAgentMessage(t, agentmgr.DisplayListData{RequestID: "req-display"}))
	})
}

func TestProcessAgentFileHandlersIgnoreNonBridgeEntries(t *testing.T) {
	var srv apiServer
	srv.fileBridges.Store("req-file", "ignore-me")

	srv.processAgentFileListed(nil, marshalAgentMessage(t, agentmgr.FileListedData{RequestID: "req-file"}))
	srv.processAgentFileData(nil, marshalAgentMessage(t, agentmgr.FileDataPayload{
		RequestID: "req-file",
		Data:      base64.StdEncoding.EncodeToString([]byte("chunk")),
	}))
	srv.processAgentFileWritten(nil, marshalAgentMessage(t, agentmgr.FileWrittenData{RequestID: "req-file"}))
	srv.processAgentFileResult(nil, marshalAgentMessage(t, agentmgr.FileResultData{RequestID: "req-file"}))
}
