package terminal

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/servicehttp"
)

type TerminalSnippet struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Command     string    `json:"command"`
	Description string    `json:"description"`
	Scope       string    `json:"scope"`
	Shortcut    string    `json:"shortcut"`
	SortOrder   int       `json:"sort_order"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (d *Deps) HandleTerminalSnippets(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/terminal/snippets" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		d.listTerminalSnippets(w, r)
	case http.MethodPost:
		d.createTerminalSnippet(w, r)
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) HandleTerminalSnippetActions(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/terminal/snippets/")
	if id == r.URL.Path || id == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "snippet path not found")
		return
	}

	if idx := strings.Index(id, "/"); idx >= 0 {
		id = id[:idx]
	}
	id = strings.TrimSpace(id)
	if id == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "snippet path not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		d.getTerminalSnippet(w, r, id)
	case http.MethodPut:
		d.updateTerminalSnippet(w, r, id)
	case http.MethodDelete:
		d.deleteTerminalSnippet(w, r, id)
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) listTerminalSnippets(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	scope := strings.TrimSpace(r.URL.Query().Get("scope"))

	var rows pgx.Rows
	var err error
	if scope != "" {
		rows, err = d.DBPool.Query(ctx,
			`SELECT id, name, command, description, scope, shortcut, sort_order, created_at, updated_at
			 FROM terminal_snippets
			 WHERE scope = $1 OR scope = 'global'
			 ORDER BY sort_order, created_at`, scope,
		)
	} else {
		rows, err = d.DBPool.Query(ctx,
			`SELECT id, name, command, description, scope, shortcut, sort_order, created_at, updated_at
			 FROM terminal_snippets
			 ORDER BY sort_order, created_at`,
		)
	}
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list snippets")
		return
	}
	defer rows.Close()

	snippets := make([]TerminalSnippet, 0)
	for rows.Next() {
		var snip TerminalSnippet
		if err := rows.Scan(
			&snip.ID, &snip.Name, &snip.Command, &snip.Description,
			&snip.Scope, &snip.Shortcut, &snip.SortOrder,
			&snip.CreatedAt, &snip.UpdatedAt,
		); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to scan snippet")
			return
		}
		snippets = append(snippets, snip)
	}
	if err := rows.Err(); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list snippets")
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"snippets": snippets})
}

func (d *Deps) createTerminalSnippet(w http.ResponseWriter, r *http.Request) {
	if !d.EnforceRateLimit(w, r, "terminal.snippets.create", 120, time.Minute) {
		return
	}

	var req struct {
		Name        string `json:"name"`
		Command     string `json:"command"`
		Description string `json:"description"`
		Scope       string `json:"scope"`
		Shortcut    string `json:"shortcut"`
		SortOrder   int    `json:"sort_order"`
	}
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid snippet payload")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Command = strings.TrimSpace(req.Command)
	req.Description = strings.TrimSpace(req.Description)
	req.Scope = strings.TrimSpace(req.Scope)
	req.Shortcut = strings.TrimSpace(req.Shortcut)

	if req.Name == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Command == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "command is required")
		return
	}
	if req.Scope == "" {
		req.Scope = "global"
	}

	now := time.Now().UTC()
	snip := TerminalSnippet{
		ID:          idgen.New("snip"),
		Name:        req.Name,
		Command:     req.Command,
		Description: req.Description,
		Scope:       req.Scope,
		Shortcut:    req.Shortcut,
		SortOrder:   req.SortOrder,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	ctx := r.Context()
	_, err := d.DBPool.Exec(ctx,
		`INSERT INTO terminal_snippets (id, name, command, description, scope, shortcut, sort_order, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		snip.ID, snip.Name, snip.Command, snip.Description,
		snip.Scope, snip.Shortcut, snip.SortOrder, snip.CreatedAt, snip.UpdatedAt,
	)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create snippet")
		return
	}

	servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"snippet": snip})
}

func (d *Deps) getTerminalSnippet(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()
	var snip TerminalSnippet
	err := d.DBPool.QueryRow(ctx,
		`SELECT id, name, command, description, scope, shortcut, sort_order, created_at, updated_at
		 FROM terminal_snippets WHERE id = $1`, id,
	).Scan(
		&snip.ID, &snip.Name, &snip.Command, &snip.Description,
		&snip.Scope, &snip.Shortcut, &snip.SortOrder,
		&snip.CreatedAt, &snip.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			servicehttp.WriteError(w, http.StatusNotFound, "snippet not found")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load snippet")
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"snippet": snip})
}

func (d *Deps) updateTerminalSnippet(w http.ResponseWriter, r *http.Request, id string) {
	if !d.EnforceRateLimit(w, r, "terminal.snippets.update", 120, time.Minute) {
		return
	}

	var req struct {
		Name        *string `json:"name"`
		Command     *string `json:"command"`
		Description *string `json:"description"`
		Scope       *string `json:"scope"`
		Shortcut    *string `json:"shortcut"`
		SortOrder   *int    `json:"sort_order"`
	}
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid snippet payload")
		return
	}

	ctx := r.Context()
	var snip TerminalSnippet
	err := d.DBPool.QueryRow(ctx,
		`SELECT id, name, command, description, scope, shortcut, sort_order, created_at, updated_at
		 FROM terminal_snippets WHERE id = $1`, id,
	).Scan(
		&snip.ID, &snip.Name, &snip.Command, &snip.Description,
		&snip.Scope, &snip.Shortcut, &snip.SortOrder,
		&snip.CreatedAt, &snip.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			servicehttp.WriteError(w, http.StatusNotFound, "snippet not found")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load snippet")
		return
	}

	if req.Name != nil {
		snip.Name = strings.TrimSpace(*req.Name)
	}
	if req.Command != nil {
		snip.Command = strings.TrimSpace(*req.Command)
	}
	if req.Description != nil {
		snip.Description = strings.TrimSpace(*req.Description)
	}
	if req.Scope != nil {
		snip.Scope = strings.TrimSpace(*req.Scope)
	}
	if req.Shortcut != nil {
		snip.Shortcut = strings.TrimSpace(*req.Shortcut)
	}
	if req.SortOrder != nil {
		snip.SortOrder = *req.SortOrder
	}

	now := time.Now().UTC()
	snip.UpdatedAt = now

	_, err = d.DBPool.Exec(ctx,
		`UPDATE terminal_snippets SET name = $2, command = $3, description = $4, scope = $5, shortcut = $6, sort_order = $7, updated_at = $8
		 WHERE id = $1`,
		snip.ID, snip.Name, snip.Command, snip.Description,
		snip.Scope, snip.Shortcut, snip.SortOrder, snip.UpdatedAt,
	)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update snippet")
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"snippet": snip})
}

func (d *Deps) deleteTerminalSnippet(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()
	tag, err := d.DBPool.Exec(ctx,
		`DELETE FROM terminal_snippets WHERE id = $1`, id,
	)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete snippet")
		return
	}
	if tag.RowsAffected() == 0 {
		servicehttp.WriteError(w, http.StatusNotFound, "snippet not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
