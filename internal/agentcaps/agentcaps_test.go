package agentcaps

import (
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/protocol"
)

// monitoringMetadata is what the monitoring edition will send: metrics, host
// details, the disk list and the network interface list, nothing else.
func monitoringMetadata() map[string]string {
	return map[string]string{
		MetaMode:                 ModeExplicit,
		MetaEdition:              EditionMonitoring,
		"cap_disks":              "list",
		"cap_network":            "list",
		"network_backend":        "ifconfig",
		"network_action_backend": "none",
		"cpu_used_percent":       "3.00",
	}
}

// Fault: an explicit agent that advertises only monitoring is sent a
// terminal, file, process or other request it cannot answer, which times out.
func TestExplicitMonitoringAgentIsOfferedOnlyWhatItAdvertises(t *testing.T) {
	metadata := monitoringMetadata()
	for _, msgType := range []string{agentmgr.MsgDiskList, agentmgr.MsgNetworkList, agentmgr.MsgConfigUpdate, agentmgr.MsgAlertNotify, agentmgr.MsgPing} {
		if !Offers(metadata, msgType) {
			t.Errorf("%s refused, want offered", msgType)
		}
	}
	for msgType := range messageNeeds {
		if msgType == agentmgr.MsgDiskList || msgType == agentmgr.MsgNetworkList {
			continue
		}
		if Offers(metadata, msgType) {
			t.Errorf("%s offered to a monitoring agent", msgType)
		}
	}
	for _, mode := range []string{"self", "os_packages"} {
		if OffersUpdate(metadata, mode) {
			t.Errorf("update.request mode %q offered to a monitoring agent", mode)
		}
	}
	if got := Edition(metadata); got != EditionMonitoring {
		t.Fatalf("edition %q, want %q", got, EditionMonitoring)
	}
}

// Fault: the rule is inferred from a missing or unknown value, so a Go agent
// (which never sends the key) or a typo loses its terminal and files.
func TestUnknownModeBehavesAsAbsent(t *testing.T) {
	for _, value := range []string{"", "true", "complete", "explicit-v2", "full"} {
		metadata := map[string]string{"cap_network": "list"}
		if value != "" {
			metadata[MetaMode] = value
		}
		for msgType := range messageNeeds {
			if !Offers(metadata, msgType) {
				t.Errorf("mode %q: %s refused, want offered", value, msgType)
			}
		}
		if !OffersUpdate(metadata, "self") || Offered(metadata) != nil {
			t.Errorf("mode %q treated as explicit", value)
		}
	}
	if Edition(map[string]string{MetaEdition: "appstore"}) != "" {
		t.Fatal("unknown edition reported")
	}
}

// Fault: a gated or core entry names a message type the protocol does not
// have (a typo), so that request is refused for every explicit agent.
func TestEveryRuleNamesAProtocolMessage(t *testing.T) {
	for msgType := range messageNeeds {
		if !protocol.IsKnownMessageType(msgType) {
			t.Errorf("rule for unknown message type %q", msgType)
		}
	}
	for msgType := range coreMessages {
		if !protocol.IsKnownMessageType(msgType) {
			t.Errorf("core entry for unknown message type %q", msgType)
		}
	}
}

// Fault: a connection keeps no advertisement, so background requests sent
// before a store read go to an explicit agent that never advertised them.
func TestRecordedConnectionFollowsTheAdvertisement(t *testing.T) {
	conn := agentmgr.NewAgentConn(nil, "rust01", "linux")
	if !ConnOffers(conn, agentmgr.MsgProcessList) {
		t.Fatal("connection with nothing recorded refused a request")
	}
	Record(conn, monitoringMetadata())
	if ConnOffers(conn, agentmgr.MsgProcessList) || !ConnOffers(conn, agentmgr.MsgDiskList) {
		t.Fatal("recorded advertisement not followed")
	}
	Record(conn, map[string]string{"cap_network": "list"})
	if !ConnOffers(conn, agentmgr.MsgProcessList) {
		t.Fatal("a later heartbeat without the signal did not restore today's behaviour")
	}
}
