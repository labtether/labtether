package resources

import (
	"errors"
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

const remoteBookmarkAPIPrefix = "/api/v1/remote-bookmarks"

// HandleRemoteBookmarks dispatches /api/v1/remote-bookmarks requests.
func (d *Deps) HandleRemoteBookmarks(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, remoteBookmarkAPIPrefix)
	path = strings.TrimPrefix(path, "/")

	if d.RemoteBookmarkStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "remote bookmark store unavailable")
		return
	}

	// Collection routes: /api/v1/remote-bookmarks
	if path == "" {
		switch r.Method {
		case http.MethodGet:
			d.handleListRemoteBookmarks(w, r)
		case http.MethodPost:
			d.handleCreateRemoteBookmark(w, r)
		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	// Sub-resource routes: /api/v1/remote-bookmarks/{id}[/credentials]
	parts := strings.SplitN(path, "/", 2)
	bmID := strings.TrimSpace(parts[0])
	if bmID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}

	if len(parts) == 2 {
		action := parts[1]
		if action == "credentials" {
			if r.Method != http.MethodGet {
				servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			d.handleGetRemoteBookmarkCredentials(w, r, bmID)
			return
		}
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodPut:
			d.handleUpdateRemoteBookmark(w, r, bmID)
		case http.MethodDelete:
			d.handleDeleteRemoteBookmark(w, r, bmID)
		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	servicehttp.WriteError(w, http.StatusNotFound, "not found")
}

// --- List ---

func (d *Deps) handleListRemoteBookmarks(w http.ResponseWriter, r *http.Request) {
	bookmarks, err := d.RemoteBookmarkStore.ListRemoteBookmarks(r.Context())
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list remote bookmarks")
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, bookmarks)
}

// --- Create ---

type remoteBookmarkCreateRequest struct {
	Label        string  `json:"label"`
	Protocol     string  `json:"protocol"`
	Host         string  `json:"host"`
	Port         int     `json:"port"`
	CredentialID *string `json:"credential_id,omitempty"`
}

func (d *Deps) handleCreateRemoteBookmark(w http.ResponseWriter, r *http.Request) {
	var req remoteBookmarkCreateRequest
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		return
	}
	req.Label = strings.TrimSpace(req.Label)
	req.Protocol = strings.TrimSpace(req.Protocol)
	req.Host = strings.TrimSpace(req.Host)

	if req.Label == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "label is required")
		return
	}
	if req.Protocol == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "protocol is required")
		return
	}
	validProtocols := map[string]bool{"vnc": true, "rdp": true, "spice": true, "ard": true}
	if !validProtocols[req.Protocol] {
		servicehttp.WriteError(w, http.StatusBadRequest, "protocol must be one of: vnc, rdp, spice, ard")
		return
	}
	if req.Host == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "host is required")
		return
	}
	if req.Port <= 0 || req.Port > 65535 {
		servicehttp.WriteError(w, http.StatusBadRequest, "port must be between 1 and 65535")
		return
	}

	bm := &persistence.RemoteBookmark{
		Label:        req.Label,
		Protocol:     req.Protocol,
		Host:         req.Host,
		Port:         req.Port,
		CredentialID: req.CredentialID,
	}
	if err := d.RemoteBookmarkStore.CreateRemoteBookmark(r.Context(), bm); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create remote bookmark")
		return
	}

	servicehttp.WriteJSON(w, http.StatusCreated, bm)
}

// --- Update ---

type remoteBookmarkUpdateRequest struct {
	Label        string  `json:"label"`
	Protocol     string  `json:"protocol"`
	Host         string  `json:"host"`
	Port         int     `json:"port"`
	CredentialID *string `json:"credential_id,omitempty"`
}

func (d *Deps) handleUpdateRemoteBookmark(w http.ResponseWriter, r *http.Request, bmID string) {
	existing, err := d.RemoteBookmarkStore.GetRemoteBookmark(r.Context(), bmID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			servicehttp.WriteError(w, http.StatusNotFound, "remote bookmark not found")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load remote bookmark")
		return
	}

	var req remoteBookmarkUpdateRequest
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		return
	}
	req.Label = strings.TrimSpace(req.Label)
	req.Protocol = strings.TrimSpace(req.Protocol)
	req.Host = strings.TrimSpace(req.Host)

	if req.Label != "" {
		existing.Label = req.Label
	}
	if req.Protocol != "" {
		existing.Protocol = req.Protocol
	}
	if req.Host != "" {
		existing.Host = req.Host
	}
	if req.Port != 0 {
		existing.Port = req.Port
	}
	if req.CredentialID != nil {
		existing.CredentialID = req.CredentialID
	}

	if err := d.RemoteBookmarkStore.UpdateRemoteBookmark(r.Context(), *existing); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update remote bookmark")
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, existing)
}

// --- Delete ---

func (d *Deps) handleDeleteRemoteBookmark(w http.ResponseWriter, r *http.Request, bmID string) {
	err := d.RemoteBookmarkStore.DeleteRemoteBookmark(r.Context(), bmID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			servicehttp.WriteError(w, http.StatusNotFound, "remote bookmark not found")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete remote bookmark")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Credentials (stub) ---

func (d *Deps) handleGetRemoteBookmarkCredentials(w http.ResponseWriter, r *http.Request, bmID string) {
	// Stub: credential decryption for remote bookmarks is not yet implemented.
	// Returns nulls so the frontend can detect unconfigured credentials gracefully.
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"id":       bmID,
		"username": nil,
		"password": nil,
	})
}
