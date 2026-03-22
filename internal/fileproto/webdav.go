package fileproto

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/studio-b12/gowebdav"
)

// WebDAVClient implements RemoteFS over WebDAV (HTTP/HTTPS).
type WebDAVClient struct {
	client *gowebdav.Client
	config ConnectionConfig
}

// Connect builds a WebDAV client, configures TLS if needed, and verifies
// connectivity by issuing a PROPFIND against the server root.
func (c *WebDAVClient) Connect(ctx context.Context, cfg ConnectionConfig) error {
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

	client := gowebdav.NewClient(baseURL, cfg.Username, cfg.Secret)
	client.SetTimeout(15 * time.Second)

	// Skip TLS verification if explicitly requested (self-signed certs).
	if skipVerify, ok := cfg.ExtraConfig["webdav_tls_skip_verify"].(bool); ok && skipVerify {
		client.SetTransport(&http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // #nosec G402 -- Explicit operator opt-in for self-signed WebDAV endpoints.
		})
	}

	// Verify connectivity.
	if err := client.Connect(); err != nil {
		return fmt.Errorf("webdav connect %s: %w", baseURL, err)
	}

	c.client = client
	return nil
}

// List returns directory entries at the given path.
// Hidden files (names starting with ".") are excluded unless showHidden is true.
func (c *WebDAVClient) List(_ context.Context, dirPath string, showHidden bool) ([]FileEntry, error) {
	entries, err := c.client.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("webdav list %s: %w", dirPath, err)
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
// The size is obtained via a separate Stat call.
func (c *WebDAVClient) Read(_ context.Context, filePath string) (io.ReadCloser, int64, error) {
	info, err := c.client.Stat(filePath)
	if err != nil {
		return nil, 0, fmt.Errorf("webdav stat %s: %w", filePath, err)
	}
	if info.IsDir() {
		return nil, 0, fmt.Errorf("webdav read: %s is a directory", filePath)
	}

	rc, err := c.client.ReadStream(filePath)
	if err != nil {
		return nil, 0, fmt.Errorf("webdav readstream %s: %w", filePath, err)
	}
	return rc, info.Size(), nil
}

// Write creates or overwrites a remote file with the contents of r.
// When size > 0, a Content-Length header is sent as a hint to the server.
func (c *WebDAVClient) Write(_ context.Context, filePath string, r io.Reader, size int64) error {
	var err error
	if size > 0 {
		err = c.client.WriteStreamWithLength(filePath, r, size, 0644)
	} else {
		err = c.client.WriteStream(filePath, r, 0644)
	}
	if err != nil {
		return fmt.Errorf("webdav write %s: %w", filePath, err)
	}
	return nil
}

// Mkdir creates the directory and any necessary parents.
func (c *WebDAVClient) Mkdir(_ context.Context, dirPath string) error {
	if err := c.client.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("webdav mkdir %s: %w", dirPath, err)
	}
	return nil
}

// Delete removes a file or directory (recursively for directories).
func (c *WebDAVClient) Delete(_ context.Context, targetPath string) error {
	if err := c.client.RemoveAll(targetPath); err != nil {
		return fmt.Errorf("webdav delete %s: %w", targetPath, err)
	}
	return nil
}

// Rename moves/renames a remote file or directory.
func (c *WebDAVClient) Rename(_ context.Context, oldPath, newPath string) error {
	if err := c.client.Rename(oldPath, newPath, true); err != nil {
		return fmt.Errorf("webdav rename %s -> %s: %w", oldPath, newPath, err)
	}
	return nil
}

// Copy duplicates a file or directory on the server side.
// WebDAV natively supports server-side COPY via the COPY method.
func (c *WebDAVClient) Copy(_ context.Context, srcPath, dstPath string) error {
	if err := c.client.Copy(srcPath, dstPath, true); err != nil {
		return fmt.Errorf("webdav copy %s -> %s: %w", srcPath, dstPath, err)
	}
	return nil
}

// Close is a no-op for WebDAV. HTTP is stateless; there is no persistent
// connection to tear down.
func (c *WebDAVClient) Close() error {
	return nil
}
