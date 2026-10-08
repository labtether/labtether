package persistence

import (
	"context"
	"errors"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/terminal"
	"time"
)

// TerminalStore provides persistence for terminal sessions and commands.
type TerminalStore interface {
	CreateSession(req terminal.CreateSessionRequest) (terminal.Session, error)
	GetSession(id string) (terminal.Session, bool, error)
	UpdateSession(session terminal.Session) error
	ListSessions() ([]terminal.Session, error)
	DeleteTerminalSession(id string) error
	AddCommand(sessionID string, req terminal.CreateCommandRequest, target, mode string) (terminal.Command, error)
	UpdateCommandResult(sessionID, commandID, status, output string) error
	GetCommand(commandID string) (terminal.Command, bool, error)
	DeleteCommand(commandID string) error
	ListCommands(sessionID string) ([]terminal.Command, error)
	ListRecentCommands(limit int) ([]terminal.Command, error)
}

// TerminalPersistentSessionStore provides persistence for durable terminal
// workspaces that can be reattached across client disconnects.
type TerminalPersistentSessionStore interface {
	CreateOrUpdatePersistentSession(req terminal.CreatePersistentSessionRequest) (terminal.PersistentSession, error)
	GetPersistentSession(id string) (terminal.PersistentSession, bool, error)
	ListPersistentSessions() ([]terminal.PersistentSession, error)
	UpdatePersistentSession(id string, req terminal.UpdatePersistentSessionRequest) (terminal.PersistentSession, error)
	MarkPersistentSessionAttached(id string, attachedAt time.Time) (terminal.PersistentSession, error)
	MarkPersistentSessionDetached(id string, detachedAt time.Time) (terminal.PersistentSession, error)
	DeletePersistentSession(id string) error
	MarkPersistentSessionArchived(id string, archivedAt time.Time) (terminal.PersistentSession, error)
	ListDetachedOlderThan(threshold time.Time) ([]terminal.PersistentSession, error)
	ListAttachedSessions() ([]terminal.PersistentSession, error)
	MarkAllAttachedAsDetached() error
}

// TerminalPersistentSessionActorStore is an optional optimization interface
// for loading persistent terminal sessions for a single actor without scanning
// the full session inventory first.
type TerminalPersistentSessionActorStore interface {
	ListPersistentSessionsByActor(actorID string) ([]terminal.PersistentSession, error)
}

// TerminalBookmarkStore provides persistence for saved terminal connection bookmarks.
type TerminalBookmarkStore interface {
	CreateBookmark(req terminal.CreateBookmarkRequest) (terminal.Bookmark, error)
	GetBookmark(id string) (terminal.Bookmark, bool, error)
	ListBookmarks(actorID string) ([]terminal.Bookmark, error)
	UpdateBookmark(id string, req terminal.UpdateBookmarkRequest) (terminal.Bookmark, error)
	DeleteBookmark(id string) error
	TouchBookmarkLastUsed(id string, at time.Time) error
}

// TerminalScrollbackStore provides persistence for terminal session scrollback buffers.
type TerminalScrollbackStore interface {
	UpsertScrollback(persistentSessionID string, buffer []byte, bufferSize int, totalLines int) error
	GetScrollback(persistentSessionID string) ([]byte, error)
	DeleteScrollback(persistentSessionID string) error
}

// FileConnection represents a saved remote file system connection (SFTP, FTP, SMB, WebDAV).
type FileConnection struct {
	ID           string         `json:"id"`
	ActorID      string         `json:"-"`
	Name         string         `json:"name"`
	Protocol     string         `json:"protocol"`
	Host         string         `json:"host"`
	Port         *int           `json:"port,omitempty"`
	InitialPath  string         `json:"initial_path"`
	CredentialID *string        `json:"credential_id,omitempty"`
	ExtraConfig  map[string]any `json:"extra_config,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// RemoteBookmark is a saved remote desktop connection to an external host.
type RemoteBookmark struct {
	ID                string    `json:"id"`
	Label             string    `json:"label"`
	Protocol          string    `json:"protocol"`
	Host              string    `json:"host"`
	Port              int       `json:"port"`
	CredentialID      *string   `json:"credential_id,omitempty"`
	HasCredentials    bool      `json:"has_credentials"`
	SPICESecurityMode string    `json:"spice_security_mode,omitempty"`
	SPICECAPEM        string    `json:"spice_ca_pem,omitempty"`
	AllowInsecureVNC  bool      `json:"allow_insecure_vnc,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// FileTransfer represents a file transfer job between two endpoints.
type FileTransfer struct {
	ID               string     `json:"id"`
	ActorID          string     `json:"-"`
	SourceType       string     `json:"source_type"`
	SourceID         string     `json:"source_id"`
	SourcePath       string     `json:"source_path"`
	DestType         string     `json:"dest_type"`
	DestID           string     `json:"dest_id"`
	DestPath         string     `json:"dest_path"`
	FileName         string     `json:"file_name"`
	FileSize         *int64     `json:"file_size,omitempty"`
	BytesTransferred int64      `json:"bytes_transferred"`
	Status           string     `json:"status"`
	Error            *string    `json:"error,omitempty"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
}

// File-transfer list bounds cap both response size and OFFSET scan work.
const (
	FileTransferListDefaultLimit = 50
	FileTransferListMaxLimit     = 100
	FileTransferListMaxOffset    = 10_000
)

var (
	ErrFileConnectionChanged = errors.New("file connection changed during host key verification")
	ErrSFTPHostKeyMismatch   = errors.New("sftp host key mismatch")
)

// FileConnectionStore provides persistence for remote file connection profiles.
type FileConnectionStore interface {
	ListFileConnections(ctx context.Context) ([]FileConnection, error)
	GetFileConnection(ctx context.Context, id string) (*FileConnection, error)
	CreateFileConnection(ctx context.Context, fc *FileConnection) error
	UpdateFileConnection(ctx context.Context, fc *FileConnection) error
	PinSFTPHostKey(ctx context.Context, connectionID, expectedHost string, expectedPort int, presentedKey string) error
	DeleteFileConnection(ctx context.Context, id string) error
}

// FileConnectionCredentialStore applies a connection edit and its linked
// credential edit in one transaction. The boolean selects profile creation
// for legacy connections that do not have a credential profile yet.
type FileConnectionCredentialStore interface {
	UpdateFileConnectionWithCredential(ctx context.Context, fc *FileConnection, profile credentials.Profile, createProfile bool) (credentials.Profile, error)
}

// RemoteBookmarkStore provides persistence for saved remote desktop bookmarks.
type RemoteBookmarkStore interface {
	ListRemoteBookmarks(ctx context.Context) ([]RemoteBookmark, error)
	GetRemoteBookmark(ctx context.Context, id string) (*RemoteBookmark, error)
	CreateRemoteBookmark(ctx context.Context, bm *RemoteBookmark) error
	UpdateRemoteBookmark(ctx context.Context, bm RemoteBookmark) error
	DeleteRemoteBookmark(ctx context.Context, id string) error
}

// FileTransferStore provides persistence for file transfer jobs.
type FileTransferStore interface {
	GetFileTransfer(ctx context.Context, id string) (*FileTransfer, error)
	CreateFileTransfer(ctx context.Context, ft *FileTransfer) error
	UpdateFileTransfer(ctx context.Context, ft *FileTransfer) error
	// ListFileTransfers returns only records owned by actorID. The result is
	// ordered newest-first by the sortable transfer ID, and total is computed
	// after applying the optional exact status filter but before pagination.
	ListFileTransfers(ctx context.Context, actorID, status string, limit, offset int) ([]FileTransfer, int, error)
	ListActiveFileTransfers(ctx context.Context) ([]FileTransfer, error)
}
