package files

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

// CleanupOrphanedTempFiles removes .lt-upload-* temp files older than 5 minutes
// that were left behind by interrupted uploads.
func (fm *Manager) CleanupOrphanedTempFiles() {
	if strings.TrimSpace(fm.BaseDir) == "" {
		return
	}

	cutoff := time.Now().Add(-5 * time.Minute)
	startedAt := time.Now()
	visited := 0
	budgetExceeded := false
	root, err := os.OpenRoot(fm.BaseDir)
	if err != nil {
		log.Printf("file: orphan temp cleanup skipped for %s: %v", fm.BaseDir, err)
		return
	}
	defer root.Close()

	_ = fs.WalkDir(root.FS(), ".", func(relPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible dirs
		}
		visited++
		if visited > orphanCleanupMaxEntries || time.Since(startedAt) > orphanCleanupScanBudget {
			budgetExceeded = true
			return filepath.SkipAll
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasPrefix(d.Name(), ".lt-upload-") {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		if info.ModTime().Before(cutoff) {
			displayPath := filepath.Join(fm.BaseDir, filepath.FromSlash(relPath))
			if rmErr := root.Remove(relPath); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
				log.Printf("file: failed to clean orphaned temp file %s: %v", displayPath, rmErr)
			} else {
				log.Printf("file: cleaned up orphaned temp file: %s", displayPath)
			}
		}
		return nil
	})
	if budgetExceeded {
		log.Printf(
			"file: orphan temp cleanup scan truncated (base=%s visited=%d budget=%s)",
			fm.BaseDir,
			visited,
			orphanCleanupScanBudget,
		)
	}
}

// HandleFileWrite handles a file write (upload) request from the hub.
func (fm *Manager) HandleFileWrite(transport MessageSender, msg agentmgr.Message) {
	var req agentmgr.FileWriteData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("file: invalid write request: %v", err)
		return
	}

	bytesWritten, err := fm.WriteChunk(req)
	if err != nil {
		fm.SendFileWritten(transport, req.RequestID, bytesWritten, err.Error())
		return
	}
	if req.Done {
		fm.SendFileWritten(transport, req.RequestID, bytesWritten, "")
	}
}

// WriteChunk processes a single upload chunk, returning bytes written so far.
func (fm *Manager) WriteChunk(req agentmgr.FileWriteData) (int64, error) {
	filePath, err := fm.ValidatePath(req.Path)
	if err != nil {
		return 0, err
	}

	fm.mu.Lock()
	if fm.writers == nil {
		fm.writers = make(map[string]*PendingWrite)
	}
	pw, exists := fm.writers[req.RequestID]
	if !exists {
		if len(fm.writers) >= maxWritePending {
			fm.mu.Unlock()
			return 0, errors.New("too many concurrent uploads (64 max); wait for current uploads to finish")
		}
		// Create temp file in the target directory.
		dir := filepath.Dir(filePath)
		if mkErr := os.MkdirAll(dir, 0o750); mkErr != nil {
			fm.mu.Unlock()
			return 0, mkErr
		}
		tmpFile, tmpErr := os.CreateTemp(dir, ".lt-upload-*")
		if tmpErr != nil {
			fm.mu.Unlock()
			return 0, tmpErr
		}
		pw = &PendingWrite{
			File:    tmpFile,
			Path:    filePath,
			TmpPath: tmpFile.Name(),
		}
		fm.writers[req.RequestID] = pw
	}
	fm.mu.Unlock()

	// Decode and write chunk.
	decoded, err := base64.StdEncoding.DecodeString(req.Data)
	if err != nil {
		fm.cleanupWrite(req.RequestID)
		return 0, errors.New("invalid base64 data")
	}

	// Enforce cumulative size limit to prevent disk exhaustion.
	if pw.Written+int64(len(decoded)) > MaxFileSize {
		fm.cleanupWrite(req.RequestID)
		return pw.Written, errors.New("file exceeds 512 MB limit")
	}

	n, err := pw.File.Write(decoded)
	if err != nil {
		fm.cleanupWrite(req.RequestID)
		return pw.Written, err
	}
	pw.Written += int64(n)

	if req.Done {
		if closeErr := pw.File.Close(); closeErr != nil {
			fm.cleanupWrite(req.RequestID)
			return pw.Written, closeErr
		}
		// Atomic rename from temp to final path.
		if err := os.Rename(pw.TmpPath, pw.Path); err != nil {
			if rmErr := os.Remove(pw.TmpPath); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
				log.Printf("file: cleanup failed for temp upload %s: %v", pw.TmpPath, rmErr)
			}
			fm.cleanupWrite(req.RequestID)
			return pw.Written, err
		}
		fm.mu.Lock()
		delete(fm.writers, req.RequestID)
		fm.mu.Unlock()
	}
	return pw.Written, nil
}

func (fm *Manager) cleanupWrite(requestID string) {
	fm.mu.Lock()
	if fm.writers == nil {
		fm.mu.Unlock()
		return
	}
	pw, ok := fm.writers[requestID]
	if ok {
		if closeErr := pw.File.Close(); closeErr != nil {
			log.Printf("file: failed to close pending writer %s: %v", requestID, closeErr)
		}
		if rmErr := os.Remove(pw.TmpPath); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			log.Printf("file: failed to remove temp upload %s: %v", pw.TmpPath, rmErr)
		}
		delete(fm.writers, requestID)
	}
	fm.mu.Unlock()
}

// HasPendingWrite returns true if there is a pending write for the given request ID.
// This is primarily used for testing cleanup behavior.
func (fm *Manager) HasPendingWrite(requestID string) bool {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	if fm.writers == nil {
		return false
	}
	_, ok := fm.writers[requestID]
	return ok
}

// CloseAll shuts down all pending file writers and removes temp files.
func (fm *Manager) CloseAll() {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	for id, pw := range fm.writers {
		if closeErr := pw.File.Close(); closeErr != nil {
			log.Printf("file: failed to close pending writer %s during shutdown: %v", id, closeErr)
		}
		if rmErr := os.Remove(pw.TmpPath); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			log.Printf("file: failed to remove pending temp upload %s during shutdown: %v", pw.TmpPath, rmErr)
		}
		delete(fm.writers, id)
	}
}
