package terminal

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
)

func persistentStreamKey(id string) string { return "persistent:" + id }

func (d *Deps) persistentSessionExists(id string) (bool, error) {
	_, present, err := d.TerminalPersistentStore.GetPersistentSession(id)
	return present, err
}

func (d *Deps) attachPersistentSession(w http.ResponseWriter, r *http.Request, persistent terminal.PersistentSession) {
	if d.SessionStreams == nil {
		d.attachPersistentSessionUnchecked(w, r, persistent)
		return
	}
	err := d.SessionStreams.RunIfExists(persistentStreamKey(persistent.ID), func() (bool, error) {
		return d.persistentSessionExists(persistent.ID)
	}, func() { d.attachPersistentSessionUnchecked(w, r, persistent) })
	if err != nil {
		if errors.Is(err, terminal.ErrSessionNotFound) {
			servicehttp.WriteError(w, http.StatusNotFound, "persistent session not found")
		} else {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to read persistent session")
		}
	}
}

// Detach is the client's notification after deleting its logical attachment.
// A later notification from an old browser must not detach a current owner.
func (d *Deps) detachPersistentSession(w http.ResponseWriter, persistent terminal.PersistentSession) {
	var detached terminal.PersistentSession
	var storeErr error
	update := func() {
		d.persistentStreamMu.Lock()
		defer d.persistentStreamMu.Unlock()
		if _, active := d.ActivePersistentConns.Load(persistent.ID); active {
			detached, _, storeErr = d.TerminalPersistentStore.GetPersistentSession(persistent.ID)
		} else {
			detached, storeErr = d.TerminalPersistentStore.MarkPersistentSessionDetached(persistent.ID, time.Now().UTC())
		}
	}
	var err error
	if d.SessionStreams != nil {
		err = d.SessionStreams.RunIfExists(persistentStreamKey(persistent.ID), func() (bool, error) {
			return d.persistentSessionExists(persistent.ID)
		}, update)
	} else {
		update()
	}
	if err == nil {
		err = storeErr
	}
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, terminal.ErrSessionNotFound) {
			status = http.StatusNotFound
		}
		servicehttp.WriteError(w, status, "failed to detach persistent session")
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"persistent_session": detached})
}

type persistentDeleteFailure struct {
	status int
	reason string
}

func (e *persistentDeleteFailure) Error() string { return e.reason }

func (d *Deps) deletePersistentSession(w http.ResponseWriter, persistent terminal.PersistentSession) {
	if !d.enforceAssetActionGuard(w, persistent.Target) {
		return
	}
	remove := func() error {
		// An attachment may have started after the endpoint's initial read but
		// before this gate. Use its current attach state when deciding tmux cleanup.
		current, present, err := d.TerminalPersistentStore.GetPersistentSession(persistent.ID)
		if err != nil {
			return err
		}
		if !present {
			return &persistentDeleteFailure{http.StatusNotFound, "persistent session not found"}
		}
		return d.deletePersistentSessionRuntime(current)
	}
	var err error
	if d.SessionStreams != nil {
		err = d.SessionStreams.Delete(persistentStreamKey(persistent.ID), remove)
	} else {
		err = remove()
	}
	if err != nil {
		var failure *persistentDeleteFailure
		if errors.As(err, &failure) {
			servicehttp.WriteError(w, failure.status, failure.reason)
		} else {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete persistent session")
		}
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true, "persistent_session_id": persistent.ID})
}

// The caller holds the saved-shell gate before acquiring any logical-session
// gate. Stop the concrete attachments and tmux before removing saved metadata.
func (d *Deps) deletePersistentSessionRuntime(persistent terminal.PersistentSession) error {
	sessions, err := d.TerminalStore.ListSessions()
	if err != nil {
		return &persistentDeleteFailure{http.StatusInternalServerError, "failed to load attached terminal sessions"}
	}
	attachedSessions := make([]terminal.Session, 0, 1)
	for _, session := range sessions {
		if strings.TrimSpace(session.PersistentSessionID) == persistent.ID {
			attachedSessions = append(attachedSessions, session)
		}
	}
	if err := d.ClosePersistentTerminalAttachments(persistent, attachedSessions); err != nil {
		return &persistentDeleteFailure{http.StatusBadGateway, err.Error()}
	}
	if err := d.TerminatePersistentTerminalRuntime(persistent); err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, ErrPersistentTerminalCleanupUnavailable) {
			status = http.StatusConflict
		}
		return &persistentDeleteFailure{status, err.Error()}
	}
	for _, session := range attachedSessions {
		remove := func() error { return d.TerminalStore.DeleteTerminalSession(session.ID) }
		var err error
		if d.SessionStreams != nil {
			err = d.SessionStreams.Delete(session.ID, remove)
		} else {
			err = remove()
		}
		if err != nil {
			return &persistentDeleteFailure{http.StatusInternalServerError, "failed to remove attached terminal sessions"}
		}
	}
	if err := d.TerminalPersistentStore.DeletePersistentSession(persistent.ID); err != nil {
		return &persistentDeleteFailure{http.StatusInternalServerError, "failed to delete persistent session"}
	}
	return nil
}
