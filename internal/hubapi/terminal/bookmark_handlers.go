package terminal

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
)

func (d *Deps) HandleBookmarks(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/terminal/bookmarks" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.TerminalBookmarkStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "terminal bookmarks are unavailable")
		return
	}

	switch r.Method {
	case http.MethodGet:
		d.listBookmarks(w, r)
	case http.MethodPost:
		d.createBookmark(w, r)
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) HandleBookmarkActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/terminal/bookmarks/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "bookmark path not found")
		return
	}
	if d.TerminalBookmarkStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "terminal bookmarks are unavailable")
		return
	}

	parts := strings.Split(path, "/")
	bookmarkID := strings.TrimSpace(parts[0])
	if bookmarkID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "bookmark path not found")
		return
	}

	bookmark, ok, err := d.TerminalBookmarkStore.GetBookmark(bookmarkID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load bookmark")
		return
	}
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "bookmark not found")
		return
	}
	if !d.canAccessOwnedSession(r, bookmark.ActorID) {
		servicehttp.WriteError(w, http.StatusForbidden, "bookmark access denied")
		return
	}

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"bookmark": bookmark})
		case http.MethodPut:
			d.updateBookmark(w, r, bookmark)
		case http.MethodDelete:
			d.deleteBookmark(w, bookmark)
		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	if len(parts) == 2 && parts[1] == "connect" {
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		d.connectBookmark(w, r, bookmark)
		return
	}

	servicehttp.WriteError(w, http.StatusNotFound, "unknown bookmark action")
}

func (d *Deps) listBookmarks(w http.ResponseWriter, r *http.Request) {
	actorID := d.PrincipalActorID(r.Context())
	queryActorID := actorID
	if d.IsOwnerActor(actorID) {
		queryActorID = "owner"
	}
	bookmarks, err := d.TerminalBookmarkStore.ListBookmarks(queryActorID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list bookmarks")
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"bookmarks": bookmarks})
}

func (d *Deps) createBookmark(w http.ResponseWriter, r *http.Request) {
	if !d.EnforceRateLimit(w, r, "terminal.bookmark.create", 60, time.Minute) {
		return
	}

	var req terminal.CreateBookmarkRequest
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid bookmark payload")
		return
	}
	req.ActorID = d.PrincipalActorID(r.Context())
	req.Title = strings.TrimSpace(req.Title)
	req.Host = strings.TrimSpace(req.Host)
	req.AssetID = strings.TrimSpace(req.AssetID)
	if req.Title == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "title is required")
		return
	}
	if req.AssetID == "" && req.Host == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "asset_id or host is required")
		return
	}

	bookmark, err := d.TerminalBookmarkStore.CreateBookmark(req)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create bookmark")
		return
	}
	servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"bookmark": bookmark})
}

func (d *Deps) updateBookmark(w http.ResponseWriter, r *http.Request, bookmark terminal.Bookmark) {
	if !d.EnforceRateLimit(w, r, "terminal.bookmark.update", 120, time.Minute) {
		return
	}

	var req terminal.UpdateBookmarkRequest
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid bookmark payload")
		return
	}
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "title is required")
			return
		}
		req.Title = &title
	}
	if req.AssetID != nil {
		assetID := strings.TrimSpace(*req.AssetID)
		req.AssetID = &assetID
	}
	if req.Host != nil {
		host := strings.TrimSpace(*req.Host)
		req.Host = &host
	}

	nextAssetID := bookmark.AssetID
	if req.AssetID != nil {
		nextAssetID = *req.AssetID
	}
	nextHost := bookmark.Host
	if req.Host != nil {
		nextHost = *req.Host
	}
	if nextAssetID == "" && nextHost == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "asset_id or host is required")
		return
	}

	updated, err := d.TerminalBookmarkStore.UpdateBookmark(bookmark.ID, req)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update bookmark")
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"bookmark": updated})
}

func (d *Deps) deleteBookmark(w http.ResponseWriter, bookmark terminal.Bookmark) {
	if err := d.TerminalBookmarkStore.DeleteBookmark(bookmark.ID); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete bookmark")
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true, "bookmark_id": bookmark.ID})
}

func (d *Deps) connectBookmark(w http.ResponseWriter, r *http.Request, bookmark terminal.Bookmark) {
	if !d.EnforceRateLimit(w, r, "terminal.bookmark.connect", 60, time.Minute) {
		return
	}

	// Touch last_used_at.
	if err := d.TerminalBookmarkStore.TouchBookmarkLastUsed(bookmark.ID, time.Now().UTC()); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update bookmark last used")
		return
	}

	// Build target string from bookmark.
	target := bookmark.AssetID
	if target == "" {
		port := 22
		if bookmark.Port != nil {
			port = *bookmark.Port
		}
		target = fmt.Sprintf("%s:%d", bookmark.Host, port)
	}

	// Create persistent session.
	persistent, err := d.TerminalPersistentStore.CreateOrUpdatePersistentSession(terminal.CreatePersistentSessionRequest{
		ActorID:    bookmark.ActorID,
		Target:     target,
		Title:      bookmark.Title,
		BookmarkID: bookmark.ID,
	})
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create persistent session")
		return
	}

	// Create ephemeral session.
	attachedAt := time.Now().UTC()
	session, err := d.TerminalStore.CreateSession(terminal.CreateSessionRequest{
		ActorID:             bookmark.ActorID,
		Target:              target,
		Mode:                "interactive",
		PersistentSessionID: persistent.ID,
		TmuxSessionName:     persistent.TmuxSessionName,
	})
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create terminal session")
		return
	}

	// Issue stream ticket.
	ticket, ticketExpiresAt, err := d.IssueStreamTicket(r.Context(), session.ID)
	if err != nil {
		_ = d.TerminalStore.DeleteTerminalSession(session.ID)
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to issue stream ticket")
		return
	}

	attached, err := d.TerminalPersistentStore.MarkPersistentSessionAttached(persistent.ID, attachedAt)
	if err != nil {
		_ = d.TerminalStore.DeleteTerminalSession(session.ID)
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to mark persistent session attached")
		return
	}

	servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{
		"session_id":               session.ID,
		"persistent_session_id":    attached.ID,
		"stream_ticket":            ticket,
		"stream_ticket_expires_at": ticketExpiresAt,
	})
}
