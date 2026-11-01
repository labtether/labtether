package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/enrollment"
	"github.com/labtether/labtether/internal/hubapi/shared"
)

func createRevokeTestAgentToken(t *testing.T, server *apiServer, assetID string) enrollment.AgentToken {
	t.Helper()
	upsertHubSSHKeyTestAgent(t, server, assetID)
	enableTestHubSSHKey(t, server, assetID)
	_, tokenHash, err := auth.GenerateSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	token, err := server.enrollmentStore.CreateAgentToken(assetID, tokenHash, "test", time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatalf("create agent token for %s: %v", assetID, err)
	}
	return token
}

func revokeTestAgentToken(t *testing.T, server *apiServer, tokenID string) map[string]string {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, "/settings/agent-tokens/"+tokenID, nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin-user", auth.RoleAdmin))
	rec := httptest.NewRecorder()
	server.handleAgentTokenActions(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke %s: status=%d body=%s", tokenID, rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode revoke response: %v", err)
	}
	return body
}

// Revoking an agent cuts the Hub off from it, so a connected agent must be
// asked to remove the Hub's root SSH key first or the key stays in
// authorized_keys. When the agent is offline the revoke must say the key
// stayed, in the response and the audit log.
func TestRevokingAgentTokenRemovesHubSSHKeyFirst(t *testing.T) {
	server := newTestAPIServer(t)
	identity := initializeTestHubSSHIdentity(t, server)

	online := createRevokeTestAgentToken(t, server, "node-online")
	conn, agent := connectHubSSHKeyTestAgent(t, server, "node-online")
	conn.SetMeta("auth.agent_token_id", online.ID)
	// As in production, nothing reaches the agent once its token is revoked.
	conn.SetCredentialValidator(func() error {
		if token, _, _ := server.enrollmentStore.GetAgentToken(online.ID); token.Status != "active" {
			return errors.New("agent token revoked")
		}
		return nil
	})

	if body := revokeTestAgentToken(t, server, online.ID); body["hub_ssh_key"] != "removal_sent" {
		t.Fatalf("connected revoke response=%v, want hub_ssh_key=removal_sent", body)
	}
	for _, want := range []string{strings.TrimSpace(identity.PublicKey), shared.HubAuthorizedKeyLine(identity.PublicKey)} {
		if got := readRawHubSSHKeyPayload(t, agent, agentmgr.MsgSSHKeyRemove); got != want {
			t.Fatalf("removed line=%q, want %q", got, want)
		}
	}
	if token, _, _ := server.enrollmentStore.GetAgentToken(online.ID); token.Status != "revoked" {
		t.Fatalf("token status=%q, want revoked", token.Status)
	}
	if _, connected := server.agentMgr.Get("node-online"); connected {
		t.Fatal("revoked agent is still connected")
	}

	offline := createRevokeTestAgentToken(t, server, "node-offline")
	if body := revokeTestAgentToken(t, server, offline.ID); body["hub_ssh_key"] != "not_removed" {
		t.Fatalf("offline revoke response=%v, want hub_ssh_key=not_removed", body)
	}
	events, err := server.auditStore.List(20, 0)
	if err != nil {
		t.Fatal(err)
	}
	recorded := map[string]any{}
	for _, event := range events {
		if event.Type == "agent.token.revoked" {
			recorded[event.Target] = event.Details["hub_ssh_key"]
		}
	}
	if recorded[online.ID] != "removal_sent" || recorded[offline.ID] != "not_removed" {
		t.Fatalf("audit hub_ssh_key by token=%v", recorded)
	}
}
