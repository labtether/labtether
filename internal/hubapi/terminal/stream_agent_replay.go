package terminal

// Capture the old history before sending terminal.start. Taking a snapshot
// after startup mixes newly queued bytes into replay, causing duplicates (or
// dropping the first live chunk when only stored history was replayed).
func (d *Deps) snapshotAgentScrollback(bridge *TerminalBridge) []byte {
	if bridge.PersistentSessionID == "" {
		return nil
	}
	bridge.OutputMu.Lock()
	var buffer []byte
	if bridge.Scrollback != nil {
		buffer = bridge.Scrollback.Snapshot()
	}
	bridge.OutputMu.Unlock()
	if len(buffer) == 0 && d.TerminalScrollbackStore != nil {
		stored, err := d.TerminalScrollbackStore.GetScrollback(bridge.PersistentSessionID)
		if err == nil {
			buffer = append([]byte(nil), stored...)
		}
	}
	return buffer
}
