package portainer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/connectorsdk"
)

// TestLivePortainerRuntimeLifecycle is an opt-in, destructive-only-to-its-own
// fixtures check against a disposable Portainer instance. Normal unit and CI
// runs skip it.
func TestLivePortainerRuntimeLifecycle(t *testing.T) {
	if os.Getenv("LABTETHER_LIVE_PORTAINER_QA") != "1" {
		t.Skip("set LABTETHER_LIVE_PORTAINER_QA=1 for disposable Portainer QA")
	}
	baseURL := strings.TrimSpace(os.Getenv("LABTETHER_PORTAINER_QA_URL"))
	password := os.Getenv("LABTETHER_PORTAINER_QA_PASSWORD")
	if baseURL == "" || password == "" {
		t.Fatal("LABTETHER_PORTAINER_QA_URL and LABTETHER_PORTAINER_QA_PASSWORD are required")
	}
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")

	client := NewClient(Config{BaseURL: baseURL, Username: "admin", Password: password, Timeout: 20 * time.Second})
	connector := NewWithClient(client)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	health, err := connector.TestConnection(ctx)
	if err != nil || health.Status != "ok" {
		t.Fatalf("Portainer health=%+v err=%v", health, err)
	}
	version, err := client.GetVersion(ctx)
	if err != nil || strings.TrimSpace(version.ServerVersion) == "" {
		t.Fatalf("Portainer version=%+v err=%v", version, err)
	}
	endpoints, err := client.GetEndpoints(ctx)
	if err != nil || len(endpoints) != 1 || endpoints[0].Status != 1 {
		t.Fatalf("Portainer endpoints=%+v err=%v", endpoints, err)
	}
	epID := endpoints[0].ID

	bad := NewWithClient(NewClient(Config{BaseURL: baseURL, Username: "admin", Password: password + "-wrong", Timeout: 5 * time.Second}))
	badHealth, err := bad.TestConnection(ctx)
	if err != nil || badHealth.Status != "failed" {
		t.Fatalf("bad-credential health=%+v err=%v", badHealth, err)
	}

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	prefix := "ltqa-portainer-"
	containerName := prefix + "container-" + suffix
	volumeName := prefix + "volume-" + suffix
	networkName := prefix + "network-" + suffix
	stackName := prefix + "stack-" + suffix
	imageRepo := prefix + "image"
	imageTag := suffix
	for _, value := range []string{containerName, volumeName, networkName, stackName, imageRepo} {
		if !strings.HasPrefix(value, prefix) {
			t.Fatalf("refusing unsafe Portainer fixture name %q", value)
		}
	}

	var containerID, networkID string
	var stackID int
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if stackID != 0 {
			_ = client.RemoveStack(cleanupCtx, stackID, epID)
		}
		if containerID != "" {
			_ = client.RemoveContainer(cleanupCtx, epID, containerID, true)
		}
		if networkID != "" {
			_ = client.RemoveNetwork(cleanupCtx, epID, networkID)
		}
		_ = client.RemoveVolume(cleanupCtx, epID, volumeName)
		_ = client.RemoveImage(cleanupCtx, epID, imageRepo+":"+imageTag)
	}()

	if err := client.PullImage(ctx, epID, "alpine:3.22"); err != nil {
		t.Fatalf("pull image: %v", err)
	}
	tagPath := fmt.Sprintf("/api/endpoints/%d/docker/images/%s/tag?repo=%s&tag=%s", epID, "alpine:3.22", imageRepo, imageTag)
	if _, err := client.post(ctx, tagPath, nil); err != nil {
		t.Fatalf("tag disposable image: %v", err)
	}
	images, err := client.GetImages(ctx, epID)
	if err != nil || !rawJSONContains(images, imageRepo+":"+imageTag) {
		t.Fatalf("image inventory missing disposable tag; count=%d err=%v", len(images), err)
	}
	if err := client.RemoveImage(ctx, epID, imageRepo+":"+imageTag); err != nil {
		t.Fatalf("remove disposable image tag: %v", err)
	}

	volume, err := client.CreateVolume(ctx, epID, volumeName, "local")
	if err != nil || !strings.Contains(string(volume), volumeName) {
		t.Fatalf("create volume=%s err=%v", volume, err)
	}
	volumes, err := client.GetVolumes(ctx, epID)
	if err != nil || !strings.Contains(string(volumes), volumeName) {
		t.Fatalf("volume inventory missing %q err=%v", volumeName, err)
	}

	network, err := client.CreateNetwork(ctx, epID, networkName, "bridge", "", "")
	if err != nil {
		t.Fatalf("create network: %v", err)
	}
	var createdNetwork struct {
		ID string `json:"Id"`
	}
	if err := json.Unmarshal(network, &createdNetwork); err != nil || createdNetwork.ID == "" {
		t.Fatalf("decode network=%s err=%v", network, err)
	}
	networkID = createdNetwork.ID
	networks, err := client.GetNetworks(ctx, epID)
	if err != nil || !rawJSONContains(networks, networkName) {
		t.Fatalf("network inventory missing %q err=%v", networkName, err)
	}

	createdContainer, err := client.post(ctx,
		fmt.Sprintf("/api/endpoints/%d/docker/containers/create?name=%s", epID, containerName),
		map[string]any{
			"Image": "alpine:3.22",
			"Cmd":   []string{"/bin/sh", "-c", "echo LTQA_PORTAINER_READY; exec sleep 600"},
			"Labels": map[string]string{
				"com.labtether.qa.scope": "disposable",
			},
		})
	if err != nil {
		t.Fatalf("create disposable container: %v", err)
	}
	var createdContainerResponse struct {
		ID string `json:"Id"`
	}
	if err := json.Unmarshal(createdContainer, &createdContainerResponse); err != nil || createdContainerResponse.ID == "" {
		t.Fatalf("decode container response=%s err=%v", createdContainer, err)
	}
	containerID = createdContainerResponse.ID
	if err := client.ContainerAction(ctx, epID, containerID, "start"); err != nil {
		t.Fatalf("start disposable container: %v", err)
	}
	waitForPortainerContainerState(t, ctx, client, epID, containerID, "running")
	logs, err := client.GetContainerLogs(ctx, epID, containerID, 20, true)
	if err != nil || !strings.Contains(logs, "LTQA_PORTAINER_READY") {
		t.Fatalf("container logs=%q err=%v", logs, err)
	}
	inspect, err := client.InspectContainer(ctx, epID, containerID)
	if err != nil || !strings.Contains(string(inspect), containerName) {
		t.Fatalf("container inspect missing name err=%v", err)
	}

	for _, action := range []struct {
		name string
		want string
	}{
		{name: "pause", want: "paused"},
		{name: "unpause", want: "running"},
		{name: "restart", want: "running"},
		{name: "stop", want: "exited"},
		{name: "start", want: "running"},
		{name: "kill", want: "exited"},
		{name: "start", want: "running"},
	} {
		if err := client.ContainerAction(ctx, epID, containerID, action.name); err != nil {
			t.Fatalf("container %s: %v", action.name, err)
		}
		waitForPortainerContainerState(t, ctx, client, epID, containerID, action.want)
	}

	execID, err := client.CreateExec(ctx, epID, containerID, []string{"/bin/sh", "-c", "printf LTQA_PORTAINER_EXEC_OK"})
	if err != nil {
		t.Fatalf("create Portainer exec: %v", err)
	}
	wsURL, wsToken, err := client.ExecWebSocketURL(ctx, epID, execID)
	if err != nil {
		t.Fatalf("Portainer exec URL: %v", err)
	}
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+wsToken)
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if err != nil {
		t.Fatalf("dial Portainer exec: %v", err)
	}
	_ = ws.SetReadDeadline(time.Now().Add(15 * time.Second))
	_, execOutput, err := ws.ReadMessage()
	_ = ws.Close()
	if err != nil || !strings.Contains(string(execOutput), "LTQA_PORTAINER_EXEC_OK") {
		t.Fatalf("Portainer exec output=%q err=%v", execOutput, err)
	}

	compose := "services:\n  qa:\n    image: alpine:3.22\n    command: [\"sleep\", \"600\"]\n"
	stackPayload, err := client.post(ctx,
		fmt.Sprintf("/api/stacks/create/standalone/string?endpointId=%d", epID),
		map[string]any{"name": stackName, "stackFileContent": compose})
	if err != nil {
		t.Fatalf("create disposable stack: %v", err)
	}
	var createdStack Stack
	if err := json.Unmarshal(stackPayload, &createdStack); err != nil || createdStack.ID == 0 {
		t.Fatalf("decode stack response=%s err=%v", stackPayload, err)
	}
	stackID = createdStack.ID
	stacks, err := client.GetStacks(ctx)
	if err != nil || !stackListContains(stacks, stackID, stackName) {
		t.Fatalf("stack inventory missing id=%d name=%q err=%v", stackID, stackName, err)
	}
	stackCompose, err := client.GetStackCompose(ctx, stackID)
	if err != nil || !strings.Contains(stackCompose, "alpine:3.22") {
		t.Fatalf("stack compose=%q err=%v", stackCompose, err)
	}
	if err := client.UpdateStackCompose(ctx, stackID, epID, compose+"    environment: [\"LTQA_STACK=updated\"]\n"); err != nil {
		t.Fatalf("update stack compose: %v", err)
	}
	if err := client.StopStack(ctx, stackID, epID); err != nil {
		t.Fatalf("stop stack: %v", err)
	}
	waitForPortainerStackStatus(t, ctx, client, stackID, 2)
	if err := client.StartStack(ctx, stackID, epID); err != nil {
		t.Fatalf("start stack: %v", err)
	}
	waitForPortainerStackStatus(t, ctx, client, stackID, 1)

	assets, err := connector.Discover(ctx)
	if err != nil || !connectorAssetsContain(assets, containerID, stackID, epID) {
		t.Fatalf("connector discovery incomplete: assets=%d err=%v", len(assets), err)
	}
	dryRun, err := connector.ExecuteAction(ctx, "container.restart", connectorsdk.ActionRequest{
		TargetID: fmt.Sprintf("portainer-container-%d-%s", epID, containerID[:12]), DryRun: true,
	})
	if err != nil || dryRun.Status != "succeeded" {
		t.Fatalf("connector dry-run=%+v err=%v", dryRun, err)
	}

	if err := client.RemoveStack(ctx, stackID, epID); err != nil {
		t.Fatalf("remove stack: %v", err)
	}
	waitForPortainerStackAbsent(t, ctx, client, stackID)
	stackID = 0
	if err := client.RemoveContainer(ctx, epID, containerID, true); err != nil {
		t.Fatalf("remove container: %v", err)
	}
	waitForPortainerContainerAbsent(t, ctx, client, epID, containerID)
	containerID = ""
	if err := client.RemoveNetwork(ctx, epID, networkID); err != nil {
		t.Fatalf("remove network: %v", err)
	}
	networkID = ""
	if err := client.RemoveVolume(ctx, epID, volumeName); err != nil {
		t.Fatalf("remove volume: %v", err)
	}
}

func rawJSONContains(values []json.RawMessage, needle string) bool {
	for _, value := range values {
		if strings.Contains(string(value), needle) {
			return true
		}
	}
	return false
}

func stackListContains(stacks []Stack, id int, name string) bool {
	for _, stack := range stacks {
		if stack.ID == id && stack.Name == name {
			return true
		}
	}
	return false
}

func connectorAssetsContain(assets []connectorsdk.Asset, containerID string, stackID, epID int) bool {
	wantContainer := fmt.Sprintf("portainer-container-%d-%s", epID, containerID[:12])
	wantStack := fmt.Sprintf("portainer-stack-%d", stackID)
	wantEndpoint := fmt.Sprintf("portainer-endpoint-%d", epID)
	found := map[string]bool{}
	for _, asset := range assets {
		if asset.ID == wantContainer || asset.ID == wantStack || asset.ID == wantEndpoint {
			found[asset.ID] = true
		}
	}
	return found[wantContainer] && found[wantStack] && found[wantEndpoint]
}

func waitForPortainerContainerState(t *testing.T, ctx context.Context, client *Client, epID int, containerID, want string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		containers, err := client.GetContainers(ctx, epID)
		if err == nil {
			for _, container := range containers {
				if container.ID == containerID && container.State == want {
					return
				}
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("container %s did not reach %s", containerID, want)
}

func waitForPortainerContainerAbsent(t *testing.T, ctx context.Context, client *Client, epID int, containerID string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		containers, err := client.GetContainers(ctx, epID)
		if err == nil {
			found := false
			for _, container := range containers {
				found = found || container.ID == containerID
			}
			if !found {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("container %s still exists", containerID)
}

func waitForPortainerStackStatus(t *testing.T, ctx context.Context, client *Client, stackID, want int) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		stacks, err := client.GetStacks(ctx)
		if err == nil {
			for _, stack := range stacks {
				if stack.ID == stackID && stack.Status == want {
					return
				}
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("stack %d did not reach status %d", stackID, want)
}

func waitForPortainerStackAbsent(t *testing.T, ctx context.Context, client *Client, stackID int) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		stacks, err := client.GetStacks(ctx)
		if err == nil {
			found := false
			for _, stack := range stacks {
				found = found || stack.ID == stackID
			}
			if !found {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("stack %d still exists", stackID)
}
