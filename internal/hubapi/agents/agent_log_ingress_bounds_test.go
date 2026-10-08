package agents

import (
	"bytes"
	"encoding/json"
	"log"
	"strings"
	"sync"
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

func TestAgentStatusLogsEscapeUntrustedLines(t *testing.T) {
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previous) })

	deps := &Deps{}
	progressConn := &agentmgr.AgentConn{AssetID: "progress-agent"}
	defer deps.trackAgentUpdate("job\nforged-job", progressConn.AssetID, time.Minute)()
	progress, err := json.Marshal(agentmgr.UpdateProgressData{
		JobID: "job\nforged-job", Stage: "download\rforged-stage", Message: "ready\nforged-message",
	})
	if err != nil {
		t.Fatal(err)
	}
	deps.ProcessAgentUpdateProgress(progressConn, agentmgr.Message{Type: agentmgr.MsgUpdateProgress, Data: progress})

	installed, err := json.Marshal(agentmgr.SSHKeyInstalledData{
		Username: "user\nforged-user", Hostname: "host\rforged-host",
	})
	if err != nil {
		t.Fatal(err)
	}
	deps.ProcessAgentSSHKeyInstalled(&agentmgr.AgentConn{AssetID: "agent\nforged-agent"}, agentmgr.Message{Type: agentmgr.MsgSSHKeyInstalled, Data: installed})

	got := output.String()
	if strings.Count(got, "\n") != 2 || strings.Contains(got, "\r") {
		t.Fatalf("untrusted status created extra log lines: %q", got)
	}
	for _, escaped := range []string{`job\nforged-job`, `download\rforged-stage`, `ready\nforged-message`, `agent\nforged-agent`, `user\nforged-user`, `host\rforged-host`} {
		if !strings.Contains(got, escaped) {
			t.Fatalf("sanitized field %q missing from log: %q", escaped, got)
		}
	}
}

func TestProcessAgentUpdateProgressRequiresMatchingLiveRequest(t *testing.T) {
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previous) })

	deps := &Deps{PendingAgentCmds: &sync.Map{}}
	owner := &agentmgr.AgentConn{AssetID: "owner"}
	other := &agentmgr.AgentConn{AssetID: "other"}
	progress := func(jobID string) agentmgr.Message {
		t.Helper()
		data, err := json.Marshal(agentmgr.UpdateProgressData{JobID: jobID, Stage: "download", Message: "halfway"})
		if err != nil {
			t.Fatal(err)
		}
		return agentmgr.Message{Type: agentmgr.MsgUpdateProgress, Data: data}
	}

	deps.ProcessAgentUpdateProgress(owner, progress("unknown"))
	finishManual := deps.trackAgentUpdate("manual-update", owner.AssetID, time.Minute)
	deps.ProcessAgentUpdateProgress(other, progress("manual-update"))
	deps.ProcessAgentUpdateProgress(owner, progress("manual-update"))
	finishManual()
	deps.ProcessAgentUpdateProgress(owner, progress("manual-update"))

	deps.trackAgentUpdate("expired-update", owner.AssetID, -time.Second)
	deps.ProcessAgentUpdateProgress(owner, progress("expired-update"))

	deps.trackAgentUpdate("automatic-update", owner.AssetID, time.Minute)
	deps.ProcessAgentUpdateProgress(owner, progress("automatic-update"))
	resultData, err := json.Marshal(agentmgr.UpdateResultData{JobID: "automatic-update"})
	if err != nil {
		t.Fatal(err)
	}
	deps.ProcessAgentUpdateResult(owner, agentmgr.Message{Type: agentmgr.MsgUpdateResult, Data: resultData})
	deps.ProcessAgentUpdateProgress(owner, progress("automatic-update"))

	got := output.String()
	if strings.Count(got, "update progress") != 2 || strings.Contains(got, "unknown") || strings.Contains(got, "expired-update") {
		t.Fatalf("unexpected update progress logs: %q", got)
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
