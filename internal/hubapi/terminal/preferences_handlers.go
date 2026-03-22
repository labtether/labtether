package terminal

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/servicehttp"
)

type TerminalPreferences struct {
	UserID        string           `json:"user_id"`
	Theme         string           `json:"theme"`
	FontFamily    string           `json:"font_family"`
	FontSize      int              `json:"font_size"`
	CursorStyle   string           `json:"cursor_style"`
	CursorBlink   bool             `json:"cursor_blink"`
	Scrollback    int              `json:"scrollback"`
	ToolbarKeys   *json.RawMessage `json:"toolbar_keys,omitempty"`
	AutoReconnect bool             `json:"auto_reconnect"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

func DefaultTerminalPreferences() TerminalPreferences {
	return TerminalPreferences{
		UserID:        "default",
		Theme:         "labtether-dark",
		FontFamily:    "JetBrains Mono",
		FontSize:      14,
		CursorStyle:   "block",
		CursorBlink:   true,
		Scrollback:    5000,
		AutoReconnect: false,
		UpdatedAt:     time.Now().UTC(),
	}
}

func (d *Deps) HandleTerminalPreferences(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/terminal/preferences" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		d.getTerminalPreferences(w, r)
	case http.MethodPut:
		d.updateTerminalPreferences(w, r)
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) getTerminalPreferences(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var prefs TerminalPreferences
	var toolbarKeys []byte

	err := d.DBPool.QueryRow(ctx,
		`SELECT user_id, theme, font_family, font_size, cursor_style, cursor_blink,
		        scrollback, toolbar_keys, auto_reconnect, updated_at
		 FROM terminal_preferences WHERE user_id = $1`, "default",
	).Scan(
		&prefs.UserID, &prefs.Theme, &prefs.FontFamily, &prefs.FontSize,
		&prefs.CursorStyle, &prefs.CursorBlink, &prefs.Scrollback,
		&toolbarKeys, &prefs.AutoReconnect, &prefs.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			prefs = DefaultTerminalPreferences()
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"preferences": prefs})
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load terminal preferences")
		return
	}

	if toolbarKeys != nil {
		raw := json.RawMessage(toolbarKeys)
		prefs.ToolbarKeys = &raw
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"preferences": prefs})
}

func (d *Deps) updateTerminalPreferences(w http.ResponseWriter, r *http.Request) {
	if !d.EnforceRateLimit(w, r, "terminal.preferences.update", 60, time.Minute) {
		return
	}

	var req struct {
		Theme         *string          `json:"theme"`
		FontFamily    *string          `json:"font_family"`
		FontSize      *int             `json:"font_size"`
		CursorStyle   *string          `json:"cursor_style"`
		CursorBlink   *bool            `json:"cursor_blink"`
		Scrollback    *int             `json:"scrollback"`
		ToolbarKeys   *json.RawMessage `json:"toolbar_keys"`
		AutoReconnect *bool            `json:"auto_reconnect"`
	}
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid terminal preferences payload")
		return
	}

	prefs := DefaultTerminalPreferences()

	ctx := r.Context()
	var existingToolbarKeys []byte
	err := d.DBPool.QueryRow(ctx,
		`SELECT user_id, theme, font_family, font_size, cursor_style, cursor_blink,
		        scrollback, toolbar_keys, auto_reconnect, updated_at
		 FROM terminal_preferences WHERE user_id = $1`, "default",
	).Scan(
		&prefs.UserID, &prefs.Theme, &prefs.FontFamily, &prefs.FontSize,
		&prefs.CursorStyle, &prefs.CursorBlink, &prefs.Scrollback,
		&existingToolbarKeys, &prefs.AutoReconnect, &prefs.UpdatedAt,
	)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load terminal preferences")
		return
	}
	if existingToolbarKeys != nil {
		raw := json.RawMessage(existingToolbarKeys)
		prefs.ToolbarKeys = &raw
	}

	if req.Theme != nil {
		prefs.Theme = strings.TrimSpace(*req.Theme)
	}
	if req.FontFamily != nil {
		prefs.FontFamily = strings.TrimSpace(*req.FontFamily)
	}
	if req.FontSize != nil {
		if *req.FontSize < 10 || *req.FontSize > 24 {
			servicehttp.WriteError(w, http.StatusBadRequest, "font_size must be between 10 and 24")
			return
		}
		prefs.FontSize = *req.FontSize
	}
	if req.CursorStyle != nil {
		cs := strings.TrimSpace(*req.CursorStyle)
		if cs != "block" && cs != "underline" && cs != "bar" {
			servicehttp.WriteError(w, http.StatusBadRequest, "cursor_style must be one of: block, underline, bar")
			return
		}
		prefs.CursorStyle = cs
	}
	if req.CursorBlink != nil {
		prefs.CursorBlink = *req.CursorBlink
	}
	if req.Scrollback != nil {
		if *req.Scrollback < 100 || *req.Scrollback > 100000 {
			servicehttp.WriteError(w, http.StatusBadRequest, "scrollback must be between 100 and 100000")
			return
		}
		prefs.Scrollback = *req.Scrollback
	}
	if req.ToolbarKeys != nil {
		prefs.ToolbarKeys = req.ToolbarKeys
	}
	if req.AutoReconnect != nil {
		prefs.AutoReconnect = *req.AutoReconnect
	}

	now := time.Now().UTC()
	prefs.UpdatedAt = now

	var toolbarKeysBytes []byte
	if prefs.ToolbarKeys != nil {
		toolbarKeysBytes = []byte(*prefs.ToolbarKeys)
	}

	_, err = d.DBPool.Exec(ctx,
		`INSERT INTO terminal_preferences (user_id, theme, font_family, font_size, cursor_style, cursor_blink, scrollback, toolbar_keys, auto_reconnect, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT (user_id) DO UPDATE SET
		   theme = EXCLUDED.theme,
		   font_family = EXCLUDED.font_family,
		   font_size = EXCLUDED.font_size,
		   cursor_style = EXCLUDED.cursor_style,
		   cursor_blink = EXCLUDED.cursor_blink,
		   scrollback = EXCLUDED.scrollback,
		   toolbar_keys = EXCLUDED.toolbar_keys,
		   auto_reconnect = EXCLUDED.auto_reconnect,
		   updated_at = EXCLUDED.updated_at`,
		prefs.UserID, prefs.Theme, prefs.FontFamily, prefs.FontSize,
		prefs.CursorStyle, prefs.CursorBlink, prefs.Scrollback,
		toolbarKeysBytes, prefs.AutoReconnect, now,
	)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to save terminal preferences")
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"preferences": prefs})
}
