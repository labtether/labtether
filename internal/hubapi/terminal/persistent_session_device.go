package terminal

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/labtether/labtether/internal/terminal"
)

// persistentTargetAwayError refuses to remove a saved session while its device
// is still in the Hub but cannot be reached to end the session there. Its text
// is shown to people in the console.
type persistentTargetAwayError struct{ device string }

func (e *persistentTargetAwayError) Error() string {
	return "This session runs on " + e.device + ", which is not connected. Connect it, then remove the session."
}

func (e *persistentTargetAwayError) Unwrap() error { return ErrPersistentTerminalCleanupUnavailable }

// persistentTargetDevice reports whether a saved session's device is still in
// the Hub, and the name to show for it. Without an asset store the device is
// treated as present, so the refusal for an unreachable device stays in force.
func (d *Deps) persistentTargetDevice(target string) (string, bool, error) {
	if d.AssetStore == nil {
		return target, true, nil
	}
	assetEntry, ok, err := d.AssetStore.GetAsset(target)
	if err != nil || !ok {
		return "", false, err
	}
	if name := strings.TrimSpace(assetEntry.Name); name != "" {
		return name, true, nil
	}
	return target, true, nil
}

// RemovePersistentSessionsForRemovedDevice drops every saved session of a
// device that was just deleted from the Hub, for all users, and clears the
// device from their workspace tabs. Device deletion is refused while the agent
// is connected, so there is no agent to ask to end the sessions; a saved SSH
// login is deleted with the device. It keeps going past a failed record and
// returns what failed.
func (d *Deps) RemovePersistentSessionsForRemovedDevice(ctx context.Context, assetID string) error {
	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return nil
	}
	var failures []error
	if d.TerminalPersistentStore != nil && d.TerminalStore != nil {
		sessions, err := d.TerminalPersistentStore.ListPersistentSessions()
		if err != nil {
			failures = append(failures, fmt.Errorf("list saved sessions: %w", err))
		}
		for _, persistent := range sessions {
			if strings.TrimSpace(persistent.Target) != assetID {
				continue
			}
			if err := d.removePersistentSessionOfRemovedDevice(persistent); err != nil {
				failures = append(failures, fmt.Errorf("saved session %s: %w", persistent.ID, err))
			}
		}
	}
	if err := d.clearDeviceFromWorkspaceTabs(ctx, assetID); err != nil {
		failures = append(failures, fmt.Errorf("workspace tabs: %w", err))
	}
	return errors.Join(failures...)
}

func (d *Deps) removePersistentSessionOfRemovedDevice(persistent terminal.PersistentSession) error {
	remove := func() error {
		attachedSessions, err := d.attachedTerminalSessions(persistent.ID)
		if err != nil {
			return err
		}
		// The agent connection is already closed; this only ends browser
		// streams still open on the saved session.
		if err := d.ClosePersistentTerminalAttachments(persistent, attachedSessions); err != nil {
			log.Printf("terminal: close_attachments_for_removed_device_failed id=%s err=%v", persistent.ID, err)
		}
		return d.removePersistentSessionRecords(persistent, attachedSessions)
	}
	if d.SessionStreams != nil {
		return d.SessionStreams.Delete(persistentStreamKey(persistent.ID), remove)
	}
	return remove()
}
