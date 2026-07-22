package agents

import (
	"bytes"
	"encoding/json"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
)

func TestProcessAgentLogStreamEnforcesTypeAndEventBounds(t *testing.T) {
	store := persistence.NewMemoryLogStore()
	deps := &Deps{LogStore: store}
	conn := &agentmgr.AgentConn{AssetID: "trusted-agent"}

	exact, err := json.Marshal(agentmgr.LogStreamData{Source: "agent", Level: "info", Message: strings.Repeat("x", logs.MaxEventMessageBytes)})
	if err != nil {
		t.Fatalf("marshal exact stream: %v", err)
	}
	deps.ProcessAgentLogStream(conn, agentmgr.Message{Type: agentmgr.MsgLogStream, Data: exact})
	if got := queryAgentLogCount(t, store); got != 1 {
		t.Fatalf("exact-limit stream stored=%d, want 1", got)
	}

	over, err := json.Marshal(agentmgr.LogStreamData{Source: "agent", Level: "info", Message: strings.Repeat("x", logs.MaxEventMessageBytes+1)})
	if err != nil {
		t.Fatalf("marshal over stream: %v", err)
	}
	deps.ProcessAgentLogStream(conn, agentmgr.Message{Type: agentmgr.MsgLogStream, Data: over})
	deps.ProcessAgentLogStream(conn, agentmgr.Message{Type: agentmgr.MsgLogStream, Data: json.RawMessage(`{"source":"agent","level":"info","message":"bad\u0000message"}`)})
	if got := queryAgentLogCount(t, store); got != 1 {
		t.Fatalf("invalid streams mutated store: count=%d", got)
	}

	deps.ProcessAgentLogStream(conn, agentmgr.Message{Type: agentmgr.MsgLogStream, Data: make([]byte, maxAgentLogStreamPayloadBytes+1)})
	if got := queryAgentLogCount(t, store); got != 1 {
		t.Fatalf("raw oversized stream mutated store: count=%d", got)
	}
}

func TestProcessAgentLogRejectionSanitizesAgentControlledIdentity(t *testing.T) {
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previous) })

	deps := &Deps{LogStore: persistence.NewMemoryLogStore()}
	conn := &agentmgr.AgentConn{AssetID: "trusted\nforged-prefix"}
	deps.ProcessAgentLogStream(conn, agentmgr.Message{
		Type: agentmgr.MsgLogStream,
		Data: make([]byte, maxAgentLogStreamPayloadBytes+1),
	})

	got := output.String()
	if strings.Contains(got, "trusted\nforged-prefix") {
		t.Fatalf("agent identity injected a log line: %q", got)
	}
	if !strings.Contains(got, `trusted\nforged-prefix`) {
		t.Fatalf("sanitized agent identity missing: %q", got)
	}
}

func TestProcessAgentLogPayloadTypeLimitsExactAndOver(t *testing.T) {
	store := persistence.NewMemoryLogStore()
	deps := &Deps{LogStore: store}
	conn := &agentmgr.AgentConn{AssetID: "trusted-agent"}

	stream := agentmgr.LogStreamData{Source: "agent", Level: "info", Message: "event"}
	streamBase, err := json.Marshal(stream)
	if err != nil {
		t.Fatalf("marshal stream base: %v", err)
	}
	stream.AssetID = strings.Repeat("x", maxAgentLogStreamPayloadBytes-len(streamBase))
	streamExact, err := json.Marshal(stream)
	if err != nil || len(streamExact) != maxAgentLogStreamPayloadBytes {
		t.Fatalf("exact stream payload bytes=%d error=%v", len(streamExact), err)
	}
	deps.ProcessAgentLogStream(conn, agentmgr.Message{Type: agentmgr.MsgLogStream, Data: streamExact})
	stream.AssetID += "x"
	streamOver, err := json.Marshal(stream)
	if err != nil || len(streamOver) != maxAgentLogStreamPayloadBytes+1 {
		t.Fatalf("over stream payload bytes=%d error=%v", len(streamOver), err)
	}
	deps.ProcessAgentLogStream(conn, agentmgr.Message{Type: agentmgr.MsgLogStream, Data: streamOver})
	if got := queryAgentLogCount(t, store); got != 1 {
		t.Fatalf("stream exact/over count=%d, want 1", got)
	}

	batch := agentmgr.LogBatchData{Entries: []agentmgr.LogStreamData{{Source: "agent", Level: "info", Message: "event"}}}
	batchBase, err := json.Marshal(batch)
	if err != nil {
		t.Fatalf("marshal batch base: %v", err)
	}
	batch.AssetID = strings.Repeat("x", maxAgentLogBatchPayloadBytes-len(batchBase))
	batchExact, err := json.Marshal(batch)
	if err != nil || len(batchExact) != maxAgentLogBatchPayloadBytes {
		t.Fatalf("exact batch payload bytes=%d error=%v", len(batchExact), err)
	}
	deps.ProcessAgentLogBatch(conn, agentmgr.Message{Type: agentmgr.MsgLogBatch, Data: batchExact})
	batch.AssetID += "x"
	batchOver, err := json.Marshal(batch)
	if err != nil || len(batchOver) != maxAgentLogBatchPayloadBytes+1 {
		t.Fatalf("over batch payload bytes=%d error=%v", len(batchOver), err)
	}
	deps.ProcessAgentLogBatch(conn, agentmgr.Message{Type: agentmgr.MsgLogBatch, Data: batchOver})
	if got := queryAgentLogCount(t, store); got != 2 {
		t.Fatalf("batch exact/over total count=%d, want 2", got)
	}
}

func TestProcessAgentLogBatchIsBoundedAndAtomic(t *testing.T) {
	store := persistence.NewMemoryLogStore()
	deps := &Deps{LogStore: store}
	conn := &agentmgr.AgentConn{AssetID: "trusted-agent"}

	exactEntries := make([]agentmgr.LogStreamData, logs.MaxEventsPerBatch)
	for index := range exactEntries {
		exactEntries[index] = agentmgr.LogStreamData{Source: "agent", Level: "info", Message: "event"}
	}
	exact, err := json.Marshal(agentmgr.LogBatchData{Entries: exactEntries})
	if err != nil {
		t.Fatalf("marshal exact batch: %v", err)
	}
	deps.ProcessAgentLogBatch(conn, agentmgr.Message{Type: agentmgr.MsgLogBatch, Data: exact})
	if got := queryAgentLogCount(t, store); got != logs.MaxEventsPerBatch {
		t.Fatalf("exact batch stored=%d, want %d", got, logs.MaxEventsPerBatch)
	}

	overEntries := append(exactEntries, agentmgr.LogStreamData{Source: "agent", Level: "info", Message: "over"})
	over, err := json.Marshal(agentmgr.LogBatchData{Entries: overEntries})
	if err != nil {
		t.Fatalf("marshal over batch: %v", err)
	}
	deps.ProcessAgentLogBatch(conn, agentmgr.Message{Type: agentmgr.MsgLogBatch, Data: over})

	invalid, err := json.Marshal(agentmgr.LogBatchData{Entries: []agentmgr.LogStreamData{
		{Source: "agent", Level: "info", Message: "valid"},
		{Source: "agent", Level: "info", Message: "bad\x00message"},
	}})
	if err != nil {
		t.Fatalf("marshal invalid batch: %v", err)
	}
	deps.ProcessAgentLogBatch(conn, agentmgr.Message{Type: agentmgr.MsgLogBatch, Data: invalid})
	deps.ProcessAgentLogBatch(conn, agentmgr.Message{Type: agentmgr.MsgLogBatch, Data: make([]byte, maxAgentLogBatchPayloadBytes+1)})
	if got := queryAgentLogCount(t, store); got != logs.MaxEventsPerBatch {
		t.Fatalf("rejected batches partially mutated store: count=%d", got)
	}
}

func queryAgentLogCount(t *testing.T, store *persistence.MemoryLogStore) int {
	t.Helper()
	events, err := store.QueryEvents(logs.QueryRequest{
		From:  time.Now().UTC().Add(-time.Minute),
		To:    time.Now().UTC().Add(time.Minute),
		Limit: logs.MaxEventsPerBatch,
	})
	if err != nil {
		t.Fatalf("query agent logs: %v", err)
	}
	return len(events)
}
