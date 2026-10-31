package dockerpkg

import (
	"encoding/json"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/connectors/docker"
)

// Agent messages reach every console and catch-all webhook through the
// broadcaster, so an agent must not choose the host or the payload shape.
func TestAgentDockerBroadcastsAreAttributedToTheSendingAgent(t *testing.T) {
	type broadcast struct {
		eventType string
		data      json.RawMessage
	}
	var sent []broadcast
	deps := &Deps{
		DockerCoordinator: docker.NewCoordinator(nil),
		Broadcast: func(eventType string, data json.RawMessage) {
			sent = append(sent, broadcast{eventType, data})
		},
	}
	conn := &agentmgr.AgentConn{AssetID: "nas01"}

	deps.ProcessAgentDockerEvents(conn, agentmgr.Message{
		Type: agentmgr.MsgDockerEvents,
		Data: json.RawMessage(`{"host_id":"other-host","type":"container","action":"start","injected":"<script>"}`),
	})
	deps.ProcessAgentDockerEvents(conn, agentmgr.Message{Type: agentmgr.MsgDockerEvents, Data: json.RawMessage(`not json`)})
	deps.ProcessAgentDockerLogsStreamMessage(conn, agentmgr.Message{
		Type: agentmgr.MsgDockerLogsStream,
		Data: json.RawMessage(`{"host":"other-host","line":"forged"}`),
	})

	if len(sent) != 1 || sent[0].eventType != "docker.event" {
		t.Fatalf("broadcasts = %+v, want one docker.event", sent)
	}
	var got map[string]any
	if err := json.Unmarshal(sent[0].data, &got); err != nil {
		t.Fatal(err)
	}
	if got["host_id"] != "nas01" || got["action"] != "start" {
		t.Fatalf("event = %v, want host_id nas01 and the agent's action", got)
	}
	if _, ok := got["injected"]; ok {
		t.Fatalf("unknown agent field passed through: %v", got)
	}
}
