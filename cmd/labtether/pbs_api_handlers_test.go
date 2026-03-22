package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubcollector"
)

func TestHandlePBSAssetsGuards(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/pbs/assets/", nil)
	rec := httptest.NewRecorder()
	sut.handlePBSAssets(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing pbs asset path, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/pbs/assets/pbs-datastore-a/details", nil)
	rec = httptest.NewRecorder()
	sut.handlePBSAssets(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for non-GET method, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/pbs/assets/pbs-datastore-a/unknown", nil)
	rec = httptest.NewRecorder()
	sut.handlePBSAssets(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown pbs asset action, got %d", rec.Code)
	}
}

func TestHandlePBSAssetDetailsAndTaskRoutes(t *testing.T) {
	const upid = "UPID-TEST-1"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/version":
			_, _ = w.Write([]byte(`{"data":{"release":"3.4-1","version":"3.4"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/admin/datastore/backup/status":
			_, _ = w.Write([]byte(`{"data":{"store":"backup","total":1000,"used":250,"avail":750,"mount-status":"mounted"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/admin/datastore/backup/groups":
			_, _ = w.Write([]byte(`{"data":[{"backup-type":"vm","backup-id":"100"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/admin/datastore/backup/snapshots":
			_, _ = w.Write([]byte(`{"data":[{"backup-type":"vm","backup-id":"100","backup-time":1700000000}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/localhost/tasks":
			_, _ = w.Write([]byte(fmt.Sprintf(`{"data":[{"upid":"%s","node":"localhost","worker_type":"verify","worker_id":"backup","status":"running","starttime":1700000000}]}`, upid)))
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/localhost/tasks/"+upid+"/status":
			_, _ = w.Write([]byte(fmt.Sprintf(`{"data":{"upid":"%s","node":"localhost","status":"running","exitstatus":""}}`, upid)))
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/localhost/tasks/"+upid+"/log":
			_, _ = w.Write([]byte(`{"data":[{"n":1,"t":"task started"},{"n":2,"t":"task running"}]}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/nodes/localhost/tasks/"+upid:
			_, _ = w.Write([]byte(`{"data":null}`))
		default:
			t.Fatalf("unexpected pbs request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	sut := newTestAPIServer(t)
	createPBSCredentialProfile(t, sut, "cred-pbs-route-1", "root@pam!labtether", "secret-1", server.URL)
	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{
			{
				ID:            "collector-pbs-1",
				AssetID:       "pbs-server-test",
				CollectorType: hubcollector.CollectorTypePBS,
				Enabled:       true,
				Config: map[string]any{
					"base_url":      server.URL,
					"token_id":      "root@pam!labtether",
					"credential_id": "cred-pbs-route-1",
					"skip_verify":   true,
				},
			},
		},
	}

	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "pbs-datastore-backup",
		Type:    "storage-pool",
		Name:    "backup",
		Source:  "pbs",
		Status:  "online",
		Metadata: map[string]string{
			"store":        "backup",
			"collector_id": "collector-pbs-1",
			"node":         "localhost",
		},
	})
	if err != nil {
		t.Fatalf("seed pbs datastore asset: %v", err)
	}

	detailsReq := httptest.NewRequest(http.MethodGet, "/pbs/assets/pbs-datastore-backup/details", nil)
	detailsRec := httptest.NewRecorder()
	sut.handlePBSAssets(detailsRec, detailsReq)
	if detailsRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from pbs asset details, got %d body=%s", detailsRec.Code, detailsRec.Body.String())
	}
	if !strings.Contains(detailsRec.Body.String(), `"kind":"datastore"`) {
		t.Fatalf("expected datastore kind in details payload, got %s", detailsRec.Body.String())
	}
	if !strings.Contains(detailsRec.Body.String(), `"store":"backup"`) {
		t.Fatalf("expected store in details payload, got %s", detailsRec.Body.String())
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/pbs/tasks/localhost/"+upid+"/status", nil)
	statusRec := httptest.NewRecorder()
	sut.handlePBSTaskRoutes(statusRec, statusReq)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from pbs task status, got %d body=%s", statusRec.Code, statusRec.Body.String())
	}
	if !strings.Contains(statusRec.Body.String(), `"task"`) {
		t.Fatalf("expected task wrapper in status response, got %s", statusRec.Body.String())
	}

	logReq := httptest.NewRequest(http.MethodGet, "/pbs/tasks/localhost/"+upid+"/log?limit=50", nil)
	logRec := httptest.NewRecorder()
	sut.handlePBSTaskRoutes(logRec, logReq)
	if logRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from pbs task log, got %d body=%s", logRec.Code, logRec.Body.String())
	}
	if !strings.Contains(logRec.Body.String(), `"task started"`) {
		t.Fatalf("expected task log lines in response, got %s", logRec.Body.String())
	}

	stopReq := httptest.NewRequest(http.MethodPost, "/pbs/tasks/localhost/"+upid+"/stop", nil)
	stopReq = stopReq.WithContext(contextWithPrincipal(stopReq.Context(), "owner", "owner"))
	stopRec := httptest.NewRecorder()
	sut.handlePBSTaskRoutes(stopRec, stopReq)
	if stopRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from pbs task stop, got %d body=%s", stopRec.Code, stopRec.Body.String())
	}
	if !strings.Contains(stopRec.Body.String(), `"status":"stopped"`) {
		t.Fatalf("expected stopped status in stop response, got %s", stopRec.Body.String())
	}
}

func TestHandlePBSAssetSnapshots(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/admin/datastore/backup/snapshots":
			_, _ = w.Write([]byte(`{"data":[
				{"backup-type":"vm","backup-id":"100","backup-time":1700000300,"size":1024,"owner":"root@pam","protected":true,"verification":{"state":"ok","upid":"UPID-1"}},
				{"backup-type":"vm","backup-id":"200","backup-time":1700000200,"comment":"nightly"},
				{"backup-type":"ct","backup-id":"101","backup-time":1700000100}
			]}`))
		default:
			t.Fatalf("unexpected pbs request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	sut := newTestAPIServer(t)
	createPBSCredentialProfile(t, sut, "cred-pbs-snaps-1", "root@pam!labtether", "secret-snaps-1", server.URL)
	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{
			{
				ID:            "collector-pbs-snaps-1",
				AssetID:       "pbs-server-snaps",
				CollectorType: hubcollector.CollectorTypePBS,
				Enabled:       true,
				Config: map[string]any{
					"base_url":      server.URL,
					"token_id":      "root@pam!labtether",
					"credential_id": "cred-pbs-snaps-1",
					"skip_verify":   true,
				},
			},
		},
	}
	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "pbs-datastore-backup",
		Type:    "storage-pool",
		Name:    "backup",
		Source:  "pbs",
		Status:  "online",
		Metadata: map[string]string{
			"store":        "backup",
			"collector_id": "collector-pbs-snaps-1",
			"node":         "localhost",
		},
	})
	if err != nil {
		t.Fatalf("seed pbs datastore asset: %v", err)
	}

	t.Run("all snapshots no filter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/pbs/assets/pbs-datastore-backup/snapshots", nil)
		rec := httptest.NewRecorder()
		sut.handlePBSAssets(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 from pbs asset snapshots, got %d body=%s", rec.Code, rec.Body.String())
		}

		var response pbsSnapshotsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to decode snapshots response: %v", err)
		}
		if response.Store != "backup" {
			t.Fatalf("expected store=backup, got %q", response.Store)
		}
		if len(response.Snapshots) != 3 {
			t.Fatalf("expected 3 snapshots, got %d", len(response.Snapshots))
		}
		// Verify descending sort by backup_time.
		if response.Snapshots[0].BackupTime < response.Snapshots[1].BackupTime {
			t.Fatalf("expected snapshots sorted descending by backup_time, got %+v", response.Snapshots)
		}
		// Verify first entry has expected fields.
		first := response.Snapshots[0]
		if first.BackupType != "vm" || first.BackupID != "100" {
			t.Fatalf("unexpected first snapshot: %+v", first)
		}
		if first.Size != 1024 {
			t.Fatalf("expected size=1024, got %d", first.Size)
		}
		if !first.Protected {
			t.Fatalf("expected protected=true for first snapshot")
		}
		if first.Owner != "root@pam" {
			t.Fatalf("expected owner=root@pam, got %q", first.Owner)
		}
		if first.Verification == nil || first.Verification.State != "ok" {
			t.Fatalf("expected verification.state=ok for first snapshot, got %+v", first.Verification)
		}
		if response.FetchedAt == "" {
			t.Fatalf("expected fetched_at to be set")
		}
	})

	t.Run("filter by type=vm", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/pbs/assets/pbs-datastore-backup/snapshots?type=vm", nil)
		rec := httptest.NewRecorder()
		sut.handlePBSAssets(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}

		var response pbsSnapshotsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to decode snapshots response: %v", err)
		}
		if len(response.Snapshots) != 2 {
			t.Fatalf("expected 2 vm snapshots after type filter, got %d", len(response.Snapshots))
		}
		for _, snap := range response.Snapshots {
			if snap.BackupType != "vm" {
				t.Fatalf("expected only vm snapshots, got backup_type=%q", snap.BackupType)
			}
		}
	})

	t.Run("filter by type=vm and id=100", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/pbs/assets/pbs-datastore-backup/snapshots?type=vm&id=100", nil)
		rec := httptest.NewRecorder()
		sut.handlePBSAssets(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}

		var response pbsSnapshotsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to decode snapshots response: %v", err)
		}
		if len(response.Snapshots) != 1 {
			t.Fatalf("expected 1 snapshot after type+id filter, got %d", len(response.Snapshots))
		}
		if response.Snapshots[0].BackupID != "100" {
			t.Fatalf("expected backup_id=100, got %q", response.Snapshots[0].BackupID)
		}
	})

	t.Run("server-kind asset missing store returns 400", func(t *testing.T) {
		_, seedErr := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: "pbs-server-nostore",
			Type:    "storage-controller",
			Name:    "pbs-server",
			Source:  "pbs",
			Status:  "online",
			Metadata: map[string]string{
				"collector_id": "collector-pbs-snaps-1",
				"node":         "localhost",
			},
		})
		if seedErr != nil {
			t.Fatalf("seed pbs server asset: %v", seedErr)
		}
		req := httptest.NewRequest(http.MethodGet, "/pbs/assets/pbs-server-nostore/snapshots", nil)
		rec := httptest.NewRecorder()
		sut.handlePBSAssets(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for server-kind without store param, got %d body=%s", rec.Code, rec.Body.String())
		}
	})
}

func TestHandlePBSAssetVerification(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/admin/datastore/backup/snapshots":
			_, _ = w.Write([]byte(`{"data":[
				{"backup-type":"vm","backup-id":"100","backup-time":1700000300,"verification":{"state":"ok","upid":"UPID-V1"}},
				{"backup-type":"vm","backup-id":"200","backup-time":1700000200,"verification":{"state":"failed","upid":"UPID-V2"}},
				{"backup-type":"ct","backup-id":"101","backup-time":1700000100}
			]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/localhost/tasks":
			_, _ = w.Write([]byte(`{"data":[]}`))
		default:
			t.Fatalf("unexpected pbs request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	sut := newTestAPIServer(t)
	createPBSCredentialProfile(t, sut, "cred-pbs-verify-1", "root@pam!labtether", "secret-verify-1", server.URL)
	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{
			{
				ID:            "collector-pbs-verify-1",
				AssetID:       "pbs-server-verify",
				CollectorType: hubcollector.CollectorTypePBS,
				Enabled:       true,
				Config: map[string]any{
					"base_url":      server.URL,
					"token_id":      "root@pam!labtether",
					"credential_id": "cred-pbs-verify-1",
					"skip_verify":   true,
				},
			},
		},
	}
	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "pbs-datastore-backup",
		Type:    "storage-pool",
		Name:    "backup",
		Source:  "pbs",
		Status:  "online",
		Metadata: map[string]string{
			"store":        "backup",
			"collector_id": "collector-pbs-verify-1",
			"node":         "localhost",
		},
	})
	if err != nil {
		t.Fatalf("seed pbs datastore asset: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/pbs/assets/pbs-datastore-backup/verification", nil)
	rec := httptest.NewRecorder()
	sut.handlePBSAssets(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from pbs asset verification, got %d body=%s", rec.Code, rec.Body.String())
	}

	var response pbsVerificationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode verification response: %v", err)
	}
	if len(response.Datastores) != 1 {
		t.Fatalf("expected 1 datastore in verification response, got %d", len(response.Datastores))
	}
	ds := response.Datastores[0]
	if ds.Store != "backup" {
		t.Fatalf("expected store=backup, got %q", ds.Store)
	}
	if ds.VerifiedCount != 1 {
		t.Fatalf("expected verified_count=1, got %d", ds.VerifiedCount)
	}
	if ds.FailedCount != 1 {
		t.Fatalf("expected failed_count=1, got %d", ds.FailedCount)
	}
	if ds.UnverifiedCount != 1 {
		t.Fatalf("expected unverified_count=1, got %d", ds.UnverifiedCount)
	}
	if ds.Status != "bad" {
		t.Fatalf("expected status=bad (failed snapshots present), got %q", ds.Status)
	}
	if response.FetchedAt == "" {
		t.Fatalf("expected fetched_at to be set in verification response")
	}
	if len(response.Warnings) != 0 {
		t.Fatalf("expected no warnings in clean verification response, got %v", response.Warnings)
	}
}

func TestHandlePBSAssetGroups(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/admin/datastore/backup/groups":
			_, _ = w.Write([]byte(`{"data":[{"backup-type":"vm","backup-id":"100","backup-count":5,"last-backup":1700000000,"owner":"root@pam"}]}`))
		default:
			t.Fatalf("unexpected pbs request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	sut := newTestAPIServer(t)
	createPBSCredentialProfile(t, sut, "cred-pbs-groups-1", "root@pam!labtether", "secret-groups-1", server.URL)
	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{
			{
				ID:            "collector-pbs-groups-1",
				AssetID:       "pbs-server-groups",
				CollectorType: hubcollector.CollectorTypePBS,
				Enabled:       true,
				Config: map[string]any{
					"base_url":      server.URL,
					"token_id":      "root@pam!labtether",
					"credential_id": "cred-pbs-groups-1",
					"skip_verify":   true,
				},
			},
		},
	}

	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "pbs-datastore-backup",
		Type:    "storage-pool",
		Name:    "backup",
		Source:  "pbs",
		Status:  "online",
		Metadata: map[string]string{
			"store":        "backup",
			"collector_id": "collector-pbs-groups-1",
			"node":         "localhost",
		},
	})
	if err != nil {
		t.Fatalf("seed pbs datastore asset: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/pbs/assets/pbs-datastore-backup/groups", nil)
	rec := httptest.NewRecorder()
	sut.handlePBSAssets(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from pbs asset groups, got %d body=%s", rec.Code, rec.Body.String())
	}

	var response pbsGroupsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode groups response: %v", err)
	}
	if len(response.Datastores) != 1 {
		t.Fatalf("expected 1 datastore in groups response, got %d", len(response.Datastores))
	}
	if response.Datastores[0].Store != "backup" {
		t.Fatalf("expected store=backup in groups response, got %q", response.Datastores[0].Store)
	}
	if len(response.Datastores[0].Groups) != 1 {
		t.Fatalf("expected 1 backup group, got %d", len(response.Datastores[0].Groups))
	}
	group := response.Datastores[0].Groups[0]
	if group.BackupType != "vm" {
		t.Fatalf("expected backup_type=vm, got %q", group.BackupType)
	}
	if group.BackupID != "100" {
		t.Fatalf("expected backup_id=100, got %q", group.BackupID)
	}
	if group.BackupCount != 5 {
		t.Fatalf("expected backup_count=5, got %d", group.BackupCount)
	}
	if group.LastBackup != 1700000000 {
		t.Fatalf("expected last_backup=1700000000, got %d", group.LastBackup)
	}
	if group.Owner != "root@pam" {
		t.Fatalf("expected owner=root@pam, got %q", group.Owner)
	}
	if response.FetchedAt == "" {
		t.Fatalf("expected fetched_at to be set in groups response")
	}
	if len(response.Warnings) != 0 {
		t.Fatalf("expected no warnings in clean groups response, got %v", response.Warnings)
	}
}
