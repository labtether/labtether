package desktop

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/servicehttp"
)

// RecordingsDir is the default directory for session recordings.
const RecordingsDir = "data/recordings"

// ActiveRecording tracks an in-progress desktop session recording.
type ActiveRecording struct {
	ID        string
	SessionID string
	AssetID   string
	ActorID   string
	Protocol  string
	FilePath  string
	File      *os.File
	mu        sync.Mutex
	StartedAt time.Time
	Bytes     int64
}

// RecordingMetadata holds metadata about a completed or active recording.
type RecordingMetadata struct {
	ID         string
	SessionID  string
	AssetID    string
	ActorID    string
	Protocol   string
	FilePath   string
	FileSize   int64
	DurationMS int64
	Status     string
	CreatedAt  time.Time
	StoppedAt  *time.Time
}

// WriteFrame writes a data frame to the recording file.
func (r *ActiveRecording) WriteFrame(data []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.File == nil || len(data) == 0 {
		return
	}
	written, err := r.File.Write(data)
	if err != nil {
		return
	}
	r.Bytes += int64(written)
}

// Stop closes the recording file.
func (r *ActiveRecording) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.File != nil {
		_ = r.File.Close()
		r.File = nil
	}
}

// HandleRecordings handles GET/POST /recordings.
func (d *Deps) HandleRecordings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		d.listRecordings(w, r)
	case http.MethodPost:
		d.startRecordingRequest(w, r)
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleRecordingActions handles POST /recordings/{sessionID}.
func (d *Deps) HandleRecordingActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/recordings/")
	if path == r.URL.Path || strings.TrimSpace(path) == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	sessionID := strings.TrimSpace(path)
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !d.authorizeRecordingSessionAccess(w, r, sessionID) {
		return
	}
	if !d.StopRecordingBySession(sessionID) {
		servicehttp.WriteError(w, http.StatusNotFound, "no active recording for session")
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"stopped":    true,
		"session_id": sessionID,
	})
}

func (d *Deps) listRecordings(w http.ResponseWriter, r *http.Request) {
	if d.DBPool == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	rows, err := d.DBPool.Query(
		r.Context(),
		`SELECT id, session_id, asset_id, actor_id, protocol, file_path, file_size, duration_ms, status, created_at, stopped_at
		 FROM session_recordings
		 ORDER BY created_at DESC
		 LIMIT 100`,
	)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to query recordings")
		return
	}
	defer rows.Close()

	recordings := make([]map[string]any, 0, 16)
	for rows.Next() {
		var (
			id         string
			sessionID  string
			assetID    string
			actorID    string
			protocol   string
			filePath   string
			fileSize   int64
			durationMS int64
			status     string
			createdAt  time.Time
			stoppedAt  *time.Time
		)
		if err := rows.Scan(
			&id,
			&sessionID,
			&assetID,
			&actorID,
			&protocol,
			&filePath,
			&fileSize,
			&durationMS,
			&status,
			&createdAt,
			&stoppedAt,
		); err != nil {
			continue
		}
		if !d.canAccessRecordingMetadata(r.Context(), sessionID, actorID) {
			continue
		}
		recordings = append(recordings, RecordingResponsePayload(RecordingMetadata{
			ID:         id,
			SessionID:  sessionID,
			AssetID:    assetID,
			ActorID:    actorID,
			Protocol:   protocol,
			FilePath:   filePath,
			FileSize:   fileSize,
			DurationMS: durationMS,
			Status:     status,
			CreatedAt:  createdAt,
			StoppedAt:  stoppedAt,
		}))
	}
	if rows.Err() != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to read recordings")
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"recordings": recordings,
	})
}

func (d *Deps) startRecordingRequest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string `json:"session_id"`
		AssetID   string `json:"asset_id,omitempty"`
		Protocol  string `json:"protocol,omitempty"`
	}
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "session_id is required")
		return
	}
	if !d.authorizeRecordingSessionAccess(w, r, sessionID) {
		return
	}
	rawBridge, ok := d.DesktopBridges.Load(sessionID)
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "desktop session bridge not found")
		return
	}
	bridge, ok := rawBridge.(*DesktopBridge)
	if !ok || bridge == nil {
		servicehttp.WriteError(w, http.StatusNotFound, "desktop session bridge not found")
		return
	}

	assetID := strings.TrimSpace(req.AssetID)
	if assetID == "" {
		if session, ok, err := d.TerminalStore.GetSession(sessionID); err == nil && ok {
			assetID = session.Target
		} else {
			assetID = sessionID
		}
	}
	protocol := NormalizeDesktopProtocol(req.Protocol)
	if protocol == "" {
		protocol = "vnc"
	}
	recording, alreadyRecording, err := bridge.StartRecordingLocked(func() (*ActiveRecording, error) {
		return d.StartRecording(sessionID, assetID, d.UserIDFromContext(r.Context()), protocol)
	})
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to start recording")
		return
	}
	if alreadyRecording {
		recordingID := ""
		if recording != nil {
			recordingID = recording.ID
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"recording_id": recordingID,
			"session_id":   sessionID,
			"status":       "already-recording",
		})
		return
	}
	servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{
		"recording_id": recording.ID,
		"session_id":   sessionID,
		"status":       "recording",
	})
}

// AuthorizeRecordingSessionAccess checks if the caller can access a recording session.
func (d *Deps) AuthorizeRecordingSessionAccess(w http.ResponseWriter, r *http.Request, sessionID string) bool {
	return d.authorizeRecordingSessionAccess(w, r, sessionID)
}

func (d *Deps) authorizeRecordingSessionAccess(w http.ResponseWriter, r *http.Request, sessionID string) bool {
	if d.TerminalStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "session store unavailable")
		return false
	}
	session, ok, err := d.TerminalStore.GetSession(strings.TrimSpace(sessionID))
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to resolve recording session")
		return false
	}
	if !ok || strings.TrimSpace(session.Mode) != "desktop" {
		servicehttp.WriteError(w, http.StatusNotFound, "desktop session not found")
		return false
	}
	if !d.canAccessRecordingSession(r.Context(), session.ActorID) {
		servicehttp.WriteError(w, http.StatusForbidden, "session access denied")
		return false
	}
	return true
}

func (d *Deps) canAccessRecordingSession(ctx context.Context, sessionActorID string) bool {
	if auth.HasAdminPrivileges(d.UserRoleFromContext(ctx)) {
		return true
	}
	actorID := d.PrincipalActorID(ctx)
	return strings.TrimSpace(sessionActorID) == actorID
}

// CanAccessRecordingMetadata checks if the caller can access recording metadata.
func (d *Deps) CanAccessRecordingMetadata(ctx context.Context, sessionID, recordingActorID string) bool {
	return d.canAccessRecordingMetadata(ctx, sessionID, recordingActorID)
}

func (d *Deps) canAccessRecordingMetadata(ctx context.Context, sessionID, recordingActorID string) bool {
	if auth.HasAdminPrivileges(d.UserRoleFromContext(ctx)) {
		return true
	}
	if d.TerminalStore != nil {
		session, ok, err := d.TerminalStore.GetSession(strings.TrimSpace(sessionID))
		if err == nil && ok && strings.TrimSpace(session.Mode) == "desktop" {
			return d.canAccessRecordingSession(ctx, session.ActorID)
		}
	}
	return strings.TrimSpace(recordingActorID) == d.PrincipalActorID(ctx)
}

// RecordingResponsePayload builds the API response payload for a recording.
func RecordingResponsePayload(entry RecordingMetadata) map[string]any {
	return map[string]any{
		"id":          entry.ID,
		"session_id":  entry.SessionID,
		"asset_id":    entry.AssetID,
		"protocol":    entry.Protocol,
		"file_size":   entry.FileSize,
		"duration_ms": entry.DurationMS,
		"status":      entry.Status,
		"created_at":  entry.CreatedAt,
		"stopped_at":  entry.StoppedAt,
	}
}

// DefaultStartRecording creates a new recording. Used as a default for Deps.StartRecording.
func DefaultStartRecording(dbPool *pgxpool.Pool, sessionID, assetID, actorID, protocol string) (*ActiveRecording, error) {
	if dbPool == nil {
		return nil, fmt.Errorf("database unavailable")
	}
	if strings.TrimSpace(assetID) == "" {
		assetID = "unknown"
	}
	if strings.TrimSpace(actorID) == "" {
		actorID = "owner"
	}
	if strings.TrimSpace(protocol) == "" {
		protocol = "vnc"
	}

	if err := os.MkdirAll(RecordingsDir, 0o700); err != nil {
		return nil, fmt.Errorf("create recordings dir: %w", err)
	}
	recordingID := idgen.New("rec")
	filename := fmt.Sprintf("%s_%s.bin", recordingID, time.Now().UTC().Format("20060102T150405"))
	filePath := filepath.Join(RecordingsDir, filename)
	file, err := os.Create(filePath) // #nosec G304 -- Recording file path is generated under the fixed recordings directory.
	if err != nil {
		return nil, fmt.Errorf("create recording file: %w", err)
	}

	now := time.Now().UTC()
	_, execErr := dbPool.Exec(
		context.Background(),
		`INSERT INTO session_recordings (id, session_id, asset_id, actor_id, protocol, file_path, file_size, duration_ms, status, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, 0, 0, 'recording', $7)`,
		recordingID,
		sessionID,
		assetID,
		actorID,
		protocol,
		filePath,
		now,
	)
	if execErr != nil {
		_ = file.Close()
		_ = os.Remove(filePath)
		return nil, fmt.Errorf("insert recording metadata: %w", execErr)
	}

	return &ActiveRecording{
		ID:        recordingID,
		SessionID: sessionID,
		AssetID:   assetID,
		ActorID:   actorID,
		Protocol:  protocol,
		FilePath:  filePath,
		File:      file,
		StartedAt: now,
	}, nil
}

// StopRecordingBySession stops the recording for a given session.
func (d *Deps) StopRecordingBySession(sessionID string) bool {
	rawBridge, ok := d.DesktopBridges.Load(strings.TrimSpace(sessionID))
	if !ok {
		return false
	}
	bridge, ok := rawBridge.(*DesktopBridge)
	if !ok || bridge == nil {
		return false
	}
	return bridge.StopRecordingLocked(d.StopRecording)
}

// DefaultStopRecording finalizes a recording. Used as a default for Deps.StopRecording.
func DefaultStopRecording(dbPool *pgxpool.Pool, rec *ActiveRecording) {
	if rec == nil {
		return
	}
	rec.Stop()
	if dbPool == nil {
		return
	}
	now := time.Now().UTC()
	durationMS := now.Sub(rec.StartedAt).Milliseconds()
	_, err := dbPool.Exec(
		context.Background(),
		`UPDATE session_recordings
		 SET status = 'completed',
		     file_size = $1,
		     duration_ms = $2,
		     stopped_at = $3
		 WHERE id = $4`,
		rec.Bytes,
		durationMS,
		now,
		rec.ID,
	)
	if err != nil {
		log.Printf("recording: failed to finalize %s: %v", rec.ID, err)
	}
}
