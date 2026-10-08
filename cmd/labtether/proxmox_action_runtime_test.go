package main

import (
	"context"
	"fmt"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectors/proxmox"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/credentials"
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
	"github.com/labtether/labtether/internal/hubcollector"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
