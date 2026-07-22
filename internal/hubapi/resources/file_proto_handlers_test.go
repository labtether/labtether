package resources

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/fileproto"
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

func TestWriteFileConnectionUploadErrorMapsAdapterLimitToPayloadTooLarge(t *testing.T) {
	rec := httptest.NewRecorder()

	writeFileConnectionUploadError(rec, fileproto.ErrTransferTooLarge)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestWriteFileConnectionUploadErrorKeepsRemoteWriteFailuresBadRequest(t *testing.T) {
	rec := httptest.NewRecorder()

	writeFileConnectionUploadError(rec, errors.New("permission denied"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "upload failed") {
		t.Fatalf("expected stable upload failure message, got %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "permission denied") {
		t.Fatalf("response leaked remote write failure detail: %s", rec.Body.String())
	}
}

func TestWriteFileProtocolErrorDoesNotReflectInternalDetails(t *testing.T) {
	rec := httptest.NewRecorder()

	writeFileProtocolError(rec, http.StatusBadRequest, "listing failed", errors.New("dial tcp 10.0.0.8:22 with token secret-value"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if got := rec.Body.String(); !strings.Contains(got, "listing failed") || strings.Contains(got, "10.0.0.8") || strings.Contains(got, "secret-value") {
		t.Fatalf("body contains unsafe or missing error text: %q", got)
	}
}

func TestWriteFileOperationErrorMapsCapacityToTooManyRequests(t *testing.T) {
	rec := httptest.NewRecorder()

	writeFileOperationError(rec, http.StatusBadRequest, "copy failed", errFileOperationCapacity)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After=%q, want 1", got)
	}
}

func TestDispatchFileProtoOpRejectsWhenInteractiveAdmissionIsFull(t *testing.T) {
	releases := make([]func(), 0, maxConcurrentInteractiveFileOperations)
	for i := 0; i < maxConcurrentInteractiveFileOperations; i++ {
		release, ok := interactiveFileOperationAdmission.tryAcquire()
		if !ok {
			t.Fatalf("acquire slot %d", i)
		}
		releases = append(releases, release)
	}
	defer func() {
		for _, release := range releases {
			release()
		}
	}()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/file-connections/test/list", nil)
	(&Deps{}).dispatchFileProtoOp(rec, req, "test", "list")

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestWriteBoundedFileListingRejectsOversizePayload(t *testing.T) {
	rec := httptest.NewRecorder()
	entries := []fileproto.FileEntry{{
		Name: strings.Repeat("x", fileproto.MaxListResponseBytes),
		Path: "/huge",
	}}

	writeBoundedFileListing(rec, "/", entries)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body length=%d", rec.Code, rec.Body.Len())
	}
	if rec.Body.Len() >= fileproto.MaxListResponseBytes {
		t.Fatalf("oversize listing was reflected: body length=%d", rec.Body.Len())
	}
}

type fileProtoOversizeReadFS struct {
	writeCalls int
}

func (f *fileProtoOversizeReadFS) Connect(context.Context, fileproto.ConnectionConfig) error {
	return nil
}
func (f *fileProtoOversizeReadFS) List(context.Context, string, bool) ([]fileproto.FileEntry, error) {
	return nil, nil
}
func (f *fileProtoOversizeReadFS) Read(context.Context, string) (io.ReadCloser, int64, error) {
	return io.NopCloser(strings.NewReader("fixture")), fileproto.MaxTransferBytes + 1, nil
}
func (f *fileProtoOversizeReadFS) Write(context.Context, string, io.Reader, int64) error {
	f.writeCalls++
	return nil
}
func (f *fileProtoOversizeReadFS) Mkdir(context.Context, string) error          { return nil }
func (f *fileProtoOversizeReadFS) Delete(context.Context, string) error         { return nil }
func (f *fileProtoOversizeReadFS) Rename(context.Context, string, string) error { return nil }
func (f *fileProtoOversizeReadFS) Copy(context.Context, string, string) error {
	return fileproto.ErrNotSupported
}
func (f *fileProtoOversizeReadFS) Close() error { return nil }

func TestCopyViaReadWriteRejectsOversizeBeforeDestinationWrite(t *testing.T) {
	fs := &fileProtoOversizeReadFS{}
	err := (&Deps{}).copyViaReadWrite(context.Background(), fs, "/source", "/dest")
	if !errors.Is(err, fileproto.ErrTransferTooLarge) {
		t.Fatalf("expected ErrTransferTooLarge, got %v", err)
	}
	if fs.writeCalls != 0 {
		t.Fatalf("destination Write called %d times", fs.writeCalls)
	}
}

type fileProtoUnlockingReadCloser struct {
	io.Reader
	close func()
}

func (r *fileProtoUnlockingReadCloser) Close() error {
	r.close()
	return nil
}

type fileProtoSerializedFS struct {
	readOpen bool
	written  string
}

func (f *fileProtoSerializedFS) Connect(context.Context, fileproto.ConnectionConfig) error {
	return nil
}
func (f *fileProtoSerializedFS) List(context.Context, string, bool) ([]fileproto.FileEntry, error) {
	return nil, nil
}
func (f *fileProtoSerializedFS) Read(context.Context, string) (io.ReadCloser, int64, error) {
	f.readOpen = true
	return &fileProtoUnlockingReadCloser{
		Reader: strings.NewReader("copy fixture"),
		close:  func() { f.readOpen = false },
	}, int64(len("copy fixture")), nil
}
func (f *fileProtoSerializedFS) Write(_ context.Context, _ string, r io.Reader, _ int64) error {
	if f.readOpen {
		return errors.New("source operation is still open")
	}
	data, err := io.ReadAll(r)
	f.written = string(data)
	return err
}
func (f *fileProtoSerializedFS) Mkdir(context.Context, string) error          { return nil }
func (f *fileProtoSerializedFS) Delete(context.Context, string) error         { return nil }
func (f *fileProtoSerializedFS) Rename(context.Context, string, string) error { return nil }
func (f *fileProtoSerializedFS) Copy(context.Context, string, string) error {
	return fileproto.ErrNotSupported
}
func (f *fileProtoSerializedFS) Close() error { return nil }

func TestCopyViaReadWriteClosesSerializedSourceBeforeDestinationWrite(t *testing.T) {
	fs := &fileProtoSerializedFS{}
	if err := (&Deps{}).copyViaReadWrite(context.Background(), fs, "/source", "/dest"); err != nil {
		t.Fatalf("copy fallback failed: %v", err)
	}
	if fs.written != "copy fixture" {
		t.Fatalf("destination data=%q", fs.written)
	}
}

func TestCopyViaReadWriteRejectsWhenStagingAdmissionIsFull(t *testing.T) {
	releases := make([]func(), 0, maxConcurrentStagedFileCopies)
	for i := 0; i < maxConcurrentStagedFileCopies; i++ {
		release, ok := stagedFileCopyAdmission.tryAcquire()
		if !ok {
			t.Fatalf("acquire slot %d", i)
		}
		releases = append(releases, release)
	}
	defer func() {
		for _, release := range releases {
			release()
		}
	}()

	fs := &fileProtoSerializedFS{}
	err := (&Deps{}).copyViaReadWrite(context.Background(), fs, "/source", "/dest")
	if !errors.Is(err, errFileOperationCapacity) {
		t.Fatalf("expected errFileOperationCapacity, got %v", err)
	}
	if fs.readOpen || fs.written != "" {
		t.Fatalf("staging overload touched remote filesystem: %+v", fs)
	}
}
