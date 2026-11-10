package desktop

import (
	"strings"
)

// agentDesktopUnreachable reports whether a Remote View session for target can
// only run through the device's agent and that agent is not connected. Session
// create and the stream ticket answered 201 for it, so a client that did not
// ask for the displays first got a session that could never work, and only
// the stream itself refused. RDP and SPICE do not go through the agent, and a
// Proxmox VM or the direct VNC proxy can still serve the device.
func (d *Deps) agentDesktopUnreachable(target, protocol string) bool {
	switch NormalizeDesktopProtocol(protocol) {
	case "rdp", "spice":
		return false
	}
	if d.AssetStore == nil || d.AgentMgr == nil || d.AgentMgr.IsConnected(target) {
		return false
	}
	asset, ok, err := d.AssetStore.GetAsset(target)
	if err != nil || !ok || !strings.EqualFold(strings.TrimSpace(asset.Source), "agent") {
		return false
	}
	if d.ResolveProxmoxSessionTarget != nil {
		if _, proxmox, err := d.ResolveProxmoxSessionTarget(target); err != nil || proxmox {
			return false
		}
	}
	return !d.desktopDirectProxyEnabled()
}
