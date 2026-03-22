package truenas

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/labtether/labtether/internal/connectorsdk"
)

func (c *Connector) Discover(ctx context.Context) ([]connectorsdk.Asset, error) {
	if !c.isConfigured() {
		return c.stubAssets(), nil
	}

	assets := make([]connectorsdk.Asset, 0, 32)

	// --- NAS host (non-fatal, from system.info) ---
	// Host asset is built after pools and disks so we can enrich it with
	// aggregate disk usage and max disk temperature.
	var sysInfo map[string]any
	if err := c.client.Call(ctx, "system.info", nil, &sysInfo); err != nil {
		log.Printf("truenas: system.info failed (skipping host asset): %v", err)
	}

	// --- storage pools (fatal if unavailable) ---
	var pools []map[string]any
	if err := c.callQuery(ctx, "pool.query", &pools); err != nil {
		return nil, fmt.Errorf("truenas pool.query: %w", err)
	}
	for _, pool := range pools {
		poolID := strings.TrimSpace(anyToString(pool["id"]))
		poolName := strings.TrimSpace(anyToString(pool["name"]))
		if poolName == "" && poolID == "" {
			continue
		}
		if poolName == "" {
			poolName = "pool-" + poolID
		}
		metadata := map[string]string{
			"pool_id":       poolID,
			"status":        strings.TrimSpace(anyToString(pool["status"])),
			"healthy":       anyToString(pool["healthy"]),
			"size_bytes":    formatFloat(anyToFloat(pool["size"])),
			"alloc_bytes":   formatFloat(anyToFloat(pool["allocated"])),
			"free_bytes":    formatFloat(anyToFloat(pool["free"])),
			"fragmentation": strings.TrimSpace(anyToString(pool["fragmentation"])),
		}

		allocBytes := anyToFloat(pool["allocated"])
		sizeBytes := anyToFloat(pool["size"])
		if sizeBytes > 0 {
			usedPercent := clampPercent((allocBytes / sizeBytes) * 100)
			metadata["disk_used_percent"] = formatFloat(usedPercent)
		}

		if scan, ok := pool["scan"].(map[string]any); ok {
			metadata["last_scrub_state"] = strings.TrimSpace(anyToString(scan["state"]))
			metadata["last_scrub_errors"] = anyToString(scan["errors"])
		}

		assets = append(assets, connectorsdk.Asset{
			ID:       "truenas-storage-pool-" + normalizeID(poolName),
			Type:     "storage-pool",
			Name:     poolName,
			Source:   c.ID(),
			Metadata: metadata,
		})
	}

	// --- datasets (non-fatal) ---
	var datasets []map[string]any
	if err := c.callQuery(ctx, "pool.dataset.query", &datasets); err != nil {
		log.Printf("truenas: pool.dataset.query failed (skipping datasets): %v", err)
	} else {
		for _, ds := range datasets {
			name := strings.TrimSpace(anyToString(ds["name"]))
			if name == "" {
				continue
			}
			assets = append(assets, connectorsdk.Asset{
				ID:     "truenas-dataset-" + normalizeID(name),
				Type:   "dataset",
				Name:   name,
				Source: c.ID(),
				Metadata: map[string]string{
					"mountpoint":  strings.TrimSpace(anyToString(nestedValue(ds, "mountpoint"))),
					"used":        formatFloat(anyToFloat(nestedValue(ds, "used"))),
					"available":   formatFloat(anyToFloat(nestedValue(ds, "available"))),
					"quota":       formatFloat(anyToFloat(nestedValue(ds, "quota"))),
					"readonly":    anyToString(nestedValue(ds, "readonly")),
					"compression": strings.TrimSpace(anyToString(nestedValue(ds, "compression"))),
				},
			})
		}
	}

	// --- disks (non-fatal) ---
	var disks []map[string]any
	if err := c.callQuery(ctx, "disk.query", &disks); err != nil {
		log.Printf("truenas: disk.query failed (skipping disks): %v", err)
	} else {
		// Fetch disk temperatures to enrich disk assets.
		var diskTemps map[string]any
		if err := c.client.Call(ctx, "disk.temperatures", nil, &diskTemps); err != nil {
			log.Printf("truenas: disk.temperatures failed (skipping temps): %v", err)
		}

		for _, disk := range disks {
			name := strings.TrimSpace(anyToString(disk["name"]))
			if name == "" {
				continue
			}
			diskMeta := map[string]string{
				"serial": strings.TrimSpace(anyToString(disk["serial"])),
				"size":   formatFloat(anyToFloat(disk["size"])),
				"model":  strings.TrimSpace(anyToString(disk["model"])),
				"type":   strings.TrimSpace(anyToString(disk["type"])),
			}
			if diskTemps != nil {
				if temp := anyToFloat(diskTemps[name]); temp > 0 {
					diskMeta["temperature_celsius"] = formatFloat(temp)
				}
			}
			assets = append(assets, connectorsdk.Asset{
				ID:       "truenas-disk-" + normalizeID(name),
				Type:     "disk",
				Name:     name,
				Source:   c.ID(),
				Metadata: diskMeta,
			})
		}
	}

	// --- build NAS host asset (deferred to here so we can enrich with pool/disk aggregates) ---
	if sysInfo != nil {
		hostname := strings.TrimSpace(anyToString(sysInfo["hostname"]))
		if hostname == "" {
			hostname = "truenas"
		}
		cores := anyToFloat(sysInfo["cores"])

		// Derive CPU percent from 1-min load average / core count.
		cpuPercent := 0.0
		if loadavg, ok := sysInfo["loadavg"].([]any); ok && len(loadavg) > 0 && cores > 0 {
			load1 := anyToFloat(loadavg[0])
			cpuPercent = clampPercent((load1 / cores) * 100)
		}

		meta := map[string]string{
			"hostname":         hostname,
			"version":          strings.TrimSpace(anyToString(sysInfo["version"])),
			"model":            strings.TrimSpace(anyToString(sysInfo["model"])),
			"cores":            formatFloat(cores),
			"physmem":          formatFloat(anyToFloat(sysInfo["physmem"])),
			"uptime":           strings.TrimSpace(anyToString(sysInfo["uptime"])),
			"ecc_memory":       anyToString(sysInfo["ecc_memory"]),
			"cpu_used_percent": formatFloat(cpuPercent),
		}

		// Aggregate disk_used_percent from pool data.
		var totalPoolSize, totalPoolAlloc float64
		for _, pool := range pools {
			totalPoolSize += anyToFloat(pool["size"])
			totalPoolAlloc += anyToFloat(pool["allocated"])
		}
		if totalPoolSize > 0 {
			meta["disk_used_percent"] = formatFloat(clampPercent((totalPoolAlloc / totalPoolSize) * 100))
		}

		// Max disk temperature across all physical disks.
		var maxTemp float64
		for _, a := range assets {
			if a.Type != "disk" {
				continue
			}
			if tempStr, ok := a.Metadata["temperature_celsius"]; ok {
				if t := parseFloat(tempStr); t > maxTemp {
					maxTemp = t
				}
			}
		}
		if maxTemp > 0 {
			meta["temperature_celsius"] = formatFloat(maxTemp)
		}

		assets = append(assets, connectorsdk.Asset{
			ID:       "truenas-host-" + normalizeID(hostname),
			Type:     "nas",
			Name:     hostname,
			Source:   c.ID(),
			Metadata: meta,
		})
	}

	// --- SMB shares (non-fatal) ---
	var smbShares []map[string]any
	if err := c.callQuery(ctx, "sharing.smb.query", &smbShares); err != nil {
		log.Printf("truenas: sharing.smb.query failed (skipping SMB shares): %v", err)
	} else {
		for _, share := range smbShares {
			name := strings.TrimSpace(anyToString(share["name"]))
			if name == "" {
				name = strings.TrimSpace(anyToString(share["id"]))
			}
			if name == "" {
				continue
			}
			assets = append(assets, connectorsdk.Asset{
				ID:     "truenas-share-smb-" + normalizeID(name),
				Type:   "share-smb",
				Name:   name,
				Source: c.ID(),
				Metadata: map[string]string{
					"path":    strings.TrimSpace(anyToString(share["path"])),
					"enabled": anyToString(share["enabled"]),
					"comment": strings.TrimSpace(anyToString(share["comment"])),
				},
			})
		}
	}

	// --- NFS shares (non-fatal) ---
	var nfsShares []map[string]any
	if err := c.callQuery(ctx, "sharing.nfs.query", &nfsShares); err != nil {
		log.Printf("truenas: sharing.nfs.query failed (skipping NFS shares): %v", err)
	} else {
		for _, share := range nfsShares {
			// NFS shares use paths as identifiers.
			path := strings.TrimSpace(anyToString(share["path"]))
			id := strings.TrimSpace(anyToString(share["id"]))
			name := path
			if name == "" {
				if id != "" {
					name = "nfs-" + id
				}
			}
			if name == "" {
				continue
			}
			assets = append(assets, connectorsdk.Asset{
				ID:     "truenas-share-nfs-" + normalizeID(name),
				Type:   "share-nfs",
				Name:   name,
				Source: c.ID(),
				Metadata: map[string]string{
					"path":    path,
					"enabled": anyToString(share["enabled"]),
				},
			})
		}
	}

	// --- services (non-fatal) ---
	var services []map[string]any
	if err := c.callQuery(ctx, "service.query", &services); err != nil {
		log.Printf("truenas: service.query failed (skipping services): %v", err)
	} else {
		for _, svc := range services {
			name := strings.TrimSpace(anyToString(svc["service"]))
			if name == "" {
				continue
			}
			state := strings.TrimSpace(anyToString(svc["state"]))
			enabled := anyToString(svc["enable"])
			assets = append(assets, connectorsdk.Asset{
				ID:     "truenas-service-" + normalizeID(name),
				Type:   "service",
				Name:   name,
				Source: c.ID(),
				Metadata: map[string]string{
					"state":   strings.ToLower(state),
					"enabled": enabled,
				},
			})
		}
	}

	// --- VMs (SCALE-only, non-fatal) ---
	var vms []map[string]any
	if err := c.callQuery(ctx, "vm.query", &vms); err != nil {
		if !IsMethodNotFound(err) {
			log.Printf("truenas: vm.query failed (skipping VMs): %v", err)
		}
		// IsMethodNotFound means TrueNAS CORE — silently skip.
	} else {
		for _, vm := range vms {
			name := strings.TrimSpace(anyToString(vm["name"]))
			vmID := strings.TrimSpace(anyToString(vm["id"]))
			if name == "" {
				if vmID != "" {
					name = "vm-" + vmID
				}
			}
			if name == "" {
				continue
			}

			statusVal := vm["status"]
			statusStr := ""
			if sm, ok := statusVal.(map[string]any); ok {
				statusStr = strings.TrimSpace(anyToString(sm["state"]))
			} else {
				statusStr = strings.TrimSpace(anyToString(statusVal))
			}

			assets = append(assets, connectorsdk.Asset{
				ID:     "truenas-vm-" + normalizeID(name),
				Type:   "vm",
				Name:   name,
				Source: c.ID(),
				Metadata: map[string]string{
					"vm_id":     vmID,
					"status":    statusStr,
					"vcpus":     anyToString(vm["vcpus"]),
					"memory":    anyToString(vm["memory"]),
					"autostart": anyToString(vm["autostart"]),
				},
			})
		}
	}

	// --- Apps (SCALE-only, non-fatal) ---
	var apps []map[string]any
	if err := c.callQuery(ctx, "app.query", &apps); err != nil {
		if !IsMethodNotFound(err) {
			log.Printf("truenas: app.query failed (skipping apps): %v", err)
		}
		// IsMethodNotFound means TrueNAS CORE — silently skip.
	} else {
		for _, app := range apps {
			name := strings.TrimSpace(anyToString(app["name"]))
			if name == "" {
				continue
			}
			assets = append(assets, connectorsdk.Asset{
				ID:     "truenas-app-" + normalizeID(name),
				Type:   "app",
				Name:   name,
				Source: c.ID(),
				Metadata: map[string]string{
					"state":   strings.TrimSpace(anyToString(app["state"])),
					"version": strings.TrimSpace(anyToString(app["version"])),
				},
			})
		}
	}

	if len(assets) == 0 {
		return c.stubAssets(), nil
	}
	return assets, nil
}
