package fileproto

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrNotSupported is returned by optional operations (e.g., Copy on FTP).
var ErrNotSupported = errors.New("operation not supported by this protocol")

// RemoteFS is the interface all protocol adapters implement.
type RemoteFS interface {
	Connect(ctx context.Context, config ConnectionConfig) error
	List(ctx context.Context, path string, showHidden bool) ([]FileEntry, error)
	Read(ctx context.Context, path string) (io.ReadCloser, int64, error)
	Write(ctx context.Context, path string, r io.Reader, size int64) error
	Mkdir(ctx context.Context, path string) error
	Delete(ctx context.Context, path string) error
	Rename(ctx context.Context, oldPath, newPath string) error
	Copy(ctx context.Context, srcPath, dstPath string) error
	Close() error
}

// ConnectionConfig holds everything needed to connect to a remote filesystem.
type ConnectionConfig struct {
	Protocol    string
	Host        string
	Port        int // 0 = use protocol default
	Username    string
	Secret      string // #nosec G117 -- Runtime auth material for remote file protocols, not a hardcoded credential.
	Passphrase  string // for encrypted private keys (SFTP)
	AuthMethod  string // "password", "private_key"
	InitialPath string
	ExtraConfig map[string]any // protocol-specific (e.g., smb_share, ftp_passive)
}

// DefaultPort returns the default port for a protocol.
func DefaultPort(protocol string) int {
	switch protocol {
	case "sftp":
		return 22
	case "smb":
		return 445
	case "ftp":
		return 21
	case "webdav":
		return 443
	default:
		return 0
	}
}

// FileEntry represents a single file or directory in a listing.
type FileEntry struct {
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	IsDir       bool      `json:"is_dir"`
	Size        int64     `json:"size"`
	ModTime     time.Time `json:"mod_time"`
	Permissions string    `json:"permissions,omitempty"`
}
