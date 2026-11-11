package docker

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/connectorsdk"
)

func TestDockerFailureWordsSayWhatDockerSaid(t *testing.T) {
	for raw, want := range map[string]string{
		"docker API DELETE /images/nginx:9.9?force=false returned 404":                                  "Docker has no image named nginx:9.9.",
		"docker API DELETE /images/ghcr.io%2Facme%2Fweb:1?force=false returned 409":                     "Docker did not remove the image ghcr.io/acme/web:1 because it is in use.",
		"docker API POST /containers/abc123/start returned 304: ":                                       "Nothing changed: the container was already in that state.",
		`docker API POST /containers/abc123/stop returned 404: {"message":"No such container: abc123"}`: "No such container: abc123.",
		`docker API /containers/abc123/json returned 500: {"message":"conflict: device busy"}`:          "Conflict: device busy.",
		"docker API DELETE /volumes/data returned 500":                                                  "Docker reported an error.",
		"compose up failed: no configuration file provided":                                             "compose up failed: no configuration file provided",
	} {
		if got := dockerFailureWords(raw); got != want {
			t.Errorf("dockerFailureWords(%q) = %q, want %q", raw, got, want)
		}
	}
}

// The agent's raw "docker API DELETE ... returned 404" reached the console
// as the action's message.
func TestCoordinatorActionFailureUsesDockerWords(t *testing.T) {
	sent := make(chan agentmgr.Message, 1)
	coord := NewCoordinator(&mockAgentCommander{sendFn: func(_ string, msg agentmgr.Message) error {
		sent <- msg
		return nil
	}})
	coord.HandleDiscovery("agent-01", makeDiscoveryMsg(agentmgr.DockerDiscoveryData{
		HostID:     "agent-01",
		Containers: []agentmgr.DockerContainerInfo{{ID: "abc123456789ab", Name: "web", State: "running"}},
	}))
	go func() {
		msg := <-sent
		var req agentmgr.DockerActionData
		_ = json.Unmarshal(msg.Data, &req)
		raw, _ := json.Marshal(agentmgr.DockerActionResultData{
			RequestID: req.RequestID,
			Error:     "docker API DELETE /containers/abc123456789ab?force=true returned 404",
		})
		coord.HandleActionResult("agent-01", agentmgr.Message{Type: agentmgr.MsgDockerActionResult, ID: req.RequestID, Data: raw})
	}()

	result, err := coord.ExecuteAction(context.Background(), "container.remove", connectorsdk.ActionRequest{
		TargetID: "docker-ct-agent-01-abc123456789",
		Params:   map[string]string{"force": "true"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "Docker has no container named abc123456789ab."; result.Status != "failed" || result.Message != want {
		t.Fatalf("got %q %q, want failed %q", result.Status, result.Message, want)
	}
}
