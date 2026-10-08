package main

import (
	"context"
	"errors"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectors/proxmox"
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/telemetry"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type proxmoxAssetStoreWithErrors struct {
	inner   persistence.AssetStore
	getErr  error
	listErr error
}

type proxmoxTelemetryStoreWithSeriesError struct {
	inner       persistence.TelemetryStore
	failingID   string
	seriesError error
}

func (s *proxmoxAssetStoreWithErrors) UpsertAssetHeartbeat(req assets.HeartbeatRequest) (assets.Asset, error) {
	return s.inner.UpsertAssetHeartbeat(req)
}

func (s *proxmoxAssetStoreWithErrors) UpdateAsset(id string, req assets.UpdateRequest) (assets.Asset, error) {
	return s.inner.UpdateAsset(id, req)
}

func (s *proxmoxAssetStoreWithErrors) ListAssets() ([]assets.Asset, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.inner.ListAssets()
}

func (s *proxmoxAssetStoreWithErrors) GetAsset(id string) (assets.Asset, bool, error) {
	if s.getErr != nil {
		return assets.Asset{}, false, s.getErr
	}
	return s.inner.GetAsset(id)
}

func (s *proxmoxAssetStoreWithErrors) DeleteAsset(id string) error {
	return s.inner.DeleteAsset(id)
}

func (s *proxmoxTelemetryStoreWithSeriesError) AppendSamples(ctx context.Context, samples []telemetry.MetricSample) error {
	return s.inner.AppendSamples(ctx, samples)
}

func (s *proxmoxTelemetryStoreWithSeriesError) Snapshot(assetID string, at time.Time) (telemetry.Snapshot, error) {
	return s.inner.Snapshot(assetID, at)
}

func (s *proxmoxTelemetryStoreWithSeriesError) Series(assetID string, start, end time.Time, step time.Duration) ([]telemetry.Series, error) {
	if assetID == s.failingID && s.seriesError != nil {
		return nil, s.seriesError
	}
	return s.inner.Series(assetID, start, end, step)
}

func TestSortProxmoxSnapshots(t *testing.T) {
	snapshots := []proxmox.Snapshot{
		{Name: "current", SnapTime: 10},
		{Name: "older", SnapTime: 20},
		{Name: "newer", SnapTime: 30},
	}

	sorted := sortProxmoxSnapshots(snapshots)
	if len(sorted) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(sorted))
	}
	if sorted[0].Name != "newer" || sorted[1].Name != "older" {
		t.Fatalf("unexpected sort order: %+v", sorted)
	}
}

func TestFilterAndSortProxmoxTasks(t *testing.T) {
	tasks := []proxmox.Task{
		{
			UPID:      "UPID:pve02:001:001:001:qmstart:101:root@pam:",
			Node:      "pve02",
			ID:        "101",
			StartTime: 200,
		},
		{
			UPID:      "UPID:pve01:001:001:001:qmstart:101:root@pam:",
			Node:      "pve01",
			ID:        "101",
			StartTime: 100,
		},
		{
			UPID:      "UPID:pve01:001:001:001:qmstop:100:root@pam:",
			Node:      "pve01",
			ID:        "100",
			StartTime: 300,
		},
	}

	filtered := filterAndSortProxmoxTasks(tasks, "pve01", "101", 10)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered task, got %d", len(filtered))
	}
	if filtered[0].Node != "pve01" || filtered[0].ID != "101" {
		t.Fatalf("unexpected filtered task: %+v", filtered[0])
	}
}

func TestSelectProxmoxHAForVMAndNode(t *testing.T) {
	resources := []proxmox.HAResource{
		{SID: "vm:101", Node: "pve01", State: "started"},
		{SID: "ct:202", Node: "pve01", State: "started"},
		{SID: "vm:999", Node: "pve02", State: "started"},
	}

	vmTarget := proxmoxSessionTarget{Kind: "qemu", VMID: "101", Node: "pve01"}
	match, related := selectProxmoxHA(resources, vmTarget)
	if match == nil || match.SID != "vm:101" {
		t.Fatalf("expected vm ha match, got %+v", match)
	}
	if len(related) != 1 {
		t.Fatalf("expected 1 related vm ha resource, got %d", len(related))
	}

	nodeTarget := proxmoxSessionTarget{Kind: "node", Node: "pve01"}
	match, related = selectProxmoxHA(resources, nodeTarget)
	if match == nil || match.Node != "pve01" {
		t.Fatalf("expected node ha match for pve01, got %+v", match)
	}
	if len(related) != 2 {
		t.Fatalf("expected 2 related node ha resources, got %d", len(related))
	}
}

func TestParseMetadataFloatAndParseAnyInt64(t *testing.T) {
	metadata := map[string]string{
		"used_percent": "84.5",
		"invalid":      "nope",
	}
	used, ok := proxmoxpkg.ParseMetadataFloat(metadata, "invalid", "used_percent")
	if !ok || used != 84.5 {
		t.Fatalf("expected proxmoxpkg.ParseMetadataFloat to resolve 84.5, got value=%v ok=%v", used, ok)
	}
	if _, ok := proxmoxpkg.ParseMetadataFloat(metadata, "missing"); ok {
		t.Fatalf("expected proxmoxpkg.ParseMetadataFloat to fail for missing key")
	}

	cases := []struct {
		value any
		want  int64
		ok    bool
	}{
		{value: int64(42), want: 42, ok: true},
		{value: int32(43), want: 43, ok: true},
		{value: int(44), want: 44, ok: true},
		{value: float64(45.9), want: 45, ok: true},
		{value: float32(46.9), want: 46, ok: true},
		{value: "47.1", want: 47, ok: true},
		{value: "nope", want: 0, ok: false},
	}
	for _, tc := range cases {
		got, ok := parseAnyInt64(tc.value)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("parseAnyInt64(%v) => (%d,%v), expected (%d,%v)", tc.value, got, ok, tc.want, tc.ok)
		}
	}
}

func TestProxmoxTaskVMIDAndRouteDispatch(t *testing.T) {
	if got := proxmoxpkg.ProxmoxTaskVMID(proxmox.Task{ID: "101"}); got != 101 {
		t.Fatalf("expected vmid from id field 101, got %d", got)
	}
	if got := proxmoxpkg.ProxmoxTaskVMID(proxmox.Task{UPID: "UPID:pve01:001:001:001:vzdump:202:root@pam:"}); got != 202 {
		t.Fatalf("expected vmid from upid field 202, got %d", got)
	}
	if got := proxmoxpkg.ProxmoxTaskVMID(proxmox.Task{ID: "bad", UPID: "UPID:bad"}); got != 0 {
		t.Fatalf("expected vmid parse fallback to 0, got %d", got)
	}

	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/proxmox/tasks/pve01/UPID-1/unknown", nil)
	rec := httptest.NewRecorder()
	sut.handleProxmoxTaskRoutes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown task route, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/proxmox/nodes/pve01/unknown", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxNodeRoutes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown node route, got %d", rec.Code)
	}
}

func TestHandleProxmoxAssetsGuards(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/proxmox/assets/", nil)
	rec := httptest.NewRecorder()
	sut.handleProxmoxAssets(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing asset path, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/proxmox/assets/asset-1/details", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxAssets(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for non-GET, got %d", rec.Code)
	}

	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "agent-host-01",
		Type:    "server",
		Name:    "agent-host-01",
		Source:  "agent",
		Status:  "online",
	})
	if err != nil {
		t.Fatalf("failed to seed non-proxmox asset: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/proxmox/assets/agent-host-01/details", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxAssets(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-proxmox asset, got %d", rec.Code)
	}
}

func TestHandleProxmoxAssetsAdditionalErrorBranches(t *testing.T) {
	t.Run("path parsing and resolve errors", func(t *testing.T) {
		sut := newTestAPIServer(t)

		req := httptest.NewRequest(http.MethodGet, "/proxmox/assets//details", nil)
		rec := httptest.NewRecorder()
		sut.handleProxmoxAssets(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for empty asset id, got %d", rec.Code)
		}

		req = httptest.NewRequest(http.MethodGet, "/proxmox/assets/proxmox-node-pve01", nil)
		rec = httptest.NewRecorder()
		sut.handleProxmoxAssets(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for missing asset action, got %d", rec.Code)
		}

		sut.assetStore = &proxmoxAssetStoreWithErrors{
			inner:  sut.assetStore,
			getErr: errors.New("asset store unavailable"),
		}
		req = httptest.NewRequest(http.MethodGet, "/proxmox/assets/proxmox-node-pve01/details", nil)
		rec = httptest.NewRecorder()
		sut.handleProxmoxAssets(rec, req)
		if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "An internal error occurred.") {
			t.Fatalf("expected sanitized error 502, got %d body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
		}
	})

	t.Run("details and storage insights load failures", func(t *testing.T) {
		detailsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/version":
				_, _ = w.Write([]byte(`{"data":{"release":"8.3"}}`))
			case "/api2/json/nodes/pve01/status":
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte(`{"errors":"status failed"}`))
			case "/api2/json/nodes/pve01/tasks", "/api2/json/cluster/ha/resources", "/api2/json/nodes/pve01/firewall/rules", "/api2/json/cluster/backup":
				_, _ = w.Write([]byte(`{"data":[]}`))
			case "/api2/json/cluster/ceph/status", "/api2/json/cluster/ceph/osd":
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"errors":{"node":"no ceph"}}`))
			case "/api2/json/nodes/pve01/disks/zfs":
				_, _ = w.Write([]byte(`{"data":[]}`))
			default:
				t.Fatalf("unexpected proxmox path: %s", r.URL.Path)
			}
		}))
		defer detailsServer.Close()

		sut := newTestAPIServer(t)
		configureSingleProxmoxCollector(t, sut, detailsServer.URL, "collector-proxmox-1")

		_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: "proxmox-node-pve01",
			Type:    "hypervisor-node",
			Name:    "pve01",
			Source:  "proxmox",
			Status:  "online",
			Metadata: map[string]string{
				"proxmox_type": "node",
				"node":         "pve01",
				"collector_id": "collector-proxmox-1",
			},
		})
		if err != nil {
			t.Fatalf("failed to seed proxmox node asset: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/proxmox/assets/proxmox-node-pve01/details", nil)
		rec := httptest.NewRecorder()
		sut.handleProxmoxAssets(rec, req)
		if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "An internal error occurred.") {
			t.Fatalf("expected sanitized error 502, got %d body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
		}

		sut.assetStore = &proxmoxAssetStoreWithErrors{
			inner:   sut.assetStore,
			listErr: errors.New("list assets failed"),
		}
		req = httptest.NewRequest(http.MethodGet, "/proxmox/assets/proxmox-node-pve01/storage/insights", nil)
		rec = httptest.NewRecorder()
		sut.handleProxmoxAssets(rec, req)
		if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "An internal error occurred.") {
			t.Fatalf("expected sanitized error 502, got %d body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
		}
		// Restore the underlying store so global cleanup hooks keep working.
		sut.assetStore = sut.assetStore.(*proxmoxAssetStoreWithErrors).inner
	})
}

func TestFilterAndSortProxmoxTasksLimitAndTieBreak(t *testing.T) {
	tasks := []proxmox.Task{
		{UPID: "UPID:pve01:...:100:", Node: "pve01", ID: "100", StartTime: 200},
		{UPID: "UPID:pve01:...:101:", Node: "pve01", ID: "101", StartTime: 200},
		{UPID: "UPID:pve01:...:102:", Node: "", ID: "102", StartTime: 210},
		{UPID: "UPID:pve02:...:103:", Node: "pve02", ID: "103", StartTime: 300},
	}

	filtered := filterAndSortProxmoxTasks(tasks, "pve01", "", 2)
	if len(filtered) != 2 {
		t.Fatalf("expected limit to reduce results to 2, got %d", len(filtered))
	}
	if filtered[0].ID != "102" {
		t.Fatalf("expected task with empty node and highest time to remain first, got %+v", filtered[0])
	}
	if filtered[1].UPID <= "UPID:pve01:...:100:" {
		t.Fatalf("expected tiebreak sort to keep lexicographically larger UPID first, got %+v", filtered[1])
	}
}

func TestProxmoxTaskMatchesVMIDVariants(t *testing.T) {
	if !proxmoxpkg.ProxmoxTaskMatchesVMID(proxmox.Task{ID: "101"}, "00101") {
		t.Fatalf("expected normalized numeric vmid match to succeed")
	}
	if !proxmoxpkg.ProxmoxTaskMatchesVMID(proxmox.Task{ID: "qemu/101"}, "101") {
		t.Fatalf("expected vmid match in composite task id")
	}
	if !proxmoxpkg.ProxmoxTaskMatchesVMID(proxmox.Task{UPID: "UPID:pve01:...:vzdump:101:root@pam:"}, "101") {
		t.Fatalf("expected vmid match in upid")
	}
	if proxmoxpkg.ProxmoxTaskMatchesVMID(proxmox.Task{ID: "102", UPID: "UPID:pve01:...:102:"}, "101") {
		t.Fatalf("did not expect mismatched vmid to match")
	}
}

func TestProxmoxSnapshotTaskAndHAEdgeBranches(t *testing.T) {
	snapshots := sortProxmoxSnapshots([]proxmox.Snapshot{
		{Name: "z-snap", SnapTime: 50},
		{Name: "a-snap", SnapTime: 50},
	})
	if len(snapshots) != 2 || snapshots[0].Name != "a-snap" {
		t.Fatalf("expected same-time snapshot tie sort by name, got %+v", snapshots)
	}

	if !proxmoxpkg.ProxmoxTaskMatchesVMID(proxmox.Task{ID: "any"}, "   ") {
		t.Fatalf("expected empty VMID filter to match all tasks")
	}

	match, related := selectProxmoxHA([]proxmox.HAResource{
		{SID: "ct:202", Node: "pve01", State: "started"},
		{SID: "vm:202", Node: "pve01", State: "started"},
	}, proxmoxSessionTarget{Kind: "lxc", VMID: "202"})
	if match == nil || match.SID != "ct:202" || len(related) != 1 {
		t.Fatalf("expected LXC HA SID ct:202 selection, got match=%+v related=%+v", match, related)
	}
}

func configureDualProxmoxCollectors(t *testing.T, sut *apiServer, collectorOneURL, collectorTwoURL string) {
	t.Helper()

	createProxmoxCredentialProfile(
		t,
		sut,
		"cred-proxmox-collector-1",
		"labtether@pve!collector1",
		"token-secret-1",
		collectorOneURL,
	)
	createProxmoxCredentialProfile(
		t,
		sut,
		"cred-proxmox-collector-2",
		"labtether@pve!collector2",
		"token-secret-2",
		collectorTwoURL,
	)

	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{
			{
				ID:            "collector-proxmox-1",
				AssetID:       "proxmox-cluster-one",
				CollectorType: hubcollector.CollectorTypeProxmox,
				Enabled:       true,
				Config: map[string]any{
					"base_url":      collectorOneURL,
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
					"base_url":      collectorTwoURL,
					"token_id":      "labtether@pve!collector2",
					"credential_id": "cred-proxmox-collector-2",
					"skip_verify":   true,
				},
			},
		},
	}
}

func configureSingleProxmoxCollector(t *testing.T, sut *apiServer, collectorURL, collectorID string) {
	t.Helper()

	credentialID := "cred-" + collectorID
	createProxmoxCredentialProfile(
		t,
		sut,
		credentialID,
		"labtether@pve!agent",
		"token-secret",
		collectorURL,
	)

	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{
			{
				ID:            collectorID,
				AssetID:       "proxmox-cluster-test",
				CollectorType: hubcollector.CollectorTypeProxmox,
				Enabled:       true,
				Config: map[string]any{
					"base_url":      collectorURL,
					"token_id":      "labtether@pve!agent",
					"credential_id": credentialID,
					"skip_verify":   true,
				},
			},
		},
	}
}
