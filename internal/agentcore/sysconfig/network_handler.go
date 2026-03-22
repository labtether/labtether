package sysconfig

import (
	"encoding/json"
	"log"
	"net"
	"strings"
	"sync"

	"github.com/labtether/labtether/internal/agentmgr"
)

// NetworkManager handles network interface info and network actions from the hub.
type NetworkManager struct {
	mu sync.Mutex

	Backend NetworkBackend

	LastMethod          string
	LastNetplanBackup   string
	LastNMConnections   []string
	LastDarwinSnapshot  *DarwinNetworkSnapshot
	LastWindowsSnapshot *WindowsNetworkSnapshot
}

var CollectNetworkInterfaces = collectNetInterfaces

func NewNetworkManager() *NetworkManager {
	return &NetworkManager{
		Backend: NewNetworkBackendForOS(),
	}
}

// CloseAll is a no-op for NetworkManager — network requests are stateless
// aside from lightweight rollback snapshots.
func (nm *NetworkManager) CloseAll() {}

// HandleNetworkList collects network interface info and sends it to the hub.
func (nm *NetworkManager) HandleNetworkList(transport MessageSender, msg agentmgr.Message) {
	var req agentmgr.NetworkListData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("network: invalid network.list request: %v", err)
		return
	}

	ifaces, err := CollectNetworkInterfaces()

	var errMsg string
	if err != nil {
		errMsg = err.Error()
		log.Printf("network: failed to collect interfaces: %v", err)
	}

	data, marshalErr := json.Marshal(agentmgr.NetworkListedData{
		RequestID:  req.RequestID,
		Interfaces: ifaces,
		Error:      errMsg,
	})
	if marshalErr != nil {
		log.Printf("network: failed to marshal network.listed response: %v", marshalErr)
		return
	}

	if sendErr := transport.Send(agentmgr.Message{
		Type: agentmgr.MsgNetworkListed,
		ID:   req.RequestID,
		Data: data,
	}); sendErr != nil {
		log.Printf("network: failed to send network.listed for request %s: %v", req.RequestID, sendErr)
	}
}

// HandleNetworkAction applies or rolls back network changes using the
// platform-specific backend.
func (nm *NetworkManager) HandleNetworkAction(transport MessageSender, msg agentmgr.Message) {
	var req agentmgr.NetworkActionData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("network: invalid network.action request: %v", err)
		return
	}

	result := agentmgr.NetworkResultData{
		RequestID: req.RequestID,
	}

	action := strings.ToLower(strings.TrimSpace(req.Action))
	switch action {
	case "apply":
		result = nm.Backend.ApplyAction(nm, req)
	case "rollback":
		result = nm.Backend.RollbackAction(nm, req)
	default:
		result.Error = "invalid action: must be apply or rollback"
	}

	nm.SendNetworkResult(transport, result)
}

func (nm *NetworkManager) SendNetworkResult(transport MessageSender, result agentmgr.NetworkResultData) {
	data, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		log.Printf("network: failed to marshal network.result: %v", marshalErr)
		return
	}

	if sendErr := transport.Send(agentmgr.Message{
		Type: agentmgr.MsgNetworkResult,
		ID:   result.RequestID,
		Data: data,
	}); sendErr != nil {
		log.Printf("network: failed to send network.result for request %s: %v", result.RequestID, sendErr)
	}
}

// collectNetInterfaces enumerates host network interfaces using the standard
// library. Per-interface counters are read via ReadIfaceStats which is
// implemented per platform (Linux reads /sys/class/net, other platforms return 0).
func collectNetInterfaces() ([]agentmgr.NetInterface, error) {
	raw, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var result []agentmgr.NetInterface
	for _, iface := range raw {
		// Skip loopback interfaces.
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		state := "down"
		if iface.Flags&net.FlagUp != 0 {
			state = "up"
		}

		mac := ""
		if iface.HardwareAddr != nil {
			mac = iface.HardwareAddr.String()
		}

		addrs, addrErr := iface.Addrs()
		var ips []string
		if addrErr == nil {
			for _, addr := range addrs {
				ips = append(ips, addr.String())
			}
		}
		if ips == nil {
			ips = []string{}
		}

		rxBytes, txBytes, rxPackets, txPackets := ReadIfaceStats(iface.Name)

		result = append(result, agentmgr.NetInterface{
			Name:      iface.Name,
			State:     state,
			MAC:       mac,
			MTU:       iface.MTU,
			IPs:       ips,
			RXBytes:   rxBytes,
			TXBytes:   txBytes,
			RXPackets: rxPackets,
			TXPackets: txPackets,
		})
	}
	return result, nil
}
