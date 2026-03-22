package pbs

import (
	"context"
	"fmt"
	"log"
	neturl "net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/connectorsdk"
)

type Connector struct {
	client    *Client
	clientErr error
}

func New() *Connector {
	timeout := 10 * time.Second
	if raw := strings.TrimSpace(os.Getenv("PBS_HTTP_TIMEOUT")); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
			timeout = parsed
		}
	}

	skipVerify := false
	if raw := strings.TrimSpace(os.Getenv("PBS_SKIP_VERIFY")); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			skipVerify = parsed
		}
	}

	client, err := NewClient(Config{
		BaseURL:     strings.TrimSpace(os.Getenv("PBS_BASE_URL")),
		TokenID:     strings.TrimSpace(os.Getenv("PBS_TOKEN_ID")),
		TokenSecret: strings.TrimSpace(os.Getenv("PBS_TOKEN_SECRET")),
		SkipVerify:  skipVerify,
		CAPEM:       strings.TrimSpace(os.Getenv("PBS_CA_CERT_PEM")),
		Timeout:     timeout,
	})
	if err != nil {
		return &Connector{clientErr: err}
	}
	return &Connector{client: client}
}

func NewWithConfig(cfg Config) *Connector {
	client, err := NewClient(cfg)
	if err != nil {
		return &Connector{clientErr: err}
	}
	return &Connector{client: client}
}

func (c *Connector) ID() string {
	return "pbs"
}

func (c *Connector) DisplayName() string {
	return "Proxmox Backup Server"
}

func (c *Connector) Capabilities() connectorsdk.Capabilities {
	return connectorsdk.Capabilities{
		DiscoverAssets: true,
		CollectMetrics: true,
		CollectEvents:  true,
		ExecuteActions: true,
	}
}

func (c *Connector) TestConnection(ctx context.Context) (connectorsdk.Health, error) {
	if c.clientErr != nil {
		return connectorsdk.Health{Status: "failed", Message: c.clientErr.Error()}, nil
	}
	if !c.isConfigured() {
		return connectorsdk.Health{
			Status:  "ok",
			Message: "pbs connector running in stub mode (missing env config)",
		}, nil
	}

	if _, err := c.client.Ping(ctx); err != nil {
		return connectorsdk.Health{Status: "failed", Message: err.Error()}, nil
	}
	version, err := c.client.GetVersion(ctx)
	if err != nil {
		return connectorsdk.Health{Status: "failed", Message: err.Error()}, nil
	}

	release := strings.TrimSpace(version.Release)
	if release == "" {
		release = strings.TrimSpace(version.Version)
	}
	if release == "" {
		return connectorsdk.Health{Status: "ok", Message: "pbs API reachable"}, nil
	}
	return connectorsdk.Health{Status: "ok", Message: fmt.Sprintf("pbs API reachable (%s)", release)}, nil
}

func (c *Connector) Discover(ctx context.Context) ([]connectorsdk.Asset, error) {
	if !c.isConfigured() {
		return c.stubAssets(), nil
	}

	datastores, err := c.client.ListDatastores(ctx)
	if err != nil {
		return nil, fmt.Errorf("pbs list datastores: %w", err)
	}

	usageByStore := map[string]DatastoreUsage{}
	if usage, usageErr := c.client.ListDatastoreUsage(ctx); usageErr == nil {
		for _, entry := range usage {
			store := strings.TrimSpace(entry.Store)
			if store != "" {
				usageByStore[store] = entry
			}
		}
	} else {
		log.Printf("pbs: status/datastore-usage failed (continuing with datastore status calls): %v", usageErr)
	}

	rootName := c.serverLabel()
	rootID := "pbs-server-" + normalizeID(rootName)
	out := []connectorsdk.Asset{
		{
			ID:     rootID,
			Type:   "storage-controller",
			Name:   rootName,
			Source: c.ID(),
			Metadata: map[string]string{
				"connector_type":  "pbs",
				"datastore_count": strconv.Itoa(len(datastores)),
			},
		},
	}

	for _, datastore := range datastores {
		store := strings.TrimSpace(datastore.Store)
		if store == "" {
			continue
		}

		status, statusErr := c.client.GetDatastoreStatus(ctx, store, true)
		if statusErr != nil {
			log.Printf("pbs: datastore status failed for %s: %v", store, statusErr)
		}

		groups, groupsErr := c.client.ListDatastoreGroups(ctx, store)
		if groupsErr != nil {
			log.Printf("pbs: datastore groups failed for %s: %v", store, groupsErr)
		}
		snapshots, snapsErr := c.client.ListDatastoreSnapshots(ctx, store)
		if snapsErr != nil {
			log.Printf("pbs: datastore snapshots failed for %s: %v", store, snapsErr)
		}

		metadata := map[string]string{
			"store":          store,
			"mount_status":   strings.TrimSpace(datastore.MountStatus),
			"comment":        strings.TrimSpace(datastore.Comment),
			"group_count":    strconv.Itoa(len(groups)),
			"snapshot_count": strconv.Itoa(len(snapshots)),
		}
		if strings.TrimSpace(datastore.Maintenance) != "" {
			metadata["maintenance_mode"] = strings.TrimSpace(datastore.Maintenance)
		}

		usage := usageByStore[store]
		total := status.Total
		used := status.Used
		avail := status.Avail
		if total <= 0 {
			total = usage.Total
		}
		if used <= 0 {
			used = usage.Used
		}
		if avail <= 0 {
			avail = usage.Avail
		}
		if total > 0 {
			metadata["total_bytes"] = strconv.FormatInt(total, 10)
		}
		if used > 0 {
			metadata["used_bytes"] = strconv.FormatInt(used, 10)
		}
		if avail > 0 {
			metadata["avail_bytes"] = strconv.FormatInt(avail, 10)
		}
		if total > 0 && used >= 0 {
			metadata["usage_percent"] = formatPercent((float64(used) / float64(total)) * 100)
		}

		if status.GCStatus != nil {
			if upid := strings.TrimSpace(status.GCStatus.UPID); upid != "" {
				metadata["gc_last_upid"] = upid
			}
			if status.GCStatus.RemovedBytes > 0 {
				metadata["gc_removed_bytes"] = strconv.FormatInt(status.GCStatus.RemovedBytes, 10)
			}
			if status.GCStatus.PendingBytes > 0 {
				metadata["gc_pending_bytes"] = strconv.FormatInt(status.GCStatus.PendingBytes, 10)
			}
		}

		var latestBackup int64
		for _, snapshot := range snapshots {
			if snapshot.BackupTime > latestBackup {
				latestBackup = snapshot.BackupTime
			}
		}
		if latestBackup > 0 {
			backupTime := time.Unix(latestBackup, 0).UTC()
			metadata["last_backup_at"] = backupTime.Format(time.RFC3339)
			metadata["days_since_backup"] = formatPercent(time.Since(backupTime).Hours() / 24)
		}

		statusValue := "online"
		mountStatus := firstNonEmpty(strings.TrimSpace(status.MountStatus), strings.TrimSpace(datastore.MountStatus))
		if mountStatus != "" {
			metadata["mount_status"] = mountStatus
		}
		if mountStatus == "notmounted" || strings.EqualFold(strings.TrimSpace(datastore.Maintenance), "offline") {
			statusValue = "degraded"
		}

		out = append(out, connectorsdk.Asset{
			ID:       "pbs-datastore-" + normalizeID(store),
			Type:     "storage-pool",
			Name:     store,
			Source:   c.ID(),
			Metadata: metadata,
		})
		out[len(out)-1].Metadata["status"] = statusValue
	}

	return out, nil
}

func (c *Connector) Actions() []connectorsdk.ActionDescriptor {
	return []connectorsdk.ActionDescriptor{
		{
			ID:             "datastore.verify",
			Name:           "Verify Datastore",
			Description:    "Start a verify task for the specified PBS datastore.",
			RequiresTarget: false,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{Key: "store", Label: "Datastore", Required: true, Description: "Datastore name (for example backup-store)."},
			},
		},
		{
			ID:             "datastore.prune",
			Name:           "Prune Datastore",
			Description:    "Start a prune task for the specified PBS datastore.",
			RequiresTarget: false,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{Key: "store", Label: "Datastore", Required: true, Description: "Datastore name (for example backup-store)."},
			},
		},
		{
			ID:             "datastore.gc",
			Name:           "Run Garbage Collection",
			Description:    "Start a GC task for the specified PBS datastore.",
			RequiresTarget: false,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{Key: "store", Label: "Datastore", Required: true, Description: "Datastore name (for example backup-store)."},
			},
		},
	}
}

func (c *Connector) ExecuteAction(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	if !c.isConfigured() {
		return connectorsdk.ActionResult{
			Status:  "failed",
			Message: "pbs connector not configured (missing base URL or API token)",
		}, nil
	}

	store := firstNonEmpty(strings.TrimSpace(req.Params["store"]), strings.TrimSpace(req.TargetID))
	if store == "" {
		return connectorsdk.ActionResult{Status: "failed", Message: "store is required"}, nil
	}

	if req.DryRun {
		return connectorsdk.ActionResult{
			Status:  "succeeded",
			Message: "dry-run: action validated",
			Output:  fmt.Sprintf("would run %s on datastore %q", actionID, store),
		}, nil
	}

	var (
		upid string
		err  error
	)
	switch actionID {
	case "datastore.verify":
		upid, err = c.client.StartVerify(ctx, store)
	case "datastore.prune":
		upid, err = c.client.StartPruneDatastore(ctx, store, PruneOptions{})
	case "datastore.gc":
		upid, err = c.client.StartGC(ctx, store)
	default:
		return connectorsdk.ActionResult{Status: "failed", Message: "unsupported action"}, nil
	}
	if err != nil {
		return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
	}
	return connectorsdk.ActionResult{
		Status:  "succeeded",
		Message: fmt.Sprintf("%s started", actionID),
		Metadata: map[string]string{
			"store": store,
			"upid":  strings.TrimSpace(upid),
		},
	}, nil
}

func (c *Connector) isConfigured() bool {
	return c.client != nil && c.client.IsConfigured()
}

func (c *Connector) stubAssets() []connectorsdk.Asset {
	return []connectorsdk.Asset{
		{
			ID:     "pbs-server-stub",
			Type:   "storage-controller",
			Name:   "pbs-stub",
			Source: c.ID(),
			Metadata: map[string]string{
				"note": "stub mode - configure PBS_BASE_URL, PBS_TOKEN_ID, and PBS_TOKEN_SECRET",
			},
		},
	}
}

func (c *Connector) serverLabel() string {
	if c == nil || c.client == nil {
		return "pbs"
	}
	baseURL := strings.TrimSpace(c.client.baseURL)
	if baseURL == "" {
		return "pbs"
	}
	parsed, err := neturl.Parse(baseURL)
	if err != nil {
		return "pbs"
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return "pbs"
	}
	return host
}

func normalizeID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-", ":", "-", ".", "-")
	return replacer.Replace(value)
}

func formatPercent(value float64) string {
	if value < 0 {
		value = 0
	}
	if value > 100 {
		value = 100
	}
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
