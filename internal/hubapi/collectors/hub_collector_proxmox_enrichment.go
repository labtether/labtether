package collectors

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/labtether/labtether/internal/connectors/proxmox"
)

func collectProxmoxGuestIdentityMetadata(
	ctx context.Context,
	client *proxmox.Client,
	resources []proxmox.Resource,
) map[string]map[string]string {
	metadataByResource := make(map[string]map[string]string)
	var mu sync.Mutex

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(4)

	for _, resource := range resources {
		resourceType := strings.ToLower(strings.TrimSpace(resource.Type))
		if resourceType != "qemu" && resourceType != "lxc" {
			continue
		}
		resource := resource
		resourceKey := proxmoxResourceIdentityKey(resource)
		if resourceKey == "" {
			continue
		}

		g.Go(func() error {
			metadata, err := loadProxmoxGuestIdentityMetadata(gctx, client, resource)
			if err != nil {
				log.Printf("hub collector proxmox: guest identity query failed for %s: %v", resourceKey, err)
				return nil
			}
			if len(metadata) == 0 {
				return nil
			}
			mu.Lock()
			metadataByResource[resourceKey] = metadata
			mu.Unlock()
			return nil
		})
	}

	_ = g.Wait()
	return metadataByResource
}

func loadProxmoxGuestIdentityMetadata(
	ctx context.Context,
	client *proxmox.Client,
	resource proxmox.Resource,
) (map[string]string, error) {
	resourceType := strings.ToLower(strings.TrimSpace(resource.Type))
	node := strings.TrimSpace(resource.Node)
	vmid := proxmoxVMIDString(resource.VMID)
	if node == "" || vmid == "" {
		return nil, nil
	}

	var (
		config map[string]any
		err    error
	)
	switch resourceType {
	case "qemu":
		config, err = client.GetQemuConfig(ctx, node, vmid)
	case "lxc":
		config, err = client.GetLXCConfig(ctx, node, vmid)
	default:
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ProxmoxGuestIdentityMetadataFromConfig(config), nil
}

func proxmoxGuestIdentityForResource(
	resource proxmox.Resource,
	metadataByResource map[string]map[string]string,
) map[string]string {
	if len(metadataByResource) == 0 {
		return nil
	}
	key := proxmoxResourceIdentityKey(resource)
	if key == "" {
		return nil
	}
	return metadataByResource[key]
}

func proxmoxResourceIdentityKey(resource proxmox.Resource) string {
	if id := strings.ToLower(strings.TrimSpace(resource.ID)); id != "" {
		return id
	}
	resourceType := strings.ToLower(strings.TrimSpace(resource.Type))
	vmid := proxmoxVMIDString(resource.VMID)
	if resourceType == "" || vmid == "" {
		return ""
	}
	return resourceType + "/" + vmid
}

func collectLatestProxmoxBackups(ctx context.Context, client *proxmox.Client, resources []proxmox.Resource) map[string]time.Time {
	latestByVMID := make(map[string]time.Time)
	storageTargets := make(map[string]struct{})

	for _, resource := range resources {
		if strings.ToLower(strings.TrimSpace(resource.Type)) != "storage" {
			continue
		}
		node := strings.TrimSpace(resource.Node)
		storage := proxmoxStorageName(resource)
		if node == "" || storage == "" {
			continue
		}
		storageTargets[node+"|"+storage] = struct{}{}
	}

	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(4)

	for target := range storageTargets {
		parts := strings.SplitN(target, "|", 2)
		if len(parts) != 2 {
			continue
		}
		node := parts[0]
		storage := parts[1]

		g.Go(func() error {
			backups, err := client.ListStorageBackups(gctx, node, storage)
			if err != nil {
				log.Printf("hub collector proxmox: backup query failed for %s/%s: %v", node, storage, err)
				return nil // non-fatal
			}

			mu.Lock()
			for _, backup := range backups {
				vmid := proxmoxVMIDString(backup.VMID)
				if vmid == "" || backup.CTime <= 0 {
					continue
				}
				backupAt := time.Unix(int64(backup.CTime), 0).UTC()
				existing, exists := latestByVMID[vmid]
				if !exists || backupAt.After(existing) {
					latestByVMID[vmid] = backupAt
				}
			}
			mu.Unlock()
			return nil
		})
	}

	_ = g.Wait()
	return latestByVMID
}
