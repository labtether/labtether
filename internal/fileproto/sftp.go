package fileproto

import (
	"context"
	"fmt"
	"io"
	"net"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/labtether/labtether/internal/securityruntime"
)

// SFTPClient implements RemoteFS over SFTP (SSH File Transfer Protocol).
type SFTPClient struct {
	sshConn             *ssh.Client
	sftp                *sftp.Client
	rawConn             net.Conn
	opMu                sync.Mutex
	config              ConnectionConfig
	CapturedHostKey     string // populated by TOFU callback
	CapturedFingerprint string // populated by TOFU callback
}

// Connect establishes an SSH connection and opens an SFTP session.
func (c *SFTPClient) Connect(ctx context.Context, cfg ConnectionConfig) error {
	opCtx, cancel := WithOperationTimeout(ctx)
	defer cancel()
	c.config = cfg

	port := cfg.Port
	if port == 0 {
		port = DefaultPort("sftp")
	}

	authMethods, err := c.buildAuth(cfg)
	if err != nil {
		return fmt.Errorf("sftp auth: %w", err)
	}

	hostKeyCallback := c.buildHostKeyCallback(cfg)

	sshCfg := &ssh.ClientConfig{
		User:            cfg.Username,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
	}

	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", port))

	// Resolve once and validate the actual destination before connecting so a
	// stored hostname cannot be used for DNS-rebinding SSRF.
	conn, err := securityruntime.DialOutboundTCPContext(opCtx, cfg.Host, port, 15*time.Second)
	if err != nil {
		return fmt.Errorf("sftp dial %s: %w", addr, err)
	}
	deadline, _ := opCtx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		closeAndLog("close raw TCP connection after failed SFTP deadline setup", conn.Close)
		return fmt.Errorf("sftp set handshake deadline: %w", err)
	}
	stopDeadline := watchConnCancellation(opCtx, conn)
	defer stopDeadline()

	// Perform SSH handshake over the raw connection.
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, sshCfg)
	if err != nil {
		closeAndLog("close raw TCP connection after failed SFTP handshake", conn.Close)
		return fmt.Errorf("sftp ssh handshake: %w", err)
	}
	c.rawConn = conn
	c.sshConn = ssh.NewClient(sshConn, chans, reqs)

	c.sftp, err = sftp.NewClient(c.sshConn)
	if err != nil {
		closeAndLog("close SSH client after failed SFTP session setup", c.sshConn.Close)
		c.rawConn = nil
		return fmt.Errorf("sftp session: %w", err)
	}
	stopDeadline()
	if err := opCtx.Err(); err != nil {
		closeAndLog("close SFTP client after cancelled setup", c.sftp.Close)
		closeAndLog("close SSH client after cancelled setup", c.sshConn.Close)
		c.rawConn = nil
		return fmt.Errorf("sftp connect canceled: %w", err)
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		closeAndLog("close SFTP client after failed deadline reset", c.sftp.Close)
		closeAndLog("close SSH client after failed deadline reset", c.sshConn.Close)
		c.rawConn = nil
		return fmt.Errorf("sftp clear handshake deadline: %w", err)
	}

	return nil
}

// List returns directory entries at the given path.
// Hidden files (names starting with ".") are excluded unless showHidden is true.
func (c *SFTPClient) List(ctx context.Context, dirPath string, showHidden bool) ([]FileEntry, error) {
	opCtx, cleanup, err := beginNetConnOperation(ctx, &c.opMu, c.rawConn)
	if err != nil {
		return nil, fmt.Errorf("sftp list %s: %w", dirPath, err)
	}
	defer cleanup()

	entries, err := c.sftp.ReadDirContext(opCtx, dirPath)
	if err != nil {
		return nil, fmt.Errorf("sftp list %s: %w", dirPath, err)
	}

	result := make([]FileEntry, 0, min(len(entries), MaxVisibleListEntries))
	limiter := newListingLimiter(showHidden)
	for _, fi := range entries {
		name := fi.Name()
		entry := FileEntry{
			Name:        name,
			Path:        path.Join(dirPath, name),
			IsDir:       fi.IsDir(),
			Size:        fi.Size(),
			ModTime:     fi.ModTime(),
			Permissions: fi.Mode().String(),
		}
		result, err = limiter.append(result, entry)
		if err != nil {
			return nil, fmt.Errorf("sftp list %s: %w", dirPath, err)
		}
	}
	return result, nil
}

// Read opens a remote file and returns an io.ReadCloser plus the file size.
func (c *SFTPClient) Read(ctx context.Context, filePath string) (io.ReadCloser, int64, error) {
	opCtx, cleanup, err := beginNetConnOperation(ctx, &c.opMu, c.rawConn)
	if err != nil {
		return nil, 0, fmt.Errorf("sftp read %s: %w", filePath, err)
	}
	info, err := c.sftp.Stat(filePath)
	if err != nil {
		cleanup()
		return nil, 0, fmt.Errorf("sftp stat %s: %w", filePath, err)
	}
	if info.IsDir() {
		cleanup()
		return nil, 0, fmt.Errorf("sftp read: %s is a directory", filePath)
	}
	if err := validateTransferSize(info.Size()); err != nil {
		cleanup()
		return nil, 0, err
	}

	f, err := c.sftp.Open(filePath)
	if err != nil {
		cleanup()
		return nil, 0, fmt.Errorf("sftp open %s: %w", filePath, err)
	}
	return newBoundedOperationReadCloser(opCtx, f, MaxTransferBytes, ErrTransferTooLarge, cleanup), info.Size(), nil
}

// Write creates or overwrites a remote file with the contents of r.
// On error, the partial file is removed as best-effort cleanup.
func (c *SFTPClient) Write(ctx context.Context, filePath string, r io.Reader, size int64) error {
	if err := validateTransferSize(size); err != nil {
		return err
	}
	opCtx, cleanup, err := beginNetConnOperation(ctx, &c.opMu, c.rawConn)
	if err != nil {
		return fmt.Errorf("sftp write %s: %w", filePath, err)
	}
	defer cleanup()

	f, err := c.sftp.Create(filePath)
	if err != nil {
		return fmt.Errorf("sftp create %s: %w", filePath, err)
	}

	limited := newBoundedReader(opCtx, r, MaxTransferBytes, ErrTransferTooLarge)
	if _, copyErr := io.CopyBuffer(f, limited, make([]byte, fileCopyBufferSize)); copyErr != nil {
		closeAndLog("close partial SFTP file", f.Close)
		c.removePartial(filePath)
		return fmt.Errorf("sftp write %s: %w", filePath, copyErr)
	}
	if readErr := limited.terminalError(); readErr != nil {
		closeAndLog("close partial SFTP file", f.Close)
		c.removePartial(filePath)
		return fmt.Errorf("sftp write %s: %w", filePath, readErr)
	}
	if err := f.Close(); err != nil {
		c.removePartial(filePath)
		return fmt.Errorf("sftp close %s: %w", filePath, err)
	}
	return nil
}

// Mkdir creates the directory and any necessary parents.
func (c *SFTPClient) Mkdir(ctx context.Context, dirPath string) error {
	_, cleanup, err := beginNetConnOperation(ctx, &c.opMu, c.rawConn)
	if err != nil {
		return fmt.Errorf("sftp mkdir %s: %w", dirPath, err)
	}
	defer cleanup()
	if err := c.sftp.MkdirAll(dirPath); err != nil {
		return fmt.Errorf("sftp mkdir %s: %w", dirPath, err)
	}
	return nil
}

// Delete removes a file or directory (recursively for directories).
func (c *SFTPClient) Delete(ctx context.Context, targetPath string) error {
	opCtx, cleanup, err := beginNetConnOperation(ctx, &c.opMu, c.rawConn)
	if err != nil {
		return fmt.Errorf("sftp delete %s: %w", targetPath, err)
	}
	defer cleanup()
	info, err := c.sftp.Stat(targetPath)
	if err != nil {
		return fmt.Errorf("sftp stat %s: %w", targetPath, err)
	}
	if info.IsDir() {
		return c.removeAll(opCtx, targetPath, 0, &deleteBudget{})
	}
	if err := c.sftp.Remove(targetPath); err != nil {
		return fmt.Errorf("sftp remove %s: %w", targetPath, err)
	}
	return nil
}

// Rename moves/renames a remote file or directory.
func (c *SFTPClient) Rename(ctx context.Context, oldPath, newPath string) error {
	_, cleanup, err := beginNetConnOperation(ctx, &c.opMu, c.rawConn)
	if err != nil {
		return fmt.Errorf("sftp rename %s -> %s: %w", oldPath, newPath, err)
	}
	defer cleanup()
	if err := c.sftp.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("sftp rename %s -> %s: %w", oldPath, newPath, err)
	}
	return nil
}

// Copy duplicates a file within the same SFTP connection.
// Directory copy is not supported and returns ErrNotSupported.
func (c *SFTPClient) Copy(ctx context.Context, srcPath, dstPath string) error {
	opCtx, cleanup, err := beginNetConnOperation(ctx, &c.opMu, c.rawConn)
	if err != nil {
		return fmt.Errorf("sftp copy %s -> %s: %w", srcPath, dstPath, err)
	}
	defer cleanup()
	info, err := c.sftp.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("sftp stat %s: %w", srcPath, err)
	}
	if info.IsDir() {
		return ErrNotSupported
	}
	if err := validateTransferSize(info.Size()); err != nil {
		return err
	}

	src, err := c.sftp.Open(srcPath)
	if err != nil {
		return fmt.Errorf("sftp open %s: %w", srcPath, err)
	}
	defer src.Close()

	dst, err := c.sftp.Create(dstPath)
	if err != nil {
		return fmt.Errorf("sftp create %s: %w", dstPath, err)
	}
	limited := newBoundedReader(opCtx, src, MaxTransferBytes, ErrTransferTooLarge)
	if _, err := io.CopyBuffer(dst, limited, make([]byte, fileCopyBufferSize)); err != nil {
		closeAndLog("close partial SFTP copy", dst.Close)
		c.removePartial(dstPath)
		return fmt.Errorf("sftp copy %s -> %s: %w", srcPath, dstPath, err)
	}
	if readErr := limited.terminalError(); readErr != nil {
		closeAndLog("close partial SFTP copy", dst.Close)
		c.removePartial(dstPath)
		return fmt.Errorf("sftp copy %s -> %s: %w", srcPath, dstPath, readErr)
	}
	if err := dst.Close(); err != nil {
		c.removePartial(dstPath)
		return fmt.Errorf("sftp close copy %s: %w", dstPath, err)
	}
	return nil
}

// Close tears down the SFTP session and underlying SSH connection.
func (c *SFTPClient) Close() error {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	setProtocolCloseDeadline(c.rawConn)
	var firstErr error
	if c.sftp != nil {
		if err := c.sftp.Close(); err != nil {
			firstErr = err
		}
	}
	if c.sshConn != nil {
		if err := c.sshConn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	} else if c.rawConn != nil {
		if err := c.rawConn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	c.sftp = nil
	c.sshConn = nil
	c.rawConn = nil
	return firstErr
}

// buildAuth constructs SSH auth methods from the connection config.
func (c *SFTPClient) buildAuth(cfg ConnectionConfig) ([]ssh.AuthMethod, error) {
	switch cfg.AuthMethod {
	case "private_key":
		var signer ssh.Signer
		var err error
		if cfg.Passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(cfg.Secret), []byte(cfg.Passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey([]byte(cfg.Secret))
		}
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil

	case "password", "":
		return []ssh.AuthMethod{ssh.Password(cfg.Secret)}, nil

	default:
		return nil, fmt.Errorf("unsupported auth method: %s", cfg.AuthMethod)
	}
}

// buildHostKeyCallback returns either a fixed host key verifier or a TOFU
// callback that captures the server's host key on first connection.
func (c *SFTPClient) buildHostKeyCallback(cfg ConnectionConfig) ssh.HostKeyCallback {
	if hk, ok := cfg.ExtraConfig["host_key"]; ok {
		if hkStr, ok := hk.(string); ok && hkStr != "" {
			pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(hkStr))
			if err == nil {
				return ssh.FixedHostKey(pubKey)
			}
			// Fall through to TOFU if the stored key can't be parsed.
		}
	}

	// TOFU: trust on first use; capture the key for later pinning.
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		c.CapturedHostKey = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
		c.CapturedFingerprint = ssh.FingerprintSHA256(key)
		return nil
	}
}

// removeAll recursively removes a directory and all its contents within a
// fixed depth and entry budget.
func (c *SFTPClient) removeAll(ctx context.Context, dirPath string, depth int, budget *deleteBudget) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := budget.enter(depth, 0); err != nil {
		return err
	}
	entries, err := c.sftp.ReadDirContext(ctx, dirPath)
	if err != nil {
		return fmt.Errorf("sftp readdir %s: %w", dirPath, err)
	}
	if err := budget.enter(depth, len(entries)); err != nil {
		return err
	}

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		fullPath := path.Join(dirPath, entry.Name())
		if entry.IsDir() {
			if err := c.removeAll(ctx, fullPath, depth+1, budget); err != nil {
				return err
			}
		} else {
			if err := c.sftp.Remove(fullPath); err != nil {
				return fmt.Errorf("sftp remove %s: %w", fullPath, err)
			}
		}
	}

	if err := c.sftp.RemoveDirectory(dirPath); err != nil {
		return fmt.Errorf("sftp rmdir %s: %w", dirPath, err)
	}
	return nil
}

func (c *SFTPClient) removePartial(filePath string) {
	if c.rawConn != nil {
		_ = c.rawConn.SetDeadline(time.Now().Add(10 * time.Second))
	}
	removeAndLog("remove partial SFTP file", func() error { return c.sftp.Remove(filePath) })
}
