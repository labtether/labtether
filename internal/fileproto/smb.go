package fileproto

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	smb2 "github.com/hirochachacha/go-smb2"
)

// SMBClient implements RemoteFS over SMB/CIFS (SMB2/3).
type SMBClient struct {
	conn    net.Conn
	session *smb2.Session
	share   *smb2.Share
	config  ConnectionConfig
}

// Connect establishes a TCP connection, authenticates via NTLMSSP, and mounts
// the share specified in ExtraConfig["smb_share"].
func (c *SMBClient) Connect(ctx context.Context, cfg ConnectionConfig) error {
	c.config = cfg

	shareName, _ := cfg.ExtraConfig["smb_share"].(string)
	if shareName == "" {
		return fmt.Errorf("smb: ExtraConfig[\"smb_share\"] is required")
	}

	port := cfg.Port
	if port == 0 {
		port = DefaultPort("smb")
	}
	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", port))

	// Dial TCP with context support.
	dialer := net.Dialer{Timeout: 15 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("smb dial %s: %w", addr, err)
	}
	c.conn = conn

	// Build NTLMSSP initiator.
	domain, _ := cfg.ExtraConfig["smb_domain"].(string)

	d := &smb2.Dialer{
		Initiator: &smb2.NTLMInitiator{
			User:     cfg.Username,
			Password: cfg.Secret,
			Domain:   domain,
		},
	}

	session, err := d.DialContext(ctx, conn)
	if err != nil {
		closeAndLog("close raw TCP connection after failed SMB session setup", conn.Close)
		c.conn = nil
		return fmt.Errorf("smb session %s: %w", addr, err)
	}
	c.session = session

	share, err := session.Mount(shareName)
	if err != nil {
		closeAndLog("log off SMB session after failed mount", session.Logoff)
		closeAndLog("close raw TCP connection after failed SMB mount", conn.Close)
		c.session = nil
		c.conn = nil
		return fmt.Errorf("smb mount %q: %w", shareName, err)
	}
	c.share = share

	return nil
}

// smbPath converts a forward-slash interface path to an SMB-relative path.
// The interface uses forward slashes with an optional leading slash; SMB paths
// are relative to the share root and use backslashes. The go-smb2 library
// normalises slashes automatically, so we only need to strip the leading slash.
func smbPath(p string) string {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return "."
	}
	return p
}

// List returns directory entries at the given path.
// Hidden files (names starting with ".") are excluded unless showHidden is true.
func (c *SMBClient) List(_ context.Context, dirPath string, showHidden bool) ([]FileEntry, error) {
	entries, err := c.share.ReadDir(smbPath(dirPath))
	if err != nil {
		return nil, fmt.Errorf("smb list %s: %w", dirPath, err)
	}

	result := make([]FileEntry, 0, len(entries))
	for _, fi := range entries {
		name := fi.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		entryPath := dirPath
		if !strings.HasSuffix(entryPath, "/") {
			entryPath += "/"
		}
		entryPath += name

		result = append(result, FileEntry{
			Name:        name,
			Path:        entryPath,
			IsDir:       fi.IsDir(),
			Size:        fi.Size(),
			ModTime:     fi.ModTime(),
			Permissions: fi.Mode().String(),
		})
	}
	return result, nil
}

// Read opens a remote file and returns an io.ReadCloser plus the file size.
func (c *SMBClient) Read(_ context.Context, filePath string) (io.ReadCloser, int64, error) {
	sp := smbPath(filePath)

	info, err := c.share.Stat(sp)
	if err != nil {
		return nil, 0, fmt.Errorf("smb stat %s: %w", filePath, err)
	}
	if info.IsDir() {
		return nil, 0, fmt.Errorf("smb read: %s is a directory", filePath)
	}

	f, err := c.share.Open(sp)
	if err != nil {
		return nil, 0, fmt.Errorf("smb open %s: %w", filePath, err)
	}
	return f, info.Size(), nil
}

// Write creates or overwrites a remote file with the contents of r.
// On error, the partial file is removed as best-effort cleanup.
func (c *SMBClient) Write(_ context.Context, filePath string, r io.Reader, _ int64) error {
	sp := smbPath(filePath)
	f, err := c.share.Create(sp)
	if err != nil {
		return fmt.Errorf("smb create %s: %w", filePath, err)
	}

	if _, copyErr := io.Copy(f, r); copyErr != nil {
		closeAndLog("close partial SMB file", f.Close)
		removeAndLog("remove partial SMB file", func() error { return c.share.Remove(sp) })
		return fmt.Errorf("smb write %s: %w", filePath, copyErr)
	}
	return f.Close()
}

// Mkdir creates the directory and any necessary parents.
func (c *SMBClient) Mkdir(_ context.Context, dirPath string) error {
	if err := c.share.MkdirAll(smbPath(dirPath), os.ModePerm); err != nil {
		return fmt.Errorf("smb mkdir %s: %w", dirPath, err)
	}
	return nil
}

// Delete removes a file or directory (recursively for directories).
func (c *SMBClient) Delete(_ context.Context, targetPath string) error {
	sp := smbPath(targetPath)

	info, err := c.share.Stat(sp)
	if err != nil {
		return fmt.Errorf("smb stat %s: %w", targetPath, err)
	}

	if info.IsDir() {
		if err := c.share.RemoveAll(sp); err != nil {
			return fmt.Errorf("smb removeall %s: %w", targetPath, err)
		}
		return nil
	}

	if err := c.share.Remove(sp); err != nil {
		return fmt.Errorf("smb remove %s: %w", targetPath, err)
	}
	return nil
}

// Rename moves/renames a remote file or directory.
func (c *SMBClient) Rename(_ context.Context, oldPath, newPath string) error {
	if err := c.share.Rename(smbPath(oldPath), smbPath(newPath)); err != nil {
		return fmt.Errorf("smb rename %s -> %s: %w", oldPath, newPath, err)
	}
	return nil
}

// Copy is not supported by SMB (no standard server-side copy operation).
func (c *SMBClient) Copy(_ context.Context, _, _ string) error {
	return ErrNotSupported
}

// Close tears down the SMB share, session, and TCP connection.
func (c *SMBClient) Close() error {
	var firstErr error
	if c.share != nil {
		if err := c.share.Umount(); err != nil {
			firstErr = err
		}
	}
	if c.session != nil {
		if err := c.session.Logoff(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if c.conn != nil {
		if err := c.conn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
