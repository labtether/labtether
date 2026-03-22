package docker

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

func TestHandleDockerActionRejectsInvalidContainerID(t *testing.T) {
	transport := newRecordingCollectorTransport(true)
	dc := NewDockerCollector("/tmp/docker.sock", transport, "asset-1", 30*time.Second)

	raw, err := json.Marshal(agentmgr.DockerActionData{
		RequestID:   "req-invalid",
		Action:      "container.stop",
		ContainerID: "../bad",
	})
	if err != nil {
		t.Fatalf("marshal docker action: %v", err)
	}

	dc.HandleDockerAction(transport, agentmgr.Message{Type: agentmgr.MsgDockerAction, Data: raw})

	msg := waitForCollectorMessage(t, transport, time.Second)
	if msg.Type != agentmgr.MsgDockerActionResult {
		t.Fatalf("message type=%q, want %q", msg.Type, agentmgr.MsgDockerActionResult)
	}

	var result agentmgr.DockerActionResultData
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		t.Fatalf("decode docker action result: %v", err)
	}
	if result.Success {
		t.Fatal("expected invalid container id action to fail")
	}
	if !strings.Contains(result.Error, "invalid container ID") {
		t.Fatalf("unexpected error %q", result.Error)
	}
	if len(dc.discoveryTriggerCh) != 0 || len(dc.statsTriggerCh) != 0 {
		t.Fatalf("expected no refresh triggers after invalid action, got discovery=%d stats=%d", len(dc.discoveryTriggerCh), len(dc.statsTriggerCh))
	}
}

func TestHandleDockerActionContainerCreateStartsContainerAndQueuesRefresh(t *testing.T) {
	transport := newRecordingCollectorTransport(true)

	srv, client := newSecureDockerClientFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/containers/create":
			if got := r.URL.Query().Get("name"); got != "web" {
				t.Fatalf("container name query=%q, want web", got)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode create container body: %v", err)
			}
			if body["Image"] != "nginx:1.27" {
				t.Fatalf("image=%v, want nginx:1.27", body["Image"])
			}
			if cmd, ok := body["Cmd"].([]any); !ok || len(cmd) != 2 || cmd[0] != "sleep" || cmd[1] != "5" {
				t.Fatalf("unexpected command payload: %#v", body["Cmd"])
			}
			if env, ok := body["Env"].([]any); !ok || len(env) != 2 || env[0] != "A=1" || env[1] != "B=2" {
				t.Fatalf("unexpected env payload: %#v", body["Env"])
			}
			exposedPorts, ok := body["ExposedPorts"].(map[string]any)
			if !ok || len(exposedPorts) != 2 {
				t.Fatalf("unexpected exposed ports payload: %#v", body["ExposedPorts"])
			}
			hostConfig, ok := body["HostConfig"].(map[string]any)
			if !ok {
				t.Fatalf("unexpected host config payload: %#v", body["HostConfig"])
			}
			portBindings, ok := hostConfig["PortBindings"].(map[string]any)
			if !ok || len(portBindings) != 2 {
				t.Fatalf("unexpected port bindings payload: %#v", hostConfig["PortBindings"])
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": "created-1"})
		case r.Method == http.MethodPost && r.URL.Path == "/containers/created-1/start":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dc := NewDockerCollector("/tmp/docker.sock", transport, "asset-1", 30*time.Second)
	dc.client = client

	raw, err := json.Marshal(agentmgr.DockerActionData{
		RequestID: "req-create",
		Action:    "container.create",
		Params: map[string]string{
			"image":   "nginx:1.27",
			"name":    "web",
			"command": "sleep 5",
			"env":     "A=1,B=2",
			"ports":   "8080:80,8443:443/tcp",
		},
	})
	if err != nil {
		t.Fatalf("marshal docker create action: %v", err)
	}

	dc.HandleDockerAction(transport, agentmgr.Message{Type: agentmgr.MsgDockerAction, Data: raw})

	msg := waitForCollectorMessage(t, transport, time.Second)
	if msg.Type != agentmgr.MsgDockerActionResult {
		t.Fatalf("message type=%q, want %q", msg.Type, agentmgr.MsgDockerActionResult)
	}

	var result agentmgr.DockerActionResultData
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		t.Fatalf("decode docker action result: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected create action success, got error %q", result.Error)
	}
	if result.Data != "created-1" {
		t.Fatalf("result data=%q, want created-1", result.Data)
	}

	if len(dc.discoveryTriggerCh) != 1 {
		t.Fatalf("discovery triggers=%d, want 1", len(dc.discoveryTriggerCh))
	}
	trigger := <-dc.discoveryTriggerCh
	if !trigger.full || !trigger.immediate {
		t.Fatalf("unexpected discovery trigger after create: %+v", trigger)
	}
	if len(dc.statsTriggerCh) != 1 {
		t.Fatalf("stats triggers=%d, want 1", len(dc.statsTriggerCh))
	}
}

func TestHandleDockerActionContainerLogsReturnsDecodedLogsWithoutRefresh(t *testing.T) {
	transport := newRecordingCollectorTransport(true)

	srv, client := newSecureDockerClientFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/containers/abc123/logs" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("tail"); got != "5000" {
			t.Fatalf("tail=%q, want capped 5000", got)
		}
		if got := r.URL.Query().Get("timestamps"); got != "true" {
			t.Fatalf("timestamps=%q, want true", got)
		}
		_, _ = w.Write(append(dockerMuxFrame(1, "line one\n"), dockerMuxFrame(2, "line two\n")...))
	}))
	defer srv.Close()

	dc := NewDockerCollector("/tmp/docker.sock", transport, "asset-1", 30*time.Second)
	dc.client = client

	raw, err := json.Marshal(agentmgr.DockerActionData{
		RequestID:   "req-logs",
		Action:      "container.logs",
		ContainerID: "abc123",
		Params: map[string]string{
			"tail":       "9000",
			"timestamps": "true",
		},
	})
	if err != nil {
		t.Fatalf("marshal docker logs action: %v", err)
	}

	dc.HandleDockerAction(transport, agentmgr.Message{Type: agentmgr.MsgDockerAction, Data: raw})

	msg := waitForCollectorMessage(t, transport, time.Second)
	if msg.Type != agentmgr.MsgDockerActionResult {
		t.Fatalf("message type=%q, want %q", msg.Type, agentmgr.MsgDockerActionResult)
	}

	var result agentmgr.DockerActionResultData
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		t.Fatalf("decode docker action result: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected logs action success, got error %q", result.Error)
	}
	if result.Data != "line one\nline two" {
		t.Fatalf("decoded logs=%q, want %q", result.Data, "line one\nline two")
	}
	if len(dc.discoveryTriggerCh) != 0 || len(dc.statsTriggerCh) != 0 {
		t.Fatalf("expected no refresh triggers for logs action, got discovery=%d stats=%d", len(dc.discoveryTriggerCh), len(dc.statsTriggerCh))
	}
}

func TestDockerLogManagerHandleLogsStartStreamsAndCleansUp(t *testing.T) {
	transport := newRecordingCollectorTransport(true)

	srv, client := newSecureDockerClientFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/containers/ct-1/logs" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(append(dockerMuxFrame(2, "stderr line\n"), dockerMuxFrame(1, "stdout line\n")...))
	}))
	defer srv.Close()

	lm := NewDockerLogManager(client)
	raw, err := json.Marshal(agentmgr.DockerLogsStartData{
		SessionID:   "logs-1",
		ContainerID: "ct-1",
		Tail:        50,
		Follow:      false,
	})
	if err != nil {
		t.Fatalf("marshal docker logs start: %v", err)
	}

	lm.HandleLogsStart(context.Background(), transport, agentmgr.Message{Type: agentmgr.MsgDockerLogsStart, Data: raw})

	first := waitForCollectorMessage(t, transport, time.Second)
	second := waitForCollectorMessage(t, transport, time.Second)
	if first.Type != agentmgr.MsgDockerLogsStream || second.Type != agentmgr.MsgDockerLogsStream {
		t.Fatalf("unexpected message types %q and %q", first.Type, second.Type)
	}

	streams := make([]agentmgr.DockerLogsStreamData, 0, 2)
	for _, msg := range []agentmgr.Message{first, second} {
		var payload agentmgr.DockerLogsStreamData
		if err := json.Unmarshal(msg.Data, &payload); err != nil {
			t.Fatalf("decode docker logs stream: %v", err)
		}
		streams = append(streams, payload)
	}
	if streams[0].Stream != "stderr" || streams[0].Data != "stderr line" {
		t.Fatalf("unexpected first stream payload: %+v", streams[0])
	}
	if streams[1].Stream != "stdout" || streams[1].Data != "stdout line" {
		t.Fatalf("unexpected second stream payload: %+v", streams[1])
	}

	waitUntil(t, time.Second, func() bool {
		lm.mu.Lock()
		defer lm.mu.Unlock()
		_, ok := lm.streams["logs-1"]
		return !ok
	}, "docker log stream cleanup")
}

func TestDockerExecManagerHandleExecStartStreamsOutputAndCleansUp(t *testing.T) {
	transport := newRecordingCollectorTransport(true)

	client := newUnixDockerClientFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/containers/ct-1/exec":
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": "exec-1"})
		case r.Method == http.MethodPost && r.URL.Path == "/exec/exec-1/start":
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("docker exec test server does not support hijacking")
			}
			conn, rw, err := hijacker.Hijack()
			if err != nil {
				t.Fatalf("hijack docker exec stream: %v", err)
			}
			defer conn.Close()

			_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
			_, _ = rw.WriteString("hello from exec\n")
			_ = rw.Flush()
		default:
			http.NotFound(w, r)
		}
	}))

	em := NewDockerExecManager(client)
	raw, err := json.Marshal(agentmgr.DockerExecStartData{
		SessionID:   "exec-1",
		ContainerID: "ct-1",
		Command:     []string{"sh"},
		TTY:         true,
	})
	if err != nil {
		t.Fatalf("marshal docker exec start: %v", err)
	}

	em.HandleExecStart(transport, agentmgr.Message{Type: agentmgr.MsgDockerExecStart, Data: raw})

	started := waitForCollectorMessage(t, transport, 2*time.Second)
	output := waitForCollectorMessage(t, transport, 2*time.Second)
	closed := waitForCollectorMessage(t, transport, 2*time.Second)

	if started.Type != agentmgr.MsgDockerExecStarted {
		t.Fatalf("started message type=%q, want %q", started.Type, agentmgr.MsgDockerExecStarted)
	}
	if output.Type != agentmgr.MsgDockerExecData {
		t.Fatalf("output message type=%q, want %q", output.Type, agentmgr.MsgDockerExecData)
	}
	if closed.Type != agentmgr.MsgDockerExecClosed {
		t.Fatalf("closed message type=%q, want %q", closed.Type, agentmgr.MsgDockerExecClosed)
	}

	var outputPayload agentmgr.DockerExecDataPayload
	if err := json.Unmarshal(output.Data, &outputPayload); err != nil {
		t.Fatalf("decode docker exec data payload: %v", err)
	}
	decoded, err := decodeBase64String(outputPayload.Data)
	if err != nil {
		t.Fatalf("decode docker exec output: %v", err)
	}
	if string(decoded) != "hello from exec\n" {
		t.Fatalf("exec output=%q, want %q", string(decoded), "hello from exec\n")
	}

	var closedPayload agentmgr.DockerExecCloseData
	if err := json.Unmarshal(closed.Data, &closedPayload); err != nil {
		t.Fatalf("decode docker exec close payload: %v", err)
	}
	if closedPayload.Reason != "exec ended" {
		t.Fatalf("close reason=%q, want exec ended", closedPayload.Reason)
	}

	waitUntil(t, time.Second, func() bool {
		em.mu.Lock()
		defer em.mu.Unlock()
		_, ok := em.sessions["exec-1"]
		return !ok
	}, "docker exec session cleanup")
}

func TestHandleComposeActionDeployWritesComposeFileAndQueuesRefresh(t *testing.T) {
	oldDetect := dockerComposeCLIDetect
	oldNewCommandContext := dockerComposeNewCommandContext
	t.Cleanup(func() {
		dockerComposeCLIDetect = oldDetect
		dockerComposeNewCommandContext = oldNewCommandContext
	})

	dockerComposeCLIDetect = func() (int, bool) { return 2, true }
	dockerComposeNewCommandContext = func(ctx context.Context, name string, args ...string) (*exec.Cmd, error) {
		if name != "docker" {
			t.Fatalf("compose command name=%q, want docker", name)
		}
		if len(args) < 5 || args[0] != "compose" || args[3] != "up" || args[4] != "-d" {
			t.Fatalf("unexpected compose command args: %v", args)
		}
		return exec.CommandContext(ctx, "sh", "-c", "printf 'compose ok'"), nil
	}

	transport := newRecordingCollectorTransport(true)
	dc := NewDockerCollector("/tmp/docker.sock", transport, "asset-1", 30*time.Second)
	dc.client = NewDockerClient("http://localhost:1")

	configDir := t.TempDir()
	raw, err := json.Marshal(agentmgr.DockerComposeActionData{
		RequestID:   "compose-1",
		StackName:   "My Stack",
		Action:      "deploy",
		ConfigDir:   configDir,
		ComposeYAML: "services:\n  app:\n    image: nginx:1.27\n",
	})
	if err != nil {
		t.Fatalf("marshal docker compose action: %v", err)
	}

	dc.HandleComposeAction(transport, agentmgr.Message{Type: agentmgr.MsgDockerComposeAction, Data: raw})

	msg := waitForCollectorMessage(t, transport, time.Second)
	if msg.Type != agentmgr.MsgDockerComposeResult {
		t.Fatalf("message type=%q, want %q", msg.Type, agentmgr.MsgDockerComposeResult)
	}

	var result agentmgr.DockerComposeResultData
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		t.Fatalf("decode docker compose result: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected compose deploy success, got error %q", result.Error)
	}
	if result.Output != "compose ok" {
		t.Fatalf("compose output=%q, want compose ok", result.Output)
	}

	composeBytes, err := os.ReadFile(filepath.Join(configDir, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("read compose file: %v", err)
	}
	if string(composeBytes) != "services:\n  app:\n    image: nginx:1.27" {
		t.Fatalf("unexpected compose file contents %q", string(composeBytes))
	}

	if len(dc.discoveryTriggerCh) != 1 {
		t.Fatalf("discovery triggers=%d, want 1", len(dc.discoveryTriggerCh))
	}
	trigger := <-dc.discoveryTriggerCh
	if !trigger.full || !trigger.immediate {
		t.Fatalf("unexpected discovery trigger after compose deploy: %+v", trigger)
	}
	if len(dc.statsTriggerCh) != 1 {
		t.Fatalf("stats triggers=%d, want 1", len(dc.statsTriggerCh))
	}
}

func dockerMuxFrame(stream byte, payload string) []byte {
	frame := make([]byte, 8+len(payload))
	frame[0] = stream
	binary.BigEndian.PutUint32(frame[4:8], uint32(len(payload)))
	copy(frame[8:], []byte(payload))
	return frame
}

func decodeBase64String(raw string) ([]byte, error) {
	payload := agentmgr.DockerExecDataPayload{Data: raw}
	decoded, err := io.ReadAll(base64Reader(payload.Data))
	if err != nil {
		return nil, err
	}
	return decoded, nil
}

func base64Reader(raw string) io.Reader {
	return base64.NewDecoder(base64.StdEncoding, strings.NewReader(raw))
}

func waitUntil(t *testing.T, timeout time.Duration, cond func() bool, label string) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", label)
}
