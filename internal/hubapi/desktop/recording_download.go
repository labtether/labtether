package desktop

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/servicehttp"
)

const maxRecordingIDLength = 128

// RecordingDownloadPath is the Hub route that streams one recording file.
func RecordingDownloadPath(recordingID string) string {
	return "/recordings/" + url.PathEscape(recordingID) + "/download"
}

// handleRecordingDownload streams a finished recording as an attachment.
// Recordings hold every byte of an interactive session, so only operators and
// above may download them, and only recordings they could list.
func (d *Deps) handleRecordingDownload(w http.ResponseWriter, r *http.Request, recordingID string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	recordingID = strings.TrimSpace(recordingID)
	if recordingID == "" || len(recordingID) > maxRecordingIDLength || strings.ContainsAny(recordingID, `/\`) {
		servicehttp.WriteError(w, http.StatusNotFound, "recording not found")
		return
	}
	if d.UserRoleFromContext == nil || !auth.HasWritePrivileges(d.UserRoleFromContext(r.Context())) {
		servicehttp.WriteError(w, http.StatusForbidden, "operator role required")
		return
	}

	lookup := d.LookupRecording
	if lookup == nil {
		lookup = d.lookupRecordingFromDB
	}
	meta, ok, err := lookup(r.Context(), recordingID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "failed to load recording")
		return
	}
	// Recordings outside the caller's visibility look the same as unknown ones.
	if !ok || !d.canAccessRecordingMetadata(r.Context(), meta.SessionID, meta.ActorID) ||
		!apiv2.AssetCheckContext(r.Context(), meta.AssetID) {
		servicehttp.WriteError(w, http.StatusNotFound, "recording not found")
		return
	}
	if strings.TrimSpace(meta.Status) == "recording" {
		servicehttp.WriteError(w, http.StatusConflict, "recording is still in progress")
		return
	}

	file, info, err := d.openRecordingFile(meta.FilePath)
	if err != nil {
		servicehttp.WriteError(w, http.StatusNotFound, "recording file not found")
		return
	}
	defer func() { _ = file.Close() }()

	name := info.Name()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, name, info.ModTime(), file)
}

func (d *Deps) recordingsDir() string {
	if dir := strings.TrimSpace(d.RecordingsDir); dir != "" {
		return dir
	}
	return LegacyRecordingsDir
}

// openRecordingFile opens a stored recording by base name only, inside the
// recordings directory (or the pre-1.7.0 legacy directory). The stored path is
// never joined as-is, so a tampered row cannot reach files elsewhere, and
// symlinks are refused.
func (d *Deps) openRecordingFile(storedPath string) (*os.File, os.FileInfo, error) {
	base := filepath.Base(filepath.Clean(strings.TrimSpace(storedPath)))
	if base == "." || base == ".." || base == string(filepath.Separator) || !strings.HasSuffix(base, ".bin") {
		return nil, nil, os.ErrNotExist
	}
	dirs := []string{d.recordingsDir()}
	if filepath.Clean(dirs[0]) != filepath.Clean(LegacyRecordingsDir) {
		dirs = append(dirs, LegacyRecordingsDir)
	}
	for _, dir := range dirs {
		candidate := filepath.Join(dir, base)
		linkInfo, err := os.Lstat(candidate)
		if err != nil || !linkInfo.Mode().IsRegular() {
			continue
		}
		file, err := os.Open(candidate) // #nosec G304 -- base name inside the recordings directory, symlinks refused.
		if err != nil {
			continue
		}
		info, err := file.Stat()
		if err != nil || !os.SameFile(linkInfo, info) {
			_ = file.Close()
			continue
		}
		return file, info, nil
	}
	return nil, nil, os.ErrNotExist
}

func (d *Deps) lookupRecordingFromDB(ctx context.Context, recordingID string) (RecordingMetadata, bool, error) {
	var meta RecordingMetadata
	if d.DBPool == nil {
		return meta, false, errors.New("database unavailable")
	}
	err := d.DBPool.QueryRow(ctx,
		`SELECT id, session_id, asset_id, actor_id, protocol, file_path, file_size, duration_ms, status, created_at, stopped_at
		FROM session_recordings WHERE id = $1`,
		recordingID,
	).Scan(&meta.ID, &meta.SessionID, &meta.AssetID, &meta.ActorID, &meta.Protocol, &meta.FilePath,
		&meta.FileSize, &meta.DurationMS, &meta.Status, &meta.CreatedAt, &meta.StoppedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return RecordingMetadata{}, false, nil
	}
	if err != nil {
		return RecordingMetadata{}, false, err
	}
	return meta, true, nil
}
