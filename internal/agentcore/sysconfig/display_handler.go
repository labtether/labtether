package sysconfig

import (
	"encoding/json"
	"log"

	"github.com/labtether/labtether/internal/agentmgr"
)

var PlatformListDisplaysFn = PlatformListDisplays

func HandleListDisplays(transport MessageSender, msg agentmgr.Message) {
	var req struct {
		RequestID string `json:"request_id"`
	}
	_ = json.Unmarshal(msg.Data, &req)

	displays, err := PlatformListDisplaysFn()
	resp := agentmgr.DisplayListData{
		RequestID: req.RequestID,
		Displays:  displays,
	}
	if err != nil {
		resp.Error = err.Error()
	}

	data, err := json.Marshal(resp)
	if err != nil {
		log.Printf("display: marshal response failed: %v", err)
		return
	}
	_ = transport.Send(agentmgr.Message{
		Type: agentmgr.MsgDesktopDisplays,
		ID:   req.RequestID,
		Data: data,
	})
}
