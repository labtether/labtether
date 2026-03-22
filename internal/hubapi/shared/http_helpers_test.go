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
