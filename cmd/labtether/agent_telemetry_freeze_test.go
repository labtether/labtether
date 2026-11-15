package main

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/telemetry/bridge"
)

// The metrics collectors ask each agent for processes, interfaces and mounts
// through the same waiters as the device pages. These pin what each does
// today when the agent does not answer, cannot be sent to, is offline, or
// another device answers.

type telemetryCollector struct {
	name    string
	sends   string
	reply   string
	waiters func(*apiServer) *sync.Map
	collect func(a *agentTelemetryAdapter) []string // the item named by each entry
	// answer is the agent's reply naming one item. took, when set, reports
	// whether the collector used an item it returns no entry for.
	answer func(requestID, item string) any
	took   func(a *agentTelemetryAdapter, item string) bool
}

func telemetryCollectors() []telemetryCollector {
	return []telemetryCollector{
		{
			name: "processes", sends: agentmgr.MsgProcessList, reply: agentmgr.MsgProcessListed,
			waiters: func(s *apiServer) *sync.Map { return &s.processBridges },
			collect: func(a *agentTelemetryAdapter) []string {
				return metricItems(a.collectProcesses(testutil.ReplyDevice, 1), func(e bridge.ProcessMetricEntry) string { return e.Labels["process_name"] })
			},
			answer: func(id, item string) any {
				return agentmgr.ProcessListedData{RequestID: id, Processes: []agentmgr.ProcessInfo{{PID: 7, Name: item, CPUPct: 1, MemPct: 1}}}
			},
		},
		{
			name: "network interfaces", sends: agentmgr.MsgNetworkList, reply: agentmgr.MsgNetworkListed,
			waiters: func(s *apiServer) *sync.Map { return &s.networkBridges },
			collect: func(a *agentTelemetryAdapter) []string {
				return metricItems(a.collectNetworkInterfaces(testutil.ReplyDevice), func(e bridge.NetworkInterfaceEntry) string { return e.Labels["interface"] })
			},
			answer: func(id, item string) any {
				return agentmgr.NetworkListedData{RequestID: id, Interfaces: []agentmgr.NetInterface{{Name: item, RXBytes: 1, TXBytes: 1}}}
			},
			// The first sample of an interface gives no rate, only a stored counter.
			took: func(a *agentTelemetryAdapter, item string) bool {
				a.networkMu.Lock()
				defer a.networkMu.Unlock()
				_, ok := a.networkCounters[testutil.ReplyDevice+"\x00"+item]
				return ok
			},
		},
		{
			name: "disk mounts", sends: agentmgr.MsgDiskList, reply: agentmgr.MsgDiskListed,
			waiters: func(s *apiServer) *sync.Map { return &s.diskBridges },
			collect: func(a *agentTelemetryAdapter) []string {
				return metricItems(a.collectDiskMounts(testutil.ReplyDevice), func(e bridge.DiskMountEntry) string { return e.Labels["mount_point"] })
			},
			answer: func(id, item string) any {
				return agentmgr.DiskListedData{RequestID: id, Mounts: []agentmgr.MountInfo{{MountPoint: item, Total: 100, Used: 40, Available: 60, UsePct: 40}}}
			},
		},
	}
}

func newTelemetryFreezeAdapter(t *testing.T, manager *agentmgr.AgentManager, wait time.Duration) (*apiServer, *agentTelemetryAdapter) {
	t.Helper()
	srv := newTestAPIServer(t)
	srv.agentMgr = manager
	adapter := newAgentTelemetryAdapter(srv)
	adapter.requestTimeout = wait
	return srv, adapter
}

func metricItems[T any](entries []T, item func(T) string) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, item(entry))
	}
	return out
}

func countWaiters(m *sync.Map) int {
	n := 0
	m.Range(func(any, any) bool { n++; return true })
	return n
}

func TestAgentTelemetryCollectorsWithoutAnAnswer(t *testing.T) {
	_, adapter := newTelemetryFreezeAdapter(t, agentmgr.NewManager(), 0)
	if got := adapter.timeout(); got != 3*time.Second {
		t.Fatalf("collector wait is %v, want 3s", got)
	}
	for _, c := range telemetryCollectors() {
		t.Run(c.name+"/no reply", func(t *testing.T) {
			peer := testutil.NewAgentPeer(t, testutil.ReplyDevice)
			srv, adapter := newTelemetryFreezeAdapter(t, peer.Manager, 10*time.Millisecond)
			got := make(chan []string, 1)
			go func() { got <- c.collect(adapter) }()
			if sent := peer.Next(t); sent.Type != c.sends {
				t.Fatalf("collector sent %q, want %q", sent.Type, c.sends)
			}
			if items := <-got; len(items) != 0 {
				t.Fatalf("collector returned %v without a reply", items)
			}
			if n := countWaiters(c.waiters(srv)); n != 0 {
				t.Fatalf("%d waiter(s) left registered", n)
			}
		})
		t.Run(c.name+"/send fails", func(t *testing.T) {
			peer := testutil.NewAgentPeer(t, testutil.ReplyDevice)
			peer.Conn.Close()
			srv, adapter := newTelemetryFreezeAdapter(t, peer.Manager, 10*time.Millisecond)
			if items := c.collect(adapter); len(items) != 0 {
				t.Fatalf("collector returned %v after a failed send", items)
			}
			if n := countWaiters(c.waiters(srv)); n != 0 {
				t.Fatalf("%d waiter(s) left registered", n)
			}
		})
		t.Run(c.name+"/offline", func(t *testing.T) {
			_, adapter := newTelemetryFreezeAdapter(t, agentmgr.NewManager(), 10*time.Millisecond)
			if items := c.collect(adapter); len(items) != 0 {
				t.Fatalf("collector returned %v for an offline agent", items)
			}
		})
	}
}

// A reply from another device, put through the live router, is dropped; the
// collector takes the right device's reply that follows.
func TestAgentTelemetryCollectorsDropRepliesFromAnotherDevice(t *testing.T) {
	for _, c := range telemetryCollectors() {
		t.Run(c.name, func(t *testing.T) {
			peer := testutil.NewAgentPeer(t, testutil.ReplyDevice)
			srv, adapter := newTelemetryFreezeAdapter(t, peer.Manager, 2*time.Second)
			route := srv.buildWSRouter()[c.reply]
			got := make(chan []string, 1)
			go func() { got <- c.collect(adapter) }()
			var request struct {
				RequestID string `json:"request_id"`
			}
			_ = json.Unmarshal(peer.Next(t).Data, &request)
			reply := func(item string) agentmgr.Message {
				data, _ := json.Marshal(c.answer(request.RequestID, item))
				return agentmgr.Message{Type: c.reply, ID: request.RequestID, Data: data}
			}
			testutil.DeliverPromptly(t, "reply from nas02", func() { route(&agentmgr.AgentConn{AssetID: "nas02"}, reply("wrong0")) })
			testutil.DeliverPromptly(t, "reply from nas01", func() { route(peer.Conn, reply("right0")) })
			items := <-got
			if c.took != nil {
				if !c.took(adapter, "right0") || c.took(adapter, "wrong0") {
					t.Fatal("collector did not take the nas01 reply alone")
				}
				return
			}
			if len(items) != 1 || items[0] != "right0" {
				t.Fatalf("collector returned %v, want [right0] from nas01", items)
			}
		})
	}
}
