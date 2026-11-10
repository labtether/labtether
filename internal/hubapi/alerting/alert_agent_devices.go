package alerting

import (
	"sync"

	"github.com/labtether/labtether/internal/agentmgr"
)

// maxUndeliveredAlertEnds bounds what is kept for one device that is away.
// An agent keeps 20 alerts, so more would only push its own alerts out.
const maxUndeliveredAlertEnds = 20

// AgentAlertDevices remembers which devices' agents were told that an alert
// is firing, and which "resolved" messages could not be delivered. One alert
// covers every device its rule watches, so the devices that breach change
// while it fires; only they are told. The memory lasts as long as the Hub
// process: after a restart the evaluator's next pass rebuilds it. A nil
// value remembers nothing, and callers fall back to the rule's devices.
type AgentAlertDevices struct {
	mu          sync.Mutex
	told        map[string]map[string]struct{}
	undelivered map[string][]agentmgr.AlertNotifyData
}

func NewAgentAlertDevices() *AgentAlertDevices {
	return &AgentAlertDevices{
		told:        map[string]map[string]struct{}{},
		undelivered: map[string][]agentmgr.AlertNotifyData{},
	}
}

// toldFor returns the devices told that the alert is firing, and whether
// anything is remembered for it.
func (a *AgentAlertDevices) toldFor(instanceID string) (map[string]struct{}, bool) {
	if a == nil {
		return nil, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	devices, known := a.told[instanceID]
	out := make(map[string]struct{}, len(devices))
	for id := range devices {
		out[id] = struct{}{}
	}
	return out, known
}

func (a *AgentAlertDevices) setTold(instanceID string, deviceIDs []string) {
	if a == nil {
		return
	}
	devices := make(map[string]struct{}, len(deviceIDs))
	for _, id := range deviceIDs {
		devices[id] = struct{}{}
	}
	a.mu.Lock()
	a.told[instanceID] = devices
	a.mu.Unlock()
}

func (a *AgentAlertDevices) forget(instanceID string) {
	if a == nil {
		return
	}
	a.mu.Lock()
	delete(a.told, instanceID)
	a.mu.Unlock()
}

// keepForLater holds a "resolved" message for a device that was told the
// alert is firing but is not connected now. Its agent kept running and
// still shows the alert; it gets the message when it connects again.
func (a *AgentAlertDevices) keepForLater(deviceID string, data agentmgr.AlertNotifyData) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	kept := a.undelivered[deviceID]
	for i := range kept {
		if kept[i].ID == data.ID {
			kept[i] = data
			return
		}
	}
	kept = append(kept, data)
	if len(kept) > maxUndeliveredAlertEnds {
		kept = kept[len(kept)-maxUndeliveredAlertEnds:]
	}
	a.undelivered[deviceID] = kept
}

// takeForLater hands back, and forgets, the messages kept for a device.
func (a *AgentAlertDevices) takeForLater(deviceID string) []agentmgr.AlertNotifyData {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	kept := a.undelivered[deviceID]
	delete(a.undelivered, deviceID)
	return kept
}
