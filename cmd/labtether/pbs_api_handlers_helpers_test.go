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

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectors/pbs"
	"github.com/labtether/labtether/internal/hubcollector"
)

func TestHandlePBSTaskRoutesAndHandlerGuards(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/pbs/tasks/", nil)
	rec := httptest.NewRecorder()
	sut.handlePBSTaskRoutes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing task path, got %d", rec.Code)
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "missing task path")

	req = httptest.NewRequest(http.MethodGet, "/pbs/tasks/node/upid/unknown", nil)
	rec = httptest.NewRecorder()
	sut.handlePBSTaskRoutes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown task action, got %d", rec.Code)
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "unknown pbs task action")

	req = httptest.NewRequest(http.MethodPost, "/pbs/tasks/node/upid/status", nil)
	rec = httptest.NewRecorder()
	sut.handlePBSTaskStatus(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for status method guard, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/pbs/tasks/node/upid/not-status", nil)
	rec = httptest.NewRecorder()
	sut.handlePBSTaskStatus(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for invalid status path, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/pbs/tasks//upid/status", nil)
	rec = httptest.NewRecorder()
	sut.handlePBSTaskStatus(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty status node, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/pbs/tasks/node//status", nil)
	rec = httptest.NewRecorder()
	sut.handlePBSTaskStatus(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty status upid, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/pbs/tasks/node/upid/status", nil)
	rec = httptest.NewRecorder()
	sut.handlePBSTaskStatus(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 for status runtime unavailable, got %d", rec.Code)
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")

	req = httptest.NewRequest(http.MethodPost, "/pbs/tasks/node/upid/log", nil)
	rec = httptest.NewRecorder()
	sut.handlePBSTaskLog(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for log method guard, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/pbs/tasks/node/upid/not-log", nil)
	rec = httptest.NewRecorder()
	sut.handlePBSTaskLog(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for invalid log path, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/pbs/tasks//upid/log", nil)
	rec = httptest.NewRecorder()
	sut.handlePBSTaskLog(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty log node, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/pbs/tasks/node/upid/log?limit=abc", nil)
	rec = httptest.NewRecorder()
	sut.handlePBSTaskLog(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 for log runtime unavailable, got %d", rec.Code)
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")

	req = httptest.NewRequest(http.MethodGet, "/pbs/tasks/node/upid/stop", nil)
	rec = httptest.NewRecorder()
	sut.handlePBSTaskStop(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for stop method guard, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/pbs/tasks/node/upid/not-stop", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "owner", "owner"))
	rec = httptest.NewRecorder()
	sut.handlePBSTaskStop(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for invalid stop path, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/pbs/tasks//upid/stop", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "owner", "owner"))
	rec = httptest.NewRecorder()
	sut.handlePBSTaskStop(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty stop node, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/pbs/tasks/node/upid/stop", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "owner", "owner"))
	rec = httptest.NewRecorder()
	sut.handlePBSTaskStop(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 for stop runtime unavailable, got %d", rec.Code)
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")
}

func TestHandlePBSTaskHandlersUpstreamErrorsAndLogLimits(t *testing.T) {
	const collectorID = "collector-pbs-task-errors"
	const credentialID = "cred-pbs-task-errors"
	const tokenID = "root@pam!task-errors"
	const upid = "UPID-TASK-ERR-1"

	t.Run("status upstream failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/node-a/tasks/"+upid+"/status" {
				http.Error(w, `{"errors":"status failed"}`, http.StatusBadGateway)
				return
			}
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}))
		defer server.Close()

		sut := newTestAPIServer(t)
		configurePBSTaskRuntime(t, sut, collectorID, credentialID, tokenID, server.URL)

		req := httptest.NewRequest(http.MethodGet, "/pbs/tasks/node-a/"+upid+"/status?collector_id="+collectorID, nil)
		rec := httptest.NewRecorder()
		sut.handlePBSTaskStatus(rec, req)
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected 502 for upstream status error, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")
	})

	t.Run("log upstream failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/node-a/tasks/"+upid+"/log" {
				http.Error(w, `{"errors":"log failed"}`, http.StatusBadGateway)
				return
			}
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}))
		defer server.Close()

		sut := newTestAPIServer(t)
		configurePBSTaskRuntime(t, sut, collectorID, credentialID, tokenID, server.URL)

		req := httptest.NewRequest(http.MethodGet, "/pbs/tasks/node-a/"+upid+"/log?collector_id="+collectorID, nil)
		rec := httptest.NewRecorder()
		sut.handlePBSTaskLog(rec, req)
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected 502 for upstream log error, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")
	})

	t.Run("stop upstream failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodDelete && r.URL.Path == "/api2/json/nodes/node-a/tasks/"+upid {
				http.Error(w, `{"errors":"stop failed"}`, http.StatusBadGateway)
				return
			}
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}))
		defer server.Close()

		sut := newTestAPIServer(t)
		configurePBSTaskRuntime(t, sut, collectorID, credentialID, tokenID, server.URL)

		req := httptest.NewRequest(http.MethodPost, "/pbs/tasks/node-a/"+upid+"/stop?collector_id="+collectorID, nil)
		req = req.WithContext(contextWithPrincipal(req.Context(), "owner", "owner"))
		rec := httptest.NewRecorder()
		sut.handlePBSTaskStop(rec, req)
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected 502 for upstream stop error, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")
	})

	t.Run("log limit clamp to 2000", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/node-a/tasks/"+upid+"/log" {
				if got := r.URL.Query().Get("limit"); got != "2000" {
					t.Fatalf("expected log limit=2000, got %q", got)
				}
				_, _ = w.Write([]byte(`{"data":[{"n":1,"t":"clamped"}]}`))
				return
			}
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}))
		defer server.Close()

		sut := newTestAPIServer(t)
		configurePBSTaskRuntime(t, sut, collectorID, credentialID, tokenID, server.URL)

		req := httptest.NewRequest(http.MethodGet, "/pbs/tasks/node-a/"+upid+"/log?collector_id="+collectorID+"&limit=9999", nil)
		rec := httptest.NewRecorder()
		sut.handlePBSTaskLog(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "clamped") {
			t.Fatalf("unexpected log payload: %s", rec.Body.String())
		}
	})

	t.Run("log invalid limit falls back to default 200", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/node-a/tasks/"+upid+"/log" {
				if got := r.URL.Query().Get("limit"); got != "200" {
					t.Fatalf("expected default log limit=200, got %q", got)
				}
				_, _ = w.Write([]byte(`{"data":[{"n":1,"t":"default-limit"}]}`))
				return
			}
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}))
		defer server.Close()

		sut := newTestAPIServer(t)
		configurePBSTaskRuntime(t, sut, collectorID, credentialID, tokenID, server.URL)

		req := httptest.NewRequest(http.MethodGet, "/pbs/tasks/node-a/"+upid+"/log?collector_id="+collectorID+"&limit=abc", nil)
		rec := httptest.NewRecorder()
		sut.handlePBSTaskLog(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "default-limit") {
			t.Fatalf("unexpected log payload: %s", rec.Body.String())
		}
	})
}

func TestHandlePBSTaskHandlersRequireCollectorWhenMultipleCollectors(t *testing.T) {
	const upid = "UPID-MULTI-1"
	var collectorOneCalls atomic.Int32
	var collectorTwoCalls atomic.Int32

	collectorOne := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		collectorOneCalls.Add(1)
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	defer collectorOne.Close()

	collectorTwo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		collectorTwoCalls.Add(1)
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	defer collectorTwo.Close()

	sut := newTestAPIServer(t)
	createPBSCredentialProfile(t, sut, "cred-pbs-multi-1", "root@pam!multi-1", "secret-1", collectorOne.URL)
	createPBSCredentialProfile(t, sut, "cred-pbs-multi-2", "root@pam!multi-2", "secret-2", collectorTwo.URL)
	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{
			{
				ID:            "collector-pbs-multi-1",
				CollectorType: hubcollector.CollectorTypePBS,
				Enabled:       true,
				Config: map[string]any{
					"base_url":      collectorOne.URL,
					"credential_id": "cred-pbs-multi-1",
					"token_id":      "root@pam!multi-1",
					"skip_verify":   true,
				},
			},
			{
				ID:            "collector-pbs-multi-2",
				CollectorType: hubcollector.CollectorTypePBS,
				Enabled:       true,
				Config: map[string]any{
					"base_url":      collectorTwo.URL,
					"credential_id": "cred-pbs-multi-2",
					"token_id":      "root@pam!multi-2",
					"skip_verify":   true,
				},
			},
		},
	}

	tests := []struct {
		name    string
		method  string
		path    string
		handler func(http.ResponseWriter, *http.Request)
	}{
		{
			name:    "status",
			method:  http.MethodGet,
			path:    "/pbs/tasks/node-a/" + upid + "/status",
			handler: sut.handlePBSTaskStatus,
		},
		{
			name:    "log",
			method:  http.MethodGet,
			path:    "/pbs/tasks/node-a/" + upid + "/log",
			handler: sut.handlePBSTaskLog,
		},
		{
			name:    "stop",
			method:  http.MethodPost,
			path:    "/pbs/tasks/node-a/" + upid + "/stop",
			handler: sut.handlePBSTaskStop,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.method == http.MethodPost {
				req = req.WithContext(contextWithPrincipal(req.Context(), "owner", "owner"))
			}
			rec := httptest.NewRecorder()
			tc.handler(rec, req)
			if rec.Code != http.StatusBadGateway {
				t.Fatalf("expected 502 when collector_id missing under multi-collector setup, got %d body=%s", rec.Code, rec.Body.String())
			}
			assertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")
		})
	}

	if collectorOneCalls.Load() != 0 {
		t.Fatalf("expected collector one to receive no upstream requests, got %d", collectorOneCalls.Load())
	}
	if collectorTwoCalls.Load() != 0 {
		t.Fatalf("expected collector two to receive no upstream requests, got %d", collectorTwoCalls.Load())
	}
}

func TestHandlePBSAssetsGuardAndErrorBranches(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/pbs/assets//details", nil)
	rec := httptest.NewRecorder()
	sut.handlePBSAssets(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for empty asset id, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/pbs/assets/missing/details", nil)
	rec = httptest.NewRecorder()
	sut.handlePBSAssets(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing pbs asset, got %d", rec.Code)
	}

	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "pbs-datastore-runtime-missing",
		Type:    "storage-pool",
		Name:    "backup",
		Source:  "pbs",
		Status:  "online",
		Metadata: map[string]string{
			"store": "backup",
		},
	}); err != nil {
		t.Fatalf("seed pbs datastore asset: %v", err)
	}
	req = httptest.NewRequest(http.MethodGet, "/pbs/assets/pbs-datastore-runtime-missing/details", nil)
	rec = httptest.NewRecorder()
	sut.handlePBSAssets(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when runtime missing, got %d", rec.Code)
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/version":
			_, _ = w.Write([]byte(`{"data":{"release":"3.2-1"}}`))
		case "/api2/json/admin/datastore/backup/status":
			http.Error(w, `{"errors":"status failed"}`, http.StatusBadGateway)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	sut = newTestAPIServer(t)
	createPBSCredentialProfile(t, sut, "cred-pbs-asset-error", "root@pam!asset", "secret-asset", server.URL)
	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{
			{
				ID:            "collector-pbs-asset-error",
				AssetID:       "pbs-server-asset-error",
				CollectorType: hubcollector.CollectorTypePBS,
				Enabled:       true,
				Config: map[string]any{
					"base_url":      server.URL,
					"credential_id": "cred-pbs-asset-error",
					"token_id":      "root@pam!asset",
					"skip_verify":   true,
				},
			},
		},
	}
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "pbs-datastore-backup",
		Type:    "storage-pool",
		Name:    "backup",
		Source:  "pbs",
		Status:  "online",
		Metadata: map[string]string{
			"store":        "backup",
			"collector_id": "collector-pbs-asset-error",
		},
	}); err != nil {
		t.Fatalf("seed pbs datastore asset: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/pbs/assets/pbs-datastore-backup/details", nil)
	rec = httptest.NewRecorder()
	sut.handlePBSAssets(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when detail load fails, got %d", rec.Code)
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")
}

func TestLoadPBSAssetDetailsDatastoreAndServerBranches(t *testing.T) {
	t.Run("datastore asset missing store metadata", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api2/json/version" {
				_, _ = w.Write([]byte(`{"data":{"release":"3.2-1"}}`))
				return
			}
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}))
		defer server.Close()

		runtime := &pbsRuntime{
			Client:      mustNewPBSClient(t, server.URL),
			CollectorID: "collector-pbs-metadata-missing",
		}
		_, err := newTestAPIServer(t).loadPBSAssetDetails(context.Background(), assets.Asset{
			ID:     "pbs-server-a",
			Type:   "storage-pool",
			Source: "pbs",
		}, runtime)
		if err == nil || !strings.Contains(err.Error(), "missing store metadata") {
			t.Fatalf("expected missing store metadata error, got %v", err)
		}
	})

	t.Run("datastore details with version fallback and filtered tasks", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/version":
				_, _ = w.Write([]byte(`{"data":{"release":"","version":"3.2"}}`))
			case "/api2/json/admin/datastore/backup/status":
				_, _ = w.Write([]byte(`{"data":{"store":"backup","total":1000,"used":250,"avail":750,"mount-status":"mounted"}}`))
			case "/api2/json/admin/datastore/backup/groups":
				_, _ = w.Write([]byte(`{"data":[{"backup-type":"vm","backup-id":"100"}]}`))
			case "/api2/json/admin/datastore/backup/snapshots":
				_, _ = w.Write([]byte(fmt.Sprintf(`{"data":[{"backup-type":"vm","backup-id":"100","backup-time":%d}]}`, time.Now().Unix()-300)))
			case "/api2/json/nodes/node-a/tasks":
				_, _ = w.Write([]byte(`{"data":[
					{"upid":"UPID:1:backup:","node":"node-a","worker_type":"verify","worker_id":"backup:vm/100","starttime":20},
					{"upid":"UPID:2:other:","node":"node-a","worker_type":"verify","worker_id":"other:vm/101","starttime":30},
					{"upid":"UPID:3:backup:","node":"node-a","worker_type":"gc","worker_id":"","starttime":10}
				]}`))
			default:
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
		}))
		defer server.Close()

		runtime := &pbsRuntime{
			Client:      mustNewPBSClient(t, server.URL),
			CollectorID: "collector-pbs-datastore-success",
		}
		response, err := newTestAPIServer(t).loadPBSAssetDetails(context.Background(), assets.Asset{
			ID:     "pbs-datastore-backup",
			Type:   "storage-pool",
			Source: "pbs",
			Metadata: map[string]string{
				"store": "backup",
				"node":  "node-a",
			},
		}, runtime)
		if err != nil {
			t.Fatalf("loadPBSAssetDetails() error = %v", err)
		}
		if response.Kind != "datastore" || response.Store != "backup" {
			t.Fatalf("unexpected datastore response kind/store: %+v", response)
		}
		if response.Version != "3.2" {
			t.Fatalf("expected version fallback to 3.2, got %q", response.Version)
		}
		if len(response.Tasks) != 2 {
			t.Fatalf("expected filtered tasks=2, got %d", len(response.Tasks))
		}
	})

	t.Run("datastore warnings when version and tasks unavailable", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/version":
				http.Error(w, `{"errors":"version failed"}`, http.StatusBadGateway)
			case "/api2/json/admin/datastore/backup/status":
				_, _ = w.Write([]byte(`{"data":{"store":"backup","total":1000,"used":250,"avail":750,"mount-status":"mounted"}}`))
			case "/api2/json/admin/datastore/backup/groups":
				_, _ = w.Write([]byte(`{"data":[]}`))
			case "/api2/json/admin/datastore/backup/snapshots":
				_, _ = w.Write([]byte(`{"data":[]}`))
			case "/api2/json/nodes/localhost/tasks":
				http.Error(w, `{"errors":"tasks failed"}`, http.StatusBadGateway)
			default:
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
		}))
		defer server.Close()

		runtime := &pbsRuntime{
			Client:      mustNewPBSClient(t, server.URL),
			CollectorID: "collector-pbs-datastore-warnings",
		}
		response, err := newTestAPIServer(t).loadPBSAssetDetails(context.Background(), assets.Asset{
			ID:     "pbs-datastore-backup",
			Type:   "storage-pool",
			Source: "pbs",
			Metadata: map[string]string{
				"store": "backup",
			},
		}, runtime)
		if err != nil {
			t.Fatalf("loadPBSAssetDetails() error = %v", err)
		}
		if len(response.Warnings) == 0 {
			t.Fatalf("expected warnings in datastore response")
		}
		if !strings.Contains(strings.Join(response.Warnings, " | "), "version unavailable") {
			t.Fatalf("expected version warning, got %v", response.Warnings)
		}
		if !strings.Contains(strings.Join(response.Warnings, " | "), "task listing unavailable") {
			t.Fatalf("expected task warning, got %v", response.Warnings)
		}
	})

	t.Run("server details usage-summary and task warnings", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/version":
				_, _ = w.Write([]byte(`{"data":{"release":"3.4-1","version":"3.4"}}`))
			case "/api2/json/status/datastore-usage":
				http.Error(w, `{"errors":"usage failed"}`, http.StatusBadGateway)
			case "/api2/json/admin/datastore":
				_, _ = w.Write([]byte(`{"data":[
					{"store":"","comment":"skip"},
					{"store":"bad","comment":"bad-comment"},
					{"store":"good","comment":"good-comment","mount-status":"mounted","maintenance":"read-only"}
				]}`))
			case "/api2/json/admin/datastore/bad/status":
				http.Error(w, `{"errors":"status failed"}`, http.StatusBadGateway)
			case "/api2/json/admin/datastore/good/status":
				_, _ = w.Write([]byte(`{"data":{"store":"good","total":1000,"used":100,"avail":900,"mount-status":""}}`))
			case "/api2/json/admin/datastore/good/groups":
				_, _ = w.Write([]byte(`{"data":[{"backup-type":"vm","backup-id":"100"}]}`))
			case "/api2/json/admin/datastore/good/snapshots":
				_, _ = w.Write([]byte(`{"data":[]}`))
			case "/api2/json/nodes/localhost/tasks":
				http.Error(w, `{"errors":"tasks failed"}`, http.StatusBadGateway)
			default:
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
		}))
		defer server.Close()

		runtime := &pbsRuntime{
			Client:      mustNewPBSClient(t, server.URL),
			CollectorID: "collector-pbs-server-warnings",
		}
		response, err := newTestAPIServer(t).loadPBSAssetDetails(context.Background(), assets.Asset{
			ID:     "pbs-server-main",
			Type:   "storage-controller",
			Source: "pbs",
		}, runtime)
		if err != nil {
			t.Fatalf("loadPBSAssetDetails() error = %v", err)
		}
		if response.Kind != "server" {
			t.Fatalf("expected server kind, got %q", response.Kind)
		}
		if len(response.Datastores) != 1 {
			t.Fatalf("expected one usable datastore summary, got %d", len(response.Datastores))
		}
		summary := response.Datastores[0]
		if summary.Store != "good" {
			t.Fatalf("expected datastore summary for store=good, got %q", summary.Store)
		}
		if summary.Comment != "good-comment" {
			t.Fatalf("expected comment fallback from datastore listing, got %q", summary.Comment)
		}
		if summary.MountStatus != "mounted" {
			t.Fatalf("expected mount-status fallback from datastore listing, got %q", summary.MountStatus)
		}
		if summary.Maintenance != "read-only" {
			t.Fatalf("expected maintenance fallback from datastore listing, got %q", summary.Maintenance)
		}
		combined := strings.Join(response.Warnings, " | ")
		if !strings.Contains(combined, "datastore usage unavailable") ||
			!strings.Contains(combined, "datastore bad unavailable") ||
			!strings.Contains(combined, "task listing unavailable") {
			t.Fatalf("unexpected server warnings: %v", response.Warnings)
		}
	})

	t.Run("server details usage-success sorted summaries and tasks", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/version":
				_, _ = w.Write([]byte(`{"data":{"release":"3.4-2","version":"3.4"}}`))
			case "/api2/json/status/datastore-usage":
				_, _ = w.Write([]byte(`{"data":[
					{"store":" beta ","total":1000,"used":400,"avail":600},
					{"store":"alpha","total":2000,"used":500,"avail":1500},
					{"store":"","total":1,"used":1,"avail":0}
				]}`))
			case "/api2/json/admin/datastore":
				_, _ = w.Write([]byte(`{"data":[
					{"store":"beta","comment":"store-beta","mount-status":"mounted"},
					{"store":"alpha","comment":"store-alpha","mount-status":"mounted"}
				]}`))
			case "/api2/json/admin/datastore/alpha/status":
				_, _ = w.Write([]byte(`{"data":{"store":"alpha","total":2000,"used":500,"avail":1500,"mount-status":"mounted"}}`))
			case "/api2/json/admin/datastore/alpha/groups":
				_, _ = w.Write([]byte(`{"data":[]}`))
			case "/api2/json/admin/datastore/alpha/snapshots":
				_, _ = w.Write([]byte(`{"data":[]}`))
			case "/api2/json/admin/datastore/beta/status":
				_, _ = w.Write([]byte(`{"data":{"store":"beta","total":1000,"used":400,"avail":600,"mount-status":"mounted"}}`))
			case "/api2/json/admin/datastore/beta/groups":
				_, _ = w.Write([]byte(`{"data":[]}`))
			case "/api2/json/admin/datastore/beta/snapshots":
				_, _ = w.Write([]byte(`{"data":[]}`))
			case "/api2/json/nodes/localhost/tasks":
				_, _ = w.Write([]byte(`{"data":[
					{"upid":"UPID-2","node":"localhost","worker_type":"verify","worker_id":"beta","starttime":20},
					{"upid":"UPID-1","node":"localhost","worker_type":"verify","worker_id":"alpha","starttime":10}
				]}`))
			default:
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
		}))
		defer server.Close()

		runtime := &pbsRuntime{
			Client:      mustNewPBSClient(t, server.URL),
			CollectorID: "collector-pbs-server-success",
		}
		response, err := newTestAPIServer(t).loadPBSAssetDetails(context.Background(), assets.Asset{
			ID:     "pbs-server-main",
			Type:   "storage-controller",
			Source: "pbs",
		}, runtime)
		if err != nil {
			t.Fatalf("loadPBSAssetDetails() error = %v", err)
		}
		if len(response.Datastores) != 2 {
			t.Fatalf("expected two datastore summaries, got %d", len(response.Datastores))
		}
		if response.Datastores[0].Store != "alpha" || response.Datastores[1].Store != "beta" {
			t.Fatalf("expected sorted datastore order [alpha,beta], got %+v", response.Datastores)
		}
		if len(response.Tasks) != 2 {
			t.Fatalf("expected server tasks to be included, got %d", len(response.Tasks))
		}
	})

	t.Run("server details fail when datastores list fails", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/version":
				_, _ = w.Write([]byte(`{"data":{"release":"3.4-1"}}`))
			case "/api2/json/status/datastore-usage":
				_, _ = w.Write([]byte(`{"data":[]}`))
			case "/api2/json/admin/datastore":
				http.Error(w, `{"errors":"list failed"}`, http.StatusBadGateway)
			default:
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
		}))
		defer server.Close()

		runtime := &pbsRuntime{
			Client:      mustNewPBSClient(t, server.URL),
			CollectorID: "collector-pbs-server-error",
		}
		_, err := newTestAPIServer(t).loadPBSAssetDetails(context.Background(), assets.Asset{
			ID:     "pbs-server-main",
			Type:   "storage-controller",
			Source: "pbs",
		}, runtime)
		if err == nil {
			t.Fatalf("expected datastores list error")
		}
	})
}

func TestLoadPBSDatastoreSummaryBranches(t *testing.T) {
	t.Run("status fetch error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api2/json/admin/datastore/backup/status" {
				http.Error(w, `{"errors":"status failed"}`, http.StatusBadGateway)
				return
			}
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}))
		defer server.Close()

		_, _, err := loadPBSDatastoreSummary(context.Background(), mustNewPBSClient(t, server.URL), "backup", pbs.DatastoreUsage{})
		if err == nil {
			t.Fatalf("expected datastore status error")
		}
	})

	t.Run("usage fallback with groups/snapshots warnings", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/admin/datastore/backup/status":
				_, _ = w.Write([]byte(`{"data":{"store":"backup","total":0,"used":0,"avail":0,"mount-status":"mounted"}}`))
			case "/api2/json/admin/datastore/backup/groups":
				http.Error(w, `{"errors":"groups failed"}`, http.StatusBadGateway)
			case "/api2/json/admin/datastore/backup/snapshots":
				http.Error(w, `{"errors":"snapshots failed"}`, http.StatusBadGateway)
			default:
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
		}))
		defer server.Close()

		summary, warnings, err := loadPBSDatastoreSummary(
			context.Background(),
			mustNewPBSClient(t, server.URL),
			"backup",
			pbs.DatastoreUsage{Store: "backup", Total: 200, Used: 20, Avail: 180},
		)
		if err != nil {
			t.Fatalf("loadPBSDatastoreSummary() error = %v", err)
		}
		if summary.TotalBytes != 200 || summary.UsedBytes != 20 || summary.AvailBytes != 180 {
			t.Fatalf("expected usage fallback totals, got %+v", summary)
		}
		if summary.UsagePercent != 10 {
			t.Fatalf("expected usage percent 10, got %v", summary.UsagePercent)
		}
		if len(warnings) != 2 {
			t.Fatalf("expected two warnings, got %d (%v)", len(warnings), warnings)
		}
	})

	t.Run("usage clamp and future backup day clamp", func(t *testing.T) {
		future := time.Now().UTC().Add(2 * time.Hour).Unix()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/admin/datastore/backup/status":
				_, _ = w.Write([]byte(`{"data":{"store":"backup","total":100,"used":150,"avail":0,"mount-status":"mounted"}}`))
			case "/api2/json/admin/datastore/backup/groups":
				_, _ = w.Write([]byte(`{"data":[]}`))
			case "/api2/json/admin/datastore/backup/snapshots":
				_, _ = w.Write([]byte(fmt.Sprintf(`{"data":[{"backup-type":"vm","backup-id":"100","backup-time":%d}]}`, future)))
			default:
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
		}))
		defer server.Close()

		summary, warnings, err := loadPBSDatastoreSummary(context.Background(), mustNewPBSClient(t, server.URL), "backup", pbs.DatastoreUsage{})
		if err != nil {
			t.Fatalf("loadPBSDatastoreSummary() error = %v", err)
		}
		if len(warnings) != 0 {
			t.Fatalf("expected no warnings, got %v", warnings)
		}
		if summary.UsagePercent != 100 {
			t.Fatalf("expected usage clamp to 100, got %v", summary.UsagePercent)
		}
		if summary.LastBackupAt == "" {
			t.Fatalf("expected last backup timestamp")
		}
		if summary.DaysSinceBackup != 0 {
			t.Fatalf("expected future backup day clamp to 0, got %v", summary.DaysSinceBackup)
		}
	})
}

func TestPBSAPIHelperFunctions(t *testing.T) {
	if node, upid, ok := parsePBSTaskPath("/pbs/tasks/node-a/UPID-1/status", "status"); !ok || node != "node-a" || upid != "UPID-1" {
		t.Fatalf("parsePBSTaskPath success mismatch: ok=%v node=%q upid=%q", ok, node, upid)
	}
	if _, _, ok := parsePBSTaskPath("/tasks/node-a/UPID-1/status", "status"); ok {
		t.Fatalf("expected parse failure for invalid prefix")
	}
	if _, _, ok := parsePBSTaskPath("/pbs/tasks/", "status"); ok {
		t.Fatalf("expected parse failure for empty task path")
	}
	if _, _, ok := parsePBSTaskPath("/pbs/tasks/node-a/UPID-1/log", "status"); ok {
		t.Fatalf("expected parse failure for action mismatch")
	}
	if _, _, ok := parsePBSTaskPath("/pbs/tasks/node-a", "status"); ok {
		t.Fatalf("expected parse failure for short task path")
	}

	tasks := []pbs.Task{
		{UPID: "UPID-A", WorkerID: "backup:vm/101", StartTime: 10},
		{UPID: "UPID-Z", WorkerID: "backup:vm/102", StartTime: 20},
		{UPID: "UPID-M", WorkerID: "other:vm/103", StartTime: 30},
		{UPID: "UPID:node:123:backup:extra:", WorkerID: "", StartTime: 20},
	}

	filtered := filterAndSortPBSTasks(tasks, "backup", 2)
	if len(filtered) != 2 {
		t.Fatalf("expected limited filtered tasks=2, got %d", len(filtered))
	}
	if filtered[0].UPID != "UPID:node:123:backup:extra:" || filtered[1].UPID != "UPID-Z" {
		t.Fatalf("unexpected filtered/sorted order: %+v", filtered)
	}

	all := filterAndSortPBSTasks(tasks, "", 0)
	if len(all) != len(tasks) {
		t.Fatalf("expected no-filter tasks=%d, got %d", len(tasks), len(all))
	}
	if all[0].StartTime < all[1].StartTime {
		t.Fatalf("expected descending sort by start time")
	}

	if got := dedupeNonEmptyWarnings(nil); got != nil {
		t.Fatalf("expected nil output for nil warnings, got %v", got)
	}
	if got := dedupeNonEmptyWarnings([]string{" ", "\t"}); got != nil {
		t.Fatalf("expected nil output for empty warnings, got %v", got)
	}
	warnings := dedupeNonEmptyWarnings([]string{" warning A ", "warning a", "warning B"})
	if len(warnings) != 2 || warnings[0] != "warning A" || warnings[1] != "warning B" {
		t.Fatalf("unexpected dedupe output: %v", warnings)
	}
}

func configurePBSTaskRuntime(t *testing.T, sut *apiServer, collectorID, credentialID, tokenID, baseURL string) {
	t.Helper()

	createPBSCredentialProfile(t, sut, credentialID, tokenID, "secret-"+collectorID, baseURL)
	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{
			{
				ID:            collectorID,
				AssetID:       "pbs-server-" + collectorID,
				CollectorType: hubcollector.CollectorTypePBS,
				Enabled:       true,
				Config: map[string]any{
					"base_url":      baseURL,
					"credential_id": credentialID,
					"token_id":      tokenID,
					"skip_verify":   true,
				},
			},
		},
	}
}

func mustNewPBSClient(t *testing.T, baseURL string) *pbs.Client {
	t.Helper()
	allowInsecureTransportForConnectorTests(t)

	client, err := pbs.NewClient(pbs.Config{
		BaseURL:     baseURL,
		TokenID:     "root@pam!labtether",
		TokenSecret: "secret",
		SkipVerify:  true,
		Timeout:     2 * time.Second,
	})
	if err != nil {
		t.Fatalf("new pbs client: %v", err)
	}
	return client
}
