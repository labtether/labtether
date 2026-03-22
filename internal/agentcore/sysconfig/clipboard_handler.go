package sysconfig

import (
	"encoding/json"
	"log"

	"github.com/labtether/labtether/internal/agentmgr"
)

var ClipboardRead = PlatformClipboardRead
var ClipboardWriteText = PlatformClipboardWriteText
var ClipboardWriteImage = PlatformClipboardWriteImage

// ClipboardManager handles clipboard read/write requests from the hub.
// Clipboard operations are stateless — each request executes immediately
// using platform-specific tooling (pbcopy/pbpaste, xclip/xsel, PowerShell).
type ClipboardManager struct{}

func NewClipboardManager() *ClipboardManager { return &ClipboardManager{} }

// CloseAll is a no-op for ClipboardManager — clipboard requests are stateless
// and require no cleanup.
func (cm *ClipboardManager) CloseAll() {}

// HandleClipboardGet reads the OS clipboard and sends the contents back to the hub.
func (cm *ClipboardManager) HandleClipboardGet(transport MessageSender, msg agentmgr.Message) {
	var req agentmgr.ClipboardGetData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("clipboard: invalid get request: %v", err)
		return
	}

	format := req.Format
	if format == "" {
		format = "text"
	}

	text, imgBase64, err := ClipboardRead(format)

	resp := agentmgr.ClipboardDataPayload{
		RequestID: req.RequestID,
	}
	if err != nil {
		resp.Error = err.Error()
	} else if format == "text" {
		resp.Format = "text"
		resp.Text = text
	} else {
		resp.Format = "image/png"
		resp.Data = imgBase64
	}

	data, err := json.Marshal(resp)
	if err != nil {
		log.Printf("clipboard: failed to marshal response: %v", err)
		return
	}
	_ = transport.Send(agentmgr.Message{
		Type: agentmgr.MsgClipboardData,
		Data: data,
	})
}

// HandleClipboardSet writes content to the OS clipboard and sends an ack back.
func (cm *ClipboardManager) HandleClipboardSet(transport MessageSender, msg agentmgr.Message) {
	var req agentmgr.ClipboardSetData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("clipboard: invalid set request: %v", err)
		return
	}

	var writeErr error
	format := req.Format
	if format == "" {
		format = "text"
	}

	switch format {
	case "text":
		writeErr = ClipboardWriteText(req.Text)
	case "image/png":
		writeErr = ClipboardWriteImage(req.Data)
	default:
		writeErr = ClipboardWriteText(req.Text)
	}

	ack := agentmgr.ClipboardSetAckData{
		RequestID: req.RequestID,
		OK:        writeErr == nil,
	}
	if writeErr != nil {
		ack.Error = writeErr.Error()
	}

	data, err := json.Marshal(ack)
	if err != nil {
		log.Printf("clipboard: failed to marshal ack: %v", err)
		return
	}
	_ = transport.Send(agentmgr.Message{
		Type: agentmgr.MsgClipboardSetAck,
		Data: data,
	})
}
