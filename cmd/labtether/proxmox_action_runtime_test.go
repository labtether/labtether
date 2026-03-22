package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectors/proxmox"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/credentials"
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
	"github.com/labtether/labtether/internal/hubcollector"
)

type stubHubCollectorStore struct {
	collectors []hubcollector.Collector
}

func (s *stubHubCollectorStore) CreateHubCollector(req hubcollector.CreateCollectorRequest) (hubcollector.Collector, error) {
	return hubcollector.Collector{}, fmt.Errorf("not implemented")
}

func (s *stubHubCollectorStore) GetHubCollector(id string) (hubcollector.Collector, bool, error) {
	for _, collector := range s.collectors {
		if collector.ID == id {
			return collector, true, nil
		}
	}
	return hubcollector.Collector{}, false, nil
}

func (s *stubHubCollectorStore) ListHubCollectors(limit int, enabledOnly bool) ([]hubcollector.Collector, error) {
	result := make([]hubcollector.Collector, 0, len(s.collectors))
	for _, collector := range s.collectors {
		if enabledOnly && !collector.Enabled {
			continue
		}
		result = append(result, collector)
	}
	return result, nil
}

func (s *stubHubCollectorStore) UpdateHubCollector(id string, req hubcollector.UpdateCollectorRequest) (hubcollector.Collector, error) {
	return hubcollector.Collector{}, fmt.Errorf("not implemented")
}

func (s *stubHubCollectorStore) DeleteHubCollector(id string) error {
	return fmt.Errorf("not implemented")
}

func (s *stubHubCollectorStore) UpdateHubCollectorStatus(id, status, lastError string, collectedAt time.Time) error {
	return nil
}

func createProxmoxCredentialProfile(t *testing.T, sut *apiServer, credentialID, username, secret, baseURL string) {
	t.Helper()
	allowInsecureTransportForConnectorTests(t)

	secretCiphertext, err := sut.secretsManager.EncryptString(secret, credentialID)
	if err != nil {
		t.Fatalf("failed to encrypt credential %s: %v", credentialID, err)
	}
	_, err = sut.credentialStore.CreateCredentialProfile(credentials.Profile{
		ID:               credentialID,
		Name:             "proxmox " + credentialID,
		Kind:             credentials.KindProxmoxAPIToken,
		Username:         username,
		Status:           "active",
		SecretCiphertext: secretCiphertext,
		Metadata:         map[string]string{"base_url": baseURL},
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("failed to store credential profile %s: %v", credentialID, err)
	}
}

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

func TestInvokeProxmoxActionMappings(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/resize") {
			_, _ = w.Write([]byte(`{"data":null}`))
			return
		}
		if r.Method == http.MethodDelete || r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"data":"UPID:ok"}`))
			return
		}
		t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	client, err := proxmox.NewClient(proxmox.Config{
		BaseURL:     server.URL,
		TokenID:     "id",
		TokenSecret: "secret",
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	runtime := proxmoxpkg.NewProxmoxRuntime(client)

	cases := []struct {
		actionID string
		vmid     string
		params   map[string]string
		wantUPID bool
	}{
		{actionID: "vm.start", vmid: "100", wantUPID: true},
		{actionID: "vm.stop", vmid: "100", wantUPID: true},
		{actionID: "vm.shutdown", vmid: "100", wantUPID: true},
		{actionID: "vm.reboot", vmid: "100", wantUPID: true},
		{actionID: "vm.snapshot", vmid: "100", params: map[string]string{"snapshot_name": "snap-1"}, wantUPID: true},
		{actionID: "vm.migrate", vmid: "100", params: map[string]string{"target_node": "pve02"}, wantUPID: true},
		{actionID: "ct.start", vmid: "200", wantUPID: true},
		{actionID: "ct.stop", vmid: "200", wantUPID: true},
		{actionID: "ct.shutdown", vmid: "200", wantUPID: true},
		{actionID: "ct.reboot", vmid: "200", wantUPID: true},
		{actionID: "ct.snapshot", vmid: "200", params: map[string]string{"snapshot_name": "snap-2"}, wantUPID: true},
		{actionID: "vm.suspend", vmid: "100", wantUPID: true},
		{actionID: "vm.resume", vmid: "100", wantUPID: true},
		{actionID: "vm.force_stop", vmid: "100", wantUPID: true},
		{actionID: "ct.force_stop", vmid: "200", wantUPID: true},
		{actionID: "vm.snapshot.delete", vmid: "100", params: map[string]string{"snapshot_name": "snap-1"}, wantUPID: true},
		{actionID: "vm.snapshot.rollback", vmid: "100", params: map[string]string{"snapshot_name": "snap-1"}, wantUPID: true},
		{actionID: "ct.snapshot.delete", vmid: "200", params: map[string]string{"snapshot_name": "snap-2"}, wantUPID: true},
		{actionID: "ct.snapshot.rollback", vmid: "200", params: map[string]string{"snapshot_name": "snap-2"}, wantUPID: true},
		{actionID: "ct.migrate", vmid: "200", params: map[string]string{"target_node": "pve02"}, wantUPID: true},
		{actionID: "vm.backup", vmid: "100", params: map[string]string{"storage": "local", "mode": "snapshot"}, wantUPID: true},
		{actionID: "ct.backup", vmid: "200", params: map[string]string{"storage": "local", "mode": "snapshot"}, wantUPID: true},
		{actionID: "vm.clone", vmid: "100", params: map[string]string{"new_id": "101", "new_name": "clone-vm"}, wantUPID: true},
		{actionID: "ct.clone", vmid: "200", params: map[string]string{"new_id": "201", "new_name": "clone-ct"}, wantUPID: true},
		{actionID: "vm.disk_resize", vmid: "100", params: map[string]string{"disk": "scsi0", "size": "+10G"}, wantUPID: false},
	}

	for _, tc := range cases {
		upid, err := proxmoxpkg.InvokeProxmoxAction(context.Background(), runtime, tc.actionID, "pve01", tc.vmid, tc.params)
		if err != nil {
			t.Fatalf("proxmoxpkg.InvokeProxmoxAction(%s) failed: %v", tc.actionID, err)
		}
		if tc.wantUPID && strings.TrimSpace(upid) == "" {
			t.Fatalf("expected action %s to return UPID", tc.actionID)
		}
		if !tc.wantUPID && strings.TrimSpace(upid) != "" {
			t.Fatalf("expected action %s to return empty UPID, got %q", tc.actionID, upid)
		}
	}

	if _, err := proxmoxpkg.InvokeProxmoxAction(context.Background(), runtime, "vm.migrate", "pve01", "100", nil); err == nil {
		t.Fatalf("expected vm.migrate without target_node to fail")
	}
	if _, err := proxmoxpkg.InvokeProxmoxAction(context.Background(), runtime, "ct.migrate", "pve01", "200", nil); err == nil {
		t.Fatalf("expected ct.migrate without target_node to fail")
	}
	if _, err := proxmoxpkg.InvokeProxmoxAction(context.Background(), runtime, "ct.clone", "pve01", "200", nil); err == nil {
		t.Fatalf("expected ct.clone without new_id to fail")
	}
	if _, err := proxmoxpkg.InvokeProxmoxAction(context.Background(), runtime, "vm.clone", "pve01", "100", nil); err == nil {
		t.Fatalf("expected vm.clone without new_id to fail")
	}
	if _, err := proxmoxpkg.InvokeProxmoxAction(context.Background(), runtime, "vm.clone", "pve01", "100", map[string]string{"new_id": "not-a-number"}); err == nil {
		t.Fatalf("expected vm.clone with non-numeric new_id to fail")
	}
	if _, err := proxmoxpkg.InvokeProxmoxAction(context.Background(), runtime, "ct.clone", "pve01", "200", map[string]string{"new_id": "not-a-number"}); err == nil {
		t.Fatalf("expected ct.clone with non-numeric new_id to fail")
	}
	if _, err := proxmoxpkg.InvokeProxmoxAction(context.Background(), runtime, "vm.snapshot.delete", "pve01", "100", nil); err == nil {
		t.Fatalf("expected vm.snapshot.delete without snapshot_name to fail")
	}
	if _, err := proxmoxpkg.InvokeProxmoxAction(context.Background(), runtime, "vm.snapshot.rollback", "pve01", "100", nil); err == nil {
		t.Fatalf("expected vm.snapshot.rollback without snapshot_name to fail")
	}
	if _, err := proxmoxpkg.InvokeProxmoxAction(context.Background(), runtime, "ct.snapshot.delete", "pve01", "200", nil); err == nil {
		t.Fatalf("expected ct.snapshot.delete without snapshot_name to fail")
	}
	if _, err := proxmoxpkg.InvokeProxmoxAction(context.Background(), runtime, "ct.snapshot.rollback", "pve01", "200", nil); err == nil {
		t.Fatalf("expected ct.snapshot.rollback without snapshot_name to fail")
	}
	if _, err := proxmoxpkg.InvokeProxmoxAction(context.Background(), runtime, "vm.snapshot", "pve01", "100", nil); err != nil {
		t.Fatalf("expected vm.snapshot default-name path to succeed: %v", err)
	}
	if _, err := proxmoxpkg.InvokeProxmoxAction(context.Background(), runtime, "ct.snapshot", "pve01", "200", nil); err != nil {
		t.Fatalf("expected ct.snapshot default-name path to succeed: %v", err)
	}
	if _, err := proxmoxpkg.InvokeProxmoxAction(context.Background(), nil, "vm.start", "pve01", "100", nil); err == nil {
		t.Fatalf("expected nil runtime to fail")
	}
	if _, err := proxmoxpkg.InvokeProxmoxAction(context.Background(), runtime, "vm.disk_resize", "pve01", "100", map[string]string{"disk": "scsi0"}); err == nil {
		t.Fatalf("expected vm.disk_resize without size to fail")
	}
	if _, err := proxmoxpkg.InvokeProxmoxAction(context.Background(), runtime, "unsupported.action", "pve01", "100", nil); err == nil {
		t.Fatalf("expected unsupported action to fail")
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

func TestExecuteActionInProcessFallbackBranch(t *testing.T) {
	sut := newTestAPIServer(t)
	result := sut.executeActionInProcess(actions.Job{
		JobID:   "job-cmd-1",
		RunID:   "run-cmd-1",
		Type:    actions.RunTypeCommand,
		Target:  "host-1",
		Command: "echo ok",
	})
	if result.Status != actions.StatusSucceeded {
		t.Fatalf("expected command-run fallback to succeed, got %+v", result)
	}
}

func TestResolveProxmoxActionTargetGuards(t *testing.T) {
	sut := newTestAPIServer(t)

	if _, _, _, err := sut.resolveProxmoxActionTarget("vm.start", ""); err == nil {
		t.Fatalf("expected empty target to fail")
	}
	if _, _, _, err := sut.resolveProxmoxActionTarget("vm.start", "pve01/"); err == nil || !strings.Contains(err.Error(), "node/vmid") {
		t.Fatalf("expected malformed node/vmid target error, got %v", err)
	}
	if _, _, _, err := sut.resolveProxmoxActionTarget("host.start", "proxmox-vm-101"); err == nil || !strings.Contains(err.Error(), "unsupported proxmox action prefix") {
		t.Fatalf("expected unsupported action prefix error, got %v", err)
	}

	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "agent-host-1",
		Type:    "server",
		Name:    "agent-host-1",
		Source:  "agent",
		Status:  "online",
	})
	if err != nil {
		t.Fatalf("failed to seed non-proxmox asset: %v", err)
	}
	if _, _, _, err := sut.resolveProxmoxActionTarget("vm.start", "agent-host-1"); err == nil || !strings.Contains(err.Error(), "not a proxmox asset") {
		t.Fatalf("expected non-proxmox target error, got %v", err)
	}

	_, err = sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-ct-200",
		Type:    "container",
		Name:    "ct-200",
		Source:  "proxmox",
		Status:  "online",
		Metadata: map[string]string{
			"proxmox_type": "lxc",
			"node":         "pve01",
			"vmid":         "200",
		},
	})
	if err != nil {
		t.Fatalf("failed to seed proxmox lxc asset: %v", err)
	}
	if _, _, _, err := sut.resolveProxmoxActionTarget("vm.start", "proxmox-ct-200"); err == nil || !strings.Contains(err.Error(), "does not match action") {
		t.Fatalf("expected target kind mismatch error, got %v", err)
	}

	node, vmid, collectorID, err := sut.resolveProxmoxActionTarget("ct.start", "proxmox-ct-200")
	if err != nil {
		t.Fatalf("expected ct target resolution to succeed, got %v", err)
	}
	if node != "pve01" || vmid != "200" || collectorID != "" {
		t.Fatalf("unexpected ct target resolution: node=%q vmid=%q collector=%q", node, vmid, collectorID)
	}

	_, err = sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-vm-missing-node",
		Type:    "vm",
		Name:    " ",
		Source:  "proxmox",
		Status:  "online",
		Metadata: map[string]string{
			"proxmox_type": "qemu",
			"vmid":         "999",
		},
	})
	if err != nil {
		t.Fatalf("failed to seed malformed proxmox asset: %v", err)
	}
	if _, _, _, err := sut.resolveProxmoxActionTarget("vm.start", "proxmox-vm-missing-node"); err == nil || !strings.Contains(err.Error(), "missing node metadata") {
		t.Fatalf("expected resolve error for missing node metadata, got %v", err)
	}
}

func TestExecuteProxmoxActionDirectErrorPropagation(t *testing.T) {
	sut := newTestAPIServer(t)
	_, err := sut.executeProxmoxActionDirect(context.Background(), "vm.start", connectorsdk.ActionRequest{
		TargetID: "",
	})
	if err == nil || !strings.Contains(err.Error(), "target is required") {
		t.Fatalf("expected target validation error, got %v", err)
	}
}

func TestExecuteProxmoxActionRuntimeFailureAndInvokeValidation(t *testing.T) {
	sut := newTestAPIServer(t)

	if _, err := sut.executeProxmoxAction(context.Background(), "vm.start", "pve01/101", nil, false); err == nil {
		t.Fatalf("expected missing runtime failure")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected proxmox request path during invoke validation: %s", r.URL.Path)
	}))
	defer server.Close()

	createProxmoxCredentialProfile(t, sut, "cred-invoke-validate", "labtether@pve!agent", "token-secret", server.URL)
	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{
			{
				ID:            "collector-proxmox-1",
				CollectorType: hubcollector.CollectorTypeProxmox,
				Enabled:       true,
				Config: map[string]any{
					"base_url":      server.URL,
					"token_id":      "labtether@pve!agent",
					"credential_id": "cred-invoke-validate",
					"skip_verify":   true,
				},
			},
		},
	}

	if _, err := sut.executeProxmoxAction(context.Background(), "vm.snapshot.delete", "pve01/101", map[string]string{
		"collector_id": "collector-proxmox-1",
	}, false); err == nil || !strings.Contains(err.Error(), "snapshot_name is required") {
		t.Fatalf("expected invoke validation failure, got %v", err)
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

func TestProxmoxActionRuntimeHelpers(t *testing.T) {
	blankErr := fmt.Errorf("   ")
	if got := proxmoxActionErrorMessage(blankErr); got != "proxmox action execution failed" {
		t.Fatalf("expected fallback error message for blank error, got %q", got)
	}
	if got := proxmoxActionErrorMessage(nil); got != "proxmox action execution failed" {
		t.Fatalf("expected fallback error message for nil error, got %q", got)
	}
	if got := proxmoxActionErrorMessage(fmt.Errorf("boom")); got != "boom" {
		t.Fatalf("expected concrete error message, got %q", got)
	}

	if got := proxmoxActionOutput(proxmoxActionExecution{Output: "  output text  ", Message: "ignored"}); got != "output text" {
		t.Fatalf("expected trimmed output, got %q", got)
	}
	if got := proxmoxActionOutput(proxmoxActionExecution{Output: " ", Message: " message text "}); got != "message text" {
		t.Fatalf("expected message fallback output, got %q", got)
	}

	if err := validateResolvedProxmoxActionTarget(proxmoxSessionTarget{Kind: "lxc", Node: "pve01", VMID: "101"}, "qemu", "vm.start"); err == nil || !strings.Contains(err.Error(), "does not match action") {
		t.Fatalf("expected kind mismatch validation error, got %v", err)
	}
	if err := validateResolvedProxmoxActionTarget(proxmoxSessionTarget{Kind: "qemu", Node: "pve01", VMID: ""}, "qemu", "vm.start"); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("expected incomplete target validation error, got %v", err)
	}
	if err := validateResolvedProxmoxActionTarget(proxmoxSessionTarget{Kind: "qemu", Node: "pve01", VMID: "101"}, "qemu", "vm.start"); err != nil {
		t.Fatalf("expected valid resolved target, got %v", err)
	}
}
