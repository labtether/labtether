package main

import (
	"errors"
	"log"
	"strings"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
)

var errHubSSHKeySettingUnavailable = errors.New("hub SSH key setting is unavailable")

// hubSSHKeyChange records what changing a device's Hub SSH key setting did.
type hubSSHKeyChange struct {
	Previous    persistence.HubSSHKeySetting
	Setting     persistence.HubSSHKeySetting
	InstallSent bool
	RemovalSent bool
}

// loadHubSSHKeySetting returns the device's Hub SSH key setting. ok is false
// when the setting cannot be read; callers then treat the key as off.
func (s *apiServer) loadHubSSHKeySetting(assetID string) (persistence.HubSSHKeySetting, bool) {
	assetID = strings.TrimSpace(assetID)
	if s.hubSSHKeyStore == nil || assetID == "" {
		return persistence.HubSSHKeySetting{AssetID: assetID}, false
	}
	setting, err := s.hubSSHKeyStore.GetHubSSHKeySetting(assetID)
	if err != nil {
		log.Printf("hub-ssh-key: failed to load setting for %s: %v", assetID, err) // #nosec G706 -- Asset IDs are Hub records; no key material is logged.
		return persistence.HubSSHKeySetting{AssetID: assetID}, false
	}
	return setting, true
}

// hubSSHKeyEnabled reports whether an admin turned on the Hub SSH key for the
// device. The Hub only adds its key to such devices and only logs in to them
// with it.
func (s *apiServer) hubSSHKeyEnabled(assetID string) bool {
	setting, ok := s.loadHubSSHKeySetting(assetID)
	return ok && setting.Enabled
}

// hubSSHKeyMayBeInstalled reports whether the Hub SSH key may be in the
// device's authorized_keys: the setting is on, a removal is pending, or the
// setting cannot be read.
func (s *apiServer) hubSSHKeyMayBeInstalled(assetID string) bool {
	setting, ok := s.loadHubSSHKeySetting(assetID)
	return !ok || setting.Enabled || setting.RemovalPending
}

func (s *apiServer) connectedAgent(assetID string) (*agentmgr.AgentConn, bool) {
	if s.agentMgr == nil {
		return nil, false
	}
	return s.agentMgr.Get(strings.TrimSpace(assetID))
}

// syncHubSSHKeyOnConnect runs when an agent connects. It installs the Hub SSH
// key on a device that has it on, and sends a removal that waited for this
// agent. A device with the key off never receives it.
func (s *apiServer) syncHubSSHKeyOnConnect(conn *agentmgr.AgentConn) {
	if conn == nil || s.currentHubSSHIdentity() == nil {
		return
	}
	s.hubIdentityOperationMu.Lock()
	defer s.hubIdentityOperationMu.Unlock()
	identity := s.currentHubSSHIdentity()
	setting, ok := s.loadHubSSHKeySetting(conn.AssetID)
	if identity == nil || !ok {
		return
	}
	switch {
	case setting.Enabled:
		if err := sendHubSSHKeyMessage(conn, agentmgr.MsgSSHKeyInstall, identity.PublicKey); err != nil {
			log.Printf("hub-ssh-key: failed to send key to %s: %v", conn.AssetID, err) // #nosec G706 -- Asset IDs are Hub records; no key material is logged.
		}
	case setting.RemovalPending:
		s.sendHubSSHKeyRemoval(conn, identity.PublicKey)
	}
}

// removeHubSSHKeyIfOff answers an "installed" reply from a device whose Hub
// SSH key is off, for example an install sent on reconnect that the agent
// finished after an admin turned the key off. Under the identity lock it
// checks the setting again, so a key turned back on meanwhile is kept. A
// removal that cannot be sent is kept as pending for the next connect.
func (s *apiServer) removeHubSSHKeyIfOff(conn *agentmgr.AgentConn) {
	if conn == nil {
		return
	}
	s.hubIdentityOperationMu.Lock()
	defer s.hubIdentityOperationMu.Unlock()
	identity := s.currentHubSSHIdentity()
	setting, ok := s.loadHubSSHKeySetting(conn.AssetID)
	if identity == nil || (ok && setting.Enabled) {
		return
	}
	if err := sendHubSSHKeyMessage(conn, agentmgr.MsgSSHKeyRemove, identity.PublicKey); err != nil {
		log.Printf("hub-ssh-key: failed to send key removal to %s: %v", conn.AssetID, err) // #nosec G706 -- Asset IDs are Hub records; no key material is logged.
		if ok {
			if _, err := s.hubSSHKeyStore.SetHubSSHKeySetting(conn.AssetID, false, true); err != nil {
				log.Printf("hub-ssh-key: failed to keep pending removal for %s: %v", conn.AssetID, err) // #nosec G706 -- Asset IDs are Hub records; no key material is logged.
			}
		}
		return
	}
	if ok && setting.RemovalPending {
		if err := s.hubSSHKeyStore.ClearHubSSHKeyRemovalPending(conn.AssetID); err != nil {
			log.Printf("hub-ssh-key: failed to clear pending removal for %s: %v", conn.AssetID, err) // #nosec G706 -- Asset IDs are Hub records; no key material is logged.
		}
	}
}

// sendHubSSHKeyRemoval asks the agent to remove the plain and commented Hub
// key lines, then clears the pending removal. Callers hold
// hubIdentityOperationMu.
func (s *apiServer) sendHubSSHKeyRemoval(conn *agentmgr.AgentConn, publicKey string) bool {
	if err := sendHubSSHKeyMessage(conn, agentmgr.MsgSSHKeyRemove, publicKey); err != nil {
		log.Printf("hub-ssh-key: failed to send key removal to %s: %v", conn.AssetID, err) // #nosec G706 -- Asset IDs are Hub records; no key material is logged.
		return false
	}
	if err := s.hubSSHKeyStore.ClearHubSSHKeyRemovalPending(conn.AssetID); err != nil {
		log.Printf("hub-ssh-key: failed to clear pending removal for %s: %v", conn.AssetID, err) // #nosec G706 -- Asset IDs are Hub records; no key material is logged.
		return false
	}
	return true
}

// applyHubSSHKeySetting stores the choice and tells a connected agent at once.
// Turning the key off on an agent device that may have it stores a pending
// removal first, so an offline agent, or one a send fails to reach, gets the
// removal when it next connects.
func (s *apiServer) applyHubSSHKeySetting(asset assets.Asset, enabled bool) (hubSSHKeyChange, error) {
	s.hubIdentityOperationMu.Lock()
	defer s.hubIdentityOperationMu.Unlock()

	var change hubSSHKeyChange
	previous, ok := s.loadHubSSHKeySetting(asset.ID)
	if !ok {
		return change, errHubSSHKeySettingUnavailable
	}
	change.Previous = previous
	conn, connected := s.connectedAgent(asset.ID)
	agentDevice := connected || strings.EqualFold(strings.TrimSpace(asset.Source), "agent")
	pending := !enabled && agentDevice && (previous.Enabled || previous.RemovalPending)
	setting, err := s.hubSSHKeyStore.SetHubSSHKeySetting(asset.ID, enabled, pending)
	if err != nil {
		return change, err
	}
	change.Setting = setting

	identity := s.currentHubSSHIdentity()
	if !connected || identity == nil {
		return change, nil
	}
	if enabled {
		if err := sendHubSSHKeyMessage(conn, agentmgr.MsgSSHKeyInstall, identity.PublicKey); err != nil {
			log.Printf("hub-ssh-key: failed to send key to %s: %v", conn.AssetID, err) // #nosec G706 -- Asset IDs are Hub records; no key material is logged.
			return change, nil
		}
		change.InstallSent = true
		return change, nil
	}
	if pending && s.sendHubSSHKeyRemoval(conn, identity.PublicKey) {
		change.RemovalSent = true
		change.Setting.RemovalPending = false
	}
	return change, nil
}
