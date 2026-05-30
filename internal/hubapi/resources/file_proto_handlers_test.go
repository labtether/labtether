package resources

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteFileConnectionUploadErrorMapsMaxBytesToPayloadTooLarge(t *testing.T) {
	rec := httptest.NewRecorder()

	writeFileConnectionUploadError(rec, &http.MaxBytesError{Limit: 512})

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "file exceeds 512 MB limit") {
		t.Fatalf("expected upload limit message, got %s", rec.Body.String())
	}
}

func TestWriteFileConnectionUploadErrorKeepsRemoteWriteFailuresBadRequest(t *testing.T) {
	rec := httptest.NewRecorder()

	writeFileConnectionUploadError(rec, errors.New("permission denied"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "upload failed: permission denied") {
		t.Fatalf("expected remote write failure message, got %s", rec.Body.String())
	}
}
