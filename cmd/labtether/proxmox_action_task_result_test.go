package main

import (
	"context"
	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/hubcollector"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExecuteProxmoxActionDryRunAndNoUPID(t *testing.T) {
	sut := newTestAPIServer(t)

	dryRun, err := sut.executeProxmoxAction(context.Background(), "vm.start", "pve01/101", map[string]string{
		"collector_id": "collector-proxmox-1",
	}, true)
	if err != nil {
		t.Fatalf("dry-run executeProxmoxAction failed: %v", err)
	}
	if dryRun.Status != "succeeded" || !strings.Contains(dryRun.Output, "would execute vm.start on pve01/101") {
		t.Fatalf("unexpected dry-run result: %+v", dryRun)
	}
	if dryRun.Metadata["collector_id"] != "collector-proxmox-1" {
		t.Fatalf("expected dry-run collector id passthrough, got %q", dryRun.Metadata["collector_id"])
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api2/json/nodes/pve01/qemu/101/resize" {
			_, _ = w.Write([]byte(`{"data":null}`))
			return
		}
		t.Fatalf("unexpected proxmox request path: %s", r.URL.Path)
	}))
	defer server.Close()

	createProxmoxCredentialProfile(t, sut, "cred-no-upid", "labtether@pve!agent", "token-secret", server.URL)
	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{
			{
				ID:            "collector-proxmox-1",
				CollectorType: hubcollector.CollectorTypeProxmox,
				Enabled:       true,
				Config: map[string]any{
					"base_url":      server.URL,
					"token_id":      "labtether@pve!agent",
					"credential_id": "cred-no-upid",
					"skip_verify":   true,
				},
			},
		},
	}

	result, err := sut.executeProxmoxAction(context.Background(), "vm.disk_resize", "pve01/101", map[string]string{
		"disk":         "scsi0",
		"size":         "+10G",
		"collector_id": "collector-proxmox-1",
	}, false)
	if err != nil {
		t.Fatalf("executeProxmoxAction no-upid path failed: %v", err)
	}
	if result.Status != "succeeded" || result.Metadata["upid"] != "" {
		t.Fatalf("expected no-upid success result, got %+v", result)
	}
	if result.Metadata["collector_id"] != "collector-proxmox-1" {
		t.Fatalf("expected collector_id metadata, got %q", result.Metadata["collector_id"])
	}
}

func TestExecuteProxmoxActionTaskExitFailureAndExecuteActionFailureResult(t *testing.T) {
	const upid = "UPID:pve01:001:001:001:qmstop:101:root@pam:"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/qemu/101/status/stop":
			_, _ = w.Write([]byte(`{"data":"` + upid + `"}`))
		case "/api2/json/nodes/pve01/tasks/" + upid + "/status":
			_, _ = w.Write([]byte(`{"data":{"status":"stopped","exitstatus":"ERROR"}}`))
		default:
			t.Fatalf("unexpected proxmox path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	sut := newTestAPIServer(t)
	createProxmoxCredentialProfile(t, sut, "cred-exit-fail", "labtether@pve!agent", "token-secret", server.URL)
	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{
			{
				ID:            "collector-proxmox-1",
				CollectorType: hubcollector.CollectorTypeProxmox,
				Enabled:       true,
				Config: map[string]any{
					"base_url":      server.URL,
					"token_id":      "labtether@pve!agent",
					"credential_id": "cred-exit-fail",
					"skip_verify":   true,
				},
			},
		},
	}

	result, err := sut.executeProxmoxAction(context.Background(), "vm.stop", "pve01/101", map[string]string{
		"collector_id": "collector-proxmox-1",
	}, false)
	if err != nil {
		t.Fatalf("executeProxmoxAction exit-failure path failed: %v", err)
	}
	if result.Status != "failed" || !strings.Contains(strings.ToLower(result.Message), "exitstatus error") {
		t.Fatalf("expected failed result from non-ok exitstatus, got %+v", result)
	}

	failedStatus := sut.executeActionInProcess(actions.Job{
		JobID:       "job-proxmox-exit-fail",
		RunID:       "run-proxmox-exit-fail",
		Type:        actions.RunTypeConnectorAction,
		ConnectorID: "proxmox",
		ActionID:    "vm.stop",
		Target:      "pve01/101",
		Params: map[string]string{
			"collector_id": "collector-proxmox-1",
		},
	})
	if failedStatus.Status != actions.StatusFailed {
		t.Fatalf("expected executeActionInProcess to map failed exec result status, got %+v", failedStatus)
	}

	failed := sut.executeActionInProcess(actions.Job{
		JobID:       "job-proxmox-fail",
		RunID:       "run-proxmox-fail",
		Type:        actions.RunTypeConnectorAction,
		ConnectorID: "proxmox",
		ActionID:    "vm.start",
		Target:      "",
	})
	if failed.Status != actions.StatusFailed || strings.TrimSpace(failed.Error) == "" {
		t.Fatalf("expected failed action result for invalid proxmox target, got %+v", failed)
	}
}

func TestExecuteProxmoxActionTaskWaitErrorAndBlankExitStatus(t *testing.T) {
	const failedUPID = "UPID:pve01:001:001:001:qmstart:101:root@pam:"
	const blankExitUPID = "UPID:pve01:001:001:001:qmstart:102:root@pam:"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/qemu/101/status/start":
			_, _ = w.Write([]byte(`{"data":"` + failedUPID + `"}`))
		case "/api2/json/nodes/pve01/tasks/" + failedUPID + "/status":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"errors":"task poll failed"}`))
		case "/api2/json/nodes/pve01/qemu/102/status/start":
			_, _ = w.Write([]byte(`{"data":"` + blankExitUPID + `"}`))
		case "/api2/json/nodes/pve01/tasks/" + blankExitUPID + "/status":
			_, _ = w.Write([]byte(`{"data":{"status":"stopped"}}`))
		default:
			t.Fatalf("unexpected proxmox path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	sut := newTestAPIServer(t)
	createProxmoxCredentialProfile(t, sut, "cred-task-wait", "labtether@pve!agent", "token-secret", server.URL)
	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{
			{
				ID:            "collector-proxmox-1",
				CollectorType: hubcollector.CollectorTypeProxmox,
				Enabled:       true,
				Config: map[string]any{
					"base_url":      server.URL,
					"token_id":      "labtether@pve!agent",
					"credential_id": "cred-task-wait",
					"skip_verify":   true,
				},
			},
		},
	}

	waitErrResult, err := sut.executeProxmoxAction(context.Background(), "vm.start", "pve01/101", map[string]string{
		"collector_id": "collector-proxmox-1",
	}, false)
	if err != nil {
		t.Fatalf("executeProxmoxAction wait error path failed: %v", err)
	}
	if waitErrResult.Status != "failed" || !strings.Contains(waitErrResult.Message, "proxmox api returned 502") {
		t.Fatalf("expected wait error to map to failed result, got %+v", waitErrResult)
	}

	blankExitResult, err := sut.executeProxmoxAction(context.Background(), "vm.start", "pve01/102", map[string]string{
		"collector_id": "collector-proxmox-1",
	}, false)
	if err != nil {
		t.Fatalf("executeProxmoxAction blank exitstatus path failed: %v", err)
	}
	if blankExitResult.Status != "succeeded" {
		t.Fatalf("expected blank exitstatus to default to success, got %+v", blankExitResult)
	}
	if blankExitResult.Metadata["exitstatus"] != "OK" {
		t.Fatalf("expected blank exitstatus to default to OK, got %q", blankExitResult.Metadata["exitstatus"])
	}
}
