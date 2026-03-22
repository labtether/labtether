package agents

import (
	"encoding/json"
	"log"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/securityruntime"
)

func (d *Deps) ProcessAgentSSHKeyRemoved(conn *agentmgr.AgentConn, _ agentmgr.Message) {
	securityruntime.Logf("agentws: SSH key removed on %s", conn.AssetID)
}

// sendSSHKeyInstall sends the hub SSH public key to a newly connected agent.
func (d *Deps) SendSSHKeyInstall(conn *agentmgr.AgentConn) {
	data, _ := json.Marshal(agentmgr.SSHKeyInstallData{
		PublicKey: d.HubIdentity.PublicKey,
	})
	if err := conn.Send(agentmgr.Message{
		Type: agentmgr.MsgSSHKeyInstall,
		Data: data,
	}); err != nil {
		log.Printf("agentws: failed to send ssh_key.install to %s: %v", conn.AssetID, err)
	}
}

// sendSSHKeyRemove sends a request to the agent to remove the hub's SSH public key.
func (d *Deps) SendSSHKeyRemove(conn *agentmgr.AgentConn) {
	data, _ := json.Marshal(agentmgr.SSHKeyRemoveData{
		PublicKey: d.HubIdentity.PublicKey,
	})
	if err := conn.Send(agentmgr.Message{
		Type: agentmgr.MsgSSHKeyRemove,
		Data: data,
	}); err != nil {
		log.Printf("agentws: failed to send ssh_key.remove to %s: %v", conn.AssetID, err)
	}
}

// processAgentSSHKeyInstalled handles the confirmation that the agent installed the hub's SSH key.
func (d *Deps) ProcessAgentSSHKeyInstalled(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.SSHKeyInstalledData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("agentws: invalid ssh_key.installed from %s: %v", conn.AssetID, err)
		return
	}

	log.Printf("agentws: SSH key installed on %s (user=%s, host=%s)", conn.AssetID, data.Username, data.Hostname)

	if d.CredentialStore == nil || d.HubIdentity == nil {
		return
	}

	if _, ok, err := d.CredentialStore.GetAssetTerminalConfig(conn.AssetID); err != nil {
		log.Printf("agentws: failed to load terminal config for %s: %v", conn.AssetID, err)
		return
	} else if ok {
		return
	}

	// Determine the host to use for SSH connections.
	host := data.Hostname
	if host == "" {
		host = conn.AssetID
	}

	// Auto-create AssetTerminalConfig so SSH sessions use the hub key.
	cfg := credentials.AssetTerminalConfig{
		AssetID:             conn.AssetID,
		Host:                host,
		Port:                22,
		Username:            data.Username,
		StrictHostKey:       true,
		CredentialProfileID: d.HubIdentity.ProfileID,
		UpdatedAt:           time.Now().UTC(),
	}

	if _, err := d.CredentialStore.SaveAssetTerminalConfig(cfg); err != nil {
		log.Printf("agentws: failed to save terminal config for %s: %v", conn.AssetID, err)
	}
}
