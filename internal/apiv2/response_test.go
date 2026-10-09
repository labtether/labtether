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
	v1 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, "already v2")
	})
	wrapped := WrapV1Handler(v1)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	wrapped(rec, req)

	var response struct {
		RequestID string `json:"request_id"`
		Data      string `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.RequestID == "" || response.Data != "already v2" {
		t.Fatalf("v2 response was double wrapped: %s", rec.Body.String())
	}
}

func TestWrapV1Handler_PassThroughAlreadyV2List(t *testing.T) {
	wrapped := WrapV1Handler(func(w http.ResponseWriter, _ *http.Request) {
		WriteList(w, http.StatusOK, []string{"one"}, 1, 1, 10)
	})
	rec := httptest.NewRecorder()
	wrapped(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	var response struct {
		RequestID string   `json:"request_id"`
		Data      []string `json:"data"`
		Meta      struct {
			Total int `json:"total"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.RequestID == "" || len(response.Data) != 1 || response.Data[0] != "one" || response.Meta.Total != 1 {
		t.Fatalf("v2 list was double wrapped: %s", rec.Body.String())
	}
}

func TestWrapV1Handler_WrapsAgentReplyWithRequestID(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload string
		field   string
	}{
		{"processes", `{"request_id":"agent-1","processes":[]}`, "processes"},
		{"files", `{"request_id":"agent-2","entries":[]}`, "entries"},
		{"packages", `{"request_id":"agent-3","packages":[]}`, "packages"},
		{"protocol data", `{"request_id":"agent-4","data":"payload"}`, "data"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wrapped := WrapV1Handler(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.payload))
			})
			rec := httptest.NewRecorder()
			wrapped(rec, httptest.NewRequest(http.MethodGet, "/", nil))
			var response struct {
				RequestID string                     `json:"request_id"`
				Data      map[string]json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if strings.HasPrefix(response.RequestID, "agent-") {
				t.Fatal("agent correlation ID was mistaken for v2 envelope ID")
			}
			if _, ok := response.Data[tc.field]; !ok {
				t.Fatalf("agent %s reply was not wrapped under data", tc.name)
			}
		})
	}
}

func TestWrapV1Handler_WrapsAgentErrorWithRequestID(t *testing.T) {
	wrapped := WrapV1Handler(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"request_id":"agent-5","error":"file denied"}`))
	})
	rec := httptest.NewRecorder()
	wrapped(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	var response struct {
		RequestID string `json:"request_id"`
		Error     string `json:"error"`
		Message   string `json:"message"`
		Status    int    `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.RequestID == "agent-5" || response.Error != "file denied" ||
		response.Message != "file denied" || response.Status != http.StatusBadRequest {
		t.Fatalf("unexpected v2 error envelope: %+v", response)
	}
}

func TestWrapV1Handler_PassThroughAlreadyV2Error(t *testing.T) {
	wrapped := WrapV1Handler(func(w http.ResponseWriter, _ *http.Request) {
		WriteError(w, http.StatusNotFound, "missing", "not found")
	})
	rec := httptest.NewRecorder()
	wrapped(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	var response struct {
		RequestID string          `json:"request_id"`
		Data      json.RawMessage `json:"data"`
		Error     string          `json:"error"`
		Status    int             `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.RequestID == "" || response.Data != nil || response.Error != "missing" || response.Status != http.StatusNotFound {
		t.Fatalf("v2 error was double wrapped: %s", rec.Body.String())
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
