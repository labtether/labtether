package main

import (
	"github.com/labtether/labtether/internal/hubcollector"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
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
