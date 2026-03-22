package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
)

func decodeChunk(t *testing.T, encoded string) string {
	t.Helper()
	if encoded == "" {
		return ""
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode chunk: %v", err)
	}
	return string(decoded)
}

func TestRelayFileUploadChunksExactChunkBoundaryAddsTerminalMarker(t *testing.T) {
	payload := []byte("abcdefgh")
	var writes []agentmgr.FileWriteData

	total, err := relayFileUploadChunks(bytes.NewReader(payload), "req-1", "/tmp/file.txt", 4, func(msg agentmgr.FileWriteData) error {
		writes = append(writes, msg)
		return nil
	})
	if err != nil {
		t.Fatalf("relay chunks: %v", err)
	}
	if total != int64(len(payload)) {
		t.Fatalf("expected total=%d, got %d", len(payload), total)
	}
	if len(writes) != 3 {
		t.Fatalf("expected 3 writes (2 chunks + done), got %d", len(writes))
	}

	if writes[0].Done || writes[1].Done {
		t.Fatalf("expected first two chunks to be non-terminal: %+v", writes)
	}
	if got := decodeChunk(t, writes[0].Data); got != "abcd" {
		t.Fatalf("unexpected first chunk: %q", got)
	}
	if got := decodeChunk(t, writes[1].Data); got != "efgh" {
		t.Fatalf("unexpected second chunk: %q", got)
	}
	if writes[2].Data != "" || !writes[2].Done || writes[2].Offset != 8 {
		t.Fatalf("unexpected terminal marker: %+v", writes[2])
	}
}

func TestRelayFileUploadChunksPartialFinalChunkUsesInlineDone(t *testing.T) {
	payload := []byte("abcdefghij")
	var writes []agentmgr.FileWriteData

	total, err := relayFileUploadChunks(bytes.NewReader(payload), "req-2", "/tmp/file.txt", 4, func(msg agentmgr.FileWriteData) error {
		writes = append(writes, msg)
		return nil
	})
	if err != nil {
		t.Fatalf("relay chunks: %v", err)
	}
	if total != int64(len(payload)) {
		t.Fatalf("expected total=%d, got %d", len(payload), total)
	}
	if len(writes) < 3 {
		t.Fatalf("expected at least 3 writes, got %d", len(writes))
	}

	doneCount := 0
	reconstructed := make([]byte, 0, len(payload))
	for _, write := range writes {
		if write.Done {
			doneCount++
		}
		if write.Data != "" {
			chunk := decodeChunk(t, write.Data)
			reconstructed = append(reconstructed, []byte(chunk)...)
		}
	}
	if doneCount != 1 {
		t.Fatalf("expected exactly one done marker, got %d writes=%+v", doneCount, writes)
	}
	if string(reconstructed) != string(payload) {
		t.Fatalf("unexpected reconstructed payload: %q", string(reconstructed))
	}
	last := writes[len(writes)-1]
	if !last.Done || last.Offset != int64(len(payload)) {
		t.Fatalf("unexpected terminal write: %+v", last)
	}
}

func TestRelayFileUploadChunksEmptyBodySendsDoneMarker(t *testing.T) {
	var writes []agentmgr.FileWriteData
	total, err := relayFileUploadChunks(bytes.NewReader(nil), "req-3", "/tmp/file.txt", 4, func(msg agentmgr.FileWriteData) error {
		writes = append(writes, msg)
		return nil
	})
	if err != nil {
		t.Fatalf("relay chunks: %v", err)
	}
	if total != 0 {
		t.Fatalf("expected total=0, got %d", total)
	}
	if len(writes) != 1 {
		t.Fatalf("expected a single done marker, got %d writes", len(writes))
	}
	if !writes[0].Done || writes[0].Data != "" || writes[0].Offset != 0 {
		t.Fatalf("unexpected done marker: %+v", writes[0])
	}
}

func TestRelayFileUploadChunksWrapsSendErrors(t *testing.T) {
	_, err := relayFileUploadChunks(bytes.NewReader([]byte("abc")), "req-4", "/tmp/file.txt", 4, func(_ agentmgr.FileWriteData) error {
		return errors.New("socket closed")
	})
	if err == nil {
		t.Fatal("expected send error")
	}
	var sendErr uploadRelaySendError
	if !errors.As(err, &sendErr) {
		t.Fatalf("expected uploadRelaySendError, got %T (%v)", err, err)
	}
}
