package system

import (
	"encoding/json"
	"log"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/wol"
)

// WoLSendFn is the function used to send Wake-on-LAN packets.
// It can be overridden in tests.
var WoLSendFn = wol.Send

// HandleWoLSend handles a Wake-on-LAN send request from the hub.
func HandleWoLSend(transport MessageSender, msg agentmgr.Message) {
	var req agentmgr.WoLSendData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("wol: invalid request payload: %v", err)
		return
	}

	result := agentmgr.WoLResultData{
		RequestID: req.RequestID,
		MAC:       req.MAC,
		OK:        false,
	}

	mac, err := wol.ParseMAC(req.MAC)
	if err != nil {
		result.Error = err.Error()
		sendWoLResult(transport, result)
		return
	}

	broadcast := req.Broadcast
	if broadcast == "" {
		broadcast = "255.255.255.255:9"
	}
	if err := WoLSendFn(mac, broadcast); err != nil {
		result.Error = err.Error()
		sendWoLResult(transport, result)
		return
	}

	result.OK = true
	sendWoLResult(transport, result)
	log.Printf("wol: sent magic packet for %s via %s", req.MAC, broadcast)
}

func sendWoLResult(transport MessageSender, result agentmgr.WoLResultData) {
	data, err := json.Marshal(result)
	if err != nil {
		log.Printf("wol: marshal result failed: %v", err)
		return
	}
	_ = transport.Send(agentmgr.Message{
		Type: agentmgr.MsgWoLResult,
		ID:   result.RequestID,
		Data: data,
	})
}
