package resources

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type shortFileDownloadWriter struct{}

func (shortFileDownloadWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return len(p) - 1, nil
}

func TestWriteFileDownloadChunkDetectsShortWrite(t *testing.T) {
	var written int64
	err := writeFileDownloadChunk(shortFileDownloadWriter{}, []byte("payload"), 0, &written, 1024)
	if err != io.ErrShortWrite {
		t.Fatalf("expected io.ErrShortWrite, got %v", err)
	}
	if written != 0 {
		t.Fatalf("expected written to remain 0 after short write, got %d", written)
	}
}

func TestWriteFileUploadRelayErrorMapsMaxBytesToPayloadTooLarge(t *testing.T) {
	rec := httptest.NewRecorder()

	writeFileUploadRelayError(rec, "/tmp/big.iso", &http.MaxBytesError{Limit: 512})

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "file exceeds 512 MB limit") {
		t.Fatalf("expected upload limit message, got %s", rec.Body.String())
	}
}

func TestFileDownloadLimitStopsBeforeWritingOversizedChunk(t *testing.T) {
	var dst strings.Builder
	written := int64(3)
	err := writeFileDownloadChunk(&dst, []byte("four"), 3, &written, 6)
	if !errors.Is(err, errFileDownloadTooLarge) {
		t.Fatalf("expected download size error, got %v", err)
	}
	if written != 3 || dst.Len() != 0 {
		t.Fatalf("oversized chunk was partially written: written=%d body=%q", written, dst.String())
	}
}

func TestWriteFileDownloadErrorMapsLimitToPayloadTooLarge(t *testing.T) {
	rec := httptest.NewRecorder()
	writeFileDownloadError(rec, errFileDownloadTooLarge)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
