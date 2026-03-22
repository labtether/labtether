package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"sync"

	"github.com/labtether/labtether/internal/agentmgr"
)

const maxLogStreams = 5

// dockerLogStream tracks an active log stream for a container.
type dockerLogStream struct {
	sessionID string
	cancel    context.CancelFunc
}

// dockerLogManager manages container log streams on the agent.
type DockerLogManager struct {
	mu      sync.Mutex
	streams map[string]*dockerLogStream
	client  *dockerClient
}

func NewDockerLogManager(client *dockerClient) *DockerLogManager {
	return &DockerLogManager{
		streams: make(map[string]*dockerLogStream),
		client:  client,
	}
}

func (lm *DockerLogManager) HandleLogsStart(ctx context.Context, transport Transport, msg agentmgr.Message) {
	var req agentmgr.DockerLogsStartData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("docker-logs: invalid start request: %v", err)
		return
	}

	if req.SessionID == "" || req.ContainerID == "" {
		log.Printf("docker-logs: missing session_id or container_id")
		return
	}

	lm.mu.Lock()
	if len(lm.streams) >= maxLogStreams {
		lm.mu.Unlock()
		log.Printf("docker-logs: max streams (%d) reached", maxLogStreams)
		return
	}
	if _, exists := lm.streams[req.SessionID]; exists {
		lm.mu.Unlock()
		return
	}
	lm.mu.Unlock()

	tail := req.Tail
	if tail <= 0 {
		tail = 100
	}

	ctx, cancel := context.WithCancel(ctx)
	stream := &dockerLogStream{
		sessionID: req.SessionID,
		cancel:    cancel,
	}

	lm.mu.Lock()
	lm.streams[req.SessionID] = stream
	lm.mu.Unlock()

	go func() {
		defer func() {
			lm.mu.Lock()
			delete(lm.streams, req.SessionID)
			lm.mu.Unlock()
		}()

		body, err := lm.client.containerLogs(ctx, req.ContainerID, tail, req.Follow, req.Timestamps)
		if err != nil {
			log.Printf("docker-logs: failed to open log stream: %v", err)
			return
		}
		defer body.Close()

		scanner := bufio.NewScanner(body)
		scanner.Buffer(make([]byte, 0, 64*1024), 64*1024) // 64 KB max line
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			line := scanner.Text()

			// Docker multiplexed log stream prepends an 8-byte header per frame:
			//   byte 0: stream type (1 = stdout, 2 = stderr)
			//   bytes 1-3: reserved (zero)
			//   bytes 4-7: frame size (big-endian uint32)
			// TTY containers omit the header entirely.
			streamName := "stdout"
			content := line
			if len(line) >= 8 {
				headerByte := line[0]
				if headerByte == 1 || headerByte == 2 {
					if headerByte == 2 {
						streamName = "stderr"
					}
					content = line[8:] // strip the 8-byte binary header
				}
			}

			logData := agentmgr.DockerLogsStreamData{
				SessionID: req.SessionID,
				Stream:    streamName,
				Data:      content,
			}
			data, _ := json.Marshal(logData)
			_ = transport.Send(agentmgr.Message{
				Type: agentmgr.MsgDockerLogsStream,
				ID:   req.SessionID,
				Data: data,
			})
		}
	}()
}

func (lm *DockerLogManager) HandleLogsStop(msg agentmgr.Message) {
	var req agentmgr.DockerLogsStopData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		return
	}

	lm.mu.Lock()
	stream, ok := lm.streams[req.SessionID]
	lm.mu.Unlock()
	if !ok {
		return
	}

	stream.cancel()
}

func (lm *DockerLogManager) CloseAll() {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	for id, stream := range lm.streams {
		stream.cancel()
		delete(lm.streams, id)
	}
}
