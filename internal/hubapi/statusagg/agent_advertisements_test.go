package statusagg

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/assets"
)

// Fault: the console cannot tell a monitoring device from a full one (no
// edition, no offered list), or the status response changes for a fleet of
// Go agents that never send the signal.
func TestStatusReportsAdvertisementsOnlyForExplicitOrNamedAgents(t *testing.T) {
	list := []assets.Asset{
		{ID: "node-go", Source: "agent", Metadata: map[string]string{"cap_network": "list,action"}},
		{ID: "node-mon", Source: "agent", Metadata: map[string]string{
			"agent_capability_mode": "explicit", "agent_edition": "monitoring", "cap_disks": "list", "cap_network": "list",
		}},
		{ID: "vm-100", Source: "proxmox", Metadata: map[string]string{"agent_capability_mode": "explicit"}},
	}
	ads := agentAdvertisements(list)
	if len(ads) != 1 {
		t.Fatalf("advertisements %+v, want only node-mon", ads)
	}
	mon := ads["node-mon"]
	if !mon.Explicit || mon.Edition != "monitoring" || strings.Join(mon.Offered, ",") != "disk.list,network.list" {
		t.Fatalf("node-mon advertisement %+v", mon)
	}

	encoded, _ := json.Marshal(LiveResponse{Assets: list[:1], AgentAdvertisements: agentAdvertisements(list[:1])})
	if strings.Contains(string(encoded), "agentAdvertisements") {
		t.Fatalf("Go-only fleet response gained the field: %s", encoded)
	}
	encoded, _ = json.Marshal(LiveResponse{AgentAdvertisements: ads})
	if !strings.Contains(string(encoded), `"agentAdvertisements":{"node-mon":{"explicit":true,"edition":"monitoring","offered":["disk.list","network.list"]}}`) {
		t.Fatalf("response %s", encoded)
	}
}
