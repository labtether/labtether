package fileproto

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"math"
	"net"
	"path"
	"strings"
	"time"

	"github.com/jlaffaye/ftp"
)

// FTPClient implements RemoteFS over FTP/FTPS.
type FTPClient struct {
	conn   *ftp.ServerConn
	config ConnectionConfig
}

// Connect dials the FTP server, optionally upgrades to TLS, and logs in.
func (c *FTPClient) Connect(ctx context.Context, cfg ConnectionConfig) error {
	c.config = cfg

	port := cfg.Port
	if port == 0 {
		port = DefaultPort("ftp")
	}
	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", port))

	opts := []ftp.DialOption{
		ftp.DialWithTimeout(10 * time.Second),
		ftp.DialWithContext(ctx),
	}

	// FTPS: explicit TLS when ExtraConfig["ftp_tls"] is true.
	if useTLS, _ := cfg.ExtraConfig["ftp_tls"].(bool); useTLS {
		opts = append(opts, ftp.DialWithExplicitTLS(&tls.Config{
			ServerName:         cfg.Host,
			InsecureSkipVerify: false,
		}))
	}

	// Active mode: disable EPSV when ExtraConfig["ftp_passive"] is explicitly false.
	// Default is passive mode (EPSV enabled).
	if passive, ok := cfg.ExtraConfig["ftp_passive"].(bool); ok && !passive {
		opts = append(opts, ftp.DialWithDisabledEPSV(true))
	}

	conn, err := ftp.Dial(addr, opts...)
	if err != nil {
		return fmt.Errorf("ftp dial %s: %w", addr, err)
	}

	if err := conn.Login(cfg.Username, cfg.Secret); err != nil {
		closeAndLog("quit FTP connection after failed login", conn.Quit)
		return fmt.Errorf("ftp login: %w", err)
	}

	c.conn = conn
	return nil
}

// List returns directory entries at the given path.
// Hidden files (names starting with ".") are excluded unless showHidden is true.
func (c *FTPClient) List(_ context.Context, dirPath string, showHidden bool) ([]FileEntry, error) {
	entries, err := c.conn.List(dirPath)
	if err != nil {
		return nil, fmt.Errorf("ftp list %s: %w", dirPath, err)
	}

	result := make([]FileEntry, 0, len(entries))
	for _, e := range entries {
		// Skip current/parent directory entries.
		if e.Name == "." || e.Name == ".." {
			continue
		}
		if !showHidden && strings.HasPrefix(e.Name, ".") {
			continue
		}
		size := int64(math.MaxInt64)
		if e.Size <= uint64(math.MaxInt64) {
			size = int64(e.Size) // #nosec G115 -- guarded by the MaxInt64 check above.
		}
		result = append(result, FileEntry{
			Name:    e.Name,
			Path:    path.Join(dirPath, e.Name),
			IsDir:   e.Type == ftp.EntryTypeFolder,
			Size:    size,
			ModTime: e.Time,
		})
	}
	return result, nil
}

// Read retrieves a remote file and returns an io.ReadCloser plus the file size.
// If the server does not support the SIZE command, size is -1 (unknown).
func (c *FTPClient) Read(_ context.Context, filePath string) (io.ReadCloser, int64, error) {
	size, err := c.conn.FileSize(filePath)
	if err != nil {
		size = -1 // SIZE not supported by all FTP servers; proceed without Content-Length
	}

	resp, err := c.conn.Retr(filePath)
	if err != nil {
		return nil, 0, fmt.Errorf("ftp retr %s: %w", filePath, err)
	}

	return resp, size, nil
}

// Write creates or overwrites a remote file with the contents of r.
func (c *FTPClient) Write(_ context.Context, filePath string, r io.Reader, _ int64) error {
	if err := c.conn.Stor(filePath, r); err != nil {
		return fmt.Errorf("ftp stor %s: %w", filePath, err)
	}
	return nil
}

// Mkdir creates a single directory (no recursive creation).
func (c *FTPClient) Mkdir(_ context.Context, dirPath string) error {
	if err := c.conn.MakeDir(dirPath); err != nil {
		return fmt.Errorf("ftp mkdir %s: %w", dirPath, err)
	}
	return nil
}

// Delete removes a file or directory. Directories are removed recursively.
func (c *FTPClient) Delete(_ context.Context, targetPath string) error {
	// Try as a file first. If that fails, try recursive directory removal.
	err := c.conn.Delete(targetPath)
	if err == nil {
		return nil
	}
	// Attempt recursive directory removal.
	if err2 := c.conn.RemoveDirRecur(targetPath); err2 != nil {
		return fmt.Errorf("ftp delete %s: file err=%v, dir err=%w", targetPath, err, err2)
	}
	return nil
}

// Rename moves/renames a remote file or directory.
func (c *FTPClient) Rename(_ context.Context, oldPath, newPath string) error {
	if err := c.conn.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("ftp rename %s -> %s: %w", oldPath, newPath, err)
	}
	return nil
}

// Copy is not supported by the FTP protocol (no server-side copy).
func (c *FTPClient) Copy(_ context.Context, _, _ string) error {
	return ErrNotSupported
}

// Close terminates the FTP session.
func (c *FTPClient) Close() error {
	if c.conn != nil {
		return c.conn.Quit()
	}
	return nil
}
