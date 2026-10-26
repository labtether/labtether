package resources

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

func TestPackagePreviewContextOwnsAttemptAndSocket(t *testing.T) {
	socket, peer, cleanup := createWoLWebSocketPair(t)
	defer cleanup()
	conn := agentmgr.NewAgentConn(socket, "preview-node", "linux")
	manager := agentmgr.NewManager()
	manager.Register(conn)
	defer manager.Unregister(conn.AssetID)
	deps := &Deps{AgentMgr: manager, PackageBridges: &sync.Map{}}
	run := func(ctx context.Context) agentmgr.CommandResultData {
		return deps.PreviewOSPackageUpdatesViaAgentContext(ctx, "same-preview", conn.AssetID, time.Minute)
	}
	read := func() string {
		t.Helper()
		if err := peer.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
			t.Fatal(err)
		}
		var msg agentmgr.Message
		if err := peer.ReadJSON(&msg); err != nil {
			t.Fatal(err)
		}
		var req packageListRequestWire
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			t.Fatal(err)
		}
		if msg.Type != agentmgr.MsgPackageList || req.Inventory != packageInventoryUpgradable || msg.ID != req.RequestID || req.RequestID == "same-preview" {
			t.Fatalf("invalid preview request: %+v", msg)
		}
		return req.RequestID
	}
	wait := func(ch <-chan agentmgr.CommandResultData) agentmgr.CommandResultData {
		t.Helper()
		select {
		case r := <-ch:
			return r
		case <-time.After(2 * time.Second):
			t.Fatal("preview still waiting")
		}
		return agentmgr.CommandResultData{}
	}
	var started []<-chan struct{}
	t.Cleanup(func() {
		conn.Close()
		for _, done := range started {
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Error("preview worker did not exit during fixture cleanup")
			}
		}
	})
	launch := func(ctx context.Context) <-chan agentmgr.CommandResultData {
		results := make(chan agentmgr.CommandResultData, 1)
		done := make(chan struct{})
		started = append(started, done)
		go func() { defer close(done); results <- run(ctx) }()
		return results
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if r := run(canceled); r.Status != "failed" || !strings.Contains(r.Output, "before dispatch") {
		t.Fatalf("already canceled: %+v", r)
	}
	ctxA, cancelA := context.WithCancel(context.Background())
	defer cancelA()
	resultsA := launch(ctxA)
	wireA := read()
	if r := deps.PreviewOSPackageUpdatesViaAgent("same-preview", conn.AssetID, time.Minute); r.Status != "failed" || !strings.Contains(r.Output, "already in use") {
		t.Fatalf("duplicate: %+v", r)
	}
	cancelA()
	if r := wait(resultsA); r.JobID != "same-preview" || r.Status != "failed" || !strings.Contains(r.Output, "canceled") {
		t.Fatalf("canceled preview: %+v", r)
	}
	ctxB, cancelB := context.WithCancel(context.Background())
	defer cancelB()
	resultsB := launch(ctxB)
	wireB := read()
	if wireB == wireA {
		t.Fatal("preview retry reused wire id")
	}
	deliver := func(sender *agentmgr.AgentConn, id string) {
		data, err := json.Marshal(packageListedResponseWire{RequestID: id, Inventory: packageInventoryUpgradable, Packages: []packageInfoWire{}})
		if err != nil {
			t.Fatal(err)
		}
		deps.ProcessAgentPackageListed(sender, agentmgr.Message{Type: agentmgr.MsgPackageListed, ID: id, Data: data})
	}
	deliver(conn, wireA)
	deliver(&agentmgr.AgentConn{AssetID: conn.AssetID}, wireB)
	raw, ok := deps.PackageBridges.Load(wireB)
	if !ok || len(raw.(*PackageBridge).Ch) != 0 {
		t.Fatal("stale preview reached replacement")
	}
	deliver(conn, wireB)
	if r := wait(resultsB); r.JobID != "same-preview" || r.Status != "succeeded" {
		t.Fatalf("current preview: %+v", r)
	}
	deps.PackageBridges.Range(func(key, value any) bool { t.Errorf("preview bridge retained: %v", key); return true })
}
