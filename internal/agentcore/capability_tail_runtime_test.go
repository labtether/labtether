package agentcore

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentcore/backends"
	"github.com/labtether/labtether/internal/agentcore/system"
	"github.com/labtether/labtether/internal/agentmgr"
)

type staticCronBackend struct {
	entries []agentmgr.CronEntry
	err     error
}

func (b staticCronBackend) ListEntries() ([]agentmgr.CronEntry, error) {
	return b.entries, b.err
}

func TestCronManagerHandleCronListReturnsEntries(t *testing.T) {
	transport, messages, cleanup := newAgentcoreCapturedTransport(t)
	defer cleanup()

	manager := &backends.CronManager{
		Backend: staticCronBackend{
			entries: []agentmgr.CronEntry{{
				Source:   "crontab",
				Schedule: "*/5 * * * *",
				Command:  "/usr/local/bin/backup",
				User:     "root",
			}},
		},
	}

	raw, err := json.Marshal(agentmgr.CronListData{RequestID: "req-cron"})
	if err != nil {
		t.Fatalf("marshal cron list request: %v", err)
	}
	manager.HandleCronList(transport, agentmgr.Message{Data: raw})

	msg := waitForCapturedAgentMessage(t, messages, agentmgr.MsgCronListed, 2*time.Second)
	var listed agentmgr.CronListedData
	if err := json.Unmarshal(msg.Data, &listed); err != nil {
		t.Fatalf("decode cron listed payload: %v", err)
	}
	if listed.RequestID != "req-cron" {
		t.Fatalf("request_id=%q, want req-cron", listed.RequestID)
	}
	if listed.Error != "" {
		t.Fatalf("unexpected error %q", listed.Error)
	}
	if len(listed.Entries) != 1 || listed.Entries[0].Command != "/usr/local/bin/backup" {
		t.Fatalf("unexpected entries %+v", listed.Entries)
	}
}

func TestCronManagerHandleCronListReportsCollectionError(t *testing.T) {
	transport, messages, cleanup := newAgentcoreCapturedTransport(t)
	defer cleanup()

	manager := &backends.CronManager{Backend: staticCronBackend{err: errors.New("collect failed")}}

	raw, err := json.Marshal(agentmgr.CronListData{RequestID: "req-cron-error"})
	if err != nil {
		t.Fatalf("marshal cron list request: %v", err)
	}
	manager.HandleCronList(transport, agentmgr.Message{Data: raw})

	msg := waitForCapturedAgentMessage(t, messages, agentmgr.MsgCronListed, 2*time.Second)
	var listed agentmgr.CronListedData
	if err := json.Unmarshal(msg.Data, &listed); err != nil {
		t.Fatalf("decode cron listed payload: %v", err)
	}
	if listed.Error != "collect failed" {
		t.Fatalf("error=%q, want collect failed", listed.Error)
	}
}

func TestCollectCrontabsFromPathsIncludesSystemCrontabAndCronDEntries(t *testing.T) {
	root := t.TempDir()
	userDir := filepath.Join(root, "spool")
	cronDDir := filepath.Join(root, "cron.d")
	systemCrontabPath := filepath.Join(root, "crontab")

	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatalf("mkdir user spool: %v", err)
	}
	if err := os.MkdirAll(cronDDir, 0o755); err != nil {
		t.Fatalf("mkdir cron.d: %v", err)
	}

	if err := os.WriteFile(filepath.Join(userDir, "alice"), []byte("*/5 * * * * /usr/local/bin/user-job\n"), 0o600); err != nil {
		t.Fatalf("write user crontab: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cronDDir, "backup"), []byte("0 * * * * root /usr/local/bin/backup\n"), 0o644); err != nil {
		t.Fatalf("write cron.d entry: %v", err)
	}
	if err := os.WriteFile(systemCrontabPath, []byte("@hourly\troot\t/usr/local/bin/hourly\n"), 0o644); err != nil {
		t.Fatalf("write system crontab: %v", err)
	}

	entries, err := backends.CollectCrontabsFromPaths([]string{userDir}, cronDDir, systemCrontabPath)
	if err != nil {
		t.Fatalf("collect crontabs: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d (%+v)", len(entries), entries)
	}

	assertCronEntry := func(schedule, command, user string) {
		t.Helper()
		for _, entry := range entries {
			if entry.Schedule == schedule && entry.Command == command && entry.User == user {
				return
			}
		}
		t.Fatalf("missing cron entry schedule=%q command=%q user=%q in %+v", schedule, command, user, entries)
	}

	assertCronEntry("*/5 * * * *", "/usr/local/bin/user-job", "alice")
	assertCronEntry("0 * * * *", "/usr/local/bin/backup", "root")
	assertCronEntry("@hourly", "/usr/local/bin/hourly", "root")
}

func TestDiskManagerHandleDiskListReturnsMounts(t *testing.T) {
	transport, messages, cleanup := newAgentcoreCapturedTransport(t)
	defer cleanup()

	originalCollectMounts := system.CollectMountsFn
	t.Cleanup(func() { system.CollectMountsFn = originalCollectMounts })

	system.CollectMountsFn = func() ([]agentmgr.MountInfo, error) {
		return []agentmgr.MountInfo{{
			Device:     "/dev/sda1",
			MountPoint: "/",
			FSType:     "ext4",
			Total:      100,
			Used:       40,
			Available:  60,
			UsePct:     40,
		}}, nil
	}

	manager := system.NewDiskManager()
	raw, err := json.Marshal(agentmgr.DiskListData{RequestID: "req-disk"})
	if err != nil {
		t.Fatalf("marshal disk list request: %v", err)
	}
	manager.HandleDiskList(transport, agentmgr.Message{Data: raw})

	msg := waitForCapturedAgentMessage(t, messages, agentmgr.MsgDiskListed, 2*time.Second)
	var listed agentmgr.DiskListedData
	if err := json.Unmarshal(msg.Data, &listed); err != nil {
		t.Fatalf("decode disk listed payload: %v", err)
	}
	if listed.RequestID != "req-disk" {
		t.Fatalf("request_id=%q, want req-disk", listed.RequestID)
	}
	if listed.Error != "" {
		t.Fatalf("unexpected error %q", listed.Error)
	}
	if len(listed.Mounts) != 1 || listed.Mounts[0].MountPoint != "/" {
		t.Fatalf("unexpected mounts %+v", listed.Mounts)
	}
}

func TestDiskManagerHandleDiskListPropagatesCollectionError(t *testing.T) {
	transport, messages, cleanup := newAgentcoreCapturedTransport(t)
	defer cleanup()

	originalCollectMounts := system.CollectMountsFn
	t.Cleanup(func() { system.CollectMountsFn = originalCollectMounts })

	system.CollectMountsFn = func() ([]agentmgr.MountInfo, error) {
		return nil, errors.New("mount collection failed")
	}

	manager := system.NewDiskManager()
	raw, err := json.Marshal(agentmgr.DiskListData{RequestID: "req-disk-error"})
	if err != nil {
		t.Fatalf("marshal disk list request: %v", err)
	}
	manager.HandleDiskList(transport, agentmgr.Message{Data: raw})

	msg := waitForCapturedAgentMessage(t, messages, agentmgr.MsgDiskListed, 2*time.Second)
	var listed agentmgr.DiskListedData
	if err := json.Unmarshal(msg.Data, &listed); err != nil {
		t.Fatalf("decode disk listed payload: %v", err)
	}
	if listed.Error != "mount collection failed" {
		t.Fatalf("error=%q, want mount collection failed", listed.Error)
	}
}

func TestUsersManagerHandleUsersListReturnsSessions(t *testing.T) {
	transport, messages, cleanup := newAgentcoreCapturedTransport(t)
	defer cleanup()

	originalCollectUserSessions := system.CollectUserSessionsFn
	t.Cleanup(func() { system.CollectUserSessionsFn = originalCollectUserSessions })

	system.CollectUserSessionsFn = func() ([]agentmgr.UserSession, error) {
		return []agentmgr.UserSession{{
			Username:   "alice",
			Terminal:   "pts/0",
			RemoteHost: "10.0.0.5",
			LoginTime:  "2026-03-08T14:30:00Z",
		}}, nil
	}

	manager := system.NewUsersManager()
	raw, err := json.Marshal(agentmgr.UsersListData{RequestID: "req-users"})
	if err != nil {
		t.Fatalf("marshal users list request: %v", err)
	}
	manager.HandleUsersList(transport, agentmgr.Message{Data: raw})

	msg := waitForCapturedAgentMessage(t, messages, agentmgr.MsgUsersListed, 2*time.Second)
	var listed agentmgr.UsersListedData
	if err := json.Unmarshal(msg.Data, &listed); err != nil {
		t.Fatalf("decode users listed payload: %v", err)
	}
	if listed.RequestID != "req-users" {
		t.Fatalf("request_id=%q, want req-users", listed.RequestID)
	}
	if listed.Error != "" {
		t.Fatalf("unexpected error %q", listed.Error)
	}
	if len(listed.Sessions) != 1 || listed.Sessions[0].Username != "alice" {
		t.Fatalf("unexpected sessions %+v", listed.Sessions)
	}
}

func TestUsersManagerHandleUsersListPropagatesCollectionError(t *testing.T) {
	transport, messages, cleanup := newAgentcoreCapturedTransport(t)
	defer cleanup()

	originalCollectUserSessions := system.CollectUserSessionsFn
	t.Cleanup(func() { system.CollectUserSessionsFn = originalCollectUserSessions })

	system.CollectUserSessionsFn = func() ([]agentmgr.UserSession, error) {
		return nil, errors.New("who failed")
	}

	manager := system.NewUsersManager()
	raw, err := json.Marshal(agentmgr.UsersListData{RequestID: "req-users-error"})
	if err != nil {
		t.Fatalf("marshal users list request: %v", err)
	}
	manager.HandleUsersList(transport, agentmgr.Message{Data: raw})

	msg := waitForCapturedAgentMessage(t, messages, agentmgr.MsgUsersListed, 2*time.Second)
	var listed agentmgr.UsersListedData
	if err := json.Unmarshal(msg.Data, &listed); err != nil {
		t.Fatalf("decode users listed payload: %v", err)
	}
	if listed.Error != "who failed" {
		t.Fatalf("error=%q, want who failed", listed.Error)
	}
}

func TestParseUserSessionsOutputPreservesNonISOLoginTime(t *testing.T) {
	sessions := system.ParseUserSessionsOutput([]byte("michael console Mar  5 21:14\n"))
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].LoginTime != "Mar 5 21:14" {
		t.Fatalf("login_time=%q, want %q", sessions[0].LoginTime, "Mar 5 21:14")
	}
}

func TestParseUserSessionsOutputParsesISOLoginTimeAndRemoteHost(t *testing.T) {
	sessions := system.ParseUserSessionsOutput([]byte("alice pts/0 2026-03-08 14:30 (10.0.0.5)\n"))
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].LoginTime != "2026-03-08T14:30:00Z" {
		t.Fatalf("login_time=%q, want 2026-03-08T14:30:00Z", sessions[0].LoginTime)
	}
	if sessions[0].RemoteHost != "10.0.0.5" {
		t.Fatalf("remote_host=%q, want 10.0.0.5", sessions[0].RemoteHost)
	}
}

func TestHandleSSHKeyInstallAndRemoveMutatesAuthorizedKeysIdempotently(t *testing.T) {
	transport, messages, cleanup := newAgentcoreCapturedTransport(t)
	defer cleanup()

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	publicKey := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIMockHubKey labtether-hub"
	authorizedKeysPath := filepath.Join(homeDir, ".ssh", "authorized_keys")

	mustHandleSSHKeyMessage := func(msgType string, payload any, wantType string) agentmgr.SSHKeyInstalledData {
		t.Helper()
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal %s payload: %v", msgType, err)
		}
		switch msgType {
		case agentmgr.MsgSSHKeyInstall:
			handleSSHKeyInstall(transport, agentmgr.Message{Type: msgType, Data: raw})
		case agentmgr.MsgSSHKeyRemove:
			handleSSHKeyRemove(transport, agentmgr.Message{Type: msgType, Data: raw})
		default:
			t.Fatalf("unsupported message type %q", msgType)
		}

		msg := waitForCapturedAgentMessage(t, messages, wantType, 2*time.Second)
		var response agentmgr.SSHKeyInstalledData
		if err := json.Unmarshal(msg.Data, &response); err != nil {
			t.Fatalf("decode %s payload: %v", wantType, err)
		}
		return response
	}

	installed := mustHandleSSHKeyMessage(agentmgr.MsgSSHKeyInstall, agentmgr.SSHKeyInstallData{PublicKey: publicKey}, agentmgr.MsgSSHKeyInstalled)
	if installed.HomeDir != homeDir {
		t.Fatalf("home_dir=%q, want %q", installed.HomeDir, homeDir)
	}

	content, err := os.ReadFile(authorizedKeysPath)
	if err != nil {
		t.Fatalf("read authorized_keys after install: %v", err)
	}
	if count := strings.Count(string(content), publicKey); count != 1 {
		t.Fatalf("expected public key once after install, got %d entries in %q", count, string(content))
	}

	_ = mustHandleSSHKeyMessage(agentmgr.MsgSSHKeyInstall, agentmgr.SSHKeyInstallData{PublicKey: publicKey}, agentmgr.MsgSSHKeyInstalled)
	content, err = os.ReadFile(authorizedKeysPath)
	if err != nil {
		t.Fatalf("read authorized_keys after reinstall: %v", err)
	}
	if count := strings.Count(string(content), publicKey); count != 1 {
		t.Fatalf("expected public key once after reinstall, got %d entries in %q", count, string(content))
	}

	_ = mustHandleSSHKeyMessage(agentmgr.MsgSSHKeyRemove, agentmgr.SSHKeyRemoveData{PublicKey: publicKey}, agentmgr.MsgSSHKeyRemoved)
	content, err = os.ReadFile(authorizedKeysPath)
	if err != nil {
		t.Fatalf("read authorized_keys after remove: %v", err)
	}
	if strings.Contains(string(content), publicKey) {
		t.Fatalf("expected public key to be removed, got %q", string(content))
	}

	_ = mustHandleSSHKeyMessage(agentmgr.MsgSSHKeyRemove, agentmgr.SSHKeyRemoveData{PublicKey: publicKey}, agentmgr.MsgSSHKeyRemoved)
	content, err = os.ReadFile(authorizedKeysPath)
	if err != nil {
		t.Fatalf("read authorized_keys after redundant remove: %v", err)
	}
	if strings.Contains(string(content), publicKey) {
		t.Fatalf("expected public key to stay removed, got %q", string(content))
	}
}

func TestHandleWoLSendRejectsInvalidMAC(t *testing.T) {
	transport, messages, cleanup := newAgentcoreCapturedTransport(t)
	defer cleanup()

	raw, err := json.Marshal(agentmgr.WoLSendData{
		RequestID: "req-wol-invalid",
		MAC:       "not-a-mac",
	})
	if err != nil {
		t.Fatalf("marshal wol request: %v", err)
	}
	system.HandleWoLSend(transport, agentmgr.Message{Type: agentmgr.MsgWoLSend, Data: raw})

	msg := waitForCapturedAgentMessage(t, messages, agentmgr.MsgWoLResult, 2*time.Second)
	var result agentmgr.WoLResultData
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		t.Fatalf("decode wol result: %v", err)
	}
	if result.OK {
		t.Fatalf("expected invalid MAC to fail")
	}
	if result.Error == "" {
		t.Fatalf("expected invalid MAC error")
	}
}

func TestHandleWoLSendUsesDefaultBroadcastAndReportsSuccess(t *testing.T) {
	transport, messages, cleanup := newAgentcoreCapturedTransport(t)
	defer cleanup()

	originalWoLSend := system.WoLSendFn
	t.Cleanup(func() { system.WoLSendFn = originalWoLSend })

	var (
		gotMAC       string
		gotBroadcast string
	)
	system.WoLSendFn = func(mac net.HardwareAddr, broadcastAddr string) error {
		gotMAC = mac.String()
		gotBroadcast = broadcastAddr
		return nil
	}

	raw, err := json.Marshal(agentmgr.WoLSendData{
		RequestID: "req-wol-success",
		MAC:       "aa:bb:cc:dd:ee:ff",
	})
	if err != nil {
		t.Fatalf("marshal wol request: %v", err)
	}
	system.HandleWoLSend(transport, agentmgr.Message{Type: agentmgr.MsgWoLSend, Data: raw})

	msg := waitForCapturedAgentMessage(t, messages, agentmgr.MsgWoLResult, 2*time.Second)
	var result agentmgr.WoLResultData
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		t.Fatalf("decode wol result: %v", err)
	}
	if !result.OK {
		t.Fatalf("expected wol success, got %+v", result)
	}
	if gotMAC != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("sent MAC=%q, want aa:bb:cc:dd:ee:ff", gotMAC)
	}
	if gotBroadcast != "255.255.255.255:9" {
		t.Fatalf("broadcast=%q, want 255.255.255.255:9", gotBroadcast)
	}
}
