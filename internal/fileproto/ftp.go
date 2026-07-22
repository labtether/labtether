package fileproto

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"math"
	"net"
	"path"
	"sync"
	"time"

	"github.com/jlaffaye/ftp"

	"github.com/labtether/labtether/internal/securityruntime"
)

// FTPClient implements RemoteFS over FTP/FTPS.
type FTPClient struct {
	conn        *ftp.ServerConn
	config      ConnectionConfig
	opMu        sync.Mutex
	connMu      sync.Mutex
	controlConn net.Conn
	dataConns   map[net.Conn]struct{}
	opDeadline  time.Time
	dataBudget  *ftpDataReadBudget
}

type trackedFTPConn struct {
	net.Conn
	readBudget *ftpDataReadBudget
	onClose    func()
	once       sync.Once
}

func (c *trackedFTPConn) Read(p []byte) (int, error) {
	if c.readBudget != nil {
		return c.readBudget.read(c.Conn, p)
	}
	return c.Conn.Read(p)
}

func (c *trackedFTPConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(c.onClose)
	return err
}

// ftpDataReadBudget is shared by every data connection opened during one FTP
// operation. jlaffaye/ftp materializes LIST responses before returning them,
// so enforcing the budget at the data socket is the only point where an
// oversized listing can be stopped before it consumes unbounded memory. A
// shared budget also prevents recursive deletes from resetting the allowance
// for each directory they enumerate.
type ftpDataReadBudget struct {
	mu        sync.Mutex
	remaining int64
	limitErr  error
}

func newFTPDataReadBudget(limit int64, limitErr error) *ftpDataReadBudget {
	if limit <= 0 {
		return nil
	}
	return &ftpDataReadBudget{remaining: limit, limitErr: limitErr}
}

func (b *ftpDataReadBudget) read(source io.Reader, p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.remaining <= 0 {
		var probe [1]byte
		n, err := source.Read(probe[:])
		if n > 0 {
			return 0, b.limitErr
		}
		return 0, err
	}
	if int64(len(p)) > b.remaining {
		p = p[:b.remaining]
	}
	n, err := source.Read(p)
	b.remaining -= int64(n)
	return n, err
}

// Connect dials the FTP server, optionally upgrades to TLS, and logs in.
func (c *FTPClient) Connect(ctx context.Context, cfg ConnectionConfig) error {
	opCtx, cancel := WithOperationTimeout(ctx)
	defer cancel()
	c.config = cfg
	c.dataConns = make(map[net.Conn]struct{})

	port := cfg.Port
	if port == 0 {
		port = DefaultPort("ftp")
	}
	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", port))
	secureDial := securityruntime.OutboundTCPDialContext(10 * time.Second)
	controlDial := true
	stopControlDeadline := func() {}
	defer func() { stopControlDeadline() }()

	opts := []ftp.DialOption{
		ftp.DialWithTimeout(10 * time.Second),
		ftp.DialWithContext(opCtx),
		// FTP opens separate passive data connections. Validate each server-
		// supplied endpoint instead of securing only the control connection.
		ftp.DialWithDialFunc(func(network, address string) (net.Conn, error) {
			parentCtx := context.Background()
			isControl := controlDial
			if isControl {
				// The first connection is the control channel and remains bound to
				// the caller's cancellation. Later data connections happen after
				// Connect returns, so they need an independent bounded context.
				controlDial = false
				parentCtx = opCtx
			}
			dialCtx, cancel := context.WithTimeout(parentCtx, 10*time.Second)
			defer cancel()
			conn, err := secureDial(dialCtx, network, address)
			if err != nil {
				return nil, err
			}
			c.connMu.Lock()
			defer c.connMu.Unlock()
			if isControl {
				deadline, _ := opCtx.Deadline()
				if err := conn.SetDeadline(deadline); err != nil {
					_ = conn.Close()
					return nil, err
				}
				stopControlDeadline = watchConnCancellation(opCtx, conn)
				c.controlConn = conn
				return conn, nil
			}
			if !c.opDeadline.IsZero() {
				if err := conn.SetDeadline(c.opDeadline); err != nil {
					_ = conn.Close()
					return nil, err
				}
			}
			var tracked *trackedFTPConn
			tracked = &trackedFTPConn{Conn: conn, readBudget: c.dataBudget, onClose: func() {
				c.connMu.Lock()
				delete(c.dataConns, tracked)
				c.connMu.Unlock()
			}}
			c.dataConns[tracked] = struct{}{}
			return tracked, nil
		}),
	}

	// FTPS: explicit TLS when ExtraConfig["ftp_tls"] is true.
	if useTLS, _ := cfg.ExtraConfig["ftp_tls"].(bool); useTLS {
		opts = append(opts, ftp.DialWithExplicitTLS(&tls.Config{
			ServerName:         cfg.Host,
			MinVersion:         tls.VersionTLS12,
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
		c.clearConnections()
		return fmt.Errorf("ftp dial %s: %w", addr, err)
	}

	if err := conn.Login(cfg.Username, cfg.Secret); err != nil {
		closeAndLog("quit FTP connection after failed login", conn.Quit)
		c.clearConnections()
		return fmt.Errorf("ftp login: %w", err)
	}
	stopControlDeadline()
	if err := opCtx.Err(); err != nil {
		closeAndLog("quit FTP connection after cancelled setup", conn.Quit)
		c.clearConnections()
		return fmt.Errorf("ftp connect canceled: %w", err)
	}
	if err := c.setDeadlines(time.Time{}); err != nil {
		closeAndLog("quit FTP connection after failed deadline reset", conn.Quit)
		c.clearConnections()
		return fmt.Errorf("ftp clear handshake deadline: %w", err)
	}

	c.conn = conn
	return nil
}

// List returns directory entries at the given path.
// Hidden files (names starting with ".") are excluded unless showHidden is true.
func (c *FTPClient) List(ctx context.Context, dirPath string, showHidden bool) ([]FileEntry, error) {
	_, cleanup, err := c.beginOperation(ctx, int64(MaxListResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("ftp list %s: %w", dirPath, err)
	}
	defer cleanup()
	entries, err := c.conn.List(dirPath)
	if err != nil {
		return nil, fmt.Errorf("ftp list %s: %w", dirPath, err)
	}

	result := make([]FileEntry, 0, min(len(entries), MaxVisibleListEntries))
	limiter := newListingLimiter(showHidden)
	for _, e := range entries {
		// Skip current/parent directory entries.
		if e.Name == "." || e.Name == ".." {
			continue
		}
		size := int64(math.MaxInt64)
		if e.Size <= uint64(math.MaxInt64) {
			size = int64(e.Size) // #nosec G115 -- guarded by the MaxInt64 check above.
		}
		entry := FileEntry{
			Name:    e.Name,
			Path:    path.Join(dirPath, e.Name),
			IsDir:   e.Type == ftp.EntryTypeFolder,
			Size:    size,
			ModTime: e.Time,
		}
		result, err = limiter.append(result, entry)
		if err != nil {
			return nil, fmt.Errorf("ftp list %s: %w", dirPath, err)
		}
	}
	return result, nil
}

// Read retrieves a remote file and returns an io.ReadCloser plus the file size.
// If the server does not support the SIZE command, size is -1 (unknown).
func (c *FTPClient) Read(ctx context.Context, filePath string) (io.ReadCloser, int64, error) {
	opCtx, cleanup, err := c.beginOperation(ctx, 0)
	if err != nil {
		return nil, 0, fmt.Errorf("ftp read %s: %w", filePath, err)
	}
	size, err := c.conn.FileSize(filePath)
	if err != nil {
		size = -1 // SIZE not supported by all FTP servers; proceed without Content-Length
	}
	if err := validateTransferSize(size); err != nil {
		cleanup()
		return nil, 0, err
	}

	resp, err := c.conn.Retr(filePath)
	if err != nil {
		cleanup()
		return nil, 0, fmt.Errorf("ftp retr %s: %w", filePath, err)
	}

	return newBoundedOperationReadCloser(opCtx, resp, MaxTransferBytes, ErrTransferTooLarge, cleanup), size, nil
}

// Write creates or overwrites a remote file with the contents of r.
func (c *FTPClient) Write(ctx context.Context, filePath string, r io.Reader, size int64) error {
	if err := validateTransferSize(size); err != nil {
		return err
	}
	opCtx, cleanup, err := c.beginOperation(ctx, 0)
	if err != nil {
		return fmt.Errorf("ftp stor %s: %w", filePath, err)
	}
	defer cleanup()
	limited := newBoundedReader(opCtx, r, MaxTransferBytes, ErrTransferTooLarge)
	storErr := c.conn.Stor(filePath, limited)
	if storErr == nil {
		storErr = limited.terminalError()
	}
	if storErr != nil {
		c.removePartial(filePath)
		return fmt.Errorf("ftp stor %s: %w", filePath, storErr)
	}
	return nil
}

// Mkdir creates a single directory (no recursive creation).
func (c *FTPClient) Mkdir(ctx context.Context, dirPath string) error {
	_, cleanup, err := c.beginOperation(ctx, 0)
	if err != nil {
		return fmt.Errorf("ftp mkdir %s: %w", dirPath, err)
	}
	defer cleanup()
	if err := c.conn.MakeDir(dirPath); err != nil {
		return fmt.Errorf("ftp mkdir %s: %w", dirPath, err)
	}
	return nil
}

// Delete removes a file or directory. Directories are removed recursively.
func (c *FTPClient) Delete(ctx context.Context, targetPath string) error {
	opCtx, cleanup, err := c.beginOperation(ctx, int64(MaxListResponseBytes))
	if err != nil {
		return fmt.Errorf("ftp delete %s: %w", targetPath, err)
	}
	defer cleanup()
	// Try as a file first. If that fails, try recursive directory removal.
	err = c.conn.Delete(targetPath)
	if err == nil {
		return nil
	}
	// Attempt recursive directory removal.
	if err2 := c.removeAll(opCtx, targetPath, 0, &deleteBudget{}); err2 != nil {
		return fmt.Errorf("ftp delete %s: file err=%v, dir err=%w", targetPath, err, err2)
	}
	return nil
}

// Rename moves/renames a remote file or directory.
func (c *FTPClient) Rename(ctx context.Context, oldPath, newPath string) error {
	_, cleanup, err := c.beginOperation(ctx, 0)
	if err != nil {
		return fmt.Errorf("ftp rename %s -> %s: %w", oldPath, newPath, err)
	}
	defer cleanup()
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
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if c.conn != nil {
		c.connMu.Lock()
		_ = c.setDeadlinesLocked(time.Now().Add(protocolCloseTimeout))
		c.connMu.Unlock()
		err := c.conn.Quit()
		c.conn = nil
		c.clearConnections()
		return err
	}
	return nil
}

func (c *FTPClient) beginOperation(parent context.Context, dataReadLimit int64) (context.Context, func(), error) {
	c.opMu.Lock()
	ctx, cancel := WithOperationTimeout(parent)
	if err := ctx.Err(); err != nil {
		cancel()
		c.opMu.Unlock()
		return nil, nil, err
	}
	deadline, _ := ctx.Deadline()
	if err := c.setOperationLimits(deadline, dataReadLimit); err != nil {
		cancel()
		c.opMu.Unlock()
		return nil, nil, err
	}
	stopDeadline := make(chan struct{})
	deadlineStopped := make(chan struct{})
	go func() {
		defer close(deadlineStopped)
		select {
		case <-ctx.Done():
			_ = c.setDeadlines(time.Now())
		case <-stopDeadline:
		}
	}()
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			close(stopDeadline)
			cancel()
			<-deadlineStopped
			_ = c.clearOperationLimits()
			c.opMu.Unlock()
		})
	}
	return ctx, cleanup, nil
}

func (c *FTPClient) setOperationLimits(deadline time.Time, dataReadLimit int64) error {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	c.opDeadline = deadline
	c.dataBudget = newFTPDataReadBudget(dataReadLimit, ErrResponseTooLarge)
	if err := c.setDeadlinesLocked(deadline); err != nil {
		c.opDeadline = time.Time{}
		c.dataBudget = nil
		return err
	}
	return nil
}

func (c *FTPClient) clearOperationLimits() error {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	c.opDeadline = time.Time{}
	c.dataBudget = nil
	return c.setDeadlinesLocked(time.Time{})
}

func (c *FTPClient) setDeadlines(deadline time.Time) error {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	c.opDeadline = deadline
	return c.setDeadlinesLocked(deadline)
}

func (c *FTPClient) setDeadlinesLocked(deadline time.Time) error {
	var firstErr error
	if c.controlConn != nil {
		firstErr = c.controlConn.SetDeadline(deadline)
	}
	for conn := range c.dataConns {
		if err := conn.SetDeadline(deadline); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (c *FTPClient) clearConnections() {
	c.connMu.Lock()
	c.controlConn = nil
	c.dataConns = nil
	c.opDeadline = time.Time{}
	c.dataBudget = nil
	c.connMu.Unlock()
}

func (c *FTPClient) removeAll(ctx context.Context, dirPath string, depth int, budget *deleteBudget) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := budget.enter(depth, 0); err != nil {
		return err
	}
	entries, err := c.conn.List(dirPath)
	if err != nil {
		return err
	}
	count := 0
	for _, entry := range entries {
		if entry.Name != "." && entry.Name != ".." {
			count++
		}
	}
	if err := budget.enter(depth, count); err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Name == "." || entry.Name == ".." {
			continue
		}
		childPath := path.Join(dirPath, entry.Name)
		if entry.Type == ftp.EntryTypeFolder {
			if err := c.removeAll(ctx, childPath, depth+1, budget); err != nil {
				return err
			}
			continue
		}
		if err := c.conn.Delete(childPath); err != nil {
			return err
		}
	}
	return c.conn.RemoveDir(dirPath)
}

func (c *FTPClient) removePartial(filePath string) {
	_ = c.setDeadlines(time.Now().Add(10 * time.Second))
	removeAndLog("remove partial FTP file", func() error { return c.conn.Delete(filePath) })
}
