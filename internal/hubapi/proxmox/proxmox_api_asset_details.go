package proxmox

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/labtether/labtether/internal/connectors/proxmox"
)

func (d *Deps) LoadProxmoxAssetDetails(ctx context.Context, assetID string, target ProxmoxSessionTarget, runtime *ProxmoxRuntime) (ProxmoxAssetDetailsResponse, error) {
	resp := ProxmoxAssetDetailsResponse{
		AssetID:     strings.TrimSpace(assetID),
		Kind:        strings.TrimSpace(target.Kind),
		Node:        strings.TrimSpace(target.Node),
		VMID:        strings.TrimSpace(target.VMID),
		CollectorID: strings.TrimSpace(runtime.collectorID),
		Config:      map[string]any{},
		Snapshots:   make([]proxmox.Snapshot, 0),
		Tasks:       make([]proxmox.Task, 0),
		HA:          ProxmoxAssetHAView{Resources: make([]proxmox.HAResource, 0)},
		Warnings:    make([]string, 0),
		FetchedAt:   time.Now().UTC().Format(time.RFC3339),
	}

	var mu sync.Mutex
	addWarning := func(msg string) {
		mu.Lock()
		resp.Warnings = append(resp.Warnings, msg)
		mu.Unlock()
	}

	g, gctx := errgroup.WithContext(ctx)

	// 1. Version (non-fatal).
	g.Go(func() error {
		if release, err := runtime.client.GetVersion(gctx); err == nil {
			mu.Lock()
			resp.Version = strings.TrimSpace(release)
			mu.Unlock()
		} else {
			addWarning("version unavailable: " + err.Error())
		}
		return nil
	})

	// 2. Config (fatal on error — matches prior behavior).
	g.Go(func() error {
		var config map[string]any
		var configErr error
		switch resp.Kind {
		case "node":
			config, configErr = runtime.client.GetNodeStatus(gctx, resp.Node)
		case "qemu":
			config, configErr = runtime.client.GetQemuConfig(gctx, resp.Node, resp.VMID)
		case "lxc":
			config, configErr = runtime.client.GetLXCConfig(gctx, resp.Node, resp.VMID)
		case "storage":
			if target.StorageName != "" {
				config, configErr = runtime.client.GetStorageStatus(gctx, resp.Node, target.StorageName)
			} else {
				addWarning("storage name unavailable")
			}
		default:
			addWarning("unsupported proxmox kind: " + resp.Kind)
		}
		if configErr != nil {
			return configErr
		}
		if config != nil {
			mu.Lock()
			resp.Config = config
			mu.Unlock()
		}
		return nil
	})

	// 3. Snapshots (non-fatal).
	if resp.Kind == "qemu" || resp.Kind == "lxc" {
		g.Go(func() error {
			var snapshots []proxmox.Snapshot
			var err error
			if resp.Kind == "qemu" {
				snapshots, err = runtime.client.ListQemuSnapshots(gctx, resp.Node, resp.VMID)
			} else {
				snapshots, err = runtime.client.ListLXCSnapshots(gctx, resp.Node, resp.VMID)
			}
			if err != nil {
				addWarning("snapshots unavailable: " + err.Error())
			} else {
				mu.Lock()
				resp.Snapshots = SortProxmoxSnapshots(snapshots)
				mu.Unlock()
			}
			return nil
		})
	}

	// 4. Tasks (non-fatal).
	g.Go(func() error {
		tasks, err := runtime.client.ListClusterTasks(gctx, resp.Node, resp.VMID, 60)
		if err != nil {
			addWarning("tasks unavailable: " + err.Error())
		} else {
			mu.Lock()
			resp.Tasks = FilterAndSortProxmoxTasks(tasks, resp.Node, resp.VMID, 30)
			mu.Unlock()
		}
		return nil
	})

	// 5. HA Resources (non-fatal).
	g.Go(func() error {
		haResources, err := runtime.client.ListHAResources(gctx)
		if err != nil {
			addWarning("ha resources unavailable: " + err.Error())
		} else {
			match, related := SelectProxmoxHA(haResources, target)
			mu.Lock()
			resp.HA.Match = match
			resp.HA.Resources = related
			mu.Unlock()
		}
		return nil
	})

	// 6. Firewall rules (non-fatal).
	if resp.Kind == "node" || resp.Kind == "qemu" || resp.Kind == "lxc" {
		g.Go(func() error {
			var rules []proxmox.FirewallRule
			var err error
			switch resp.Kind {
			case "node":
				rules, err = runtime.client.GetNodeFirewallRules(gctx, resp.Node)
			case "qemu":
				rules, err = runtime.client.GetVMFirewallRules(gctx, resp.Node, resp.VMID, "qemu")
			case "lxc":
				rules, err = runtime.client.GetVMFirewallRules(gctx, resp.Node, resp.VMID, "lxc")
			}
			if err != nil {
				addWarning("firewall rules unavailable: " + err.Error())
			} else if len(rules) > 0 {
				mu.Lock()
				resp.FirewallRules = rules
				mu.Unlock()
			}
			return nil
		})
	}

	// 7. Backup schedules (non-fatal, cluster-level — relevant to all asset types).
	g.Go(func() error {
		schedules, err := runtime.client.GetBackupSchedules(gctx)
		if err != nil {
			addWarning("backup schedules unavailable: " + err.Error())
		} else if len(schedules) > 0 {
			mu.Lock()
			resp.BackupSchedules = schedules
			mu.Unlock()
		}
		return nil
	})

	// 8. Ceph cluster status (non-fatal — cluster may not have Ceph).
	g.Go(func() error {
		cephStatus, err := runtime.client.GetCephStatus(gctx)
		if err != nil {
			// Ceph not available — silently skip (not worth a warning).
			return nil
		}
		mu.Lock()
		resp.CephStatus = cephStatus
		mu.Unlock()
		return nil
	})

	// 9. Ceph OSDs (non-fatal).
	g.Go(func() error {
		osds, err := runtime.client.GetCephOSDs(gctx)
		if err != nil {
			// Ceph not available — silently skip.
			return nil
		}
		if len(osds) > 0 {
			mu.Lock()
			resp.CephOSDs = osds
			mu.Unlock()
		}
		return nil
	})

	// 10. ZFS pools (non-fatal, node-type assets only).
	if resp.Kind == "node" {
		g.Go(func() error {
			pools, err := runtime.client.GetNodeZFSPools(gctx, resp.Node)
			if err != nil {
				addWarning("zfs pools unavailable: " + err.Error())
			} else if len(pools) > 0 {
				mu.Lock()
				resp.ZFSPools = pools
				mu.Unlock()
			}
			return nil
		})
	}

	// 11. Storage content (non-fatal, storage-kind assets only).
	if resp.Kind == "storage" && resp.Node != "" && target.StorageName != "" {
		g.Go(func() error {
			content, err := runtime.client.GetStorageContent(gctx, resp.Node, target.StorageName)
			if err != nil {
				addWarning("storage content unavailable: " + err.Error())
			} else if len(content) > 0 {
				// Sort by content type, then by volid.
				sort.Slice(content, func(i, j int) bool {
					if content[i].Content != content[j].Content {
						return content[i].Content < content[j].Content
					}
					return content[i].VolID < content[j].VolID
				})
				mu.Lock()
				resp.StorageContent = content
				mu.Unlock()
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return ProxmoxAssetDetailsResponse{}, err
	}

	if len(resp.Warnings) == 0 {
		resp.Warnings = nil
	}
	if len(resp.Snapshots) == 0 {
		resp.Snapshots = nil
	}
	if len(resp.Tasks) == 0 {
		resp.Tasks = nil
	}
	if len(resp.HA.Resources) == 0 {
		resp.HA.Resources = nil
	}
	if len(resp.FirewallRules) == 0 {
		resp.FirewallRules = nil
	}
	if len(resp.BackupSchedules) == 0 {
		resp.BackupSchedules = nil
	}
	if len(resp.CephOSDs) == 0 {
		resp.CephOSDs = nil
	}
	if len(resp.ZFSPools) == 0 {
		resp.ZFSPools = nil
	}
	if len(resp.StorageContent) == 0 {
		resp.StorageContent = nil
	}
	return resp, nil
}
