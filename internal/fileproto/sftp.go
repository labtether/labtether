package fileproto

import (
	"context"
	"fmt"
	"io"
	"net"
	"path"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// SFTPClient implements RemoteFS over SFTP (SSH File Transfer Protocol).
type SFTPClient struct {
	sshConn             *ssh.Client
	sftp                *sftp.Client
	config              ConnectionConfig
	CapturedHostKey     string // populated by TOFU callback
	CapturedFingerprint string // populated by TOFU callback
}

// Connect establishes an SSH connection and opens an SFTP session.
func (c *SFTPClient) Connect(ctx context.Context, cfg ConnectionConfig) error {
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

	// Use context-aware dialer so callers can cancel/timeout.
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("sftp dial %s: %w", addr, err)
	}

	// Perform SSH handshake over the raw connection.
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, sshCfg)
	if err != nil {
		closeAndLog("close raw TCP connection after failed SFTP handshake", conn.Close)
		return fmt.Errorf("sftp ssh handshake: %w", err)
	}
	c.sshConn = ssh.NewClient(sshConn, chans, reqs)

	c.sftp, err = sftp.NewClient(c.sshConn)
	if err != nil {
		closeAndLog("close SSH client after failed SFTP session setup", c.sshConn.Close)
		return fmt.Errorf("sftp session: %w", err)
	}

	return nil
}

// List returns directory entries at the given path.
// Hidden files (names starting with ".") are excluded unless showHidden is true.
func (c *SFTPClient) List(_ context.Context, dirPath string, showHidden bool) ([]FileEntry, error) {
	entries, err := c.sftp.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("sftp list %s: %w", dirPath, err)
	}

	result := make([]FileEntry, 0, len(entries))
	for _, fi := range entries {
		name := fi.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		result = append(result, FileEntry{
			Name:        name,
			Path:        path.Join(dirPath, name),
			IsDir:       fi.IsDir(),
			Size:        fi.Size(),
			ModTime:     fi.ModTime(),
			Permissions: fi.Mode().String(),
		})
	}
	return result, nil
}

// Read opens a remote file and returns an io.ReadCloser plus the file size.
func (c *SFTPClient) Read(_ context.Context, filePath string) (io.ReadCloser, int64, error) {
	info, err := c.sftp.Stat(filePath)
	if err != nil {
		return nil, 0, fmt.Errorf("sftp stat %s: %w", filePath, err)
	}
	if info.IsDir() {
		return nil, 0, fmt.Errorf("sftp read: %s is a directory", filePath)
	}

	f, err := c.sftp.Open(filePath)
	if err != nil {
		return nil, 0, fmt.Errorf("sftp open %s: %w", filePath, err)
	}
	return f, info.Size(), nil
}

// Write creates or overwrites a remote file with the contents of r.
// On error, the partial file is removed as best-effort cleanup.
func (c *SFTPClient) Write(_ context.Context, filePath string, r io.Reader, _ int64) error {
	f, err := c.sftp.Create(filePath)
	if err != nil {
		return fmt.Errorf("sftp create %s: %w", filePath, err)
	}

	if _, copyErr := io.Copy(f, r); copyErr != nil {
		closeAndLog("close partial SFTP file", f.Close)
		removeAndLog("remove partial SFTP file", func() error { return c.sftp.Remove(filePath) })
		return fmt.Errorf("sftp write %s: %w", filePath, copyErr)
	}
	return f.Close()
}

// Mkdir creates the directory and any necessary parents.
func (c *SFTPClient) Mkdir(_ context.Context, dirPath string) error {
	if err := c.sftp.MkdirAll(dirPath); err != nil {
		return fmt.Errorf("sftp mkdir %s: %w", dirPath, err)
	}
	return nil
}

// Delete removes a file or directory (recursively for directories).
func (c *SFTPClient) Delete(_ context.Context, targetPath string) error {
	info, err := c.sftp.Stat(targetPath)
	if err != nil {
		return fmt.Errorf("sftp stat %s: %w", targetPath, err)
	}
	if info.IsDir() {
		return c.removeAll(targetPath)
	}
	if err := c.sftp.Remove(targetPath); err != nil {
		return fmt.Errorf("sftp remove %s: %w", targetPath, err)
	}
	return nil
}

// Rename moves/renames a remote file or directory.
func (c *SFTPClient) Rename(_ context.Context, oldPath, newPath string) error {
	if err := c.sftp.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("sftp rename %s -> %s: %w", oldPath, newPath, err)
	}
	return nil
}

// Copy duplicates a file within the same SFTP connection.
// Directory copy is not supported and returns ErrNotSupported.
func (c *SFTPClient) Copy(_ context.Context, srcPath, dstPath string) error {
	info, err := c.sftp.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("sftp stat %s: %w", srcPath, err)
	}
	if info.IsDir() {
		return ErrNotSupported
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
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("sftp copy %s -> %s: %w", srcPath, dstPath, err)
	}
	return nil
}

// Close tears down the SFTP session and underlying SSH connection.
func (c *SFTPClient) Close() error {
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
	}
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

// removeAll recursively removes a directory and all its contents.
func (c *SFTPClient) removeAll(dirPath string) error {
	entries, err := c.sftp.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("sftp readdir %s: %w", dirPath, err)
	}

	for _, entry := range entries {
		fullPath := path.Join(dirPath, entry.Name())
		if entry.IsDir() {
			if err := c.removeAll(fullPath); err != nil {
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
