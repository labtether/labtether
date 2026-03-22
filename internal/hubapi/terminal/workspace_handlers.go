package terminal

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/servicehttp"
)

type WorkspaceTab struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Layout     string          `json:"layout"`
	Panes      json.RawMessage `json:"panes"`
	PanelSizes json.RawMessage `json:"panel_sizes"`
	SortOrder  int             `json:"sort_order"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

func (d *Deps) HandleWorkspaceTabs(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/terminal/workspace/tabs" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		d.listWorkspaceTabs(w, r)
	case http.MethodPost:
		d.createWorkspaceTab(w, r)
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) HandleWorkspaceTabActions(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/terminal/workspace/tabs/")
	if id == r.URL.Path || id == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "workspace tab path not found")
		return
	}

	if idx := strings.Index(id, "/"); idx >= 0 {
		id = id[:idx]
	}
	id = strings.TrimSpace(id)
	if id == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "workspace tab path not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		d.getWorkspaceTab(w, r, id)
	case http.MethodPut:
		d.updateWorkspaceTab(w, r, id)
	case http.MethodDelete:
		d.deleteWorkspaceTab(w, r, id)
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) listWorkspaceTabs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := d.DBPool.Query(ctx,
		`SELECT id, name, layout, panes, panel_sizes, sort_order, created_at, updated_at
		 FROM terminal_workspace_tabs
		 ORDER BY sort_order, created_at`,
	)
	if err != nil {
		if terminalWorkspaceSchemaMissing(err) {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "terminal workspace schema is outdated; run db migrations")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list workspace tabs")
		return
	}
	defer rows.Close()

	tabs := make([]WorkspaceTab, 0)
	for rows.Next() {
		var tab WorkspaceTab
		if err := rows.Scan(
			&tab.ID, &tab.Name, &tab.Layout, &tab.Panes, &tab.PanelSizes,
			&tab.SortOrder, &tab.CreatedAt, &tab.UpdatedAt,
		); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to scan workspace tab")
			return
		}
		tabs = append(tabs, tab)
	}
	if err := rows.Err(); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list workspace tabs")
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"tabs": tabs})
}

func (d *Deps) createWorkspaceTab(w http.ResponseWriter, r *http.Request) {
	if !d.EnforceRateLimit(w, r, "terminal.workspace.create", 120, time.Minute) {
		return
	}

	var req struct {
		Name       string          `json:"name"`
		Layout     string          `json:"layout"`
		Panes      json.RawMessage `json:"panes"`
		PanelSizes json.RawMessage `json:"panel_sizes"`
		SortOrder  int             `json:"sort_order"`
	}
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid workspace tab payload")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Layout = strings.TrimSpace(req.Layout)

	if req.Name == "" {
		req.Name = "Terminal"
	}
	if req.Layout == "" {
		req.Layout = "single"
	}
	if len(req.Panes) == 0 {
		req.Panes = json.RawMessage(`[]`)
	}

	if len(req.PanelSizes) == 0 {
		req.PanelSizes = json.RawMessage(`{}`)
	}

	now := time.Now().UTC()
	tab := WorkspaceTab{
		ID:         idgen.New("tab"),
		Name:       req.Name,
		Layout:     req.Layout,
		Panes:      req.Panes,
		PanelSizes: req.PanelSizes,
		SortOrder:  req.SortOrder,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	ctx := r.Context()
	_, err := d.DBPool.Exec(ctx,
		`INSERT INTO terminal_workspace_tabs (id, name, layout, panes, panel_sizes, sort_order, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		tab.ID, tab.Name, tab.Layout, tab.Panes, tab.PanelSizes, tab.SortOrder, tab.CreatedAt, tab.UpdatedAt,
	)
	if err != nil {
		if terminalWorkspaceSchemaMissing(err) {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "terminal workspace schema is outdated; run db migrations")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create workspace tab")
		return
	}

	servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"tab": tab})
}

func (d *Deps) getWorkspaceTab(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()
	var tab WorkspaceTab
	err := d.DBPool.QueryRow(ctx,
		`SELECT id, name, layout, panes, panel_sizes, sort_order, created_at, updated_at
		 FROM terminal_workspace_tabs WHERE id = $1`, id,
	).Scan(
		&tab.ID, &tab.Name, &tab.Layout, &tab.Panes, &tab.PanelSizes,
		&tab.SortOrder, &tab.CreatedAt, &tab.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			servicehttp.WriteError(w, http.StatusNotFound, "workspace tab not found")
			return
		}
		if terminalWorkspaceSchemaMissing(err) {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "terminal workspace schema is outdated; run db migrations")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load workspace tab")
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"tab": tab})
}

func (d *Deps) updateWorkspaceTab(w http.ResponseWriter, r *http.Request, id string) {
	if !d.EnforceRateLimit(w, r, "terminal.workspace.update", 120, time.Minute) {
		return
	}

	var req struct {
		Name       *string          `json:"name"`
		Layout     *string          `json:"layout"`
		Panes      *json.RawMessage `json:"panes"`
		PanelSizes *json.RawMessage `json:"panel_sizes"`
		SortOrder  *int             `json:"sort_order"`
	}
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid workspace tab payload")
		return
	}

	ctx := r.Context()
	var tab WorkspaceTab
	err := d.DBPool.QueryRow(ctx,
		`SELECT id, name, layout, panes, panel_sizes, sort_order, created_at, updated_at
		 FROM terminal_workspace_tabs WHERE id = $1`, id,
	).Scan(
		&tab.ID, &tab.Name, &tab.Layout, &tab.Panes, &tab.PanelSizes,
		&tab.SortOrder, &tab.CreatedAt, &tab.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			servicehttp.WriteError(w, http.StatusNotFound, "workspace tab not found")
			return
		}
		if terminalWorkspaceSchemaMissing(err) {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "terminal workspace schema is outdated; run db migrations")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load workspace tab")
		return
	}

	if req.Name != nil {
		tab.Name = strings.TrimSpace(*req.Name)
	}
	if req.Layout != nil {
		tab.Layout = strings.TrimSpace(*req.Layout)
	}
	if req.Panes != nil {
		tab.Panes = *req.Panes
	}
	if req.PanelSizes != nil {
		tab.PanelSizes = *req.PanelSizes
	}
	if req.SortOrder != nil {
		tab.SortOrder = *req.SortOrder
	}

	now := time.Now().UTC()
	tab.UpdatedAt = now

	_, err = d.DBPool.Exec(ctx,
		`UPDATE terminal_workspace_tabs SET name = $2, layout = $3, panes = $4, panel_sizes = $5, sort_order = $6, updated_at = $7
		 WHERE id = $1`,
		tab.ID, tab.Name, tab.Layout, tab.Panes, tab.PanelSizes, tab.SortOrder, tab.UpdatedAt,
	)
	if err != nil {
		if terminalWorkspaceSchemaMissing(err) {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "terminal workspace schema is outdated; run db migrations")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update workspace tab")
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"tab": tab})
}

func (d *Deps) deleteWorkspaceTab(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()
	tag, err := d.DBPool.Exec(ctx,
		`DELETE FROM terminal_workspace_tabs WHERE id = $1`, id,
	)
	if err != nil {
		if terminalWorkspaceSchemaMissing(err) {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "terminal workspace schema is outdated; run db migrations")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete workspace tab")
		return
	}
	if tag.RowsAffected() == 0 {
		servicehttp.WriteError(w, http.StatusNotFound, "workspace tab not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func terminalWorkspaceSchemaMissing(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "42P01" || pgErr.Code == "42703"
}
