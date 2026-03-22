package agentcore

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentcore/sysconfig"
	"github.com/labtether/labtether/internal/agentmgr"
)

func TestNetworkManagerHandleNetworkListUsesCollectorAndReportsErrors(t *testing.T) {
	originalCollect := sysconfig.CollectNetworkInterfaces
	t.Cleanup(func() {
		sysconfig.CollectNetworkInterfaces = originalCollect
	})

	t.Run("success", func(t *testing.T) {
		sysconfig.CollectNetworkInterfaces = func() ([]agentmgr.NetInterface, error) {
			return []agentmgr.NetInterface{{
				Name:  "eth0",
				State: "up",
				IPs:   []string{"192.168.1.10/24"},
			}}, nil
		}

		transport, messages, cleanup := newDesktopRuntimeTransport(t)
		defer cleanup()

		manager := &networkManager{Backend: linuxNetworkBackend{}}
		manager.HandleNetworkList(transport, agentmgr.Message{
			Type: agentmgr.MsgNetworkList,
			Data: mustMarshalDesktopRuntime(t, agentmgr.NetworkListData{RequestID: "req-list"}),
		})

		msg := readDesktopRuntimeMessage(t, messages)
		if msg.Type != agentmgr.MsgNetworkListed {
			t.Fatalf("message type=%q, want %q", msg.Type, agentmgr.MsgNetworkListed)
		}
		var listed agentmgr.NetworkListedData
		if err := json.Unmarshal(msg.Data, &listed); err != nil {
			t.Fatalf("decode network listed payload: %v", err)
		}
		if listed.RequestID != "req-list" {
			t.Fatalf("request_id=%q, want req-list", listed.RequestID)
		}
		if len(listed.Interfaces) != 1 || listed.Interfaces[0].Name != "eth0" {
			t.Fatalf("unexpected interfaces %+v", listed.Interfaces)
		}
		if listed.Error != "" {
			t.Fatalf("unexpected error %q", listed.Error)
		}
	})

	t.Run("error", func(t *testing.T) {
		sysconfig.CollectNetworkInterfaces = func() ([]agentmgr.NetInterface, error) {
			return nil, errors.New("enumeration failed")
		}

		transport, messages, cleanup := newDesktopRuntimeTransport(t)
		defer cleanup()

		manager := &networkManager{Backend: linuxNetworkBackend{}}
		manager.HandleNetworkList(transport, agentmgr.Message{
			Type: agentmgr.MsgNetworkList,
			Data: mustMarshalDesktopRuntime(t, agentmgr.NetworkListData{RequestID: "req-error"}),
		})

		msg := readDesktopRuntimeMessage(t, messages)
		var listed agentmgr.NetworkListedData
		if err := json.Unmarshal(msg.Data, &listed); err != nil {
			t.Fatalf("decode network listed payload: %v", err)
		}
		if listed.RequestID != "req-error" {
			t.Fatalf("request_id=%q, want req-error", listed.RequestID)
		}
		if listed.Error != "enumeration failed" {
			t.Fatalf("error=%q, want enumeration failed", listed.Error)
		}
	})
}

func TestApplyActionNetplanSuccessStoresRollbackState(t *testing.T) {
	restoreNetworkActionSeams := stubNetworkActionSeams(t)

	sysconfig.ResolveNetworkMethodFn = func(string) (string, error) { return "netplan", nil }
	sysconfig.BackupNetplanConfigFn = func() (string, error) { return "/tmp/netplan-backup", nil }
	sysconfig.NetworkRunCommandWithTimeout = func(time.Duration, string, ...string) ([]byte, error) {
		return []byte("applied"), nil
	}
	sysconfig.VerifyNetworkConnectivity = func(string) error { return nil }

	nm := &networkManager{Backend: linuxNetworkBackend{}}
	result := nm.ApplyActionLinux(agentmgr.NetworkActionData{
		RequestID:    "req-netplan",
		Action:       "apply",
		Method:       "netplan",
		VerifyTarget: "1.1.1.1",
	})

	restoreNetworkActionSeams()

	if !result.OK {
		t.Fatalf("expected ok result, got %+v", result)
	}
	if result.Output != "applied" {
		t.Fatalf("output=%q, want applied", result.Output)
	}
	if result.RollbackReference != "/tmp/netplan-backup" {
		t.Fatalf("rollback reference=%q, want /tmp/netplan-backup", result.RollbackReference)
	}
	if nm.LastMethod != "netplan" || nm.LastNetplanBackup != "/tmp/netplan-backup" {
		t.Fatalf("unexpected rollback state method=%q backup=%q", nm.LastMethod, nm.LastNetplanBackup)
	}
}

func TestApplyActionNetplanConnectivityFailureTriggersRollback(t *testing.T) {
	restoreNetworkActionSeams := stubNetworkActionSeams(t)

	sysconfig.ResolveNetworkMethodFn = func(string) (string, error) { return "netplan", nil }
	sysconfig.BackupNetplanConfigFn = func() (string, error) { return "/tmp/netplan-backup", nil }

	var restoreRef string
	sysconfig.RestoreNetplanConfigFn = func(ref string) error {
		restoreRef = ref
		return nil
	}
	sysconfig.NetworkRunCommandWithTimeout = func(_ time.Duration, name string, args ...string) ([]byte, error) {
		if name == "netplan" && len(args) == 1 && args[0] == "apply" {
			if restoreRef == "" {
				return []byte("applied"), nil
			}
			return []byte("rollback applied"), nil
		}
		return nil, nil
	}
	sysconfig.VerifyNetworkConnectivity = func(string) error { return errors.New("ping failed") }

	nm := &networkManager{Backend: linuxNetworkBackend{}}
	result := nm.ApplyActionLinux(agentmgr.NetworkActionData{
		RequestID:    "req-netplan-rollback",
		Action:       "apply",
		Method:       "netplan",
		VerifyTarget: "1.1.1.1",
	})

	restoreNetworkActionSeams()

	if result.OK {
		t.Fatalf("expected failed result after rollback, got %+v", result)
	}
	if !result.RollbackAttempted || !result.RollbackSucceeded {
		t.Fatalf("expected successful rollback, got %+v", result)
	}
	if restoreRef != "/tmp/netplan-backup" {
		t.Fatalf("restore ref=%q, want /tmp/netplan-backup", restoreRef)
	}
	if !strings.Contains(result.Error, "rollback applied") {
		t.Fatalf("error=%q, want rollback applied", result.Error)
	}
	if result.RollbackOutput != "rollback applied" {
		t.Fatalf("rollback output=%q, want rollback applied", result.RollbackOutput)
	}
}

func TestApplyActionNMCLIUsesConnectionAndSnapshotsRollback(t *testing.T) {
	restoreNetworkActionSeams := stubNetworkActionSeams(t)

	sysconfig.ResolveNetworkMethodFn = func(string) (string, error) { return "nmcli", nil }
	sysconfig.CollectActiveNMConnectionsFn = func() ([]string, error) { return []string{"Home LAN", "VPN"}, nil }
	sysconfig.NetworkRunCommandWithTimeout = func(_ time.Duration, name string, args ...string) ([]byte, error) {
		if name != "nmcli" {
			t.Fatalf("name=%q, want nmcli", name)
		}
		if got := strings.Join(args, " "); got != "connection up Home LAN" {
			t.Fatalf("args=%q, want connection up Home LAN", got)
		}
		return []byte("connection activated"), nil
	}
	sysconfig.VerifyNetworkConnectivity = func(string) error { return nil }

	nm := &networkManager{Backend: linuxNetworkBackend{}}
	result := nm.ApplyActionLinux(agentmgr.NetworkActionData{
		RequestID:  "req-nmcli",
		Action:     "apply",
		Method:     "nmcli",
		Connection: "Home LAN",
	})

	restoreNetworkActionSeams()

	if !result.OK {
		t.Fatalf("expected ok result, got %+v", result)
	}
	if result.Output != "connection activated" {
		t.Fatalf("output=%q, want connection activated", result.Output)
	}
	if nm.LastMethod != "nmcli" {
		t.Fatalf("last method=%q, want nmcli", nm.LastMethod)
	}
	if got := strings.Join(nm.LastNMConnections, ","); got != "Home LAN,VPN" {
		t.Fatalf("last nmcli connections=%q, want Home LAN,VPN", got)
	}
}

func TestRollbackActionUsesSnapshotsAndReportsMissingState(t *testing.T) {
	restoreNetworkActionSeams := stubNetworkActionSeams(t)
	defer restoreNetworkActionSeams()

	t.Run("missing snapshot", func(t *testing.T) {
		nm := &networkManager{Backend: linuxNetworkBackend{}}
		result := nm.RollbackActionLinux(agentmgr.NetworkActionData{
			RequestID: "req-missing",
			Action:    "rollback",
			Method:    "auto",
		})
		if result.Error != "no rollback snapshot is available yet" {
			t.Fatalf("error=%q, want no rollback snapshot is available yet", result.Error)
		}
	})

	t.Run("netplan auto", func(t *testing.T) {
		restoreRef := ""
		sysconfig.RestoreNetplanConfigFn = func(ref string) error {
			restoreRef = ref
			return nil
		}
		sysconfig.NetworkRunCommandWithTimeout = func(_ time.Duration, name string, args ...string) ([]byte, error) {
			return []byte("restored"), nil
		}

		nm := &networkManager{
			Backend:           linuxNetworkBackend{},
			LastMethod:        "netplan",
			LastNetplanBackup: "/tmp/netplan-backup",
		}
		result := nm.RollbackActionLinux(agentmgr.NetworkActionData{
			RequestID: "req-rollback-netplan",
			Action:    "rollback",
			Method:    "auto",
		})
		if !result.OK || !result.RollbackSucceeded {
			t.Fatalf("expected successful rollback, got %+v", result)
		}
		if restoreRef != "/tmp/netplan-backup" {
			t.Fatalf("restore ref=%q, want /tmp/netplan-backup", restoreRef)
		}
		if result.RollbackReference != "/tmp/netplan-backup" {
			t.Fatalf("rollback reference=%q, want /tmp/netplan-backup", result.RollbackReference)
		}
	})

	t.Run("nmcli explicit", func(t *testing.T) {
		sysconfig.ActivateNMConnectionsFn = func(connections []string) (string, error) {
			if got := strings.Join(connections, ","); got != "Home LAN,VPN" {
				t.Fatalf("connections=%q, want Home LAN,VPN", got)
			}
			return "reconnected", nil
		}

		nm := &networkManager{
			Backend:           linuxNetworkBackend{},
			LastMethod:        "nmcli",
			LastNMConnections: []string{"Home LAN", "VPN"},
		}
		result := nm.RollbackActionLinux(agentmgr.NetworkActionData{
			RequestID: "req-rollback-nmcli",
			Action:    "rollback",
			Method:    "nmcli",
		})
		if !result.OK || !result.RollbackSucceeded {
			t.Fatalf("expected successful rollback, got %+v", result)
		}
		if result.RollbackOutput != "reconnected" {
			t.Fatalf("rollback output=%q, want reconnected", result.RollbackOutput)
		}
	})
}

func TestResolveNetworkMethodAndVerifyConnectivity(t *testing.T) {
	originalHasCommand := sysconfig.NetworkHasCommand
	originalRunCommand := sysconfig.NetworkRunCommandWithTimeout
	t.Cleanup(func() {
		sysconfig.NetworkHasCommand = originalHasCommand
		sysconfig.NetworkRunCommandWithTimeout = originalRunCommand
	})

	t.Run("resolve auto preference", func(t *testing.T) {
		sysconfig.NetworkHasCommand = func(name string) bool {
			switch name {
			case "netplan", "nmcli":
				return true
			default:
				return false
			}
		}
		method, err := sysconfig.ResolveNetworkMethod("auto")
		if err != nil {
			t.Fatalf("resolve method: %v", err)
		}
		if method != "netplan" {
			t.Fatalf("method=%q, want netplan", method)
		}
	})

	t.Run("resolve missing explicit tool", func(t *testing.T) {
		sysconfig.NetworkHasCommand = func(name string) bool { return false }
		_, err := sysconfig.ResolveNetworkMethod("nmcli")
		if err == nil || !strings.Contains(err.Error(), "nmcli is not installed") {
			t.Fatalf("expected missing nmcli error, got %v", err)
		}
	})

	t.Run("verify skips ping when unavailable", func(t *testing.T) {
		sysconfig.NetworkHasCommand = func(name string) bool { return name != "ping" }
		sysconfig.NetworkRunCommandWithTimeout = func(_ time.Duration, name string, args ...string) ([]byte, error) {
			if name != "ip" {
				t.Fatalf("name=%q, want ip", name)
			}
			return []byte("default via 10.0.0.1 dev eth0"), nil
		}
		if err := sysconfig.VerifyConnectivity(""); err != nil {
			t.Fatalf("verify connectivity: %v", err)
		}
	})

	t.Run("verify reports missing default route", func(t *testing.T) {
		sysconfig.NetworkHasCommand = func(string) bool { return true }
		sysconfig.NetworkRunCommandWithTimeout = func(_ time.Duration, name string, args ...string) ([]byte, error) {
			return []byte(""), nil
		}
		err := sysconfig.VerifyConnectivity("")
		if err == nil || !strings.Contains(err.Error(), "no default route detected after apply") {
			t.Fatalf("expected missing default route error, got %v", err)
		}
	})
}

func stubNetworkActionSeams(t *testing.T) func() {
	t.Helper()

	originalResolve := sysconfig.ResolveNetworkMethodFn
	originalBackup := sysconfig.BackupNetplanConfigFn
	originalRestore := sysconfig.RestoreNetplanConfigFn
	originalVerify := sysconfig.VerifyNetworkConnectivity
	originalCollectActive := sysconfig.CollectActiveNMConnectionsFn
	originalActivate := sysconfig.ActivateNMConnectionsFn
	originalRun := sysconfig.NetworkRunCommandWithTimeout

	return func() {
		sysconfig.ResolveNetworkMethodFn = originalResolve
		sysconfig.BackupNetplanConfigFn = originalBackup
		sysconfig.RestoreNetplanConfigFn = originalRestore
		sysconfig.VerifyNetworkConnectivity = originalVerify
		sysconfig.CollectActiveNMConnectionsFn = originalCollectActive
		sysconfig.ActivateNMConnectionsFn = originalActivate
		sysconfig.NetworkRunCommandWithTimeout = originalRun
	}
}
