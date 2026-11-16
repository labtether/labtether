package agents_test

import (
	"encoding/json"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/hubapi/testutil/hubdeps"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
	"testing"
	"time"
)

type countingBatchLogStore struct {
	persistence.LogStore
	appendEventsCalls int
	appendEventsCount int
}

func (c *countingBatchLogStore) AppendEvents(events []logs.Event) error {
	c.appendEventsCalls++
	c.appendEventsCount += len(events)
	for _, event := range events {
		if err := c.LogStore.AppendEvent(event); err != nil {
			return err
		}
	}
	return nil
}

func TestProcessAgentTelemetryIgnoresPayloadAssetID(t *testing.T) {
	sut := hubdeps.New(t)
	conn := &agentmgr.AgentConn{AssetID: "trusted-node"}

	payload, err := json.Marshal(agentmgr.TelemetryData{
		AssetID:       "spoofed-node",
		CPUPercent:    42,
		MemoryPercent: 55,
		DiskPercent:   67,
	})
	if err != nil {
		t.Fatalf("failed to marshal telemetry payload: %v", err)
	}

	sut.AgentsDeps().ProcessAgentTelemetry(conn, agentmgr.Message{Type: agentmgr.MsgTelemetry, Data: payload})

	at := time.Now().UTC().Add(time.Second)
	trustedSnapshot, err := sut.TelemetryStore.Snapshot("trusted-node", at)
	if err != nil {
		t.Fatalf("failed to read trusted snapshot: %v", err)
	}
	if trustedSnapshot.CPUUsedPercent == nil {
		t.Fatalf("expected trusted asset telemetry sample")
	}
	spoofedSnapshot, err := sut.TelemetryStore.Snapshot("spoofed-node", at)
	if err != nil {
		t.Fatalf("failed to read spoofed snapshot: %v", err)
	}
	if spoofedSnapshot.CPUUsedPercent != nil || spoofedSnapshot.MemoryUsedPercent != nil || spoofedSnapshot.DiskUsedPercent != nil {
		t.Fatalf("did not expect spoofed asset telemetry samples")
	}
}

func TestProcessAgentTelemetryRejectsOversizedPayload(t *testing.T) {
	sut := hubdeps.New(t)
	conn := &agentmgr.AgentConn{AssetID: "trusted-node"}

	// Keep this just over the fixed telemetry cap in the Hub handler.
	payload := make([]byte, (64<<10)+1)
	sut.AgentsDeps().ProcessAgentTelemetry(conn, agentmgr.Message{Type: agentmgr.MsgTelemetry, Data: payload})

	snapshot, err := sut.TelemetryStore.Snapshot("trusted-node", time.Now().UTC().Add(time.Second))
	if err != nil {
		t.Fatalf("failed to read trusted snapshot: %v", err)
	}
	if snapshot.CPUUsedPercent != nil || snapshot.MemoryUsedPercent != nil || snapshot.DiskUsedPercent != nil {
		t.Fatal("did not expect telemetry samples from an oversized payload")
	}
}

func TestProcessAgentLogHandlersIgnorePayloadAssetID(t *testing.T) {
	sut := hubdeps.New(t)
	conn := &agentmgr.AgentConn{AssetID: "trusted-node"}

	streamPayload, err := json.Marshal(agentmgr.LogStreamData{
		AssetID: "spoofed-node",
		Source:  "agent",
		Level:   "info",
		Message: "single message",
	})
	if err != nil {
		t.Fatalf("failed to marshal log stream payload: %v", err)
	}
	sut.AgentsDeps().ProcessAgentLogStream(conn, agentmgr.Message{Type: agentmgr.MsgLogStream, Data: streamPayload})

	batchPayload, err := json.Marshal(agentmgr.LogBatchData{
		Entries: []agentmgr.LogStreamData{
			{
				AssetID: "spoofed-node",
				Source:  "agent",
				Level:   "warn",
				Message: "batched message",
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal log batch payload: %v", err)
	}
	sut.AgentsDeps().ProcessAgentLogBatch(conn, agentmgr.Message{Type: agentmgr.MsgLogBatch, Data: batchPayload})

	from := time.Now().UTC().Add(-time.Minute)
	to := time.Now().UTC().Add(time.Minute)

	trustedEvents, err := sut.LogStore.QueryEvents(logs.QueryRequest{
		AssetID: "trusted-node",
		From:    from,
		To:      to,
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("failed to query trusted logs: %v", err)
	}
	if len(trustedEvents) != 2 {
		t.Fatalf("expected 2 trusted events, got %d", len(trustedEvents))
	}

	spoofedEvents, err := sut.LogStore.QueryEvents(logs.QueryRequest{
		AssetID: "spoofed-node",
		From:    from,
		To:      to,
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("failed to query spoofed logs: %v", err)
	}
	if len(spoofedEvents) != 0 {
		t.Fatalf("expected 0 spoofed events, got %d", len(spoofedEvents))
	}
}

func TestProcessAgentLogBatchUsesBatchAppenderWhenAvailable(t *testing.T) {
	sut := hubdeps.New(t)
	counter := &countingBatchLogStore{LogStore: sut.LogStore}
	sut.LogStore = counter
	conn := &agentmgr.AgentConn{AssetID: "trusted-node"}

	batchPayload, err := json.Marshal(agentmgr.LogBatchData{
		Entries: []agentmgr.LogStreamData{
			{Source: "agent", Level: "info", Message: "one"},
			{Source: "agent", Level: "warning", Message: "two"},
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal log batch payload: %v", err)
	}
	sut.AgentsDeps().ProcessAgentLogBatch(conn, agentmgr.Message{Type: agentmgr.MsgLogBatch, Data: batchPayload})

	if counter.appendEventsCalls != 1 {
		t.Fatalf("appendEventsCalls=%d, want 1", counter.appendEventsCalls)
	}
	if counter.appendEventsCount != 2 {
		t.Fatalf("appendEventsCount=%d, want 2", counter.appendEventsCount)
	}
}

func TestProcessAgentCommandResultRejectsMismatchedSender(t *testing.T) {
	sut := hubdeps.New(t)
	resultCh := make(chan agentmgr.CommandResultData, 1)
	sut.PendingAgentCmds.Store("job-1", shared.PendingAgentCommand{
		ResultCh:          resultCh,
		ExpectedAssetID:   "node-1",
		ExpectedSessionID: "sess-1",
		ExpectedCommandID: "cmd-1",
	})

	payload, err := json.Marshal(agentmgr.CommandResultData{
		JobID:     "job-1",
		SessionID: "sess-1",
		CommandID: "cmd-1",
		Status:    "succeeded",
		Output:    "ok",
	})
	if err != nil {
		t.Fatalf("marshal command result payload: %v", err)
	}

	sut.AgentsDeps().ProcessAgentCommandResult(&agentmgr.AgentConn{AssetID: "node-2"}, agentmgr.Message{Data: payload})
	select {
	case <-resultCh:
		t.Fatal("expected mismatched sender result to be ignored")
	default:
	}

	sut.AgentsDeps().ProcessAgentCommandResult(&agentmgr.AgentConn{AssetID: "node-1"}, agentmgr.Message{Data: payload})
	select {
	case result := <-resultCh:
		if result.SessionID != "sess-1" || result.CommandID != "cmd-1" {
			t.Fatalf("unexpected correlated result %+v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("expected matched sender result to be delivered")
	}
}
