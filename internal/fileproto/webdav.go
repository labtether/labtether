package fileproto

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/studio-b12/gowebdav"

	"github.com/labtether/labtether/internal/securityruntime"
)

// WebDAVClient implements RemoteFS over WebDAV (HTTP/HTTPS).
type WebDAVClient struct {
	client    *gowebdav.Client
	config    ConnectionConfig
	baseURL   string
	transport http.RoundTripper
	auth      gowebdav.Authorizer
}

// Connect builds a WebDAV client, configures TLS if needed, and verifies
// connectivity by issuing a PROPFIND against the server root.
func (c *WebDAVClient) Connect(ctx context.Context, cfg ConnectionConfig) error {
	opCtx, cancel := WithOperationTimeout(ctx)
	defer cancel()
	if err := opCtx.Err(); err != nil {
		return fmt.Errorf("webdav connect canceled: %w", err)
	}
	c.config = cfg

	port := cfg.Port
	if port == 0 {
		port = DefaultPort("webdav")
	}

	// Determine scheme: HTTPS for port 443 or when explicitly configured.
	scheme := "http"
	if port == 443 {
		scheme = "https"
	}
	if useTLS, ok := cfg.ExtraConfig["webdav_tls"].(bool); ok && useTLS {
		scheme = "https"
	}

	baseURL := fmt.Sprintf("%s://%s:%d", scheme, cfg.Host, port)

	// Append optional base path (e.g., /remote.php/dav/files/user).
	if basePath, ok := cfg.ExtraConfig["webdav_base_path"].(string); ok && basePath != "" {
		basePath = strings.TrimRight(basePath, "/")
		if !strings.HasPrefix(basePath, "/") {
			basePath = "/" + basePath
		}
		baseURL += basePath
	}

	auth := gowebdav.NewAutoAuth(cfg.Username, cfg.Secret)
	client := gowebdav.NewAuthClient(baseURL, auth)
	connectTimeout := 15 * time.Second
	if deadline, ok := opCtx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("webdav connect canceled: %w", context.DeadlineExceeded)
		}
		if remaining < connectTimeout {
			connectTimeout = remaining
		}
	}
	client.SetTimeout(connectTimeout)

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialTLSContext = nil
	transport.DialContext = securityruntime.OutboundTCPDialContext(connectTimeout)
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}

	// Skip TLS verification if explicitly requested (self-signed certs).
	if skipVerify, ok := cfg.ExtraConfig["webdav_tls_skip_verify"].(bool); ok && skipVerify {
		transport.TLSClientConfig.InsecureSkipVerify = true // #nosec G402 -- Explicit operator opt-in for self-signed WebDAV endpoints.
	}
	boundedTransport := &boundedWebDAVTransport{base: transport}
	client.SetTransport(boundedTransport)
	connectCtx, cancelConnect := context.WithTimeout(opCtx, connectTimeout)
	defer cancelConnect()
	client.SetInterceptor(func(_ string, req *http.Request) {
		*req = *req.WithContext(connectCtx)
	})

	// Verify connectivity.
	if err := client.Connect(); err != nil {
		return fmt.Errorf("webdav connect %s: %w", baseURL, err)
	}
	// OPTIONS is frequently unauthenticated. A bounded depth-zero PROPFIND
	// verifies the configured path and completes authentication negotiation
	// before streaming requests, which must not buffer request bodies for retry.
	if _, err := client.Stat("/"); err != nil {
		return fmt.Errorf("webdav stat %s: %w", baseURL, err)
	}
	if err := boundedTransport.terminalError(); err != nil {
		return fmt.Errorf("webdav stat %s: %w", baseURL, err)
	}

	c.client = client
	c.baseURL = baseURL
	c.transport = transport
	c.auth = auth
	return nil
}

// operationClient returns a request-scoped client. gowebdav stores its
// interceptor on the client itself, so sharing one client and changing the
// interceptor for each call would race and could attach the wrong caller's
// context. The HTTP transport is safe to share and retains connection pooling.
func (c *WebDAVClient) operationClient(parent context.Context, streamingBody bool) (*gowebdav.Client, *boundedWebDAVTransport, context.Context, context.CancelFunc, error) {
	if c.baseURL == "" || c.transport == nil {
		return nil, nil, nil, nil, errors.New("webdav client is not connected")
	}

	opCtx, cancel := WithOperationTimeout(parent)
	if err := opCtx.Err(); err != nil {
		cancel()
		return nil, nil, nil, nil, err
	}

	auth := c.auth
	if auth == nil {
		auth = gowebdav.NewAutoAuth(c.config.Username, c.config.Secret)
	}
	if streamingBody {
		auth = &webDAVStreamingAuthorizer{base: auth}
	}
	client := gowebdav.NewAuthClient(c.baseURL, auth)
	boundedTransport := &boundedWebDAVTransport{base: c.transport}
	client.SetTransport(boundedTransport)
	if deadline, ok := opCtx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			cancel()
			return nil, nil, nil, nil, context.DeadlineExceeded
		}
		client.SetTimeout(remaining)
	} else {
		client.SetTimeout(MaxOperationDuration)
	}
	client.SetInterceptor(func(_ string, req *http.Request) {
		*req = *req.WithContext(opCtx)
	})
	return client, boundedTransport, opCtx, cancel, nil
}

// webDAVStreamingAuthorizer prevents gowebdav's automatic authentication
// layer from teeing a non-seekable upload into an in-memory retry buffer. The
// base authorizer has already negotiated credentials during Connect.
type webDAVStreamingAuthorizer struct {
	base gowebdav.Authorizer
}

func (a *webDAVStreamingAuthorizer) NewAuthenticator(body io.Reader) (gowebdav.Authenticator, io.Reader) {
	auth, _ := a.base.NewAuthenticator(nil)
	return &webDAVNoRetryAuthenticator{base: auth}, body
}

func (a *webDAVStreamingAuthorizer) AddAuthenticator(key string, factory gowebdav.AuthFactory) {
	a.base.AddAuthenticator(key, factory)
}

type webDAVNoRetryAuthenticator struct {
	base gowebdav.Authenticator
}

func (a *webDAVNoRetryAuthenticator) Authorize(client *http.Client, req *http.Request, requestPath string) error {
	return a.base.Authorize(client, req, requestPath)
}

func (a *webDAVNoRetryAuthenticator) Verify(client *http.Client, resp *http.Response, requestPath string) (bool, error) {
	redo, err := a.base.Verify(client, resp, requestPath)
	if redo {
		return false, errors.New("webdav authentication changed during streaming request")
	}
	return false, err
}

func (a *webDAVNoRetryAuthenticator) Clone() gowebdav.Authenticator {
	return &webDAVNoRetryAuthenticator{base: a.base.Clone()}
}

func (a *webDAVNoRetryAuthenticator) Close() error {
	return a.base.Close()
}

// boundedWebDAVTransport prevents XML error/multistatus responses and file
// downloads from consuming unbounded memory or bandwidth inside gowebdav.
type boundedWebDAVTransport struct {
	base http.RoundTripper
	mu   sync.Mutex
	err  error
}

func (t *boundedWebDAVTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	limit := int64(MaxListResponseBytes)
	limitErr := ErrResponseTooLarge
	if req.Method == http.MethodGet {
		limit = MaxTransferBytes
		limitErr = ErrTransferTooLarge
	}
	resp.Body = &boundedResponseBody{
		ReadCloser: resp.Body,
		reader:     newBoundedReader(req.Context(), resp.Body, limit, limitErr),
		transport:  t,
	}
	return resp, nil
}

func (t *boundedWebDAVTransport) recordError(err error) {
	if err == nil || errors.Is(err, io.EOF) {
		return
	}
	t.mu.Lock()
	if t.err == nil {
		t.err = err
	}
	t.mu.Unlock()
}

func (t *boundedWebDAVTransport) terminalError() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.err
}

type boundedResponseBody struct {
	io.ReadCloser
	reader    io.Reader
	transport *boundedWebDAVTransport
}

func (b *boundedResponseBody) Read(p []byte) (int, error) {
	n, err := b.reader.Read(p)
	b.transport.recordError(err)
	return n, err
}

// List returns directory entries at the given path.
// Hidden files (names starting with ".") are excluded unless showHidden is true.
func (c *WebDAVClient) List(ctx context.Context, dirPath string, showHidden bool) ([]FileEntry, error) {
	client, boundedTransport, _, cancel, err := c.operationClient(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("webdav list %s: %w", dirPath, err)
	}
	defer cancel()

	entries, err := client.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("webdav list %s: %w", dirPath, err)
	}
	if err := boundedTransport.terminalError(); err != nil {
		return nil, fmt.Errorf("webdav list %s: %w", dirPath, err)
	}

	limiter := newListingLimiter(showHidden)
	result := make([]FileEntry, 0, min(len(entries), MaxVisibleListEntries))
	for _, fi := range entries {
		name := fi.Name()
		result, err = limiter.append(result, FileEntry{
			Name:        name,
			Path:        path.Join(dirPath, name),
			IsDir:       fi.IsDir(),
			Size:        fi.Size(),
			ModTime:     fi.ModTime(),
			Permissions: fi.Mode().String(),
		})
		if err != nil {
			return nil, fmt.Errorf("webdav list %s: %w", dirPath, err)
		}
	}
	return result, nil
}

// Read opens a remote file and returns an io.ReadCloser plus the file size.
// The size is obtained via a separate Stat call.
func (c *WebDAVClient) Read(ctx context.Context, filePath string) (io.ReadCloser, int64, error) {
	client, boundedTransport, opCtx, cancel, err := c.operationClient(ctx, false)
	if err != nil {
		return nil, 0, fmt.Errorf("webdav read %s: %w", filePath, err)
	}

	info, err := client.Stat(filePath)
	if err != nil {
		cancel()
		return nil, 0, fmt.Errorf("webdav stat %s: %w", filePath, err)
	}
	if err := boundedTransport.terminalError(); err != nil {
		cancel()
		return nil, 0, fmt.Errorf("webdav stat %s: %w", filePath, err)
	}
	if info.IsDir() {
		cancel()
		return nil, 0, fmt.Errorf("webdav read: %s is a directory", filePath)
	}
	if err := validateTransferSize(info.Size()); err != nil {
		cancel()
		return nil, 0, fmt.Errorf("webdav read %s: %w", filePath, err)
	}

	rc, err := client.ReadStream(filePath)
	if err != nil {
		cancel()
		return nil, 0, fmt.Errorf("webdav readstream %s: %w", filePath, err)
	}
	return newBoundedOperationReadCloser(opCtx, rc, MaxTransferBytes, ErrTransferTooLarge, cancel), info.Size(), nil
}

// Write creates or overwrites a remote file with the contents of r.
// When size > 0, a Content-Length header is sent as a hint to the server.
func (c *WebDAVClient) Write(ctx context.Context, filePath string, r io.Reader, size int64) error {
	if err := validateTransferSize(size); err != nil {
		return fmt.Errorf("webdav write %s: %w", filePath, err)
	}
	client, boundedTransport, opCtx, cancel, err := c.operationClient(ctx, true)
	if err != nil {
		return fmt.Errorf("webdav write %s: %w", filePath, err)
	}
	defer cancel()

	limited := newBoundedReader(opCtx, r, MaxTransferBytes, ErrTransferTooLarge)
	contentLength := size
	if contentLength <= 0 {
		contentLength = -1 // force chunked streaming without buffering the body
	}
	err = client.WriteStreamWithLength(filePath, limited, contentLength, 0644)
	readerErr := limited.terminalError()
	if readerErr == nil {
		readerErr = boundedTransport.terminalError()
	}
	if err != nil || readerErr != nil {
		c.removePartial(filePath)
		if readerErr != nil {
			return fmt.Errorf("webdav write %s: %w", filePath, readerErr)
		}
		return fmt.Errorf("webdav write %s: %w", filePath, err)
	}
	return nil
}

// Mkdir creates the directory and any necessary parents.
func (c *WebDAVClient) Mkdir(ctx context.Context, dirPath string) error {
	client, boundedTransport, _, cancel, err := c.operationClient(ctx, false)
	if err != nil {
		return fmt.Errorf("webdav mkdir %s: %w", dirPath, err)
	}
	defer cancel()
	if err := client.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("webdav mkdir %s: %w", dirPath, err)
	}
	if err := boundedTransport.terminalError(); err != nil {
		return fmt.Errorf("webdav mkdir %s: %w", dirPath, err)
	}
	return nil
}

// Delete removes a file or directory (recursively for directories).
func (c *WebDAVClient) Delete(ctx context.Context, targetPath string) error {
	client, boundedTransport, _, cancel, err := c.operationClient(ctx, false)
	if err != nil {
		return fmt.Errorf("webdav delete %s: %w", targetPath, err)
	}
	defer cancel()
	// Keep WebDAV's native, atomic server-side recursive DELETE semantics.
	if err := client.RemoveAll(targetPath); err != nil {
		return fmt.Errorf("webdav delete %s: %w", targetPath, err)
	}
	if err := boundedTransport.terminalError(); err != nil {
		return fmt.Errorf("webdav delete %s: %w", targetPath, err)
	}
	return nil
}

// Rename moves/renames a remote file or directory.
func (c *WebDAVClient) Rename(ctx context.Context, oldPath, newPath string) error {
	client, boundedTransport, _, cancel, err := c.operationClient(ctx, false)
	if err != nil {
		return fmt.Errorf("webdav rename %s -> %s: %w", oldPath, newPath, err)
	}
	defer cancel()
	if err := client.Rename(oldPath, newPath, true); err != nil {
		return fmt.Errorf("webdav rename %s -> %s: %w", oldPath, newPath, err)
	}
	if err := boundedTransport.terminalError(); err != nil {
		return fmt.Errorf("webdav rename %s -> %s: %w", oldPath, newPath, err)
	}
	return nil
}

// Copy duplicates a file or directory on the server side.
// WebDAV natively supports server-side COPY via the COPY method.
func (c *WebDAVClient) Copy(ctx context.Context, srcPath, dstPath string) error {
	client, boundedTransport, _, cancel, err := c.operationClient(ctx, false)
	if err != nil {
		return fmt.Errorf("webdav copy %s -> %s: %w", srcPath, dstPath, err)
	}
	defer cancel()

	info, err := client.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("webdav copy stat %s: %w", srcPath, err)
	}
	if err := boundedTransport.terminalError(); err != nil {
		return fmt.Errorf("webdav copy stat %s: %w", srcPath, err)
	}
	// Regular files must respect the transfer limit. Directories retain native
	// server-side COPY semantics rather than a client-side recursive walk.
	if !info.IsDir() {
		if err := validateTransferSize(info.Size()); err != nil {
			return fmt.Errorf("webdav copy %s -> %s: %w", srcPath, dstPath, err)
		}
	}
	if err := client.Copy(srcPath, dstPath, true); err != nil {
		return fmt.Errorf("webdav copy %s -> %s: %w", srcPath, dstPath, err)
	}
	if err := boundedTransport.terminalError(); err != nil {
		return fmt.Errorf("webdav copy %s -> %s: %w", srcPath, dstPath, err)
	}
	return nil
}

func (c *WebDAVClient) removePartial(filePath string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, _, _, opCancel, err := c.operationClient(ctx, false)
	if err != nil {
		return
	}
	defer opCancel()
	_ = client.RemoveAll(filePath)
}

// Close releases idle HTTP connections retained by the shared transport.
func (c *WebDAVClient) Close() error {
	if transport, ok := c.transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
	return nil
}
