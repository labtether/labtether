package admin

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminInternalErrorDoesNotReflectDetails(t *testing.T) {
	rec := httptest.NewRecorder()

	writeAdminInternalError(rec, http.StatusInternalServerError, "failed to load tls settings", errors.New("read /data/tls/private.key: secret-value"))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if got := rec.Body.String(); strings.Contains(got, "private.key") || strings.Contains(got, "secret-value") {
		t.Fatalf("body leaked internal error detail: %q", got)
	}
}
