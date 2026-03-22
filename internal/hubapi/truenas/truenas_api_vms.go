package truenas

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	tnconnector "github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/servicehttp"
)

type TrueNASVMsResponse struct {
	AssetID   string           `json:"asset_id"`
	VMs       []map[string]any `json:"vms"`
	Supported bool             `json:"supported"`
	Warnings  []string         `json:"warnings,omitempty"`
	FetchedAt string           `json:"fetched_at"`
}

type TrueNASVMActionResponse struct {
	AssetID   string   `json:"asset_id"`
	VMID      int      `json:"vm_id"`
	Action    string   `json:"action"`
	Message   string   `json:"message"`
	Warnings  []string `json:"warnings,omitempty"`
	FetchedAt string   `json:"fetched_at"`
}

func (d *Deps) HandleTrueNASVMs(ctx context.Context, w http.ResponseWriter, r *http.Request, asset assets.Asset, runtime *TruenasRuntime, subParts []string) {
	// GET  /truenas/assets/{id}/vms              → list VMs (SCALE only; graceful on CORE)
	// POST /truenas/assets/{id}/vms/{id}/start   → start VM
	// POST /truenas/assets/{id}/vms/{id}/stop    → stop VM
	// POST /truenas/assets/{id}/vms/{id}/restart → restart VM

	if len(subParts) == 0 {
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		vms := make([]map[string]any, 0, 16)
		warnings := make([]string, 0, 2)
		supported := true
		if err := CallTrueNASQueryWithRetries(ctx, runtime.Client, "vm.query", &vms); err != nil {
			if tnconnector.IsMethodNotFound(err) {
				supported = false
			} else {
				warnings = AppendTrueNASWarning(warnings, "VMs unavailable: "+err.Error())
			}
			vms = nil
		}
		servicehttp.WriteJSON(w, http.StatusOK, TrueNASVMsResponse{
			AssetID:   strings.TrimSpace(asset.ID),
			VMs:       vms,
			Supported: supported,
			Warnings:  warnings,
			FetchedAt: time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	idStr := strings.TrimSpace(subParts[0])
	if idStr == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "vm id is required")
		return
	}
	vmID, err := strconv.Atoi(idStr)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "vm id must be an integer")
		return
	}

	if len(subParts) < 2 {
		servicehttp.WriteError(w, http.StatusNotFound, "unknown vm action")
		return
	}

	vmAction := strings.TrimSpace(subParts[1])
	switch vmAction {
	case "start", "stop", "restart":
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !d.RequireAdminAuth(w, r) {
			return
		}
		method := "vm." + vmAction
		if err := CallTrueNASMethodWithRetries(ctx, runtime.Client, method, []any{vmID}, nil); err != nil {
			if tnconnector.IsMethodNotFound(err) {
				servicehttp.WriteError(w, http.StatusNotImplemented, "VM management is not supported on TrueNAS CORE")
				return
			}
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to "+vmAction+" VM: "+err.Error())
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, TrueNASVMActionResponse{
			AssetID:   strings.TrimSpace(asset.ID),
			VMID:      vmID,
			Action:    vmAction,
			Message:   "VM " + idStr + " " + vmAction + "ed",
			FetchedAt: time.Now().UTC().Format(time.RFC3339),
		})
	default:
		servicehttp.WriteError(w, http.StatusNotFound, "unknown vm action: "+vmAction)
	}
}
