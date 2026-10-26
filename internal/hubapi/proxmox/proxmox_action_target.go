package proxmox

import (
	"fmt"
	"strconv"
	"strings"

	proxmoxconnector "github.com/labtether/labtether/internal/connectors/proxmox"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/modelmap"
)

func (d *Deps) ResolveProxmoxActionDestination(actionID, target string, params map[string]string) (node, vmid, collectorID string, err error) {
	actionID = resolveProxmoxActionID(actionID, target)
	node, vmid, collectorID, err = d.ResolveProxmoxActionTarget(actionID, target)
	if err != nil {
		return "", "", "", err
	}
	parsedVMID, parseErr := strconv.Atoi(vmid)
	if parseErr != nil || parsedVMID <= 0 {
		return "", "", "", fmt.Errorf("invalid Proxmox VM ID")
	}
	vmid = strconv.Itoa(parsedVMID)
	pinnedCollector := strings.TrimSpace(params["collector_id"])
	if strings.TrimSpace(collectorID) != "" && pinnedCollector != "" && collectorID != pinnedCollector {
		return "", "", "", fmt.Errorf("Proxmox action target collector changed")
	}
	if strings.TrimSpace(collectorID) == "" {
		collectorID = pinnedCollector
	}
	collectorID, err = d.resolveProxmoxActionCollectorID(collectorID)
	if err != nil {
		return "", "", "", err
	}
	return node, vmid, collectorID, nil
}

func (d *Deps) resolveProxmoxActionCollectorID(collectorID string) (string, error) {
	collectorID = strings.TrimSpace(collectorID)
	if collectorID != "" {
		return collectorID, nil
	}
	if d.HubCollectorStore == nil {
		return "", nil
	}

	collectors, err := d.HubCollectorStore.ListHubCollectors(200, true)
	if err != nil {
		return "", fmt.Errorf("failed to list hub collectors: %w", err)
	}

	proxmoxCollectorID := ""
	proxmoxCollectorCount := 0
	for _, collector := range collectors {
		if collector.CollectorType != hubcollector.CollectorTypeProxmox {
			continue
		}
		proxmoxCollectorCount++
		if proxmoxCollectorID == "" {
			proxmoxCollectorID = strings.TrimSpace(collector.ID)
		}
		if proxmoxCollectorCount > 1 {
			return "", fmt.Errorf("collector_id is required when multiple proxmox collectors are configured")
		}
	}

	return proxmoxCollectorID, nil
}

func (d *Deps) ResolveProxmoxActionTarget(actionID, target string) (node, vmid, collectorID string, err error) {
	actionID = strings.TrimSpace(actionID)
	target = strings.TrimSpace(target)
	if target == "" {
		return "", "", "", fmt.Errorf("target is required")
	}

	if strings.Contains(target, "/") {
		parts := strings.SplitN(target, "/", 2)
		node = strings.TrimSpace(parts[0])
		vmid = strings.TrimSpace(parts[1])
		if node == "" || vmid == "" {
			return "", "", "", fmt.Errorf("target must be node/vmid")
		}
		return node, vmid, "", nil
	}

	expectedKind := ""
	switch {
	case strings.HasPrefix(actionID, "vm."):
		expectedKind = "qemu"
	case strings.HasPrefix(actionID, "ct."):
		expectedKind = "lxc"
	default:
		return "", "", "", fmt.Errorf("unsupported proxmox action prefix: %s", actionID)
	}

	resolved, ok, resolveErr := d.ResolveProxmoxSessionTarget(target)
	if resolveErr != nil {
		return "", "", "", resolveErr
	}
	if !ok {
		return "", "", "", fmt.Errorf("target %s is not a proxmox asset", target)
	}
	if err := ValidateResolvedProxmoxActionTarget(resolved, expectedKind, actionID); err != nil {
		return "", "", "", err
	}
	return strings.TrimSpace(resolved.Node), strings.TrimSpace(resolved.VMID), strings.TrimSpace(resolved.CollectorID), nil
}

func ValidateResolvedProxmoxActionTarget(resolved ProxmoxSessionTarget, expectedKind, actionID string) error {
	if strings.ToLower(strings.TrimSpace(resolved.Kind)) != expectedKind {
		return fmt.Errorf("target kind %s does not match action %s", resolved.Kind, actionID)
	}
	if strings.TrimSpace(resolved.Node) == "" || strings.TrimSpace(resolved.VMID) == "" {
		return fmt.Errorf("proxmox asset target is incomplete")
	}
	return nil
}

func resolveProxmoxActionID(actionID, target string) string {
	return modelmap.ResolveActionID(actionID, target, (&proxmoxconnector.Connector{}).Actions())
}
