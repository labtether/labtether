package proxmox

import (
	"github.com/labtether/labtether/internal/hubapi/shared"
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/connectors/proxmox"
	"github.com/labtether/labtether/internal/servicehttp"
)

// handleProxmoxAssetFirewall handles firewall CRUD for /proxmox/assets/{id}/firewall.
//
//   GET    /proxmox/assets/{id}/firewall       — list rules (redirects to details data)
//   POST   /proxmox/assets/{id}/firewall       — create rule
//   PUT    /proxmox/assets/{id}/firewall/{pos} — update rule at position
//   DELETE /proxmox/assets/{id}/firewall/{pos} — delete rule at position
func (d *Deps) handleProxmoxAssetFirewall(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/proxmox/assets/")
	// path is now: {id}/firewall or {id}/firewall/{pos}
	parts := strings.SplitN(path, "/", 4)
	if len(parts) < 2 {
		servicehttp.WriteError(w, http.StatusNotFound, "expected /proxmox/assets/{id}/firewall")
		return
	}
	assetID := strings.TrimSpace(parts[0])
	if assetID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "asset id is required")
		return
	}
	// parts[1] == "firewall"
	// parts[2] (optional) == position

	target, ok, err := d.ResolveProxmoxSessionTarget(assetID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to resolve proxmox asset: "+shared.SanitizeUpstreamError(err.Error()))
		return
	}
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "asset is not proxmox-backed")
		return
	}

	runtime, err := d.LoadProxmoxRuntime(target.CollectorID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to load proxmox runtime: "+shared.SanitizeUpstreamError(err.Error()))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		d.proxmoxFirewallList(w, ctx, target, runtime)
	case http.MethodPost:
		if !d.RequireAdminAuth(w, r) {
			return
		}
		d.proxmoxFirewallCreate(w, r, ctx, target, runtime)
	case http.MethodPut:
		if !d.RequireAdminAuth(w, r) {
			return
		}
		if len(parts) < 3 || strings.TrimSpace(parts[2]) == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "position is required for PUT")
			return
		}
		pos, parseErr := strconv.Atoi(strings.TrimSpace(parts[2]))
		if parseErr != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "position must be numeric")
			return
		}
		d.proxmoxFirewallUpdate(w, r, ctx, target, runtime, pos)
	case http.MethodDelete:
		if !d.RequireAdminAuth(w, r) {
			return
		}
		if len(parts) < 3 || strings.TrimSpace(parts[2]) == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "position is required for DELETE")
			return
		}
		pos, parseErr := strconv.Atoi(strings.TrimSpace(parts[2]))
		if parseErr != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "position must be numeric")
			return
		}
		d.proxmoxFirewallDelete(w, ctx, target, runtime, pos)
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) proxmoxFirewallList(w http.ResponseWriter, ctx context.Context, target ProxmoxSessionTarget, runtime *ProxmoxRuntime) {
	var rules []proxmox.FirewallRule
	var err error
	switch strings.ToLower(strings.TrimSpace(target.Kind)) {
	case "node":
		rules, err = runtime.client.GetNodeFirewallRules(ctx, target.Node)
	case "qemu":
		rules, err = runtime.client.GetVMFirewallRules(ctx, target.Node, target.VMID, "qemu")
	case "lxc":
		rules, err = runtime.client.GetVMFirewallRules(ctx, target.Node, target.VMID, "lxc")
	default:
		servicehttp.WriteError(w, http.StatusBadRequest, "firewall not supported for kind: "+target.Kind)
		return
	}
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to fetch firewall rules: "+shared.SanitizeUpstreamError(err.Error()))
		return
	}
	if rules == nil {
		rules = []proxmox.FirewallRule{}
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"rules": rules})
}

func (d *Deps) proxmoxFirewallCreate(w http.ResponseWriter, r *http.Request, ctx context.Context, target ProxmoxSessionTarget, runtime *ProxmoxRuntime) {
	var rule proxmox.FirewallRule
	if err := shared.DecodeJSONBody(w, r, &rule); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var err error
	switch strings.ToLower(strings.TrimSpace(target.Kind)) {
	case "node":
		err = runtime.client.CreateNodeFirewallRule(ctx, target.Node, rule)
	case "qemu":
		err = runtime.client.CreateVMFirewallRule(ctx, target.Node, target.VMID, "qemu", rule)
	case "lxc":
		err = runtime.client.CreateVMFirewallRule(ctx, target.Node, target.VMID, "lxc", rule)
	default:
		servicehttp.WriteError(w, http.StatusBadRequest, "firewall not supported for kind: "+target.Kind)
		return
	}
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to create firewall rule: "+shared.SanitizeUpstreamError(err.Error()))
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"status": "created"})
}

func (d *Deps) proxmoxFirewallUpdate(w http.ResponseWriter, r *http.Request, ctx context.Context, target ProxmoxSessionTarget, runtime *ProxmoxRuntime, pos int) {
	var rule proxmox.FirewallRule
	if err := shared.DecodeJSONBody(w, r, &rule); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var err error
	switch strings.ToLower(strings.TrimSpace(target.Kind)) {
	case "node":
		err = runtime.client.UpdateNodeFirewallRule(ctx, target.Node, pos, rule)
	case "qemu":
		err = runtime.client.UpdateVMFirewallRule(ctx, target.Node, target.VMID, "qemu", pos, rule)
	case "lxc":
		err = runtime.client.UpdateVMFirewallRule(ctx, target.Node, target.VMID, "lxc", pos, rule)
	default:
		servicehttp.WriteError(w, http.StatusBadRequest, "firewall not supported for kind: "+target.Kind)
		return
	}
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to update firewall rule: "+shared.SanitizeUpstreamError(err.Error()))
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (d *Deps) proxmoxFirewallDelete(w http.ResponseWriter, ctx context.Context, target ProxmoxSessionTarget, runtime *ProxmoxRuntime, pos int) {
	var err error
	switch strings.ToLower(strings.TrimSpace(target.Kind)) {
	case "node":
		err = runtime.client.DeleteNodeFirewallRule(ctx, target.Node, pos)
	case "qemu":
		err = runtime.client.DeleteVMFirewallRule(ctx, target.Node, target.VMID, "qemu", pos)
	case "lxc":
		err = runtime.client.DeleteVMFirewallRule(ctx, target.Node, target.VMID, "lxc", pos)
	default:
		servicehttp.WriteError(w, http.StatusBadRequest, "firewall not supported for kind: "+target.Kind)
		return
	}
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to delete firewall rule: "+shared.SanitizeUpstreamError(err.Error()))
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
