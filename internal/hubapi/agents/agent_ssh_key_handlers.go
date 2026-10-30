package agents

import (
	"encoding/json"
	"log"
	"net"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/securityruntime"
)

func (d *Deps) ProcessAgentSSHKeyRemoved(conn *agentmgr.AgentConn, _ agentmgr.Message) {
	securityruntime.Logf("agentws: SSH key removed on %s", conn.AssetID)
}

// sendSSHKeyInstall sends the hub SSH public key to a newly connected agent,
// with the comment uninstall uses to find every Hub key.
func (d *Deps) SendSSHKeyInstall(conn *agentmgr.AgentConn) {
	identity := d.currentHubIdentity()
	if identity == nil {
		return
	}
	data, _ := json.Marshal(agentmgr.SSHKeyInstallData{
		PublicKey: shared.HubAuthorizedKeyLine(identity.PublicKey),
	})
	if err := conn.Send(agentmgr.Message{
		Type: agentmgr.MsgSSHKeyInstall,
		Data: data,
	}); err != nil {
		log.Printf("agentws: failed to send ssh_key.install to %s: %v", conn.AssetID, err)
	}
}

// SendSSHKeyRemove asks the agent to remove the hub's SSH public key, both the
// plain line older Hubs installed and the commented line. It returns an error
// when a message could not be sent; with no Hub SSH key it sends nothing.
func (d *Deps) SendSSHKeyRemove(conn *agentmgr.AgentConn) error {
	identity := d.currentHubIdentity()
	if identity == nil {
		return nil
	}
	for _, line := range shared.HubAuthorizedKeyRemovals(identity.PublicKey) {
		data, _ := json.Marshal(agentmgr.SSHKeyRemoveData{PublicKey: line})
		if err := conn.Send(agentmgr.Message{
			Type: agentmgr.MsgSSHKeyRemove,
			Data: data,
		}); err != nil {
			log.Printf("agentws: failed to send ssh_key.remove to %s: %v", conn.AssetID, err)
			return err
		}
	}
	return nil
}

// processAgentSSHKeyInstalled handles the confirmation that the agent installed the hub's SSH key.
func (d *Deps) ProcessAgentSSHKeyInstalled(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.SSHKeyInstalledData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("agentws: invalid ssh_key.installed from %s: %v", conn.AssetID, err)
		return
	}

	securityruntime.Logf("agentws: SSH key installed on %s (user=%s, host=%s)", conn.AssetID, data.Username, data.Hostname)

	identity := d.currentHubIdentity()
	if identity == nil {
		return
	}
	// A device with the Hub SSH key off gets no SSH login config, even when
	// its agent reports a key the Hub never sent. The reply can arrive after
	// the key was turned off (an install sent on reconnect), so the key may
	// be on the device now: ask the agent to remove it again.
	if d.HubSSHKeyEnabled == nil || !d.HubSSHKeyEnabled(conn.AssetID) {
		d.removeHubSSHKeyAfterInstall(conn)
		return
	}
	if d.CredentialStore == nil {
		return
	}

	if _, ok, err := d.CredentialStore.GetAssetTerminalConfig(conn.AssetID); err != nil {
		log.Printf("agentws: failed to load terminal config for %s: %v", conn.AssetID, err)
		return
	} else if ok {
		return
	}

	// SSH to the address this connection came from. The agent's own hostname
	// report must not choose where the Hub's key is used.
	host := ""
	if ip := net.ParseIP(strings.TrimSpace(conn.Meta("ssh_source_ip"))); ip != nil && !ip.IsLoopback() && !ip.IsUnspecified() {
		host = ip.String()
	}
	if host == "" {
		securityruntime.Logf("agentws: no usable source address for %s; SSH fallback needs a manual terminal config", conn.AssetID)
		return
	}

	// Auto-create AssetTerminalConfig so SSH sessions use the hub key. A
	// machine or system account is not an SSH user; the terminal then asks
	// for one.
	cfg := credentials.AssetTerminalConfig{
		AssetID:             conn.AssetID,
		Host:                host,
		Port:                22,
		Username:            shared.SSHLoginUser(data.Username),
		StrictHostKey:       true,
		CredentialProfileID: identity.ProfileID,
		UpdatedAt:           time.Now().UTC(),
	}

	if _, err := d.CredentialStore.SaveAssetTerminalConfig(cfg); err != nil {
		log.Printf("agentws: failed to save terminal config for %s: %v", conn.AssetID, err)
	}
}

func (d *Deps) removeHubSSHKeyAfterInstall(conn *agentmgr.AgentConn) {
	if d.RemoveHubSSHKeyIfOff != nil {
		d.RemoveHubSSHKeyIfOff(conn)
		return
	}
	_ = d.SendSSHKeyRemove(conn)
}
