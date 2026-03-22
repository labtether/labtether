package terminal

import (
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const TerminalStreamWriteDeadline = 10 * time.Second

type TerminalStreamEvent struct {
	Marker   string `json:"lt_event,omitempty"`
	Type     string `json:"type"`
	Stage    string `json:"stage,omitempty"`
	Message  string `json:"message,omitempty"`
	Attempt  int    `json:"attempt,omitempty"`
	Attempts int    `json:"attempts,omitempty"`
	Elapsed  int64  `json:"elapsed_ms,omitempty"`
	HopIndex int    `json:"hop_index,omitempty"`
	HopCount int    `json:"hop_count,omitempty"`
	HopHost  string `json:"hop_host,omitempty"`
}

type TerminalControlMessage struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
}

// IsControlMessage checks if a payload looks like a JSON control message.
func IsControlMessage(payload []byte) bool {
	trimmed := strings.TrimSpace(string(payload))
	return len(trimmed) > 0 && trimmed[0] == '{'
}

func ParseTerminalSize(colsRaw, rowsRaw string) (int, int) {
	cols := 160
	rows := 40

	if parsed, err := strconv.Atoi(strings.TrimSpace(colsRaw)); err == nil {
		if parsed >= 40 && parsed <= 500 {
			cols = parsed
		}
	}
	if parsed, err := strconv.Atoi(strings.TrimSpace(rowsRaw)); err == nil {
		if parsed >= 12 && parsed <= 300 {
			rows = parsed
		}
	}

	return cols, rows
}

func WriteTerminalStatus(wsConn *websocket.Conn, stage, message string, attempt, attempts int, elapsedMs int64) error {
	return WriteTerminalEvent(wsConn, TerminalStreamEvent{
		Type:     "status",
		Stage:    strings.TrimSpace(stage),
		Message:  strings.TrimSpace(message),
		Attempt:  attempt,
		Attempts: attempts,
		Elapsed:  elapsedMs,
	})
}

func WriteTerminalReady(wsConn *websocket.Conn, message string, elapsedMs int64) error {
	return WriteTerminalEvent(wsConn, TerminalStreamEvent{
		Type:    "ready",
		Stage:   "connected",
		Message: strings.TrimSpace(message),
		Elapsed: elapsedMs,
	})
}

func WriteTerminalError(wsConn *websocket.Conn, stage, message string) error {
	return WriteTerminalEvent(wsConn, TerminalStreamEvent{
		Type:    "error",
		Stage:   strings.TrimSpace(stage),
		Message: strings.TrimSpace(message),
	})
}

func WriteTerminalHopProgress(wsConn *websocket.Conn, hopIndex, hopCount int, hopHost, message string) error {
	return WriteTerminalEvent(wsConn, TerminalStreamEvent{
		Type:     "status",
		Stage:    "connecting",
		Message:  strings.TrimSpace(message),
		HopIndex: hopIndex,
		HopCount: hopCount,
		HopHost:  strings.TrimSpace(hopHost),
	})
}

func WriteTerminalEvent(wsConn *websocket.Conn, event TerminalStreamEvent) error {
	if wsConn == nil {
		return nil
	}
	if strings.TrimSpace(event.Type) == "" {
		return nil
	}
	if strings.TrimSpace(event.Marker) == "" {
		event.Marker = "terminal"
	}
	_ = wsConn.SetWriteDeadline(time.Now().Add(TerminalStreamWriteDeadline))
	return wsConn.WriteJSON(event)
}
