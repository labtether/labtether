package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteTopologyInternalErrorDoesNotReflectDetails(t *testing.T) {
	rec := httptest.NewRecorder()

	writeTopologyInternalError(rec, "failed to list assets", errors.New("postgres://admin:secret@db.internal/labtether"))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if got := rec.Body.String(); strings.Contains(got, "admin:secret") || strings.Contains(got, "db.internal") {
		t.Fatalf("body leaked internal error detail: %q", got)
	}
}
