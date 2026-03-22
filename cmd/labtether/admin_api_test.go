package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/persistence"
)

func TestAdminResetRequiresPost(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/admin/reset", nil)
	rec := httptest.NewRecorder()
	sut.handleAdminReset(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestAdminResetRequiresConfirmation(t *testing.T) {
	sut := newTestAPIServer(t)

	body := bytes.NewReader([]byte(`{"confirm":"wrong"}`))
	req := httptest.NewRequest(http.MethodPost, "/admin/reset", body)
	rec := httptest.NewRecorder()
	sut.handleAdminReset(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestAdminResetMissingBody(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodPost, "/admin/reset", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	sut.handleAdminReset(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestAdminResetSuccess(t *testing.T) {
	sut := newTestAPIServer(t)
	store := sut.adminResetStore.(*persistence.MemoryAdminResetStore)

	body := bytes.NewReader([]byte(`{"confirm":"RESET"}`))
	req := httptest.NewRequest(http.MethodPost, "/admin/reset", body)
	rec := httptest.NewRecorder()
	sut.handleAdminReset(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if !store.ResetCalled {
		t.Fatal("expected ResetAllData to be called")
	}

	var result persistence.AdminResetResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result.TablesCleared != 38 {
		t.Fatalf("expected 38 tables cleared, got %d", result.TablesCleared)
	}
	if result.ResetAt.IsZero() {
		t.Fatal("expected non-zero reset_at")
	}
}
