package apiv2

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSON(rec, http.StatusOK, map[string]string{"name": "test"})
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"request_id"`) {
		t.Error("response should contain request_id")
	}
	if !strings.Contains(body, `"data"`) {
		t.Error("response should contain data")
	}
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, http.StatusNotFound, "asset_not_found", "no asset named 'nope'")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"error"`) {
		t.Error("should contain error field")
	}
	if !strings.Contains(body, `"asset_not_found"`) {
		t.Error("should contain error code")
	}
}

func TestWriteList(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteList(rec, http.StatusOK, []string{"a", "b"}, 2, 1, 50)
	body := rec.Body.String()
	if !strings.Contains(body, `"meta"`) {
		t.Error("should contain meta")
	}
	if !strings.Contains(body, `"total"`) {
		t.Error("meta should contain total")
	}
}

func TestNewRequestID(t *testing.T) {
	id1 := NewRequestID()
	id2 := NewRequestID()
	if id1 == id2 {
		t.Error("request IDs should be unique")
	}
	if !strings.HasPrefix(id1, "req_") {
		t.Errorf("should start with req_, got %q", id1)
	}
}

func TestWrapV1Handler_WrapsSuccessResponse(t *testing.T) {
	v1 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"items":[1,2,3]}`))
	})
	wrapped := WrapV1Handler(v1)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	wrapped(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"request_id"`) {
		t.Error("wrapped response should contain request_id")
	}
	if !strings.Contains(body, `"data"`) {
		t.Error("wrapped response should contain data key")
	}
	if !strings.Contains(body, `"items"`) {
		t.Error("wrapped response should contain original payload")
	}
}

func TestWrapV1Handler_WrapsErrorResponse(t *testing.T) {
	v1 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	})
	wrapped := WrapV1Handler(v1)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	wrapped(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"request_id"`) {
		t.Error("error response should contain request_id")
	}
	if !strings.Contains(body, `"error"`) {
		t.Error("error response should contain error key")
	}
}

func TestWrapV1Handler_PassThroughAlreadyV2(t *testing.T) {
	reqID := NewRequestID()
	v1 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		payload, _ := json.Marshal(map[string]any{
			"request_id": reqID,
			"data":       "already v2",
		})
		_, _ = w.Write(payload)
	})
	wrapped := WrapV1Handler(v1)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	wrapped(rec, req)

	body := rec.Body.String()
	// The original request_id must be preserved (no double-wrapping).
	if !strings.Contains(body, reqID) {
		t.Errorf("pass-through response should retain original request_id %q", reqID)
	}
	if strings.Count(body, `"request_id"`) != 1 {
		t.Error("response should have exactly one request_id (no double-wrapping)")
	}
}

func TestWrapV1Handler_PassThroughNonJSON(t *testing.T) {
	v1 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("binary data"))
	})
	wrapped := WrapV1Handler(v1)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	wrapped(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "binary data" {
		t.Errorf("non-JSON body should be passed through unchanged, got %q", rec.Body.String())
	}
}
