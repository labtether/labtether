package shared

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONBodyAcceptsSingleObject(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{\"name\":\"node-1\"}\n"))
	rec := httptest.NewRecorder()

	var payload struct {
		Name string `json:"name"`
	}
	if err := DecodeJSONBody(rec, req, &payload); err != nil {
		t.Fatalf("DecodeJSONBody returned error: %v", err)
	}
	if payload.Name != "node-1" {
		t.Fatalf("expected payload name node-1, got %q", payload.Name)
	}
}

func TestDecodeJSONBodyRejectsTrailingTokens(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{\"name\":\"node-1\"}{\"x\":1}"))
	rec := httptest.NewRecorder()

	var payload struct {
		Name string `json:"name"`
	}
	if err := DecodeJSONBody(rec, req, &payload); err == nil {
		t.Fatal("expected trailing token parse error")
	}
}

func TestDecodeOptionalJSONBodyAcceptsOnlyEmptyOrValidJSON(t *testing.T) {
	for _, tc := range []struct {
		name    string
		body    string
		present bool
		wantErr bool
	}{
		{name: "empty", body: "", present: false},
		{name: "valid", body: `{"name":"node-1"}`, present: true},
		{name: "malformed", body: `{"name":`, wantErr: true},
		{name: "trailing", body: `{} {}`, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			var payload struct {
				Name string `json:"name"`
			}
			present, err := DecodeOptionalJSONBody(rec, req, &payload)
			if (err != nil) != tc.wantErr || present != tc.present {
				t.Fatalf("present=%v err=%v, want present=%v wantErr=%v", present, err, tc.present, tc.wantErr)
			}
		})
	}
}

func TestJSONDecodeErrorStatusReportsOversizedBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`"`+strings.Repeat("x", MaxJSONBodyBytes+1)+`"`))
	rec := httptest.NewRecorder()
	var payload string
	_, err := DecodeOptionalJSONBody(rec, req, &payload)
	if err == nil {
		t.Fatal("expected oversized body error")
	}
	if status := JSONDecodeErrorStatus(err); status != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d, want %d", status, http.StatusRequestEntityTooLarge)
	}
}
