package fileproto

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	smb2 "github.com/hirochachacha/go-smb2"

	"github.com/labtether/labtether/internal/securityruntime"
)

// SMBClient implements RemoteFS over SMB/CIFS (SMB2/3).
type SMBClient struct {
	conn    net.Conn
	session *smb2.Session
	share   *smb2.Share
	opMu    sync.Mutex
	config  ConnectionConfig
}

// Connect establishes a TCP connection, authenticates via NTLMSSP, and mounts
// the share specified in ExtraConfig["smb_share"].
func (c *SMBClient) Connect(ctx context.Context, cfg ConnectionConfig) error {
	opCtx, cancel := WithOperationTimeout(ctx)
	defer cancel()
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

	// Resolve once and validate the actual destination before connecting so a
	// stored hostname cannot be used for DNS-rebinding SSRF.
	conn, err := securityruntime.DialOutboundTCPContext(opCtx, cfg.Host, port, 15*time.Second)
	if err != nil {
		return fmt.Errorf("smb dial %s: %w", addr, err)
	}
	c.conn = conn
	deadline, _ := opCtx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		closeAndLog("close raw TCP connection after failed SMB deadline setup", conn.Close)
		c.conn = nil
		return fmt.Errorf("smb set handshake deadline: %w", err)
	}
	stopDeadline := watchConnCancellation(opCtx, conn)
	defer stopDeadline()

	// Build NTLMSSP initiator.
	domain, _ := cfg.ExtraConfig["smb_domain"].(string)

	d := &smb2.Dialer{
		Initiator: &smb2.NTLMInitiator{
			User:     cfg.Username,
			Password: cfg.Secret,
			Domain:   domain,
		},
	}

	session, err := d.DialContext(opCtx, conn)
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
	stopDeadline()
	if err := opCtx.Err(); err != nil {
		closeAndLog("unmount SMB share after cancelled setup", share.Umount)
		closeAndLog("log off SMB session after cancelled setup", session.Logoff)
		closeAndLog("close raw TCP connection after cancelled setup", conn.Close)
		c.share = nil
		c.session = nil
		c.conn = nil
		return fmt.Errorf("smb connect canceled: %w", err)
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		closeAndLog("unmount SMB share after failed deadline reset", share.Umount)
		closeAndLog("log off SMB session after failed deadline reset", session.Logoff)
		closeAndLog("close raw TCP connection after failed deadline reset", conn.Close)
		c.share = nil
		c.session = nil
		c.conn = nil
		return fmt.Errorf("smb clear handshake deadline: %w", err)
	}

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
func (c *SMBClient) List(ctx context.Context, dirPath string, showHidden bool) ([]FileEntry, error) {
	opCtx, cleanup, err := beginNetConnOperation(ctx, &c.opMu, c.conn)
	if err != nil {
		return nil, fmt.Errorf("smb list %s: %w", dirPath, err)
	}
	defer cleanup()

	dir, err := c.share.Open(smbPath(dirPath))
	if err != nil {
		return nil, fmt.Errorf("smb list %s: %w", dirPath, err)
	}
	defer closeAndLog("close SMB directory after listing", dir.Close)

	result := make([]FileEntry, 0, 256)
	limiter := newListingLimiter(showHidden)
	for {
		if err := opCtx.Err(); err != nil {
			return nil, fmt.Errorf("smb list %s: %w", dirPath, err)
		}
		entries, readErr := dir.Readdir(256)
		for _, fi := range entries {
			name := fi.Name()
			entryPath := dirPath
			if !strings.HasSuffix(entryPath, "/") {
				entryPath += "/"
			}
			entryPath += name

			entry := FileEntry{
				Name:        name,
				Path:        entryPath,
				IsDir:       fi.IsDir(),
				Size:        fi.Size(),
				ModTime:     fi.ModTime(),
				Permissions: fi.Mode().String(),
			}
			result, err = limiter.append(result, entry)
			if err != nil {
				return nil, fmt.Errorf("smb list %s: %w", dirPath, err)
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("smb list %s: %w", dirPath, readErr)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

// Read opens a remote file and returns an io.ReadCloser plus the file size.
func (c *SMBClient) Read(ctx context.Context, filePath string) (io.ReadCloser, int64, error) {
	opCtx, cleanup, err := beginNetConnOperation(ctx, &c.opMu, c.conn)
	if err != nil {
		return nil, 0, fmt.Errorf("smb read %s: %w", filePath, err)
	}
	sp := smbPath(filePath)

	info, err := c.share.Stat(sp)
	if err != nil {
		cleanup()
		return nil, 0, fmt.Errorf("smb stat %s: %w", filePath, err)
	}
	if info.IsDir() {
		cleanup()
		return nil, 0, fmt.Errorf("smb read: %s is a directory", filePath)
	}
	if err := validateTransferSize(info.Size()); err != nil {
		cleanup()
		return nil, 0, err
	}

	f, err := c.share.Open(sp)
	if err != nil {
		cleanup()
		return nil, 0, fmt.Errorf("smb open %s: %w", filePath, err)
	}
	return newBoundedOperationReadCloser(opCtx, f, MaxTransferBytes, ErrTransferTooLarge, cleanup), info.Size(), nil
}

// Write creates or overwrites a remote file with the contents of r.
// On error, the partial file is removed as best-effort cleanup.
func (c *SMBClient) Write(ctx context.Context, filePath string, r io.Reader, size int64) error {
	if err := validateTransferSize(size); err != nil {
		return err
	}
	opCtx, cleanup, err := beginNetConnOperation(ctx, &c.opMu, c.conn)
	if err != nil {
		return fmt.Errorf("smb write %s: %w", filePath, err)
	}
	defer cleanup()
	sp := smbPath(filePath)
	f, err := c.share.Create(sp)
	if err != nil {
		return fmt.Errorf("smb create %s: %w", filePath, err)
	}

	limited := newBoundedReader(opCtx, r, MaxTransferBytes, ErrTransferTooLarge)
	if _, copyErr := io.CopyBuffer(f, limited, make([]byte, fileCopyBufferSize)); copyErr != nil {
		closeAndLog("close partial SMB file", f.Close)
		c.removePartial(sp)
		return fmt.Errorf("smb write %s: %w", filePath, copyErr)
	}
	if readErr := limited.terminalError(); readErr != nil {
		closeAndLog("close partial SMB file", f.Close)
		c.removePartial(sp)
		return fmt.Errorf("smb write %s: %w", filePath, readErr)
	}
	if err := f.Close(); err != nil {
		c.removePartial(sp)
		return fmt.Errorf("smb close %s: %w", filePath, err)
	}
	return nil
}

// Mkdir creates the directory and any necessary parents.
func (c *SMBClient) Mkdir(ctx context.Context, dirPath string) error {
	_, cleanup, err := beginNetConnOperation(ctx, &c.opMu, c.conn)
	if err != nil {
		return fmt.Errorf("smb mkdir %s: %w", dirPath, err)
	}
	defer cleanup()
	if err := c.share.MkdirAll(smbPath(dirPath), os.ModePerm); err != nil {
		return fmt.Errorf("smb mkdir %s: %w", dirPath, err)
	}
	return nil
}

// Delete removes a file or directory (recursively for directories).
func (c *SMBClient) Delete(ctx context.Context, targetPath string) error {
	opCtx, cleanup, err := beginNetConnOperation(ctx, &c.opMu, c.conn)
	if err != nil {
		return fmt.Errorf("smb delete %s: %w", targetPath, err)
	}
	defer cleanup()
	sp := smbPath(targetPath)

	info, err := c.share.Stat(sp)
	if err != nil {
		return fmt.Errorf("smb stat %s: %w", targetPath, err)
	}

	if info.IsDir() {
		if err := c.removeAll(opCtx, sp, 0, &deleteBudget{}); err != nil {
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
func (c *SMBClient) Rename(ctx context.Context, oldPath, newPath string) error {
	_, cleanup, err := beginNetConnOperation(ctx, &c.opMu, c.conn)
	if err != nil {
		return fmt.Errorf("smb rename %s -> %s: %w", oldPath, newPath, err)
	}
	defer cleanup()
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
	c.opMu.Lock()
	defer c.opMu.Unlock()
	setProtocolCloseDeadline(c.conn)
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
	c.share = nil
	c.session = nil
	c.conn = nil
	return firstErr
}

func (c *SMBClient) removeAll(ctx context.Context, targetPath string, depth int, budget *deleteBudget) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := budget.enter(depth, 0); err != nil {
		return err
	}
	dir, err := c.share.Open(targetPath)
	if err != nil {
		return err
	}
	for {
		entries, readErr := dir.Readdir(256)
		if err := budget.enter(depth, len(entries)); err != nil {
			closeAndLog("close SMB directory after delete limit", dir.Close)
			return err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				closeAndLog("close SMB directory after cancelled delete", dir.Close)
				return err
			}
			name := entry.Name()
			if name == "." || name == ".." {
				continue
			}
			childPath := targetPath + "/" + name
			if entry.IsDir() {
				if err := c.removeAll(ctx, childPath, depth+1, budget); err != nil {
					closeAndLog("close SMB directory after recursive delete failure", dir.Close)
					return err
				}
				continue
			}
			if err := c.share.Remove(childPath); err != nil {
				closeAndLog("close SMB directory after file delete failure", dir.Close)
				return err
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			closeAndLog("close SMB directory after read failure", dir.Close)
			return readErr
		}
	}
	if err := dir.Close(); err != nil {
		return err
	}
	return c.share.Remove(targetPath)
}

func (c *SMBClient) removePartial(filePath string) {
	if c.conn != nil {
		_ = c.conn.SetDeadline(time.Now().Add(10 * time.Second))
	}
	removeAndLog("remove partial SMB file", func() error { return c.share.Remove(filePath) })
}
