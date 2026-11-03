package dockerpkg

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectors/docker"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/persistence"
)

// Stopping a container whose Docker host was offline answered "cannot bind
// connector action target". The binding now says which device is offline
// and when its agent was last seen.
func TestBindActionOnOfflineDockerHostSaysTheHostIsOffline(t *testing.T) {
	store := persistence.NewMemoryAssetStore()
	if _, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: "docker01", Name: "docker01", Source: "agent", Type: "host", Status: "online"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: "docker-ct-docker01-1359523b1fbc", Name: "web", Source: "docker",
		Type: "docker-container", Status: "online", Metadata: map[string]string{"agent_id": "docker01"}}); err != nil {
		t.Fatal(err)
	}
	agent, _, _ := store.GetAsset("docker01")
	manager := agentmgr.NewManager()
	deps := &Deps{DockerCoordinator: docker.NewCoordinator(manager), AgentMgr: manager, AssetStore: store}

	_, err := deps.BindAction(context.Background(), "container.stop", connectorsdk.ActionRequest{TargetID: "docker-ct-docker01-1359523b1fbc"})
	var offline *connectorsdk.TargetOfflineError
	if !errors.As(err, &offline) {
		t.Fatalf("bind error = %v, want TargetOfflineError", err)
	}
	want := "docker01 is offline. Its agent was last seen at " + agent.LastSeenAt.UTC().Format("2006-01-02 15:04 UTC") + "."
	if err.Error() != want {
		t.Fatalf("message = %q, want %q", err.Error(), want)
	}
	if agent.LastSeenAt.IsZero() || time.Since(agent.LastSeenAt) > time.Minute {
		t.Fatalf("fixture last seen = %v", agent.LastSeenAt)
	}
}
