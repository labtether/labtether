package agents

import (
	"errors"
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/enrollment"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

// What revoking an agent token did about the Hub SSH key on its device.
const (
	// HubSSHKeyRemovalSent: the connected agent was asked to remove the key
	// before its token was revoked.
	HubSSHKeyRemovalSent = "removal_sent"
	// HubSSHKeyNotRemoved: the Hub could not reach the agent, so the key may
	// still be in the device's authorized_keys.
	HubSSHKeyNotRemoved = "not_removed"
)

const agentTokenRevokedAuditType = "agent.token.revoked"

// HandleAgentTokenActions handles DELETE /settings/agent-tokens/{id} (admin-auth).
//
// Once a token is revoked the Hub can no longer send to its agent, so a
// connected agent is asked to remove the Hub SSH key first. The request is
// checked before that message, so a refused revoke changes nothing. When the
// agent cannot be reached, the response and the audit log say the key may
// still be on the device.
func (d *Deps) HandleAgentTokenActions(w http.ResponseWriter, r *http.Request) {
	if d.EnrollmentStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "enrollment not available")
		return
	}

	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/settings/agent-tokens/"))
	if id == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "token id required")
		return
	}
	if r.Method != http.MethodDelete {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	token, found, err := d.EnrollmentStore.GetAgentToken(id)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to revoke agent token")
		return
	}
	if !found {
		servicehttp.WriteError(w, http.StatusNotFound, "agent token not found")
		return
	}

	conns := d.agentConnsUsingToken(id)
	hubSSHKey := ""
	if token.Status == "active" {
		hubSSHKey = d.removeHubSSHKeyBeforeRevoke(token, conns)
	}

	if err := d.EnrollmentStore.RevokeAgentToken(id); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			servicehttp.WriteError(w, http.StatusNotFound, "agent token not found")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to revoke agent token")
		return
	}
	// Drop a matching local socket immediately. Other hub replicas enforce the
	// same revocation before their next inbound message, ping, or outbound send
	// through the connection credential validator.
	for _, conn := range conns {
		d.AgentMgr.UnregisterIfMatch(conn.AssetID, conn)
	}
	d.auditAgentTokenRevoked(r, token, hubSSHKey)

	response := map[string]string{"status": "revoked"}
	if hubSSHKey != "" {
		response["hub_ssh_key"] = hubSSHKey
		response["asset_id"] = token.AssetID
	}
	servicehttp.WriteJSON(w, http.StatusOK, response)
}

// agentConnsUsingToken returns the local agent connections authenticated
// with the token.
func (d *Deps) agentConnsUsingToken(tokenID string) []*agentmgr.AgentConn {
	if d.AgentMgr == nil {
		return nil
	}
	var conns []*agentmgr.AgentConn
	for _, assetID := range d.AgentMgr.ConnectedAssets() {
		conn, ok := d.AgentMgr.Get(assetID)
		if ok && strings.TrimSpace(conn.Meta("auth.agent_token_id")) == tokenID {
			conns = append(conns, conn)
		}
	}
	return conns
}

// removeHubSSHKeyBeforeRevoke sends the same removal a device delete sends
// to the agent using the token, while the token is still valid. It returns
// "" when there is no Hub SSH key that could be on the device.
func (d *Deps) removeHubSSHKeyBeforeRevoke(token enrollment.AgentToken, conns []*agentmgr.AgentConn) string {
	if d.currentHubIdentity() == nil {
		return ""
	}
	for _, conn := range conns {
		if conn.AssetID != token.AssetID {
			continue
		}
		if d.sendHubSSHKeyRemoval(conn) != nil {
			return HubSSHKeyNotRemoved
		}
		return HubSSHKeyRemovalSent
	}
	if d.HubSSHKeyMayBeInstalled != nil && !d.HubSSHKeyMayBeInstalled(token.AssetID) {
		return ""
	}
	return HubSSHKeyNotRemoved
}

func (d *Deps) sendHubSSHKeyRemoval(conn *agentmgr.AgentConn) error {
	if d.SendHubSSHKeyRemoval != nil {
		return d.SendHubSSHKeyRemoval(conn)
	}
	return d.SendSSHKeyRemove(conn)
}

func (d *Deps) auditAgentTokenRevoked(r *http.Request, token enrollment.AgentToken, hubSSHKey string) {
	if d.AppendAuditEventBestEffort == nil {
		return
	}
	event := audit.NewEvent(agentTokenRevokedAuditType)
	if d.PrincipalActorID != nil {
		event.ActorID = strings.TrimSpace(d.PrincipalActorID(r.Context()))
	}
	event.Target = token.ID
	event.Decision = "applied"
	event.Details = map[string]any{"asset_id": token.AssetID}
	if hubSSHKey != "" {
		event.Details["hub_ssh_key"] = hubSSHKey
	}
	d.AppendAuditEventBestEffort(event, "api warning: failed to append agent token revoke audit event")
}
