package main

import (
	"context"
	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubcollector"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestExecuteProxmoxActionDirectUsesCollectorRuntime(t *testing.T) {
	var taskPolls atomic.Int32
	const upid = "UPID:pve01:001:001:001:qmstart:101:root@pam:"

	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/qemu/101/status/start":
			_, _ = w.Write([]byte(`{"data":"` + upid + `"}`))
		case "/api2/json/nodes/pve01/tasks/" + upid + "/status":
			call := taskPolls.Add(1)
			if call == 1 {
				_, _ = w.Write([]byte(`{"data":{"status":"running"}}`))
			} else {
				_, _ = w.Write([]byte(`{"data":{"status":"stopped","exitstatus":"OK"}}`))
			}
		default:
			t.Fatalf("unexpected proxmox request path: %s", r.URL.Path)
		}
	}))
	defer mock.Close()

	sut := newTestAPIServer(t)

	credentialID := "cred-proxmox-1"
	createProxmoxCredentialProfile(t, sut, credentialID, "labtether@pve!agent", "token-secret", mock.URL)

	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{
			{
				ID:            "collector-proxmox-1",
				AssetID:       "proxmox-cluster-test",
				CollectorType: hubcollector.CollectorTypeProxmox,
				Enabled:       true,
				Config: map[string]any{
					"base_url":      mock.URL,
					"token_id":      "labtether@pve!agent",
					"credential_id": credentialID,
					"skip_verify":   true,
				},
			},
		},
	}

	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-vm-101",
		Type:    "vm",
		Name:    "web-01",
		Source:  "proxmox",
		Status:  "online",
		Metadata: map[string]string{
			"proxmox_type": "qemu",
			"node":         "pve01",
			"vmid":         "101",
			"collector_id": "collector-proxmox-1",
		},
	})
	if err != nil {
		t.Fatalf("failed to seed proxmox asset: %v", err)
	}

	result, err := sut.executeProxmoxActionDirect(context.Background(), "vm.start", connectorsdk.ActionRequest{
		TargetID: "proxmox-vm-101",
	})
	if err != nil {
		t.Fatalf("executeProxmoxActionDirect failed: %v", err)
	}
	if result.Status != "succeeded" {
		t.Fatalf("expected succeeded status, got %s (message=%s output=%s)", result.Status, result.Message, result.Output)
	}
	if result.Metadata["exitstatus"] != "OK" {
		t.Fatalf("expected exitstatus OK, got %q", result.Metadata["exitstatus"])
	}
	if result.Metadata["collector_id"] != "collector-proxmox-1" {
		t.Fatalf("expected collector_id to be propagated, got %q", result.Metadata["collector_id"])
	}
	if taskPolls.Load() < 2 {
		t.Fatalf("expected task status to be polled, got %d calls", taskPolls.Load())
	}
}

func TestExecuteProxmoxActionDirectNodeVMIDTargetUsesCollectorParam(t *testing.T) {
	var collectorOneCalls atomic.Int32
	var collectorTwoCalls atomic.Int32
	const upid = "UPID:pve02:001:001:001:qmstart:101:root@pam:"

	collectorOne := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		collectorOneCalls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"errors":"wrong collector selected"}`))
	}))
	defer collectorOne.Close()

	collectorTwo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		collectorTwoCalls.Add(1)
		switch r.URL.Path {
		case "/api2/json/nodes/pve02/qemu/101/status/start":
			_, _ = w.Write([]byte(`{"data":"` + upid + `"}`))
		case "/api2/json/nodes/pve02/tasks/" + upid + "/status":
			_, _ = w.Write([]byte(`{"data":{"status":"stopped","exitstatus":"OK"}}`))
		default:
			t.Fatalf("unexpected proxmox request path: %s", r.URL.Path)
		}
	}))
	defer collectorTwo.Close()

	sut := newTestAPIServer(t)

	createProxmoxCredentialProfile(
		t,
		sut,
		"cred-proxmox-collector-1",
		"labtether@pve!collector1",
		"token-secret-1",
		collectorOne.URL,
	)
	createProxmoxCredentialProfile(
		t,
		sut,
		"cred-proxmox-collector-2",
		"labtether@pve!collector2",
		"token-secret-2",
		collectorTwo.URL,
	)

	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{
			{
				ID:            "collector-proxmox-1",
				AssetID:       "proxmox-cluster-one",
				CollectorType: hubcollector.CollectorTypeProxmox,
				Enabled:       true,
				Config: map[string]any{
					"base_url":      collectorOne.URL,
					"token_id":      "labtether@pve!collector1",
					"credential_id": "cred-proxmox-collector-1",
					"skip_verify":   true,
				},
			},
			{
				ID:            "collector-proxmox-2",
				AssetID:       "proxmox-cluster-two",
				CollectorType: hubcollector.CollectorTypeProxmox,
				Enabled:       true,
				Config: map[string]any{
					"base_url":      collectorTwo.URL,
					"token_id":      "labtether@pve!collector2",
					"credential_id": "cred-proxmox-collector-2",
					"skip_verify":   true,
				},
			},
		},
	}

	result, err := sut.executeProxmoxActionDirect(context.Background(), "vm.start", connectorsdk.ActionRequest{
		TargetID: "pve02/101",
		Params: map[string]string{
			"collector_id": "collector-proxmox-2",
		},
	})
	if err != nil {
		t.Fatalf("executeProxmoxActionDirect failed: %v", err)
	}
	if result.Status != "succeeded" {
		t.Fatalf("expected succeeded status, got %s (message=%s output=%s)", result.Status, result.Message, result.Output)
	}
	if result.Metadata["collector_id"] != "collector-proxmox-2" {
		t.Fatalf("expected collector_id collector-proxmox-2, got %q", result.Metadata["collector_id"])
	}
	if collectorOneCalls.Load() != 0 {
		t.Fatalf("expected collector one to receive no requests, got %d", collectorOneCalls.Load())
	}
	if collectorTwoCalls.Load() < 2 {
		t.Fatalf("expected collector two action + task poll calls, got %d", collectorTwoCalls.Load())
	}
}

func TestExecuteProxmoxActionDirectNodeVMIDTargetRequiresCollectorWhenMultipleConfigured(t *testing.T) {
	var collectorOneCalls atomic.Int32
	var collectorTwoCalls atomic.Int32

	collectorOne := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		collectorOneCalls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"errors":"unexpected collector one request"}`))
	}))
	defer collectorOne.Close()

	collectorTwo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		collectorTwoCalls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"errors":"unexpected collector two request"}`))
	}))
	defer collectorTwo.Close()

	sut := newTestAPIServer(t)

	createProxmoxCredentialProfile(
		t,
		sut,
		"cred-proxmox-collector-1",
		"labtether@pve!collector1",
		"token-secret-1",
		collectorOne.URL,
	)
	createProxmoxCredentialProfile(
		t,
		sut,
		"cred-proxmox-collector-2",
		"labtether@pve!collector2",
		"token-secret-2",
		collectorTwo.URL,
	)

	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{
			{
				ID:            "collector-proxmox-1",
				AssetID:       "proxmox-cluster-one",
				CollectorType: hubcollector.CollectorTypeProxmox,
				Enabled:       true,
				Config: map[string]any{
					"base_url":      collectorOne.URL,
					"token_id":      "labtether@pve!collector1",
					"credential_id": "cred-proxmox-collector-1",
					"skip_verify":   true,
				},
			},
			{
				ID:            "collector-proxmox-2",
				AssetID:       "proxmox-cluster-two",
				CollectorType: hubcollector.CollectorTypeProxmox,
				Enabled:       true,
				Config: map[string]any{
					"base_url":      collectorTwo.URL,
					"token_id":      "labtether@pve!collector2",
					"credential_id": "cred-proxmox-collector-2",
					"skip_verify":   true,
				},
			},
		},
	}

	_, err := sut.executeProxmoxActionDirect(context.Background(), "vm.start", connectorsdk.ActionRequest{
		TargetID: "pve02/101",
	})
	if err == nil || !strings.Contains(err.Error(), "collector_id is required") {
		t.Fatalf("expected multi-collector validation error, got %v", err)
	}
	if collectorOneCalls.Load() != 0 {
		t.Fatalf("expected collector one to receive no requests, got %d", collectorOneCalls.Load())
	}
	if collectorTwoCalls.Load() != 0 {
		t.Fatalf("expected collector two to receive no requests, got %d", collectorTwoCalls.Load())
	}
}

func TestExecuteActionInProcessUsesProxmoxPath(t *testing.T) {
	const upid = "UPID:pve01:001:001:001:qmstart:101:root@pam:"
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/qemu/101/status/start":
			_, _ = w.Write([]byte(`{"data":"` + upid + `"}`))
		case "/api2/json/nodes/pve01/tasks/" + upid + "/status":
			_, _ = w.Write([]byte(`{"data":{"status":"stopped","exitstatus":"OK"}}`))
		default:
			t.Fatalf("unexpected proxmox request path: %s", r.URL.Path)
		}
	}))
	defer mock.Close()

	sut := newTestAPIServer(t)
	credentialID := "cred-proxmox-path"
	createProxmoxCredentialProfile(t, sut, credentialID, "labtether@pve!agent", "token-secret", mock.URL)

	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{
			{
				ID:            "collector-proxmox-1",
				AssetID:       "proxmox-cluster-test",
				CollectorType: hubcollector.CollectorTypeProxmox,
				Enabled:       true,
				Config: map[string]any{
					"base_url":      mock.URL,
					"token_id":      "labtether@pve!agent",
					"credential_id": credentialID,
					"skip_verify":   true,
				},
			},
		},
	}

	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-vm-101",
		Type:    "vm",
		Name:    "web-01",
		Source:  "proxmox",
		Status:  "online",
		Metadata: map[string]string{
			"proxmox_type": "qemu",
			"node":         "pve01",
			"vmid":         "101",
			"collector_id": "collector-proxmox-1",
		},
	})
	if err != nil {
		t.Fatalf("failed to seed proxmox asset: %v", err)
	}

	result := sut.executeActionInProcess(actions.Job{
		JobID:       "job-proxmox-1",
		RunID:       "run-proxmox-1",
		Type:        actions.RunTypeConnectorAction,
		ActorID:     "owner",
		Target:      "proxmox-vm-101",
		ConnectorID: "proxmox",
		ActionID:    "vm.start",
	})
	if result.Status != actions.StatusSucceeded {
		t.Fatalf("expected succeeded action result, got status=%s error=%s output=%s", result.Status, result.Error, result.Output)
	}
	if !strings.Contains(result.Output, "vm.start on pve01/101") {
		t.Fatalf("expected proxmox action output to include target, got %q", result.Output)
	}
	if len(result.Steps) != 1 || result.Steps[0].Status != actions.StatusSucceeded {
		t.Fatalf("expected successful connector_execute step, got %+v", result.Steps)
	}
}

func TestExecuteActionInProcessFallbackBranch(t *testing.T) {
	sut := newTestAPIServer(t)
	result := sut.executeActionInProcess(actions.Job{
		JobID:   "job-cmd-1",
		RunID:   "run-cmd-1",
		Type:    actions.RunTypeCommand,
		Target:  "host-1",
		Command: "echo ok",
	})
	if result.Status != actions.StatusFailed || !strings.Contains(result.Error, "not connected") {
		t.Fatalf("expected disconnected command-run fallback to fail closed, got %+v", result)
	}
}
