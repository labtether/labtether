package proxmox

import (
	"context"
	"strings"

	"github.com/labtether/labtether/internal/connectorsdk"
)

func (c *Connector) Discover(ctx context.Context) ([]connectorsdk.Asset, error) {
	if !c.isConfigured() {
		return c.stubAssets(), nil
	}

	resources, err := c.client.GetClusterResources(ctx)
	if err != nil {
		return nil, err
	}

	assets := make([]connectorsdk.Asset, 0, len(resources))
	for _, resource := range resources {
		switch strings.ToLower(strings.TrimSpace(resource.Type)) {
		case "node":
			node := strings.TrimSpace(resource.Node)
			if node == "" {
				node = strings.TrimSpace(resource.Name)
			}
			if node == "" {
				continue
			}
			assets = append(assets, connectorsdk.Asset{
				ID:     "proxmox-node-" + normalizeID(node),
				Type:   "hypervisor-node",
				Name:   node,
				Source: c.ID(),
				Metadata: map[string]string{
					"node":    node,
					"status":  strings.TrimSpace(resource.Status),
					"cpu":     formatFloat(resource.CPU),
					"maxmem":  formatFloat(resource.MaxMem),
					"maxdisk": formatFloat(resource.MaxDisk),
				},
			})
		case "qemu":
			node := strings.TrimSpace(resource.Node)
			vmid := vmidString(resource.VMID)
			if vmid == "" || node == "" {
				continue
			}
			name := strings.TrimSpace(resource.Name)
			if name == "" {
				name = "vm-" + vmid
			}
			assets = append(assets, connectorsdk.Asset{
				ID:     "proxmox-vm-" + vmid,
				Type:   "vm",
				Name:   name,
				Source: c.ID(),
				Metadata: map[string]string{
					"node":     node,
					"vmid":     vmid,
					"status":   strings.TrimSpace(resource.Status),
					"template": anyToString(resource.Template),
					"hastate":  strings.TrimSpace(resource.HAState),
				},
			})
		case "lxc":
			node := strings.TrimSpace(resource.Node)
			vmid := vmidString(resource.VMID)
			if vmid == "" || node == "" {
				continue
			}
			name := strings.TrimSpace(resource.Name)
			if name == "" {
				name = "ct-" + vmid
			}
			assets = append(assets, connectorsdk.Asset{
				ID:     "proxmox-ct-" + vmid,
				Type:   "container",
				Name:   name,
				Source: c.ID(),
				Metadata: map[string]string{
					"node":     node,
					"vmid":     vmid,
					"status":   strings.TrimSpace(resource.Status),
					"template": anyToString(resource.Template),
					"hastate":  strings.TrimSpace(resource.HAState),
				},
			})
		case "storage":
			storageID := strings.TrimSpace(resource.ID)
			if storageID == "" {
				storageID = strings.TrimSpace(resource.Name)
			}
			if storageID == "" {
				continue
			}
			displayName := strings.TrimSpace(resource.Name)
			if displayName == "" {
				displayName = storageID
			}
			metadata := map[string]string{
				"storage_id": storageID,
				"status":     strings.TrimSpace(resource.Status),
			}
			if strings.TrimSpace(resource.Node) != "" {
				metadata["node"] = strings.TrimSpace(resource.Node)
			}
			if strings.TrimSpace(resource.PlugInType) != "" {
				metadata["plugintype"] = strings.TrimSpace(resource.PlugInType)
			}
			if strings.TrimSpace(resource.Content) != "" {
				metadata["content"] = strings.TrimSpace(resource.Content)
			}

			assets = append(assets, connectorsdk.Asset{
				ID:       "proxmox-storage-" + normalizeID(storageID),
				Type:     "storage-pool",
				Name:     displayName,
				Source:   c.ID(),
				Metadata: metadata,
			})
		}
	}

	if len(assets) == 0 {
		return c.stubAssets(), nil
	}
	return assets, nil
}
func (c *Connector) stubAssets() []connectorsdk.Asset {
	return []connectorsdk.Asset{
		{
			ID:     "proxmox-node-pve01",
			Type:   "hypervisor-node",
			Name:   "pve01",
			Source: c.ID(),
			Metadata: map[string]string{
				"cluster": "homelab",
				"status":  "online",
			},
		},
		{
			ID:     "proxmox-vm-100",
			Type:   "vm",
			Name:   "labtether-dev",
			Source: c.ID(),
			Metadata: map[string]string{
				"node": "pve01",
				"vmid": "100",
			},
		},
		{
			ID:     "proxmox-ct-101",
			Type:   "container",
			Name:   "monitoring-ct",
			Source: c.ID(),
			Metadata: map[string]string{
				"node": "pve01",
				"vmid": "101",
			},
		},
	}
}
