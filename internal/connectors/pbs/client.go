package pbs

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	neturl "net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/securityruntime"
)

type Config struct {
	BaseURL     string
	TokenID     string
	TokenSecret string
	SkipVerify  bool
	CAPEM       string
	Timeout     time.Duration
}

type Client struct {
	baseURL     string
	tokenID     string
	tokenSecret string
	httpClient  *http.Client
}

func NewClient(cfg Config) (*Client, error) {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			// #nosec G402 -- user-controlled homelab setting for self-signed certs.
			InsecureSkipVerify: cfg.SkipVerify, //nolint:gosec // #nosec G402 -- user-controlled homelab setting
		},
		MaxIdleConns:        20,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     20,
		IdleConnTimeout:     90 * time.Second,
	}
	transport.DialContext = securityruntime.PreferSameSubnetPrivateDialContext(&net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
	})
	if strings.TrimSpace(cfg.CAPEM) != "" {
		pool := x509.NewCertPool()
		if ok := pool.AppendCertsFromPEM([]byte(cfg.CAPEM)); !ok {
			return nil, fmt.Errorf("invalid PBS CA PEM")
		}
		transport.TLSClientConfig.RootCAs = pool
	}

	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	baseURL = strings.TrimSuffix(baseURL, "/api2/json")

	return &Client{
		baseURL:     baseURL,
		tokenID:     strings.TrimSpace(cfg.TokenID),
		tokenSecret: strings.TrimSpace(cfg.TokenSecret),
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}, nil
}

func (c *Client) IsConfigured() bool {
	if c == nil {
		return false
	}
	return c.baseURL != "" && c.tokenID != "" && c.tokenSecret != ""
}

func (c *Client) GetVersion(ctx context.Context) (Version, error) {
	var out Version
	if err := c.getData(ctx, "/api2/json/version", &out); err != nil {
		return Version{}, err
	}
	return out, nil
}

func (c *Client) Ping(ctx context.Context) (PingResponse, error) {
	var out PingResponse
	if err := c.getData(ctx, "/api2/json/ping", &out); err != nil {
		return PingResponse{}, err
	}
	return out, nil
}

func (c *Client) ListDatastores(ctx context.Context) ([]Datastore, error) {
	var out []Datastore
	if err := c.getData(ctx, "/api2/json/admin/datastore", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetDatastoreStatus(ctx context.Context, store string, verbose bool) (DatastoreStatus, error) {
	path := fmt.Sprintf("/api2/json/admin/datastore/%s/status", neturl.PathEscape(strings.TrimSpace(store)))
	if verbose {
		path += "?verbose=1"
	}
	var out DatastoreStatus
	if err := c.getData(ctx, path, &out); err != nil {
		return DatastoreStatus{}, err
	}
	if out.Store == "" {
		out.Store = strings.TrimSpace(store)
	}
	return out, nil
}

func (c *Client) ListDatastoreGroups(ctx context.Context, store string) ([]BackupGroup, error) {
	path := fmt.Sprintf("/api2/json/admin/datastore/%s/groups", neturl.PathEscape(strings.TrimSpace(store)))
	var out []BackupGroup
	if err := c.getData(ctx, path, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) ListDatastoreSnapshots(ctx context.Context, store string) ([]BackupSnapshot, error) {
	path := fmt.Sprintf("/api2/json/admin/datastore/%s/snapshots", neturl.PathEscape(strings.TrimSpace(store)))
	var out []BackupSnapshot
	if err := c.getData(ctx, path, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) ListDatastoreUsage(ctx context.Context) ([]DatastoreUsage, error) {
	var out []DatastoreUsage
	if err := c.getData(ctx, "/api2/json/status/datastore-usage", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) ListNodeTasks(ctx context.Context, node string, limit int) ([]Task, error) {
	trimmedNode := strings.TrimSpace(node)
	if trimmedNode == "" {
		trimmedNode = "localhost"
	}

	path := fmt.Sprintf("/api2/json/nodes/%s/tasks", neturl.PathEscape(trimmedNode))
	if limit > 0 {
		path += "?limit=" + strconv.Itoa(limit)
	}

	var out []Task
	if err := c.getData(ctx, path, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetTaskStatus(ctx context.Context, node, upid string) (TaskStatus, error) {
	trimmedNode := strings.TrimSpace(node)
	if trimmedNode == "" {
		trimmedNode = "localhost"
	}
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/tasks/%s/status",
		neturl.PathEscape(trimmedNode),
		neturl.PathEscape(strings.TrimSpace(upid)),
	)
	var out TaskStatus
	if err := c.getData(ctx, path, &out); err != nil {
		return TaskStatus{}, err
	}
	return out, nil
}

func (c *Client) GetTaskLog(ctx context.Context, node, upid string, limit int) ([]TaskLogLine, error) {
	trimmedNode := strings.TrimSpace(node)
	if trimmedNode == "" {
		trimmedNode = "localhost"
	}
	if limit <= 0 {
		limit = 200
	}

	path := fmt.Sprintf(
		"/api2/json/nodes/%s/tasks/%s/log?limit=%d",
		neturl.PathEscape(trimmedNode),
		neturl.PathEscape(strings.TrimSpace(upid)),
		limit,
	)

	var out []TaskLogLine
	if err := c.getData(ctx, path, &out); err == nil {
		return out, nil
	}

	// Some PBS versions may return raw log strings for this endpoint.
	var fallback []string
	if err := c.getData(ctx, path, &fallback); err != nil {
		return nil, err
	}
	lines := make([]TaskLogLine, 0, len(fallback))
	for i, text := range fallback {
		lines = append(lines, TaskLogLine{LineNo: i, Text: text})
	}
	return lines, nil
}

func (c *Client) StopTask(ctx context.Context, node, upid string) error {
	trimmedNode := strings.TrimSpace(node)
	if trimmedNode == "" {
		trimmedNode = "localhost"
	}
	path := fmt.Sprintf(
		"/api2/json/nodes/%s/tasks/%s",
		neturl.PathEscape(trimmedNode),
		neturl.PathEscape(strings.TrimSpace(upid)),
	)
	_, err := c.requestRaw(ctx, http.MethodDelete, path, nil)
	return err
}

func (c *Client) StartVerify(ctx context.Context, store string) (string, error) {
	path := fmt.Sprintf("/api2/json/admin/datastore/%s/verify", neturl.PathEscape(strings.TrimSpace(store)))
	return c.postTask(ctx, path, nil)
}

func (c *Client) StartPruneDatastore(ctx context.Context, store string, opts PruneOptions) (string, error) {
	values := neturl.Values{}
	if opts.DryRun {
		values.Set("dry-run", "1")
	}
	if opts.KeepLast > 0 {
		values.Set("keep-last", strconv.Itoa(opts.KeepLast))
	}
	if opts.KeepHourly > 0 {
		values.Set("keep-hourly", strconv.Itoa(opts.KeepHourly))
	}
	if opts.KeepDaily > 0 {
		values.Set("keep-daily", strconv.Itoa(opts.KeepDaily))
	}
	if opts.KeepWeekly > 0 {
		values.Set("keep-weekly", strconv.Itoa(opts.KeepWeekly))
	}
	if opts.KeepMonthly > 0 {
		values.Set("keep-monthly", strconv.Itoa(opts.KeepMonthly))
	}
	if opts.KeepYearly > 0 {
		values.Set("keep-yearly", strconv.Itoa(opts.KeepYearly))
	}
	path := fmt.Sprintf("/api2/json/admin/datastore/%s/prune-datastore", neturl.PathEscape(strings.TrimSpace(store)))
	return c.postTask(ctx, path, values)
}

func (c *Client) StartGC(ctx context.Context, store string) (string, error) {
	path := fmt.Sprintf("/api2/json/admin/datastore/%s/gc", neturl.PathEscape(strings.TrimSpace(store)))
	return c.postTask(ctx, path, nil)
}

// --- Verify Jobs ---

func (c *Client) ListVerifyJobs(ctx context.Context) ([]VerifyJob, error) {
	var out []VerifyJob
	if err := c.getData(ctx, "/api2/json/config/verify", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) CreateVerifyJob(ctx context.Context, values neturl.Values) error {
	_, err := c.requestRaw(ctx, http.MethodPost, "/api2/json/config/verify", values)
	return err
}

func (c *Client) UpdateVerifyJob(ctx context.Context, id string, values neturl.Values) error {
	path := fmt.Sprintf("/api2/json/config/verify/%s", neturl.PathEscape(strings.TrimSpace(id)))
	_, err := c.requestRaw(ctx, http.MethodPut, path, values)
	return err
}

func (c *Client) DeleteVerifyJob(ctx context.Context, id string) error {
	path := fmt.Sprintf("/api2/json/config/verify/%s", neturl.PathEscape(strings.TrimSpace(id)))
	_, err := c.requestRaw(ctx, http.MethodDelete, path, nil)
	return err
}

func (c *Client) RunVerifyJob(ctx context.Context, id string) (string, error) {
	path := fmt.Sprintf("/api2/json/admin/verify/%s/run", neturl.PathEscape(strings.TrimSpace(id)))
	return c.postTask(ctx, path, nil)
}

// --- Prune Jobs ---

func (c *Client) ListPruneJobs(ctx context.Context) ([]PruneJob, error) {
	var out []PruneJob
	if err := c.getData(ctx, "/api2/json/config/prune", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) CreatePruneJob(ctx context.Context, values neturl.Values) error {
	_, err := c.requestRaw(ctx, http.MethodPost, "/api2/json/config/prune", values)
	return err
}

func (c *Client) UpdatePruneJob(ctx context.Context, id string, values neturl.Values) error {
	path := fmt.Sprintf("/api2/json/config/prune/%s", neturl.PathEscape(strings.TrimSpace(id)))
	_, err := c.requestRaw(ctx, http.MethodPut, path, values)
	return err
}

func (c *Client) DeletePruneJob(ctx context.Context, id string) error {
	path := fmt.Sprintf("/api2/json/config/prune/%s", neturl.PathEscape(strings.TrimSpace(id)))
	_, err := c.requestRaw(ctx, http.MethodDelete, path, nil)
	return err
}

func (c *Client) RunPruneJob(ctx context.Context, id string) (string, error) {
	path := fmt.Sprintf("/api2/json/admin/prune/%s/run", neturl.PathEscape(strings.TrimSpace(id)))
	return c.postTask(ctx, path, nil)
}

// --- Sync Jobs ---

func (c *Client) ListSyncJobs(ctx context.Context) ([]SyncJob, error) {
	var out []SyncJob
	if err := c.getData(ctx, "/api2/json/config/sync", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) CreateSyncJob(ctx context.Context, values neturl.Values) error {
	_, err := c.requestRaw(ctx, http.MethodPost, "/api2/json/config/sync", values)
	return err
}

func (c *Client) UpdateSyncJob(ctx context.Context, id string, values neturl.Values) error {
	path := fmt.Sprintf("/api2/json/config/sync/%s", neturl.PathEscape(strings.TrimSpace(id)))
	_, err := c.requestRaw(ctx, http.MethodPut, path, values)
	return err
}

func (c *Client) DeleteSyncJob(ctx context.Context, id string) error {
	path := fmt.Sprintf("/api2/json/config/sync/%s", neturl.PathEscape(strings.TrimSpace(id)))
	_, err := c.requestRaw(ctx, http.MethodDelete, path, nil)
	return err
}

func (c *Client) RunSyncJob(ctx context.Context, id string) (string, error) {
	path := fmt.Sprintf("/api2/json/admin/sync/%s/run", neturl.PathEscape(strings.TrimSpace(id)))
	return c.postTask(ctx, path, nil)
}

// --- Remotes ---

func (c *Client) ListRemotes(ctx context.Context) ([]Remote, error) {
	var out []Remote
	if err := c.getData(ctx, "/api2/json/config/remote", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) CreateRemote(ctx context.Context, values neturl.Values) error {
	_, err := c.requestRaw(ctx, http.MethodPost, "/api2/json/config/remote", values)
	return err
}

// --- Traffic Control ---

func (c *Client) ListTrafficControl(ctx context.Context) ([]TrafficRule, error) {
	var out []TrafficRule
	if err := c.getData(ctx, "/api2/json/config/traffic-control", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) CreateTrafficControl(ctx context.Context, values neturl.Values) error {
	_, err := c.requestRaw(ctx, http.MethodPost, "/api2/json/config/traffic-control", values)
	return err
}

func (c *Client) UpdateTrafficControl(ctx context.Context, name string, values neturl.Values) error {
	path := fmt.Sprintf("/api2/json/config/traffic-control/%s", neturl.PathEscape(strings.TrimSpace(name)))
	_, err := c.requestRaw(ctx, http.MethodPut, path, values)
	return err
}

func (c *Client) DeleteTrafficControl(ctx context.Context, name string) error {
	path := fmt.Sprintf("/api2/json/config/traffic-control/%s", neturl.PathEscape(strings.TrimSpace(name)))
	_, err := c.requestRaw(ctx, http.MethodDelete, path, nil)
	return err
}

// --- Snapshot/Group actions ---

// ForgetSnapshot deletes a single snapshot.
// Calls DELETE /api2/json/admin/datastore/{store}/snapshots with query params.
func (c *Client) ForgetSnapshot(ctx context.Context, store, backupType, backupID string, backupTime int64) error {
	path := fmt.Sprintf(
		"/api2/json/admin/datastore/%s/snapshots?backup-type=%s&backup-id=%s&backup-time=%d",
		neturl.PathEscape(strings.TrimSpace(store)),
		neturl.QueryEscape(strings.TrimSpace(backupType)),
		neturl.QueryEscape(strings.TrimSpace(backupID)),
		backupTime,
	)
	_, err := c.requestRaw(ctx, http.MethodDelete, path, nil)
	return err
}

// ForgetGroup deletes all snapshots in a backup group.
// Calls DELETE /api2/json/admin/datastore/{store}/groups with query params.
func (c *Client) ForgetGroup(ctx context.Context, store, backupType, backupID string) error {
	path := fmt.Sprintf(
		"/api2/json/admin/datastore/%s/groups?backup-type=%s&backup-id=%s",
		neturl.PathEscape(strings.TrimSpace(store)),
		neturl.QueryEscape(strings.TrimSpace(backupType)),
		neturl.QueryEscape(strings.TrimSpace(backupID)),
	)
	_, err := c.requestRaw(ctx, http.MethodDelete, path, nil)
	return err
}

// --- Datastore maintenance ---

// SetDatastoreMaintenanceMode sets or clears the maintenance mode for a datastore.
// Use mode="" to clear, or mode="read-only"/"offline" to set.
func (c *Client) SetDatastoreMaintenanceMode(ctx context.Context, store, mode string) error {
	path := fmt.Sprintf("/api2/json/config/datastore/%s", neturl.PathEscape(strings.TrimSpace(store)))
	values := neturl.Values{}
	if strings.TrimSpace(mode) == "" {
		values.Set("delete", "maintenance-mode")
	} else {
		values.Set("maintenance-mode", strings.TrimSpace(mode))
	}
	_, err := c.requestRaw(ctx, http.MethodPut, path, values)
	return err
}

// --- Certificates ---

func (c *Client) GetCertificateInfo(ctx context.Context) ([]CertInfo, error) {
	var out []CertInfo
	if err := c.getData(ctx, "/api2/json/nodes/localhost/certificates/info", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) postTask(ctx context.Context, path string, values neturl.Values) (string, error) {
	payload, err := c.requestRaw(ctx, http.MethodPost, path, values)
	if err != nil {
		return "", err
	}
	return parseDataString(payload)
}

func (c *Client) getData(ctx context.Context, path string, out any) error {
	payload, err := c.requestRaw(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	return unmarshalDataEnvelope(payload, out)
}

func (c *Client) requestRaw(ctx context.Context, method, path string, values neturl.Values) ([]byte, error) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	url := c.baseURL + path

	var body io.Reader
	if values != nil {
		body = strings.NewReader(values.Encode())
	}

	req, err := securityruntime.NewOutboundRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if values != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.Header.Set("Authorization", fmt.Sprintf("PBSAPIToken %s:%s", c.tokenID, c.tokenSecret))

	resp, err := securityruntime.DoOutboundRequest(c.httpClient, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("pbs api returned %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	return payload, nil
}

func unmarshalDataEnvelope(payload []byte, out any) error {
	var wrapped struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(payload, &wrapped); err != nil {
		return fmt.Errorf("decode pbs envelope: %w", err)
	}
	if len(wrapped.Data) == 0 || string(wrapped.Data) == "null" {
		return nil
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(wrapped.Data, out); err != nil {
		return fmt.Errorf("decode pbs data: %w", err)
	}
	return nil
}

func parseDataString(payload []byte) (string, error) {
	var wrapped struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(payload, &wrapped); err != nil {
		return "", fmt.Errorf("decode pbs envelope: %w", err)
	}
	if len(wrapped.Data) == 0 || string(wrapped.Data) == "null" {
		return "", fmt.Errorf("pbs api returned empty task response")
	}

	var value string
	if err := json.Unmarshal(wrapped.Data, &value); err == nil {
		return strings.TrimSpace(value), nil
	}

	var object map[string]any
	if err := json.Unmarshal(wrapped.Data, &object); err == nil {
		if upid := strings.TrimSpace(fmt.Sprintf("%v", object["upid"])); upid != "" && upid != "<nil>" {
			return upid, nil
		}
	}

	return strings.TrimSpace(string(wrapped.Data)), nil
}
