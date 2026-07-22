package truenas

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTrueNASErrorResponsesDoNotReflectUpstreamDetails(t *testing.T) {
	secretErr := errors.New("https://nas.internal/api with key truenas-secret")
	rec := httptest.NewRecorder()

	writeTrueNASError(rec, http.StatusBadRequest, "invalid request body", secretErr)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if got := rec.Body.String(); !strings.Contains(got, "invalid request body") || strings.Contains(got, "nas.internal") || strings.Contains(got, "truenas-secret") {
		t.Fatalf("body contains unsafe or missing error text: %q", got)
	}
	if got := trueNASWarning("disk temperatures unavailable", secretErr); got != "disk temperatures unavailable" {
		t.Fatalf("warning = %q", got)
	}
}
