package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentcore/backends"
	"github.com/labtether/labtether/internal/agentcore/system"
	"github.com/labtether/labtether/internal/agentmgr"
)

type stubPackageBackend struct {
	listPackages []agentmgr.PackageInfo
	listErr      error
	actionResult backends.PackageActionResult
	actionErr    error
	actionCalls  []stubBackendsPackageActionCall
}

type stubBackendsPackageActionCall struct {
	action   string
	packages []string
}

func (s *stubPackageBackend) ListPackages() ([]agentmgr.PackageInfo, error) {
	return s.listPackages, s.listErr
}

func (s *stubPackageBackend) PerformAction(action string, packages []string) (backends.PackageActionResult, error) {
	s.actionCalls = append(s.actionCalls, stubBackendsPackageActionCall{
		action:   action,
		packages: append([]string(nil), packages...),
	})
	return s.actionResult, s.actionErr
}

type stubServiceBackend struct {
	listServices []agentmgr.ServiceInfo
	listErr      error
	actionOutput string
	actionErr    error
	actionCalls  []stubServiceActionCall
}

type stubServiceActionCall struct {
	action  string
	service string
}

func (s *stubServiceBackend) ListServices() ([]agentmgr.ServiceInfo, error) {
	return s.listServices, s.listErr
}

func (s *stubServiceBackend) PerformAction(action, service string) (string, error) {
	s.actionCalls = append(s.actionCalls, stubServiceActionCall{
		action:  action,
		service: service,
	})
	return s.actionOutput, s.actionErr
}

type stubLogBackend struct {
	entries []agentmgr.LogStreamData
	err     error
	reqs    []agentmgr.JournalQueryData
}

func (s *stubLogBackend) QueryEntries(req agentmgr.JournalQueryData) ([]agentmgr.LogStreamData, error) {
	s.reqs = append(s.reqs, req)
	return s.entries, s.err
}

func (s *stubLogBackend) StreamEntries(_ context.Context, _ func(agentmgr.LogStreamData)) error {
	return nil
}

func TestProcessManagerHandleProcessListSortsLimitsAndReportsErrors(t *testing.T) {
	originalCollectProcesses := system.CollectProcessesFn
	t.Cleanup(func() {
		system.CollectProcessesFn = originalCollectProcesses
	})

	t.Run("success", func(t *testing.T) {
		system.CollectProcessesFn = func() ([]agentmgr.ProcessInfo, error) {
			return []agentmgr.ProcessInfo{
				{PID: 101, Name: "alpha", CPUPct: 10, MemPct: 25},
				{PID: 102, Name: "beta", CPUPct: 40, MemPct: 5},
				{PID: 103, Name: "gamma", CPUPct: 1, MemPct: 90},
			}, nil
		}

		transport, messages, cleanup := newDesktopRuntimeTransport(t)
		defer cleanup()

		manager := system.NewProcessManager()
		manager.HandleProcessList(transport, agentmgr.Message{
			Type: agentmgr.MsgProcessList,
			Data: mustMarshalDesktopRuntime(t, agentmgr.ProcessListData{
				RequestID: "req-process-list",
				SortBy:    "memory",
				Limit:     2,
			}),
		})

		msg := readDesktopRuntimeMessage(t, messages)
		if msg.Type != agentmgr.MsgProcessListed {
			t.Fatalf("message type=%q, want %q", msg.Type, agentmgr.MsgProcessListed)
		}

		var listed agentmgr.ProcessListedData
		if err := json.Unmarshal(msg.Data, &listed); err != nil {
			t.Fatalf("decode process listed payload: %v", err)
		}
		if listed.RequestID != "req-process-list" {
			t.Fatalf("request_id=%q, want req-process-list", listed.RequestID)
		}
		if listed.Error != "" {
			t.Fatalf("unexpected error %q", listed.Error)
		}
		if got, want := len(listed.Processes), 2; got != want {
			t.Fatalf("len(processes)=%d, want %d", got, want)
		}
		if listed.Processes[0].PID != 103 || listed.Processes[1].PID != 101 {
			t.Fatalf("unexpected process order %+v", listed.Processes)
		}
	})

	t.Run("error", func(t *testing.T) {
		system.CollectProcessesFn = func() ([]agentmgr.ProcessInfo, error) {
			return nil, errors.New("ps failed")
		}

		transport, messages, cleanup := newDesktopRuntimeTransport(t)
		defer cleanup()

		manager := system.NewProcessManager()
		manager.HandleProcessList(transport, agentmgr.Message{
			Type: agentmgr.MsgProcessList,
			Data: mustMarshalDesktopRuntime(t, agentmgr.ProcessListData{RequestID: "req-process-error"}),
		})

		msg := readDesktopRuntimeMessage(t, messages)
		var listed agentmgr.ProcessListedData
		if err := json.Unmarshal(msg.Data, &listed); err != nil {
			t.Fatalf("decode process listed payload: %v", err)
		}
		if listed.RequestID != "req-process-error" {
			t.Fatalf("request_id=%q, want req-process-error", listed.RequestID)
		}
		if listed.Error != "ps failed" {
			t.Fatalf("error=%q, want ps failed", listed.Error)
		}
	})
}

func TestProcessManagerHandleProcessKillGuardsAndReportsResults(t *testing.T) {
	originalKillProcess := system.KillProcessFn
	t.Cleanup(func() {
		system.KillProcessFn = originalKillProcess
	})

	t.Run("rejects pid one", func(t *testing.T) {
		killCalled := false
		system.KillProcessFn = func(int, string) error {
			killCalled = true
			return nil
		}

		transport, messages, cleanup := newDesktopRuntimeTransport(t)
		defer cleanup()

		manager := system.NewProcessManager()
		manager.HandleProcessKill(transport, agentmgr.Message{
			Type: agentmgr.MsgProcessKill,
			ID:   "req-process-kill-guard",
			Data: mustMarshalDesktopRuntime(t, agentmgr.ProcessKillData{PID: 1}),
		})

		msg := readDesktopRuntimeMessage(t, messages)
		if msg.Type != agentmgr.MsgProcessKillResult {
			t.Fatalf("message type=%q, want %q", msg.Type, agentmgr.MsgProcessKillResult)
		}
		if msg.ID != "req-process-kill-guard" {
			t.Fatalf("message id=%q, want req-process-kill-guard", msg.ID)
		}

		var result agentmgr.ProcessKillResultData
		if err := json.Unmarshal(msg.Data, &result); err != nil {
			t.Fatalf("decode process kill result: %v", err)
		}
		if result.Error != "refusing to signal PID <= 1" {
			t.Fatalf("error=%q, want refusing to signal PID <= 1", result.Error)
		}
		if killCalled {
			t.Fatal("expected killProcess not to be called")
		}
	})

	t.Run("success", func(t *testing.T) {
		calledPID := 0
		calledSignal := ""
		system.KillProcessFn = func(pid int, signal string) error {
			calledPID = pid
			calledSignal = signal
			return nil
		}

		transport, messages, cleanup := newDesktopRuntimeTransport(t)
		defer cleanup()

		manager := system.NewProcessManager()
		manager.HandleProcessKill(transport, agentmgr.Message{
			Type: agentmgr.MsgProcessKill,
			ID:   "req-process-kill-success",
			Data: mustMarshalDesktopRuntime(t, agentmgr.ProcessKillData{PID: 42, Signal: "SIGKILL"}),
		})

		msg := readDesktopRuntimeMessage(t, messages)
		if msg.ID != "req-process-kill-success" {
			t.Fatalf("message id=%q, want req-process-kill-success", msg.ID)
		}
		var result agentmgr.ProcessKillResultData
		if err := json.Unmarshal(msg.Data, &result); err != nil {
			t.Fatalf("decode process kill result: %v", err)
		}
		if !result.Success || result.PID != 42 {
			t.Fatalf("unexpected kill result %+v", result)
		}
		if calledPID != 42 || calledSignal != "SIGKILL" {
			t.Fatalf("kill called with pid=%d signal=%q", calledPID, calledSignal)
		}
	})

	t.Run("error", func(t *testing.T) {
		system.KillProcessFn = func(int, string) error {
			return errors.New("permission denied")
		}

		transport, messages, cleanup := newDesktopRuntimeTransport(t)
		defer cleanup()

		manager := system.NewProcessManager()
		manager.HandleProcessKill(transport, agentmgr.Message{
			Type: agentmgr.MsgProcessKill,
			ID:   "req-process-kill-error",
			Data: mustMarshalDesktopRuntime(t, agentmgr.ProcessKillData{PID: 77, Signal: "SIGTERM"}),
		})

		msg := readDesktopRuntimeMessage(t, messages)
		if msg.ID != "req-process-kill-error" {
			t.Fatalf("message id=%q, want req-process-kill-error", msg.ID)
		}
		var result agentmgr.ProcessKillResultData
		if err := json.Unmarshal(msg.Data, &result); err != nil {
			t.Fatalf("decode process kill result: %v", err)
		}
		if result.Error != "permission denied" {
			t.Fatalf("error=%q, want permission denied", result.Error)
		}
	})
}

func TestPackageManagerHandlePackageListAndAction(t *testing.T) {
	t.Run("list success and error", func(t *testing.T) {
		transport, messages, cleanup := newDesktopRuntimeTransport(t)
		defer cleanup()

		successBackend := &stubPackageBackend{
			listPackages: []agentmgr.PackageInfo{{Name: "jq", Version: "1.7", Status: "installed"}},
		}
		manager := &backends.PackageManager{Backend: successBackend}
		manager.HandlePackageList(transport, agentmgr.Message{
			Type: agentmgr.MsgPackageList,
			Data: mustMarshalDesktopRuntime(t, agentmgr.PackageListData{RequestID: "req-package-list"}),
		})

		msg := readDesktopRuntimeMessage(t, messages)
		if msg.Type != agentmgr.MsgPackageListed {
			t.Fatalf("message type=%q, want %q", msg.Type, agentmgr.MsgPackageListed)
		}
		var listed agentmgr.PackageListedData
		if err := json.Unmarshal(msg.Data, &listed); err != nil {
			t.Fatalf("decode package listed payload: %v", err)
		}
		if listed.RequestID != "req-package-list" || len(listed.Packages) != 1 || listed.Packages[0].Name != "jq" {
			t.Fatalf("unexpected package list %+v", listed)
		}

		errorBackend := &stubPackageBackend{listErr: errors.New("rpm failed")}
		manager = &backends.PackageManager{Backend: errorBackend}
		manager.HandlePackageList(transport, agentmgr.Message{
			Type: agentmgr.MsgPackageList,
			Data: mustMarshalDesktopRuntime(t, agentmgr.PackageListData{RequestID: "req-package-error"}),
		})

		msg = readDesktopRuntimeMessage(t, messages)
		if err := json.Unmarshal(msg.Data, &listed); err != nil {
			t.Fatalf("decode package listed payload: %v", err)
		}
		if listed.Error != "rpm failed" {
			t.Fatalf("error=%q, want rpm failed", listed.Error)
		}
	})

	t.Run("action normalizes packages and reports result", func(t *testing.T) {
		transport, messages, cleanup := newDesktopRuntimeTransport(t)
		defer cleanup()

		backend := &stubPackageBackend{
			actionResult: backends.PackageActionResult{
				Output:         "installed",
				RebootRequired: true,
			},
		}
		manager := &backends.PackageManager{Backend: backend}
		manager.HandlePackageAction(transport, agentmgr.Message{
			Type: agentmgr.MsgPackageAction,
			Data: mustMarshalDesktopRuntime(t, agentmgr.PackageActionData{
				RequestID: "req-package-action",
				Action:    "install",
				Packages:  []string{" jq ", "jq", "", "curl"},
			}),
		})

		msg := readDesktopRuntimeMessage(t, messages)
		if msg.Type != agentmgr.MsgPackageResult {
			t.Fatalf("message type=%q, want %q", msg.Type, agentmgr.MsgPackageResult)
		}
		var result agentmgr.PackageResultData
		if err := json.Unmarshal(msg.Data, &result); err != nil {
			t.Fatalf("decode package result payload: %v", err)
		}
		if !result.OK || result.Output != "installed" || !result.RebootRequired {
			t.Fatalf("unexpected package result %+v", result)
		}
		if got, want := backend.actionCalls, []stubBackendsPackageActionCall{{
			action:   "install",
			packages: []string{"jq", "curl"},
		}}; !reflect.DeepEqual(got, want) {
			t.Fatalf("backend calls=%#v, want %#v", got, want)
		}
	})

	t.Run("invalid action", func(t *testing.T) {
		transport, messages, cleanup := newDesktopRuntimeTransport(t)
		defer cleanup()

		backend := &stubPackageBackend{}
		manager := &backends.PackageManager{Backend: backend}
		manager.HandlePackageAction(transport, agentmgr.Message{
			Type: agentmgr.MsgPackageAction,
			Data: mustMarshalDesktopRuntime(t, agentmgr.PackageActionData{
				RequestID: "req-package-invalid",
				Action:    "repair",
			}),
		})

		msg := readDesktopRuntimeMessage(t, messages)
		var result agentmgr.PackageResultData
		if err := json.Unmarshal(msg.Data, &result); err != nil {
			t.Fatalf("decode package result payload: %v", err)
		}
		if result.Error != "invalid package action" {
			t.Fatalf("error=%q, want invalid package action", result.Error)
		}
		if len(backend.actionCalls) != 0 {
			t.Fatalf("expected no backend calls, got %#v", backend.actionCalls)
		}
	})
}

func TestDetectLinuxPackageManagerPrefersSupportedOrder(t *testing.T) {
	originalLookPath := backends.LinuxPackageLookPath
	t.Cleanup(func() {
		backends.LinuxPackageLookPath = originalLookPath
	})

	backends.LinuxPackageLookPath = func(name string) (string, error) {
		switch name {
		case "apt-get", "dnf":
			return "/usr/bin/" + name, nil
		default:
			return "", exec.ErrNotFound
		}
	}

	manager, err := backends.DetectLinuxPackageManager()
	if err != nil {
		t.Fatalf("detectLinuxPackageManager returned error: %v", err)
	}
	if manager != "apt-get" {
		t.Fatalf("manager=%q, want apt-get", manager)
	}
}

func TestLinuxPackageBackendListPackagesSelectsAvailableInventorySource(t *testing.T) {
	originalLookPath := backends.LinuxPackageLookPath
	originalDpkgLister := backends.LinuxPackageDpkgLister
	originalRPMLister := backends.LinuxPackageRPMLister
	t.Cleanup(func() {
		backends.LinuxPackageLookPath = originalLookPath
		backends.LinuxPackageDpkgLister = originalDpkgLister
		backends.LinuxPackageRPMLister = originalRPMLister
	})

	t.Run("prefers dpkg", func(t *testing.T) {
		backends.LinuxPackageLookPath = func(name string) (string, error) {
			switch name {
			case "dpkg-query", "rpm":
				return "/usr/bin/" + name, nil
			default:
				return "", exec.ErrNotFound
			}
		}
		backends.LinuxPackageDpkgLister = func() ([]agentmgr.PackageInfo, error) {
			return []agentmgr.PackageInfo{{Name: "jq"}}, nil
		}
		backends.LinuxPackageRPMLister = func() ([]agentmgr.PackageInfo, error) {
			t.Fatal("expected rpm lister not to be called")
			return nil, nil
		}

		packages, err := backends.LinuxPackageBackend{}.ListPackages()
		if err != nil {
			t.Fatalf("ListPackages returned error: %v", err)
		}
		if len(packages) != 1 || packages[0].Name != "jq" {
			t.Fatalf("unexpected packages %+v", packages)
		}
	})

	t.Run("falls back to rpm", func(t *testing.T) {
		backends.LinuxPackageLookPath = func(name string) (string, error) {
			switch name {
			case "rpm":
				return "/usr/bin/rpm", nil
			default:
				return "", exec.ErrNotFound
			}
		}
		backends.LinuxPackageDpkgLister = func() ([]agentmgr.PackageInfo, error) {
			t.Fatal("expected dpkg lister not to be called")
			return nil, nil
		}
		backends.LinuxPackageRPMLister = func() ([]agentmgr.PackageInfo, error) {
			return []agentmgr.PackageInfo{{Name: "podman"}}, nil
		}

		packages, err := backends.LinuxPackageBackend{}.ListPackages()
		if err != nil {
			t.Fatalf("ListPackages returned error: %v", err)
		}
		if len(packages) != 1 || packages[0].Name != "podman" {
			t.Fatalf("unexpected packages %+v", packages)
		}
	})

	t.Run("returns unsupported when no manager exists", func(t *testing.T) {
		backends.LinuxPackageLookPath = func(string) (string, error) {
			return "", exec.ErrNotFound
		}

		_, err := backends.LinuxPackageBackend{}.ListPackages()
		if !errors.Is(err, backends.ErrNoLinuxPackageManager) {
			t.Fatalf("error=%v, want backends.ErrNoLinuxPackageManager", err)
		}
	})
}

func TestLinuxPackageBackendPerformActionRunsCommandsAndAggregatesOutput(t *testing.T) {
	originalDetectManager := backends.DetectLinuxPackageManagerFn
	originalBuildCommands := backends.BuildLinuxPackageActionCommandsFn
	originalRunCommand := backends.RunLinuxPackageCommand
	originalRebootRequired := backends.DetectLinuxRebootRequiredFn
	t.Cleanup(func() {
		backends.DetectLinuxPackageManagerFn = originalDetectManager
		backends.BuildLinuxPackageActionCommandsFn = originalBuildCommands
		backends.RunLinuxPackageCommand = originalRunCommand
		backends.DetectLinuxRebootRequiredFn = originalRebootRequired
	})

	backends.DetectLinuxPackageManagerFn = func() (string, error) { return "apt-get", nil }
	backends.BuildLinuxPackageActionCommandsFn = func(manager, action string, packages []string) ([]backends.PackageActionCommand, error) {
		if manager != "apt-get" || action != "install" || !reflect.DeepEqual(packages, []string{"jq"}) {
			t.Fatalf("unexpected command build inputs manager=%q action=%q packages=%v", manager, action, packages)
		}
		return []backends.PackageActionCommand{
			{Name: "apt-get", Args: []string{"update"}},
			{Name: "apt-get", Args: []string{"-y", "install", "jq"}},
		}, nil
	}

	var calls []string
	backends.RunLinuxPackageCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		if len(args) > 0 && args[0] == "update" {
			return []byte("metadata refreshed"), nil
		}
		return []byte("package installed"), nil
	}
	backends.DetectLinuxRebootRequiredFn = func() bool { return true }

	result, err := backends.LinuxPackageBackend{}.PerformAction("install", []string{"jq"})
	if err != nil {
		t.Fatalf("PerformAction returned error: %v", err)
	}
	if got, want := calls, []string{"apt-get update", "apt-get -y install jq"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("calls=%v, want %v", got, want)
	}
	if result.Output != "metadata refreshed\npackage installed" {
		t.Fatalf("output=%q, want metadata refreshed\\npackage installed", result.Output)
	}
	if !result.RebootRequired {
		t.Fatal("expected reboot_required=true")
	}
}

func TestLinuxPackageBackendPerformActionReturnsTimeoutWithPartialOutput(t *testing.T) {
	originalDetectManager := backends.DetectLinuxPackageManagerFn
	originalBuildCommands := backends.BuildLinuxPackageActionCommandsFn
	originalRunCommand := backends.RunLinuxPackageCommand
	originalRebootRequired := backends.DetectLinuxRebootRequiredFn
	t.Cleanup(func() {
		backends.DetectLinuxPackageManagerFn = originalDetectManager
		backends.BuildLinuxPackageActionCommandsFn = originalBuildCommands
		backends.RunLinuxPackageCommand = originalRunCommand
		backends.DetectLinuxRebootRequiredFn = originalRebootRequired
	})

	backends.DetectLinuxPackageManagerFn = func() (string, error) { return "apt-get", nil }
	backends.BuildLinuxPackageActionCommandsFn = func(string, string, []string) ([]backends.PackageActionCommand, error) {
		return []backends.PackageActionCommand{{Name: "apt-get", Args: []string{"upgrade"}}}, nil
	}
	backends.RunLinuxPackageCommand = func(context.Context, string, ...string) ([]byte, error) {
		return []byte("slow output"), context.DeadlineExceeded
	}
	backends.DetectLinuxRebootRequiredFn = func() bool { return false }

	result, err := backends.LinuxPackageBackend{}.PerformAction("upgrade", nil)
	if err == nil || err.Error() != "package action timed out" {
		t.Fatalf("error=%v, want package action timed out", err)
	}
	if result.Output != "slow output" {
		t.Fatalf("output=%q, want slow output", result.Output)
	}
}

func TestServiceManagerHandleServiceListAndAction(t *testing.T) {
	t.Run("list success and error", func(t *testing.T) {
		transport, messages, cleanup := newDesktopRuntimeTransport(t)
		defer cleanup()

		successBackend := &stubServiceBackend{
			listServices: []agentmgr.ServiceInfo{{
				Name:        "sshd",
				Description: "OpenSSH Daemon",
				ActiveState: "active",
				SubState:    "running",
				Enabled:     "enabled",
				LoadState:   "loaded",
			}},
		}
		manager := &backends.ServiceManager{Backend: successBackend}
		manager.HandleServiceList(transport, agentmgr.Message{
			Type: agentmgr.MsgServiceList,
			Data: mustMarshalDesktopRuntime(t, agentmgr.ServiceListData{RequestID: "req-service-list"}),
		})

		msg := readDesktopRuntimeMessage(t, messages)
		if msg.Type != agentmgr.MsgServiceListed {
			t.Fatalf("message type=%q, want %q", msg.Type, agentmgr.MsgServiceListed)
		}
		var listed agentmgr.ServiceListedData
		if err := json.Unmarshal(msg.Data, &listed); err != nil {
			t.Fatalf("decode service listed payload: %v", err)
		}
		if listed.RequestID != "req-service-list" || len(listed.Services) != 1 || listed.Services[0].Name != "sshd" {
			t.Fatalf("unexpected service list %+v", listed)
		}

		errorBackend := &stubServiceBackend{listErr: errors.New("systemctl failed")}
		manager = &backends.ServiceManager{Backend: errorBackend}
		manager.HandleServiceList(transport, agentmgr.Message{
			Type: agentmgr.MsgServiceList,
			Data: mustMarshalDesktopRuntime(t, agentmgr.ServiceListData{RequestID: "req-service-error"}),
		})

		msg = readDesktopRuntimeMessage(t, messages)
		if err := json.Unmarshal(msg.Data, &listed); err != nil {
			t.Fatalf("decode service listed payload: %v", err)
		}
		if listed.Error != "systemctl failed" {
			t.Fatalf("error=%q, want systemctl failed", listed.Error)
		}
	})

	t.Run("action success and validation", func(t *testing.T) {
		transport, messages, cleanup := newDesktopRuntimeTransport(t)
		defer cleanup()

		backend := &stubServiceBackend{actionOutput: "restarted"}
		manager := &backends.ServiceManager{Backend: backend}
		manager.HandleServiceAction(transport, agentmgr.Message{
			Type: agentmgr.MsgServiceAction,
			Data: mustMarshalDesktopRuntime(t, agentmgr.ServiceActionData{
				RequestID: "req-service-action",
				Service:   "sshd",
				Action:    "restart",
			}),
		})

		msg := readDesktopRuntimeMessage(t, messages)
		if msg.Type != agentmgr.MsgServiceResult {
			t.Fatalf("message type=%q, want %q", msg.Type, agentmgr.MsgServiceResult)
		}
		var result agentmgr.ServiceResultData
		if err := json.Unmarshal(msg.Data, &result); err != nil {
			t.Fatalf("decode service result payload: %v", err)
		}
		if !result.OK || result.Output != "restarted" {
			t.Fatalf("unexpected service result %+v", result)
		}
		if got, want := backend.actionCalls, []stubServiceActionCall{{action: "restart", service: "sshd"}}; !reflect.DeepEqual(got, want) {
			t.Fatalf("backend calls=%#v, want %#v", got, want)
		}

		manager.HandleServiceAction(transport, agentmgr.Message{
			Type: agentmgr.MsgServiceAction,
			Data: mustMarshalDesktopRuntime(t, agentmgr.ServiceActionData{
				RequestID: "req-service-invalid",
				Service:   "sshd",
				Action:    "reload",
			}),
		})
		msg = readDesktopRuntimeMessage(t, messages)
		if err := json.Unmarshal(msg.Data, &result); err != nil {
			t.Fatalf("decode service result payload: %v", err)
		}
		if !strings.Contains(result.Error, "invalid action") {
			t.Fatalf("error=%q, want invalid action", result.Error)
		}
	})
}

func TestLinuxServiceBackendListServicesParsesSystemctlOutput(t *testing.T) {
	originalRunCommand := backends.RunLinuxServiceCommand
	t.Cleanup(func() {
		backends.RunLinuxServiceCommand = originalRunCommand
	})

	backends.RunLinuxServiceCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name != "systemctl" {
			t.Fatalf("command name=%q, want systemctl", name)
		}
		switch args[0] {
		case "list-unit-files":
			return []byte("sshd.service enabled\ncron.service disabled\n"), nil
		case "list-units":
			return []byte(strings.Join([]string{
				"sshd.service loaded active running OpenSSH Daemon",
				"cron.service loaded inactive dead Cron Daemon",
			}, "\n")), nil
		default:
			t.Fatalf("unexpected systemctl args=%v", args)
			return nil, nil
		}
	}

	services, err := backends.LinuxServiceBackend{}.ListServices()
	if err != nil {
		t.Fatalf("ListServices returned error: %v", err)
	}
	if got, want := len(services), 2; got != want {
		t.Fatalf("len(services)=%d, want %d", got, want)
	}
	if services[0].Name != "sshd" || services[0].Enabled != "enabled" || services[0].Description != "OpenSSH Daemon" {
		t.Fatalf("unexpected first service %+v", services[0])
	}
	if services[1].Name != "cron" || services[1].Enabled != "disabled" || services[1].ActiveState != "inactive" {
		t.Fatalf("unexpected second service %+v", services[1])
	}
}

func TestLinuxServiceBackendListServicesReportsNonSystemdFailure(t *testing.T) {
	originalRunCommand := backends.RunLinuxServiceCommand
	t.Cleanup(func() {
		backends.RunLinuxServiceCommand = originalRunCommand
	})

	backends.RunLinuxServiceCommand = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch args[0] {
		case "list-unit-files":
			return nil, errors.New("System has not been booted with systemd as init system")
		case "list-units":
			return nil, errors.New("System has not been booted with systemd as init system")
		default:
			t.Fatalf("unexpected systemctl args=%v", args)
			return nil, nil
		}
	}

	_, err := backends.LinuxServiceBackend{}.ListServices()
	if err == nil || !strings.Contains(err.Error(), "systemctl list-units") {
		t.Fatalf("error=%v, want systemctl list-units failure", err)
	}
}

func TestLinuxServiceBackendPerformActionReturnsTimeoutWithOutput(t *testing.T) {
	originalRunCommand := backends.RunLinuxServiceCommand
	t.Cleanup(func() {
		backends.RunLinuxServiceCommand = originalRunCommand
	})

	backends.RunLinuxServiceCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name != "systemctl" || len(args) != 2 || args[0] != "restart" || args[1] != "sshd" {
			t.Fatalf("unexpected systemctl invocation %q %v", name, args)
		}
		return []byte("stopping sshd"), context.DeadlineExceeded
	}

	output, err := backends.LinuxServiceBackend{}.PerformAction("restart", "sshd")
	if err == nil || err.Error() != "systemctl timed out" {
		t.Fatalf("error=%v, want systemctl timed out", err)
	}
	if output != "stopping sshd" {
		t.Fatalf("output=%q, want stopping sshd", output)
	}
}

func TestJournalManagerHandleJournalQueryReturnsEntriesAndErrors(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		transport, messages, cleanup := newDesktopRuntimeTransport(t)
		defer cleanup()

		backend := &stubLogBackend{
			entries: []agentmgr.LogStreamData{{
				Timestamp: "2026-03-08T12:00:00Z",
				Level:     "error",
				Message:   "denied",
				Source:    "sshd",
			}},
		}
		manager := &backends.JournalManager{Backend: backend}
		manager.HandleJournalQuery(transport, agentmgr.Message{
			Type: agentmgr.MsgJournalQuery,
			Data: mustMarshalDesktopRuntime(t, agentmgr.JournalQueryData{
				RequestID: "req-journal",
				Unit:      "sshd.service",
				Limit:     50,
			}),
		})

		msg := readDesktopRuntimeMessage(t, messages)
		if msg.Type != agentmgr.MsgJournalEntries {
			t.Fatalf("message type=%q, want %q", msg.Type, agentmgr.MsgJournalEntries)
		}
		var listed agentmgr.JournalEntriesData
		if err := json.Unmarshal(msg.Data, &listed); err != nil {
			t.Fatalf("decode journal entries payload: %v", err)
		}
		if listed.RequestID != "req-journal" || len(listed.Entries) != 1 || listed.Entries[0].Source != "sshd" {
			t.Fatalf("unexpected journal response %+v", listed)
		}
		if got := backend.reqs; len(got) != 1 || got[0].Unit != "sshd.service" || got[0].Limit != 50 {
			t.Fatalf("backend requests=%+v", got)
		}
	})

	t.Run("error", func(t *testing.T) {
		transport, messages, cleanup := newDesktopRuntimeTransport(t)
		defer cleanup()

		backend := &stubLogBackend{err: errors.New("journalctl unavailable")}
		manager := &backends.JournalManager{Backend: backend}
		manager.HandleJournalQuery(transport, agentmgr.Message{
			Type: agentmgr.MsgJournalQuery,
			Data: mustMarshalDesktopRuntime(t, agentmgr.JournalQueryData{RequestID: "req-journal-error"}),
		})

		msg := readDesktopRuntimeMessage(t, messages)
		var listed agentmgr.JournalEntriesData
		if err := json.Unmarshal(msg.Data, &listed); err != nil {
			t.Fatalf("decode journal entries payload: %v", err)
		}
		if listed.Error != "journalctl unavailable" {
			t.Fatalf("error=%q, want journalctl unavailable", listed.Error)
		}
	})
}

func TestLinuxLogBackendQueryEntriesUsesFiltersAndParsesResults(t *testing.T) {
	shellPath, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is required for journal query test")
	}

	originalLookPath := backends.JournalLookPath
	originalNewCommand := backends.NewJournalCommandContext
	originalTimeout := backends.JournalQueryTimeout
	t.Cleanup(func() {
		backends.JournalLookPath = originalLookPath
		backends.NewJournalCommandContext = originalNewCommand
		backends.JournalQueryTimeout = originalTimeout
	})

	backends.JournalLookPath = func(name string) (string, error) {
		if name != "journalctl" {
			t.Fatalf("lookPath called with %q", name)
		}
		return "/usr/bin/journalctl", nil
	}

	var capturedName string
	var capturedArgs []string
	backends.NewJournalCommandContext = func(ctx context.Context, name string, args ...string) (*exec.Cmd, error) {
		capturedName = name
		capturedArgs = append([]string(nil), args...)
		script := `printf '%s\n' '{"__REALTIME_TIMESTAMP":"1700000000000000","_SYSTEMD_UNIT":"sshd.service","PRIORITY":"3","MESSAGE":"denied password","SYSLOG_IDENTIFIER":"sshd"}' '{"__REALTIME_TIMESTAMP":"1700000001000000","_SYSTEMD_UNIT":"cron.service","PRIORITY":"6","MESSAGE":"job complete"}'`
		return exec.CommandContext(ctx, shellPath, "-c", script), nil
	}
	backends.JournalQueryTimeout = 2 * time.Second

	entries, err := backends.LinuxLogBackend{}.QueryEntries(agentmgr.JournalQueryData{
		RequestID: "req-journal-query",
		Since:     "1h ago",
		Until:     "now",
		Unit:      "sshd.service",
		Priority:  "err",
		Search:    "denied",
		Limit:     5,
	})
	if err != nil {
		t.Fatalf("QueryEntries returned error: %v", err)
	}
	if capturedName != "journalctl" {
		t.Fatalf("command name=%q, want journalctl", capturedName)
	}
	if got, want := capturedArgs, []string{
		"--no-pager",
		"--output=json",
		"-n", "5",
		"-r",
		"--since", "1h ago",
		"--until", "now",
		"-u", "sshd.service",
		"-p", "err",
		"--grep", "denied",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("args=%v, want %v", got, want)
	}
	if got, want := len(entries), 2; got != want {
		t.Fatalf("len(entries)=%d, want %d", got, want)
	}
	if entries[0].Source != "sshd" || entries[0].Level != "error" || entries[0].Message != "denied password" {
		t.Fatalf("unexpected first entry %+v", entries[0])
	}
	if entries[1].Source != "cron" || entries[1].Level != "info" || entries[1].Message != "job complete" {
		t.Fatalf("unexpected second entry %+v", entries[1])
	}
}

func TestLinuxLogBackendQueryEntriesTimesOut(t *testing.T) {
	shellPath, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is required for journal timeout test")
	}

	originalLookPath := backends.JournalLookPath
	originalNewCommand := backends.NewJournalCommandContext
	originalTimeout := backends.JournalQueryTimeout
	t.Cleanup(func() {
		backends.JournalLookPath = originalLookPath
		backends.NewJournalCommandContext = originalNewCommand
		backends.JournalQueryTimeout = originalTimeout
	})

	backends.JournalLookPath = func(string) (string, error) { return "/usr/bin/journalctl", nil }
	backends.NewJournalCommandContext = func(ctx context.Context, _ string, _ ...string) (*exec.Cmd, error) {
		return exec.CommandContext(ctx, shellPath, "-c", "sleep 1"), nil
	}
	backends.JournalQueryTimeout = 20 * time.Millisecond

	_, err = backends.LinuxLogBackend{}.QueryEntries(agentmgr.JournalQueryData{Limit: 10})
	if err == nil || err.Error() != "journalctl query timed out" {
		t.Fatalf("error=%v, want journalctl query timed out", err)
	}
}
