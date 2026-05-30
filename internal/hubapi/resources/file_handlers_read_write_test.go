package resources

import (
	"io"
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
