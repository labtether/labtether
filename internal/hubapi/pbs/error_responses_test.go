package pbs

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPBSErrorResponsesDoNotReflectUpstreamDetails(t *testing.T) {
	secretErr := errors.New("dial tcp 10.0.0.9:8007 with token pbs-secret")
	rec := httptest.NewRecorder()

	writePBSError(rec, http.StatusBadRequest, "invalid form body", secretErr)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if got := rec.Body.String(); !strings.Contains(got, "invalid form body") || strings.Contains(got, "10.0.0.9") || strings.Contains(got, "pbs-secret") {
		t.Fatalf("body contains unsafe or missing error text: %q", got)
	}
	if got := pbsWarning("task listing unavailable", secretErr); got != "task listing unavailable" {
		t.Fatalf("warning = %q", got)
	}
}
